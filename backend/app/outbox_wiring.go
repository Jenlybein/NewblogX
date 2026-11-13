package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	aipkg "myblogx/internal/ai"
	"myblogx/internal/ai/moderation"
	"myblogx/internal/apperr"
	articleapp "myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/review"
	chatdomain "myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/kernel/enum/article_review_enum"
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

	// 质量闸门（ADR-0030）：评分先进来（上面的 scoreSubscriber），再做判定，
	// 最后才是合规判定。订阅者按注册顺序串行执行，所以质量未达标的文章
	// 会在合规放行之前被退回草稿 —— 读者永远看不到一个没过质量线的稿子。
	relay.On(articleapp.EventArticleCreated, qualityGateSubscriber(infra))
	relay.On(articleapp.EventArticlePublished, qualityGateSubscriber(infra))

	// 合规判定（票 312 / ADR-0024）：文章创建与发布各跑一次。
	// 与评分并列挂在同一批事件上，但**结论落在自己的表**（article_risk_models），
	// 不借用评分记录——两者判的是不同的问题，失败与重试口径也不同。
	relay.On(articleapp.EventArticleCreated, complianceSubscriber(infra))
	relay.On(articleapp.EventArticlePublished, complianceSubscriber(infra))

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

// qualityGateSubscriber 在评分之后、合规判定之前做一次质量闸门判定（ADR-0030）。
//
// 失败语义：拿不到分数 → 返回 error 交给发件箱重试；重试耗尽进死信，文章继续
// 停在「已发布待可见」（对读者不可见），不会把没评过的稿子当干净放行。
func qualityGateSubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		articleID := e.AggregateID
		if articleID.IsZero() {
			return nil
		}
		policy := deferredQualityGate(infra)
		if !policy.Enabled {
			return nil
		}

		var article noticeArticleRow
		err := infra.DB.WithContext(ctx).Table(article.TableName()).
			Select("id, title, author_id, publish_status, visibility_status").
			Where("id = ?", articleID).Take(&article).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		// 只判「已发布待可见」：草稿、已拒、已公开的文章不归这条链路管。
		if article.PublishStatus != int(domain.StatusPublished) ||
			article.Visibility != string(domain.VisibilityPendingReview) {
			return nil
		}

		articleSvc := newArticleService(infra)
		verdict, err := articleSvc.AssessDeferredQuality(ctx, articleID, policy)
		if err != nil {
			return err
		}
		if verdict.Passed {
			return nil
		}

		if _, err := articleSvc.DemotePendingForQuality(ctx, articleID, "文章质量未达标"); err != nil {
			return err
		}
		return notifyQualityDemoted(ctx, infra, article, verdict)
	}
}

// deferredQualityGate 取后台链路的闸门配置（与 HTTP 层同一份站点生效值）。
func deferredQualityGate(infra *Infra) articleapp.QualityGatePolicy {
	if infra == nil || infra.Effective == nil {
		return articleapp.QualityGatePolicy{}
	}
	gate := infra.Effective.Article().QualityGate.Effective()
	return articleapp.QualityGatePolicy{
		Enabled:            gate.Enabled,
		Deferred:           true,
		PassScore:          gate.PassScore,
		GrayBandWidth:      gate.GrayBandWidth,
		BandPoorMax:        gate.BandPoorMax,
		BandUnqualifiedMax: gate.BandUnqualifiedMax,
		BandNormalMax:      gate.BandNormalMax,
		BandGoodMax:        gate.BandGoodMax,
	}
}

// notifyQualityDemoted 通知作者"评分未达标、已退回草稿"（带分数与放行线）。
//
// 去重口径与 notifySubscriber 一致（同接收者 + 同链接 + 同内容）：中继是
// at-least-once，重放不该让作者收到两条一样的通知。
func notifyQualityDemoted(
	ctx context.Context, infra *Infra, article noticeArticleRow, verdict articleapp.QualityAssessment,
) error {
	content := fmt.Sprintf(
		"您的文章《%s》未通过发布质量线：%d 分，放行线 %d 分。已退回草稿，修改后可重新提交。",
		article.Title, verdict.Score, verdict.Required,
	)
	linkHref := fmt.Sprintf("/article/%s", article.ID)

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
	return notificationService.NotifySystem(ctx, notificationapp.SystemNotice{
		Receiver:   article.AuthorID.Uint64(),
		ActionUser: article.AuthorID.Uint64(),
		Content:    content,
		LinkTitle:  article.Title,
		LinkHref:   linkHref,
	})
}

