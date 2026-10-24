package app

import (
	"context"
	"encoding/json"
	"fmt"

	articleapp "myblogx/internal/article/app"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/ai_service/article_score_service"
	"myblogx/service/message_service"
)

// WireOutboxRelay 启动 outbox 中继，并注册领域事件的真实下游。
//
// 为什么独立于 WireRiver：
// river 只在"启用 ES 同步"时才启动，而领域事件的投递不能因为没开 ES 就停摆。
// 中继自带补偿扫描，即使 canal 没跑也能投递；canal 跑着时，river 会把
// outbox 表的 INSERT 直接推给中继，走低延迟路径。
//
// 这里注册的下游，就是旧链路里散落在 handler 中、用裸 goroutine 触发的那些副作用
// （评分、系统通知）。现在它们变成"事件落表 → 中继投递"，崩溃不丢、失败可重试。
func WireOutboxRelay(infra *Infra) {
	if infra == nil || infra.DB == nil {
		return
	}

	relay := outboxrelay.New(infra.DB, infra.Logger, outboxrelay.Options{})

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

	// 注册为进程内实例，river 的 CDC 路径通过它入队。
	outboxrelay.SetCurrent(relay)
	relay.Run()
}

// scoreSubscriber 确保文章有 AI 质量评分。
//
// 用同步版 EnsureArticleScoreIfMissing 而不是 ...Async：
// 中继本身已经是异步的，用同步版才能拿到错误，进而走重试与死信。
// 该操作天然幂等（"缺失时才生成"），重复投递不会产生重复评分记录。
func scoreSubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		articleID := ctype.ID(e.AggregateID)
		if articleID == 0 {
			return nil
		}

		// 文章已经不在了（例如投递前被删除）就没什么可评分的。
		// 不加这层判断的话，一次删除就会换来 5 次重试 + 一条死信。
		var count int64
		if err := infra.DB.WithContext(ctx).Model(&models.ArticleModel{}).
			Where("id = ?", articleID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return nil
		}

		return article_score_service.EnsureArticleScoreIfMissing(infra.DB, infra.RuntimeSite, articleID)
	}
}

// notifySubscriber 在审核通过/拒绝时给作者发系统通知。
//
// 文案与旧链路保持一致（旧代码里是裸 goroutine 调 InsertSystemMessage），
// 避免用户可见内容发生变化。
func notifySubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		articleID := ctype.ID(e.AggregateID)
		if articleID == 0 {
			return nil
		}

		var article models.ArticleModel
		if err := infra.DB.WithContext(ctx).Where("id = ?", articleID).Take(&article).Error; err != nil {
			// 查不到（多半已删除）：没有可通知的对象，直接结束，不重试。
			return nil
		}

		var content string
		switch e.EventName {
		case articleapp.EventArticlePublished:
			content = fmt.Sprintf("您的文章《%s》审核通过!", article.Title)
		case articleapp.EventArticleRejected:
			content = fmt.Sprintf("您的文章《%s》审核失败，请修改后再提交!\n失败原因：%s", article.Title, "")
		default:
			return nil
		}

		linkHref := fmt.Sprintf("/article/%d", articleID)

		// 幂等保护：中继是 at-least-once，崩溃重放可能让同一条事件再投一次。
		// 消息表不能加去重列（零 DDL 约束禁止改动既有表），
		// 所以用"同接收者 + 同链接 + 同内容已存在"来去重。
		var exists int64
		if err := infra.DB.WithContext(ctx).Model(&models.ArticleMessageModel{}).
			Where("receiver_id = ? AND link_herf = ? AND content = ?", article.AuthorID, linkHref, content).
			Count(&exists).Error; err != nil {
			return err
		}
		if exists > 0 {
			return nil
		}

		authorID := article.AuthorID
		// InsertSystemMessage 没有 error 返回值（内部自行记日志），
		// 所以这里只能保证"该发的发了"，无法据此触发重试。
		message_service.InsertSystemMessage(infra.DB, infra.Logger, message_service.SystemMessage{
			ReceiverID:   authorID,
			ActionUserID: &authorID,
			Content:      content,
			LinkTitle:    article.Title,
			LinkHerf:     linkHref,
		})
		return nil
	}
}

// interactionPayload 是点赞/收藏事件体（article_id + actor_id）。
type interactionPayload struct {
	ArticleID uint64 `json:"article_id"`
	ActorID   uint64 `json:"actor_id"`
}

// loadInteraction 读出互动通知需要的信息：文章（作者、标题）与操作者。
//
// 返回 ok=false 表示文章已删除或事件体缺失，此时不该重试——
// 重试也不会成功，只会白白耗尽次数并进死信。
func loadInteraction(ctx context.Context, infra *Infra, e outboxrelay.Event) (article models.ArticleModel, actorID ctype.ID, ok bool) {
	var payload interactionPayload
	if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
		return models.ArticleModel{}, 0, false
	}
	if e.AggregateID == 0 || payload.ActorID == 0 {
		return models.ArticleModel{}, 0, false
	}
	if err := infra.DB.WithContext(ctx).Where("id = ?", ctype.ID(e.AggregateID)).
		Take(&article).Error; err != nil {
		return models.ArticleModel{}, 0, false
	}
	return article, ctype.ID(payload.ActorID), true
}

// diggNotifySubscriber 点赞通知：告诉作者"某某赞了你的文章"。
func diggNotifySubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		article, actorID, ok := loadInteraction(ctx, infra, e)
		if !ok {
			return nil
		}
		// InsertArticleDiggMessage 内部会先按 (操作者, 类型, 文章) 查重，
		// 重复投递不会产生重复消息，所以这里不用再做幂等。
		message_service.InsertArticleDiggMessage(infra.DB, infra.Logger, message_service.ArticleDiggMessage{
			ReceiverID:   article.AuthorID,
			ActionUserID: actorID,
			ArticleID:    article.ID,
			ArticleTitle: article.Title,
		})
		return nil
	}
}

// favorNotifySubscriber 收藏通知：告诉作者"某某收藏了你的文章"。
func favorNotifySubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		article, actorID, ok := loadInteraction(ctx, infra, e)
		if !ok {
			return nil
		}
		message_service.InsertArticleFavorMessage(infra.DB, infra.Logger, message_service.ArticleFavorMessage{
			ReceiverID:   article.AuthorID,
			ActionUserID: actorID,
			ArticleID:    article.ID,
			ArticleTitle: article.Title,
		})
		return nil
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
