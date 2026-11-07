package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	articleapp "myblogx/internal/article/app"
	"myblogx/internal/article/domain/review"
	chatdomain "myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	notificationapp "myblogx/internal/notification/app"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/platform/es/articlesync"

	"gorm.io/gorm"
)

// OutboxWiringOptions 是本进程中继策略的**显式输入**，由 main 按进程角色决定。
//
// 为什么不再从装配结果反推：私信事件的下游只有 WS 连接池，只有 api / all 角色
// 有它。若靠"WireChat 有没有跑过"来推导 SkipEventNames，WireChat 与
// WireOutboxRelay 的调用顺序一变，api 进程就会静默跳过 chat.message.sent，
// 事件被标 done、通知永久丢失。把判断上移到装配处，顺序就不再影响行为。
type OutboxWiringOptions struct {
	// DeliverChatPush 表示本进程持有私信 WS 连接池，可以投递私信通知。
	//
	// 为 false 时 relayOptions 会把两条私信事件放进 SkipEventNames：
	// 这些事件保持 pending，等真正持有连接池的进程领取。
	DeliverChatPush bool
}

// WireOutboxRelay 启动 outbox 中继，并注册领域事件的真实下游。
//
// 为什么独立于 WireRiver：
// river 只在"启用 ES 同步"时才启动，而领域事件的投递不能因为没开 ES 就停摆。
// 中继自带补偿扫描，即使 canal 没跑也能投递；canal 跑着时，river 会把
// outbox 表的 INSERT 直接推给中继，走低延迟路径。
//
// 这里注册的下游，就是旧链路里散落在 handler 中、用裸 goroutine 触发的那些副作用
// （评分、系统通知）。现在它们变成"事件落表 → 中继投递"，崩溃不丢、失败可重试。
func WireOutboxRelay(infra *Infra, opts OutboxWiringOptions) {
	if infra == nil || infra.DB == nil {
		return
	}

	relay := outboxrelay.New(infra.DB, infra.Logger, relayOptions(infra, opts))

	registerOutboxSubscribers(relay, infra)

	// 注册为进程内实例，river 的 CDC 路径通过它入队。
	outboxrelay.SetCurrent(relay)
	relay.Run()
}

// registerOutboxSubscribers 把领域事件的真实下游挂到中继上。
//
// 单独拆出来的理由：测试要能在**不启动 worker / 补偿扫描**的前提下同步投递一条事件，
// 同时必须经过与生产同一份订阅表。如果测试自己 replay.On(...) 一遍，
// "生产忘了注册"这类缺陷永远测不出来。
func registerOutboxSubscribers(relay *outboxrelay.Relay, infra *Infra) {
	if relay == nil {
		return
	}

	// 评分：创建与发布都要保证有评分记录（复刻旧 create/examine/review 的行为）。
	relay.On(articleapp.EventArticleCreated, scoreSubscriber(infra))
	relay.On(articleapp.EventArticlePublished, scoreSubscriber(infra))

	// 通知：审核结果与旧链路一致（通过 / 拒绝各一条）。
	relay.On(articleapp.EventArticlePublished, notifySubscriber(infra))
	relay.On(articleapp.EventArticleRejected, notifySubscriber(infra))

	// 通知：互动类（点赞 / 收藏）。
	// 旧实现里这两处是 handler 中的裸 goroutine，删掉旧代码后必须在这里接回来，
	// 否则"有人点赞/收藏你的文章"的通知会静默消失。
	relay.On(articleapp.EventArticleDigg, diggNotifySubscriber(infra))
	relay.On(articleapp.EventArticleFavorited, favorNotifySubscriber(infra))

	// 私信：HTTP 落库是唯一真相，WS 只做通知。
	//
	// 发送与已读的推送都从这里投递，不在业务事务里直接写连接池：
	// 推送失败、进程崩溃都不会丢（事件已随消息一起落库，中继会重投）。
	// 订阅者在投递时从 Infra 现读连接池，所以注册顺序不影响投递；
	// 没有连接池的进程靠 relayOptions 的 SkipEventNames 根本不领取这两条事件。
	relay.On(chatdomain.EventMessageSent, chatSubscriber(infra))
	relay.On(chatdomain.EventMessagesRead, chatSubscriber(infra))

	// 搜索索引：文章删除后必须真的把文档移除。
	//
	// 这是本次修复的根因之一：事件早就在事务里落进发件箱，但没有订阅者，
	// 中继把"没人订阅"当成成功直接标 done，索引文档永远留着。删除是软删
	// （库里只更新 deleted_at），ES 不会因为查询过滤不到就算消失。
	// 写副作用放在这里而不是删除事务里，理由见包注释：失败可重试、耗尽进死信。
	relay.On(articleapp.EventArticleDeleted, articleDeletedIndexSubscriber(infra))
}