// complianceSubscriber 触发一次模型合规判定并落库（票 312 / ADR-0024）。
//
// 失败语义：拿不到结论时返回 error，交给发件箱重试；重试耗尽后进死信，
// 后台看到的是"这篇文章没有合规结论"（未判定），而不是被当成"干净"放行。
func complianceSubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		articleID := e.AggregateID
		if articleID.IsZero() {
			return nil
		}
		// 一次读全：存在性、标题（通知要用）与作者（额度归属要用）。
		var article noticeArticleRow
		err := infra.DB.WithContext(ctx).Table(article.TableName()).
			Select("id, title, author_id").Where("id = ?", articleID).Take(&article).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 文章已经不在了（多半投递前被删除）：没有可判定的对象，直接结束不重试。
			return nil
		}
		if err != nil {
			return err
		}

		articleSvc := newArticleService(infra)
		if aiService == nil {
			// AI 未装配：不能把文章永远留在 pending_review（作者以为发了、
			// 读者却看不到）。转人工，由管理员决定放行还是驳回。
			return articleSvc.BackToReviewForCompliance(ctx, articleID,
				article_review_enum.SourceModelFailure)
		}

		// 归属：异步链路没有登录态，只能回查作者与角色。管理员豁免在额度收费点
		// 只认 IsAdmin 布尔，不认识角色枚举（ADR-0028）。
		isAdmin, err := userIsAdmin(ctx, infra, article.AuthorID)
		if err != nil {
			return err
		}
		checkCtx := aipkg.WithCallMeta(ctx, aipkg.CallMeta{
			UserID:  article.AuthorID,
			IsAdmin: isAdmin,
		})

		result, err := aiService.CheckArticleCompliance(checkCtx, articleID.Uint64())
		if err != nil {
			if apperr.IsCode(err, apperr.CodeAIDailyLimitExceeded) {
				// 额度用完不是"判定失败"：重试注定再被拒、还会白扣一次。
				// 落点见 deferComplianceForQuota（审核中 + 无任务 + 次日重投 + 通知）。
				return deferComplianceForQuota(ctx, infra, articleSvc, article)
			}
			// 判定失败：**在订阅者内部重试一次**，然后转人工。
			//
			// 不能借用发件箱的 e.Attempts 来数重试次数：EventArticlePublished
			// 上还挂着评分、通知等兄弟订阅者，runSubscribers 用 errors.Join
			// 汇总——任一兄弟失败就会重投整批，Attempts 会涨到 2/3/4/5，
			// 于是一次"评分失败"会把已判 clean 的文章也误改成人工审核。
			// 自己重试一次与兄弟彻底解耦，且严格只有一次。
			result, err = aiService.CheckArticleCompliance(checkCtx, articleID.Uint64())
			if err != nil {
				return articleSvc.BackToReviewForCompliance(ctx, articleID,
					article_review_enum.SourceModelFailure)
			}
		}

		if saveErr := articleSvc.SaveComplianceResult(ctx, articleID, articleapp.ComplianceResult{
			Verdict:       result.Verdict,
			RiskLevel:     result.RiskLevel,
			Categories:    result.Categories,
			Reasons:       result.Reasons,
			PromptVersion: result.PromptVersion,
			ModelName:     result.ModelName,
		}); saveErr != nil {
			return saveErr
		}

		// 分流（票 302 新规则）：提交时同步扫描干净的文章以
		// Published + pending_review 落库，作者立刻拿到"已发布"回执，
		// 这里才决定它对不对读者可见。
		switch moderation.Verdict(result.Verdict) {
		case moderation.VerdictClean:
			// 模型判干净：真正放开可见。幂等，重复投递不会产生额外写入。
			return articleSvc.PublishForCompliance(ctx, articleID)
		case moderation.VerdictSuspect:
			// 存疑：误判会直接落到作者身上且他没有申诉渠道，一律交人。
			// 从"已发布待可见"改回审核中并建一条待处理任务。
			return articleSvc.BackToReviewForCompliance(ctx, articleID,
				article_review_enum.SourceModelSuspect)
		case moderation.VerdictViolation:
			// 高置信违规自动拒（票 313 / ADR-0024）：档位 violation 且风险 ≥ 2。
			// 风险 1 的 violation 也交人，理由同 suspect。
			if result.RiskLevel >= autoRejectRiskLevel {
				return articleSvc.AutoRejectForCompliance(ctx, articleID)
			}
			return articleSvc.BackToReviewForCompliance(ctx, articleID,
				article_review_enum.SourceModelSuspect)
		}
		// 未知档位不当作干净放行：失败关闭，交人工。
		return articleSvc.BackToReviewForCompliance(ctx, articleID,
			article_review_enum.SourceModelSuspect)
	}
}

