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

	"github.com/go-redis/redis/v8"
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
// 订阅者 —— 那等于把这条保证又退回成约定。状态机那一侧另有一道保险：
// `domain.Article.PublishForCompliance` 要求调用方显式传入「质量结论已通过」，
// 不传就转换失败，所以即使将来有人绕过本函数也无法把文章推成可见。
func publishJudgeSubscriber(infra *Infra) outboxrelay.Subscriber {
	return func(ctx context.Context, e outboxrelay.Event) error {
		return judgePublish(ctx, infra, e, productionPublishJudgeHooks(infra))
	}
}

// publishJudgeHooks 是发布判定这一步的外部依赖。
//
// 为什么是结构体参数而不是包级变量：`docs/architecture/governance.md` 3.1 明确
// 不要"包级变量被用例覆盖"那套 —— 包级可变状态让用例之间互相污染，也让"这条分支
// 到底跑的是真实现还是替身"变得看不出来。依赖从参数注入，生产装配在
// productionPublishJudgeHooks 里，用例自己构造一个假依赖。
type publishJudgeHooks struct {
	// policy 是本次判定用的闸门策略（每次投递现读，后台改完下一次即生效）。
	policy articleapp.QualityGatePolicy
	// ensureScore 保证文章有一条可用评分；缺失时同步补算（带超时）。
	ensureScore func(ctx context.Context, articleID ctype.ID) error
	// scoreFailures 返回"这篇文章连续第几次拿不到质量结论"，用于决定什么时候转人工。
	//
	// 刻意不用 outboxrelay.Event.Attempts：那是整条事件的投递次数，
	// EventArticlePublished 上还挂着通知等兄弟订阅者，任一兄弟失败都会重投整批，
	// Attempts 会跟着涨 —— 拿它数"评分失败次数"会把一次故障误判成三次。
	// 仓库在 outbox_wiring.go 的合规重试那一处也写着同一条理由。
	scoreFailures func(ctx context.Context, articleID ctype.ID) (int, error)
	// clearFailures 在结论落定（通过 / 退回 / 转人工）后清零计数。
	clearFailures func(ctx context.Context, articleID ctype.ID) error
	// assess 读现有评分做质量判定。
	assess func(ctx context.Context, articleID ctype.ID, policy articleapp.QualityGatePolicy) (articleapp.QualityAssessment, error)
	// demote 把未达线的文章退回草稿并通知作者。
	demote func(ctx context.Context, article noticeArticleRow, verdict articleapp.QualityAssessment) error
	// escalate 把等不到质量结论的文章转人工。
	escalate func(ctx context.Context, article noticeArticleRow) error
	// compliance 是合规判定（它是「可见」的唯一出口）。
	compliance outboxrelay.Subscriber
	// warnf 记录"降级但继续"的路径，便于线上复盘（闸门关闭时评分失败就是这一类）。
	warnf func(format string, args ...any)
}

// productionPublishJudgeHooks 是生产装配。
func productionPublishJudgeHooks(infra *Infra) publishJudgeHooks {
	return publishJudgeHooks{
		policy:      deferredQualityGate(infra),
		ensureScore: func(ctx context.Context, articleID ctype.ID) error { return ensureScoreReady(ctx, infra, articleID) },
		scoreFailures: func(ctx context.Context, articleID ctype.ID) (int, error) {
			return recordScoreFailure(ctx, infra, articleID)
		},
		clearFailures: func(ctx context.Context, articleID ctype.ID) error { return clearScoreFailure(ctx, infra, articleID) },
		assess: func(ctx context.Context, articleID ctype.ID, policy articleapp.QualityGatePolicy) (articleapp.QualityAssessment, error) {
			return newArticleService(infra).AssessDeferredQuality(ctx, articleID, policy)
		},
		demote: func(ctx context.Context, article noticeArticleRow, verdict articleapp.QualityAssessment) error {
			if _, err := newArticleService(infra).DemotePendingForQuality(ctx, article.ID, "文章质量未达标"); err != nil {
				return err
			}
			return notifyQualityDemoted(ctx, infra, article, verdict)
		},
		escalate: func(ctx context.Context, article noticeArticleRow) error {
			return newArticleService(infra).BackToReviewForCompliance(
				ctx, article.ID, article_review_enum.SourceModelFailure)
		},
		compliance: complianceSubscriber(infra, true),
		warnf: func(format string, args ...any) {
			if infra.Logger != nil {
				infra.Logger.Warnf(format, args...)
			}
		},
	}
}