// articleDeletedIndexSubscriber 在文章删除后移除对应的搜索索引文档。
//
// 幂等：中继是 at-least-once，重复投递会再删一次；文档已经不在了时
// DeleteESDocs 按"目的已达成"处理（404 不算失败），所以重投是安全的。
func articleDeletedIndexSubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(_ context.Context, e outboxrelay.Event) error {
		if infra == nil {
			return nil
		}
		articleID := e.AggregateID
		if articleID.IsZero() {
			return nil
		}
		// 未启用 ES 时没有可删的索引文档；不早退会把删除事件误判成失败并进死信。
		if infra.ESClient == nil {
			return nil
		}
		return articlesync.DeleteESDocs(infra.DB, infra.ESClient, []ctype.ID{articleID})
	}
}

// relayOptions 把配置里的保留期参数映射成中继参数。
//
// 0 值交给 outboxrelay 的 withDefaults 兜底，所以这里不用再判 0：
// "没配置"与"配成 0"都按默认保留期处理。
func relayOptions(infra *Infra, wire OutboxWiringOptions) outboxrelay.Options {
	opts := outboxrelay.Options{}
	// 私信事件的唯一下游是 WS 推送，只有持有连接池的进程（api / all）能做。
	// 其他角色的中继（river）绝对不能领取它们：领取后找不到连接池，
	// 订阅者只能"什么都不做"地返回成功，事件被标记 done，通知就永久丢了。
	// 判据是**显式传入的装配选项**，不是"WireChat 有没有跑过"。
	if !wire.DeliverChatPush {
		opts.SkipEventNames = []string{chatdomain.EventMessageSent, chatdomain.EventMessagesRead}
	}
	if infra == nil || infra.Config == nil {
		return opts
	}
	o := infra.Config.Outbox
	opts.RetentionDone = time.Duration(o.RetentionHours) * time.Hour
	opts.RetentionDead = time.Duration(o.DeadRetentionHours) * time.Hour
	opts.CleanupInterval = time.Duration(o.CleanupIntervalMinutes) * time.Minute
	opts.CleanupLimit = o.CleanupBatchSize
	return opts
}

// scoreSubscriber 确保文章有 AI 质量评分。
//
// 用同步版 EnsureArticleScoreIfMissing 而不是 ...Async：
// 中继本身已经是异步的，用同步版才能拿到错误，进而走重试与死信。
// 该操作天然幂等（"缺失时才生成"），重复投递不会产生重复评分记录。
func scoreSubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		articleID := e.AggregateID
		if articleID.IsZero() {
			return nil
		}

		// 文章已经不在了（例如投递前被删除）就没什么可评分的。
		// 不加这层判断的话，一次删除就会换来 5 次重试 + 一条死信。
		var count int64
		if err := infra.DB.WithContext(ctx).Table(noticeArticleRow{}.TableName()).
			Where("id = ?", articleID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return nil
		}

		if aiService == nil {
			return nil
		}
		return aiService.EnsureScoreIfMissing(ctx, articleID.Uint64())
	}
}

// notifySubscriber 在审核通过/拒绝时给作者发系统通知。
//
// 文案与旧链路保持一致（旧代码里是裸 goroutine 调 InsertSystemMessage），
// 避免用户可见内容发生变化。
func notifySubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		articleID := e.AggregateID
		if articleID.IsZero() {
			return nil
		}

		var article noticeArticleRow
		if err := infra.DB.WithContext(ctx).Where("id = ?", articleID).Take(&article).Error; err != nil {
			// 查不到（多半已删除）：没有可通知的对象，直接结束，不重试。
			return nil
		}

		var content string
		switch e.EventName {
		case articleapp.EventArticlePublished:
			content = fmt.Sprintf("您的文章《%s》审核通过!", article.Title)
		case articleapp.EventArticleRejected:
			// 原因取自**审核任务记录**，不是重新查文章：驳回理由一直存在任务行上
			// （review.Task.Reason），缺的只是"把它带到通知正文里"这一步。
			// 不要去文章表找原因——那里从来没有过这个字段，读不到只会绕回兜底文案。
			reason, err := rejectionReason(ctx, infra, articleID)
			if err != nil {
				return err
			}
			content = rejectionContent(article.Title, reason)
		default:
			return nil
		}

		linkHref := fmt.Sprintf("/article/%s", articleID)

		// 幂等保护：中继是 at-least-once，崩溃重放可能让同一条事件再投一次。
		// 消息表不能加去重列（零 DDL 约束禁止改动既有表），
		// 所以用"同接收者 + 同链接 + 同内容已存在"来去重。
		var exists int64
		if err := infra.DB.WithContext(ctx).Table(noticeMessageRow{}.TableName()).
			Where("receiver_id = ? AND link_herf = ? AND content = ?", article.AuthorID, linkHref, content).
			Count(&exists).Error; err != nil {
			return err
		}
		if exists > 0 {
			return nil
		}

		if notificationService == nil {
			return nil
		}
		authorID := article.AuthorID
		return notificationService.NotifySystem(ctx, notificationapp.SystemNotice{
			Receiver:   authorID.Uint64(),
			ActionUser: authorID.Uint64(),
			Content:    content,
			LinkTitle:  article.Title,
			LinkHref:   linkHref,
		})
	}
}