// userIsAdmin 回查作者是否管理员（异步链路没有登录态）。
//
// 查不到用户就按"非管理员"处理：作者已被删除的文章不该因为一个孤儿行
// 让整条事件无限重试。
func userIsAdmin(ctx context.Context, infra *Infra, userID ctype.ID) (bool, error) {
	if userID.IsZero() {
		return false, nil
	}
	var row noticeUserRoleRow
	err := infra.DB.WithContext(ctx).Table(row.TableName()).
		Select("role").Where("id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.Role == enum.RoleAdmin, nil
}

// deferComplianceForQuota 在作者当日合规判定额度用尽时收尾一条事件。
//
// 三件事必须一起发生（ADR-0028）：
//  1. 文章改回「审核中」且**不建人工任务**（明天会自动重审）；
//  2. 给作者一条系统通知，说清楚"明天自动重审、期间读者看不到"；
//  3. 把本条事件推迟到次日零点后重投（既不算完成也不算失败）。
//
// 任何一步失败都返回错误：事件会因为失败重试，但 DeferForCompliance 幂等，
// 重试只会重复一次"已经完成"的转态（ErrInvalidTransition 被吞掉）。
func deferComplianceForQuota(
	ctx context.Context, infra *Infra, articleSvc *articleapp.Service, article noticeArticleRow,
) error {
	if err := articleSvc.DeferForCompliance(ctx, article.ID); err != nil {
		return err
	}
	if err := notifyQuotaDeferred(ctx, infra, article); err != nil {
		return err
	}
	return &outboxrelay.RescheduleError{
		At:     nextQuotaReviewTime(time.Now()),
		Reason: "AI 合规判定额度已用完，次日自动重审",
	}
}

// notifyQuotaDeferred 给作者发一条「额度不足、明日自动重审」的系统通知。
//
// 去重与 notifySubscriber 同一套（中继是 at-least-once，事件被推迟后仍可能
// 重复投递；消息表不能加去重列，所以按 接收者 + 链接 + 正文 判重）。
func notifyQuotaDeferred(ctx context.Context, infra *Infra, article noticeArticleRow) error {
	content := fmt.Sprintf(
		"今日的 AI 审核次数已用完，您的文章《%s》将在明日额度恢复后自动重审。重审通过前，读者看不到这篇文章。",
		article.Title)
	linkHref := fmt.Sprintf("/article/%s", article.ID)

	var exists int64
	if err := infra.DB.WithContext(ctx).Table(noticeMessageRow{}.TableName()).
		Where("receiver_id = ? AND link_herf = ? AND content = ?", article.AuthorID, linkHref, content).
		Count(&exists).Error; err != nil {
		return err
	}
	if exists > 0 || notificationService == nil {
		return nil
	}
	return notificationService.NotifySystem(ctx, notificationapp.SystemNotice{
		Receiver:   article.AuthorID.Uint64(),
		ActionUser: article.AuthorID.Uint64(),
		Content:    content,
		LinkTitle:  article.Title,
		LinkHref:   linkHref,
	})
}

// nextQuotaReviewTime 是"额度恢复后重审"的落点：次日零点后 5 分钟 + 0–10 分钟抖动。
//
// 抖动是必要的：额度会在同一时刻对所有人恢复，没有抖动时当天所有被推迟的
// 合规判定会在同一分钟涌入模型接口。
func nextQuotaReviewTime(now time.Time) time.Time {
	y, m, d := now.Date()
	next := time.Date(y, m, d, 0, 5, 0, 0, now.Location()).AddDate(0, 0, 1)
	return next.Add(time.Duration(now.UnixNano()%10) * time.Minute)
}

// autoRejectRiskLevel 是"高置信"的门槛：风险级别 ≥ 2 才自动拒。
//
// 定 2 而不是 1：1 是"低风险"（措辞可疑），那种程度误判最常见，交人看更合适。
const autoRejectRiskLevel = 2

// 合规档位不在本地重声明：词表唯一来源是 ai/moderation.Verdict*，
// 这里直接用别名比较，避免"改了一处忘了另一处"（GLOSSARY「枚举别名」）。

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
			// 票 302 新规则下这个事件也可能来自"干净文章提交即发布、但还在
			// 等内容检查"（pending_review），此时并没人工审核过。用中性文案，
			// 不谎称"审核通过"；作者侧已有的"已发布"提示负责说明后续等待。
			content = fmt.Sprintf("您的文章《%s》已发布!", article.Title)
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
//
// 系统直接拒（票 302 同步风险命中 / ADR-0025 合规自动拒）是**终局、不给改**，
// 仍说"请修改后再提交"会把作者引向一个必然被拒的动作，所以换一句陈述式文案。
func rejectionContent(title, reason string) string {
	text := review.ReasonText(reason)
	if text == review.ReasonLabel(review.ReasonRiskCheck) || text == articleapp.ComplianceRejectionLabel() {
		return fmt.Sprintf("您的文章《%s》未通过内容检查，已被拒绝。\n原因：%s", title, text)
	}
	return fmt.Sprintf("您的文章《%s》审核失败，请修改后再提交!\n失败原因：%s", title, text)
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
		// 没有已拒绝的审核任务：可能是合规自动拒，或提交时同步风险扫描直接拒
		// （票 302 新规则）——这两种的任务仍是待处理，理由不在 rejected 行上。
		// 给作者固定文案，而不是空串：空串会让通知里出现"理由："后面什么都没有。
		var pendingRow noticePendingReasonRow
		pendingErr := infra.DB.WithContext(ctx).Table(pendingRow.TableName()).
			Select("source").
			Where("article_id = ? AND status = ? AND source IN ?", articleID,
				string(review.StatusPending),
				[]string{string(article_review_enum.SourceRiskCheck), string(article_review_enum.SourceCompliance)}).
			Order("created_at DESC, id DESC").
			Take(&pendingRow).Error
		if pendingErr == nil {
			if pendingRow.Source == string(article_review_enum.SourceRiskCheck) {
				return review.ReasonLabel(review.ReasonRiskCheck), nil
			}
			return articleapp.ComplianceRejectionLabel(), nil
		}
		var riskRow noticeRiskRow
		riskErr := infra.DB.WithContext(ctx).Table(riskRow.TableName()).
			Where("article_id = ?", articleID).Take(&riskRow).Error
		if riskErr == nil && riskRow.Verdict == string(moderation.VerdictViolation) {
			return articleapp.ComplianceRejectionLabel(), nil
		}
		return "", nil
	}
	return row.Reason, err
}

// noticePendingReasonRow 读待处理复盘任务的来源（票 302：系统直接拒的通知文案）。
type noticePendingReasonRow struct {
	Source string `gorm:"column:source"`
}

func (noticePendingReasonRow) TableName() string { return "article_review_task_models" }

// noticeRiskRow 是读合规结论用的最小投影（票 313：自动拒后通知要给出固定文案）。
type noticeRiskRow struct {
	ArticleID ctype.ID `gorm:"column:article_id"`
	Verdict   string   `gorm:"column:verdict"`
}

// TableName 返回合规风险表的表名。
func (noticeRiskRow) TableName() string { return "article_risk_models" }

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
