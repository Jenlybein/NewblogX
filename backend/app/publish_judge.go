package app

import (
	"context"
	"errors"
	"time"

	"myblogx/internal/apperr"
	articleapp "myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	article_review_enum "myblogx/internal/kernel/enum/article_review_enum"
	outboxrelay "myblogx/internal/platform/cdc/outboxrelay"

	"gorm.io/gorm"
)

// 发布判定：把「评分 → 质量闸门 → 合规判定」合并成**一个**订阅者。
//
// # 为什么必须是一个
//
// 2026-11-27 线上漏放了一篇 6 分文章（放行线 52）：质量闸门在那一刻还没读到
// 评分（并发的评分刚提交、`rows:0`），于是返回 `article.score_unavailable`
// 让事件重试；但中继对失败的订阅者是**收集错误后继续执行后面的订阅者**
// （outboxrelay 的 runSubscribers 用 errors.Join），于是合规判定照样跑、
// 判 clean 之后把文章推成 visible。等重试回来时闸门只看「已发布待可见」，
// 文章已经不是待可见，闸门从此对它永不再判 —— 门永久失效。
//
// 结论：只要"质量判定"与"合规放行"是两个订阅者，它们之间的顺序就只是注释里的
// 承诺，任何一次错误都会让后面那个**不可逆的推进**先发生。所以这里合并成一个
// 订阅者：函数内的顺序是代码事实，前一步返回错误就不会走到下一步。
//
// # 对外承诺
//
// 任何会把文章推向「对读者可见」的新逻辑都必须加进本函数内部，不要另注册一个
// 订阅者 —— 那等于把这条保证又退回成约定。
func publishJudgeSubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		return judgePublish(ctx, infra, e, deferredQualityGate(infra))
	}
}

// judgePublish 是发布判定的正文；policy 由调用方给出，便于用例直接钉住边界。
func judgePublish(
	ctx context.Context, infra *Infra, e outboxrelay.Event, policy articleapp.QualityGatePolicy,
) error {
	articleID := e.AggregateID
	if articleID.IsZero() {
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

	// 闸门关闭是显式降级：只看合规判定，与引入闸门之前一致。
	if !policy.Enabled {
		return complianceSubscriber(infra)(ctx, e)
	}

	// ① 分数必须**在本步真的存在**。
	//
	// 只靠"另一个订阅者先评分"是不可靠的：那次评分可能在别处并发跑着、
	// 还没提交，读到的就是空集（线上就是这么漏的）。所以这里自己保证：
	// 缺分就同步补一次，最多等 scoreWaitTimeout。
	// 服务在"确实要判定"时才构造：拿不到分数这条路径连服务都不该建，
	// 否则一次模型故障会连带把与它无关的装配依赖也拉进来。
	if err := ensureScoreReady(ctx, infra, articleID); err != nil {
		return settleQualityUnavailable(ctx, infra, article, e.Attempts, err)
	}

	svc := newArticleService(infra)
	verdict, err := svc.AssessDeferredQuality(ctx, articleID, policy)
	if err != nil {
		return settleQualityUnavailable(ctx, infra, article, e.Attempts, err)
	}
	if !verdict.Passed {
		if _, err := svc.DemotePendingForQuality(ctx, articleID, "文章质量未达标"); err != nil {
			return err
		}
		return notifyQualityDemoted(ctx, infra, article, verdict)
	}

	// ② 质量已过，才允许合规判定去决定可见性（它是 visible 的唯一出口）。
	return complianceSubscriber(infra)(ctx, e)
}

const (
	// scoreWaitTimeout 是"补一次评分"的最长等待。
	//
	// 一次评分是 5–30 秒的模型调用，60 秒足够覆盖正常尾延迟，又不会让发件箱
	// 的一条事件占着 goroutine 太久。超时算失败（走重试），不算通过。
	scoreWaitTimeout = 60 * time.Second

	// scoreEscalateAttempts 是"拿不到分数"允许的重试次数。
	//
	// 超过就转人工：不能让文章永远停在「已发布待可见」—— 作者以为发了、
	// 读者又看不到，是最难解释的一种状态（ADR-0030 的失败语义）。
	scoreEscalateAttempts = 3
)

// ensureScoreReady 保证文章有一条可用评分记录；缺失时同步补一次（带超时）。
func ensureScoreReady(ctx context.Context, infra *Infra, articleID ctype.ID) error {
	if aiService == nil {
		return apperr.New(apperr.CodeArticleScoreUnavailable)
	}
	waitCtx, cancel := context.WithTimeout(ctx, scoreWaitTimeout)
	defer cancel()
	if err := aiService.EnsureScoreIfMissing(waitCtx, articleID.Uint64()); err != nil {
		return apperr.Wrap(apperr.CodeArticleScoreUnavailable, err)
	}
	return nil
}

// settleQualityUnavailable 处理"质量结论拿不到"：前几次让事件重试，
// 到上限转人工并收尾这条事件。
//
// 转人工而不落草稿：作者没做错任何事，把稿子退回草稿等于让他为一次模型故障
// 重写一遍。人看一眼放行更快，也更符合"失败关闭但不惩罚作者"。
func settleQualityUnavailable(
	ctx context.Context,
	infra *Infra,
	article noticeArticleRow,
	attempts int,
	cause error,
) error {
	if attempts < scoreEscalateAttempts {
		return cause
	}
	return escalateToManualReview(ctx, infra, article)
}

// escalateToManualReview 把等不到质量结论的文章转人工。
//
// 抽成变量是为了让用例能替换：这条分支要真的写库（改状态 + 建审核任务），
// 而单元测试里的 Infra 撑不起完整的业务服务装配，硬造一套只会测到装配代码。
var escalateToManualReview = func(ctx context.Context, infra *Infra, article noticeArticleRow) error {
	return newArticleService(infra).BackToReviewForCompliance(
		ctx, article.ID, article_review_enum.SourceModelFailure)
}