// rejectionContent 是驳回通知的正文。
//
// 失败原因交给 review.ReasonText 渲染：历史任务没填原因时落兜底文案，
// 不能留一句"失败原因："后面什么都没有。文案与"作者打开被打回的草稿"
// 看到的是同一句话，替换文案只改 review.ReasonText 一处。
func rejectionContent(title, reason string) string {
	return fmt.Sprintf("您的文章《%s》审核失败，请修改后再提交!\n失败原因：%s",
		title, review.ReasonText(reason))
}

// rejectionReason 取这篇文章最近一次驳回时审核员写下的原因。
//
// 查的是审核任务表：原因本来就记在任务行上（review.Task.Reason），
// 本票修的只是"它从来没被带到通知正文里"。历史任务可能没填原因
// （#303 之前不是必填），这里如实返回空串，由 review.ReasonText 统一兜底；
// 不要在这里另写一份兜底文案。
//
// 查不到任务行（例如任务已被清理）同样返回空串：没有记录不算通知失败，
// 返回错误只会让中继白白重试到死信。
func rejectionReason(ctx context.Context, infra *Infra, articleID ctype.ID) (string, error) {
	var row noticeReviewReasonRow
	err := infra.DB.WithContext(ctx).
		Table(row.TableName()).
		Select("reason").
		Where("article_id = ? AND status = ?", articleID, string(review.StatusRejected)).
		Order("reviewed_at DESC, id DESC").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return row.Reason, err
}

// interactionPayload 是点赞/收藏事件体（article_id + actor_id）。
type interactionPayload struct {
	ArticleID ctype.ID `json:"article_id"`
	ActorID   ctype.ID `json:"actor_id"`
}

// loadInteraction 读出互动通知需要的信息：文章（作者、标题）与操作者。
//
// 返回 ok=false 表示文章已删除或事件体缺失，此时不该重试——
// 重试也不会成功，只会白白耗尽次数并进死信。
func loadInteraction(ctx context.Context, infra *Infra, e outboxrelay.Event) (article noticeArticleRow, actorID ctype.ID, ok bool) {
	var payload interactionPayload
	if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
		return noticeArticleRow{}, "", false
	}
	if e.AggregateID.IsZero() || payload.ActorID.IsZero() {
		return noticeArticleRow{}, "", false
	}
	if err := infra.DB.WithContext(ctx).Where("id = ?", e.AggregateID).
		Take(&article).Error; err != nil {
		return noticeArticleRow{}, "", false
	}
	return article, payload.ActorID, true
}

// diggNotifySubscriber 点赞通知：告诉作者"某某赞了你的文章"。
func diggNotifySubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		article, actorID, ok := loadInteraction(ctx, infra, e)
		if !ok {
			return nil
		}
		if notificationService == nil {
			return nil
		}
		// 通知服务内部按 (操作者, 类型, 文章) 查重，
		// 中继重复投递不会产生重复消息，所以这里不用再做幂等。
		return notificationService.NotifyArticleDigg(ctx, notificationapp.InteractionNotice{
			Receiver:     article.AuthorID.Uint64(),
			ActionUser:   actorID.Uint64(),
			ArticleID:    article.ID,
			ArticleTitle: article.Title,
		})
	}
}

// favorNotifySubscriber 收藏通知：告诉作者"某某收藏了你的文章"。
func favorNotifySubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		article, actorID, ok := loadInteraction(ctx, infra, e)
		if !ok {
			return nil
		}
		if notificationService == nil {
			return nil
		}
		return notificationService.NotifyArticleFavor(ctx, notificationapp.InteractionNotice{
			Receiver:     article.AuthorID.Uint64(),
			ActionUser:   actorID.Uint64(),
			ArticleID:    article.ID,
			ArticleTitle: article.Title,
		})
	}
}

// CloseOutboxRelay 停掉中继（停机时调用）。
func CloseOutboxRelay() {
	relay := outboxrelay.Current()
	if relay == nil {
		return
	}
	relay.Close()
}