// judgePublish 是发布判定的正文。
func judgePublish(ctx context.Context, infra *Infra, e outboxrelay.Event, deps publishJudgeHooks) error {
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
	//
	// 管理员人工放行落的就是「已公开」，所以那一路**刻意**不进来 —— 管理员是终审，
	// 他的决定就是结论，再跑一遍闸门等于让机器推翻人（ADR-0030 的豁免口径）。
	if article.PublishStatus != int(domain.StatusPublished) ||
		article.Visibility != string(domain.VisibilityPendingReview) {
		return nil
	}

	// ① 分数必须**在本步真的存在**。
	//
	// 只靠"另一个订阅者先评分"是不可靠的：那次评分可能在别处并发跑着、
	// 还没提交，读到的就是空集（线上就是这么漏的）。所以这里自己保证：
	// 缺分就同步补一次，最多等 scoreWaitTimeout。
	//
	// 闸门关闭时也补，只是失败**不阻塞**：此时评分只用于审核队列展示
	// （管理员要看到分数才愿意越级放行），不该让一次模型故障卡住整条发布链路。
	scoreErr := deps.ensureScore(ctx, articleID)
	if scoreErr != nil {
		if !deps.policy.Enabled {
			deps.warnf("发布判定：闸门关闭，评分不可用但不阻塞合规判定: article=%s err=%v", articleID, scoreErr)
			return deps.compliance(ctx, e)
		}
		return settleQualityUnavailable(ctx, article, deps, scoreErr)
	}

	// 闸门关闭是显式降级：只看合规判定，与引入闸门之前一致。
	if !deps.policy.Enabled {
		return deps.compliance(ctx, e)
	}

	verdict, err := deps.assess(ctx, articleID, deps.policy)
	if err != nil {
		return settleQualityUnavailable(ctx, article, deps, err)
	}
	if !verdict.Passed {
		// 结论落定：清零失败计数，免得几天后同一篇文章的一次偶发失败被算成累计。
		_ = deps.clearFailures(ctx, articleID)
		return deps.demote(ctx, article, verdict)
	}

	_ = deps.clearFailures(ctx, articleID)

	// ② 质量已过，才允许合规判定去决定可见性（它是 visible 的唯一出口）。
	return deps.compliance(ctx, e)
}

const (
	// scoreWaitTimeout 是"补一次评分"的最长等待。
	//
	// 一次评分是 5–30 秒的模型调用，60 秒足够覆盖正常尾延迟，又不会让发件箱
	// 的一条事件占着 goroutine 太久。超时算失败（走重试），不算通过。
	scoreWaitTimeout = 60 * time.Second

	// scoreEscalateFailures 是"拿不到分数"允许的失败次数。
	//
	// 超过就转人工：不能让文章永远停在「已发布待可见」—— 作者以为发了、
	// 读者又看不到，是最难解释的一种状态（ADR-0030 的失败语义）。
	scoreEscalateFailures = 3

	// scoreFailureKeyPrefix 与 scoreFailureTTL 决定"连续失败"的窗口。
	//
	// 计数按**文章**而不是按事件：同一篇文章可能被多次发布（改稿重提），
	// 但"这篇文章最近连续几次拿不到分数"这个问题的答案只与文章有关。
	// 6 小时足够覆盖一次模型故障的恢复期，又不会让上周的失败算进今天。
	scoreFailureKeyPrefix = "article:quality:fail:"
	scoreFailureTTL       = 6 * time.Hour
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

// recordScoreFailure 给"这篇文章又没拿到质量结论"加一次，返回累计次数。
//
// Redis 不可用时返回 0 且不报错：数不出次数就不该转人工（宁可继续重试，
// 也不要因为一个计数服务故障把作者的文章推进人工队列）。
func recordScoreFailure(ctx context.Context, infra *Infra, articleID ctype.ID) (int, error) {
	if infra.Redis == nil {
		return 0, nil
	}
	key := scoreFailureKeyPrefix + articleID.String()
	pipe := infra.Redis.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, scoreFailureTTL)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return int(incr.Val()), nil
}

// clearScoreFailure 在结论落定后清零，返回的错误一律忽略（它只是计数的卫生问题）。
func clearScoreFailure(ctx context.Context, infra *Infra, articleID ctype.ID) error {
	if infra.Redis == nil {
		return nil
	}
	return infra.Redis.Del(ctx, scoreFailureKeyPrefix+articleID.String()).Err()
}

// settleQualityUnavailable 处理"质量结论拿不到"：前几次让事件重试，
// 到上限转人工并收尾这条事件。
//
// 转人工而不落草稿：作者没做错任何事，把稿子退回草稿等于让他为一次模型故障
// 重写一遍。人看一眼放行更快，也更符合"失败关闭但不惩罚作者"。
func settleQualityUnavailable(
	ctx context.Context,
	article noticeArticleRow,
	deps publishJudgeHooks,
	cause error,
) error {
	failures, _ := deps.scoreFailures(ctx, article.ID)
	if failures < scoreEscalateFailures {
		return cause
	}
	_ = deps.clearFailures(ctx, article.ID)
	return deps.escalate(ctx, article)
}
