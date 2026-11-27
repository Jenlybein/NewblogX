package app

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	aiapp "myblogx/internal/ai/app"
	"myblogx/internal/analytics/adapter/clickhouse"
	"myblogx/internal/apperr"
	articlehttp "myblogx/internal/article/adapter/http"
	"myblogx/internal/article/adapter/mysql"
	adapterredis "myblogx/internal/article/adapter/redis"
	articleapp "myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/grouping"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
	filterapp "myblogx/internal/filter/app"
	filterdomain "myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	mediamysql "myblogx/internal/media/adapter/mysql"
	"myblogx/internal/platform/http/router"
	"myblogx/internal/platform/idgen"
	redis_service "myblogx/internal/platform/redis"
	usermysql "myblogx/internal/user/adapter/mysql"
	"myblogx/utils/jwts"
)

// WireArticle 装配 article 模块。
//
// 旧 api/article_api 已删除，这里是 article 路由的唯一实现，
// 启动时无条件注入处理器。
func WireArticle(infra *Infra) {
	if infra == nil {
		return
	}

	// 事件出口不再在这里装配：写侧只往事务内的 outbox 表插行，
	// 真正的下游订阅者（评分 / 系统通知）挂在 outbox 中继上，
	// 见 app/outbox_wiring.go 的 WireOutboxRelay。
	svc := newArticleService(infra)
	// 挂到 Infra：关键词风险页的处置动作要复用**这一份**装配（票 398）。
	infra.ArticleService = svc

	h := articlehttp.NewHandler(
		svc,
		infra.Logger,
		infra.Effective,
		viewerFromContext,
		&imageResolver{db: infra.DB},
		ImageDisplay(),
	)
	router.SetArticleHandler(h)
}

// newArticleService 构造 article 用例服务。
//
// 单独抽出来是为了让装配层复用**同一份**读模型：站点右栏的推荐合集卡要数
// 目录树里的公开文章、回退封面，这套口径只应由 article 模块提供（#223）。
func newArticleService(infra *Infra) *articleapp.Service {
	counterOptions := redis_service.Options{Client: infra.Redis, Logger: infra.Logger}
	// 作者统计的写入逻辑归 user 模块，这里把函数交给 article 的 UoW，
	// 由它在自己的事务里调用 —— 跨模块写仍然同事务，但不再互相 import。
	uow := mysql.NewTxManager(infra.DB, counterOptions, usermysql.ApplyArticleDelta)
	// 投稿配额（票 304 / ADR-0022）：普通用户每自然日可提交的次数上限。
	// 上限取生效值的**现读闭包**，所以后台改完下一次请求即生效；管理员豁免在
	// 用例层判定（这里不传角色）。Redis 不可用时适配器返回错误 —— 失败关闭。
	quota := adapterredis.NewSubmitQuota(counterOptions, infra.Effective.ArticleSubmitDailyMaxFunc())
	return articleapp.New(
		uow,
		realClock{},
		snowflakeIDs{},
		quota,
		newArticleModerationScores(infra.DB),
		qualityGatePort{},
	).
		// 内容过滤（票 397 / ADR-0032）：两个面各自一个 bridge，面枚举钉死在装配层。
		// 文章面多接一个"block 档命中是否拒发"的现读判定（content_filter.auto_dispose），
		// 合集面不接 —— 本期只有文章标题 / 摘要 / 正文按配置拒发。
		WithContentFilter(articleTextFilterBridge{svc: infra.Filter}).
		// 命中处置档位（串行链路第①环）：用例层拿它决定"命中之后文章去哪"。
		//
		// 桥只报事实（哪一档真的被打星），档位判定在用例层做 —— 三档化之后
		// "报事实"与"定落点"必须分开：draft 与 reject 用的是同一份命中事实。
		WithFilterDisposition(contentFilterDispositionReader(infra)).
		WithCollectionContentFilter(collectionTextFilterBridge{svc: infra.Filter}).
		WithActivity(&articleActivity{opts: analyticsLogOptions(infra)})
}

// contentFilterDisposition 返回两档命中处置的现读闭包（2026-11-28 三档化）。
//
// 配置在运行时（content_filter.auto_dispose，后台可改），所以每次过滤都现读一次。
// 读到空串或没装配时按 release：默认必须与改造前逐字节一致，"配置读不到"
// 不能变成把所有人的文章拒掉或退回草稿（ADR-0032 决策 11 的同一条口径）。
//
// 这里**不做取值校验**：非法值在保存侧（site/app 的 normalizeAutoDispose）就被
// 拒掉了，能进运行时的只有三个合法值。运行期再做一次映射等于把同一份规则抄两处。
func contentFilterDisposition(infra *Infra) func() (block, watch string) {
	return func() (string, string) {
		if infra == nil || infra.RuntimeSite == nil {
			return articleapp.HitRelease, articleapp.HitRelease
		}
		dispose := infra.RuntimeSite.GetRuntimeContentFilter().AutoDispose
		// 旧值 mask 就地读成 release（2026-11-28 三档化）：
		//
		// 库里的 JSON 是按 key 存的，老配置里写的是 mask —— 它表达的正是「只打星
		// 照常走」。保存路径会把它升级成 release（normalizeDisposeAction），但
		// 在运营还没打开过设置页之前，读面必须已经按新语义工作，否则一次发版
		// 就让线上所有命中词从「放行」变成「被当成未知档位」。
		return upgradeLegacyDisposition(dispose.BlockAction), upgradeLegacyDisposition(dispose.WatchAction)
	}
}

// contentFilterDispositionReader 把上面的闭包转成用例层的端口形状。
//
// 两处注入（bridge 与用例服务）共用同一个闭包实例，是为了让"过滤时看到的档位"
// 与"落点时用的档位"来自**同一次现读**——各读一次会让两个值在一次请求内漂开。
func contentFilterDispositionReader(infra *Infra) func() articleapp.FilterDisposition {
	read := contentFilterDisposition(infra)
	return func() articleapp.FilterDisposition {
		block, watch := read()
		return articleapp.FilterDisposition{Block: block, Watch: watch}
	}
}

// 管理员强制退回草稿的站内信走**发件箱**（票 481），不在这里同步发。
//
// 同步发过一次，留下一个真实的漏通知：事务提交后发信失败 → 用例把错误返给前端 →
// 管理员重试时文章已是草稿、用例按"目的已达成"提前返回，通知再也发不出去。
// 订阅者见 outbox_wiring.go 的 forceDemoteNoticeSubscriber。

// articleTextFilterBridge 把内容过滤服务接成 article 模块的文章面端口（票 397）。
//
// 与评论面同构：文章模块的端口不带 surface（它只知道"这是我的标题 / 正文"），
// 面枚举由装配层决定。分成文章与合集两个类型而不是一个带参数的实现，是为了让
// "把合集桥接给文章"变成编译错误，而不是一条悄悄写错面的命中记录。
//
// **"命中即拒"只在这一条桥上生效**（block 档 + 站点配了 reject）：合集面、评论面、
// 昵称 / 简介 / 私信本期都保持"只打星"。把处置口径钉在两个具体类型上，而不是
// 让过滤服务自己读配置，是因为拒发的语义要落在"哪个面"上 —— 评论被拒是发不出去
// 一条评论，文章被拒是整篇不落库，两者不是同一件事。
type articleTextFilterBridge struct {
	svc *filterapp.Service
}

// FilterArticleText 替换文章文本里的命中词并记一条关键词风险。
//
// 命中记录在 svc.Filter 内部就写完了（拒绝 ≠ 不记录）：这里只把"有没有
// 真被打星的 block 命中"与站点处置口径合取成给用例层的事实。
func (b articleTextFilterBridge) FilterArticleText(ctx context.Context, in articleapp.FilterText) (articleapp.FilteredText, error) {
	if b.svc == nil {
		return articleapp.FilteredText{Masked: in.Text}, nil
	}
	got, err := b.svc.Filter(ctx, filterapp.FilterInput{
		Surface:   filterdomain.SurfaceArticle,
		UserID:    in.UserID,
		ContentID: in.ContentID,
		Text:      in.Text,
	})
	if err != nil {
		return articleapp.FilteredText{}, err
	}
	// 两档的事实原样上报，**处置**交给用例层按现读档位判定。
	//
	// 改造前这里把"有没有 block 命中"与"是否拒发"合取成一个 bool 就返回给用例层，
	// 现在三档化之后不能这么做了：draft 与 reject 都要用同一份命中事实，
	// 而哪档对应哪个处置只有拿到配置才知道 —— 那是用例层（article/app）的活。
	return articleapp.FilteredText{
		Masked:          got.Masked,
		FilteredCount:   got.FilteredCount,
		BlockTierMasked: got.BlockTierMasked,
		WatchTierMasked: got.WatchTierMasked,
	}, nil
}

// collectionTextFilterBridge 把内容过滤服务接成合集面端口（票 397）。
type collectionTextFilterBridge struct {
	svc *filterapp.Service
}

// FilterCollectionText 替换合集标题 / 描述里的命中词并记一条关键词风险。
func (b collectionTextFilterBridge) FilterCollectionText(ctx context.Context, in articleapp.FilterText) (articleapp.FilteredText, error) {
	if b.svc == nil {
		return articleapp.FilteredText{Masked: in.Text}, nil
	}
	got, err := b.svc.Filter(ctx, filterapp.FilterInput{
		Surface:   filterdomain.SurfaceCollection,
		UserID:    in.UserID,
		ContentID: in.ContentID,
		Text:      in.Text,
	})
	if err != nil {
		return articleapp.FilteredText{}, err
	}
	return articleapp.FilteredText{Masked: got.Masked, FilteredCount: got.FilteredCount}, nil
}

// articleActivity 用分析模块的 ClickHouse 读面实现 article 的活动端口（票 354）。
//
// 这一层就是"端口由使用方定义、实现在装配层"的落点：article 不认识 clickhouse 包，
// 而个人概览要的恰恰是分析库才有的数据。把它放在装配层也意味着只有这里需要
// 同时知道两边，别的模块照旧互不认识。
type articleActivity struct {
	opts clickhouse.Options
}

var _ articleapp.ActivityPort = (*articleActivity)(nil)

// DailyActivity 返回最近 days 天里每天成功的创作动作数。
//
// 读失败**不返回错误**（上层会把它当成"热力图留空"，整页照常打开），但要留下日志：
// 之前这条路径把错误整个吞掉，`toDate` 扫不进 string 的 bug 因此安静地活了很久。
func (a *articleActivity) DailyActivity(
	_ context.Context, userID domain.UserID, days int,
) (map[string]int, error) {
	items, err := clickhouse.CountDailyStudioActivity(a.opts, userID.Uint64(), days)
	if err != nil {
		if a.opts.Logger != nil {
			a.opts.Logger.Warnf("个人概览：读取每日活跃度失败，热力图留空 user_id=%d err=%v", userID.Uint64(), err)
		}
		return nil, err
	}
	out := make(map[string]int, len(items))
	for _, item := range items {
		out[item.Day] = item.Count
	}
	return out, nil
}

// realClock 是生产时钟。
type realClock struct{}

// Now 返回当前时间。
func (realClock) Now() time.Time { return time.Now() }

// snowflakeIDs 用项目既有的雪花生成器产生 ID。
//
// 生成失败直接 panic：InitSnowflake 成功后不会失败，
// 真失败说明装配有问题，早暴露比写入 0 号记录好。
type snowflakeIDs struct{}

// NextArticleID 返回下一个文章 ID。
func (snowflakeIDs) NextArticleID() domain.ArticleID {
	id, err := idgen.Next()
	if err != nil {
		panic("article: 雪花 ID 生成失败: " + err.Error())
	}
	return domain.ArticleID(id)
}

// NextReviewTaskID 返回下一个审核任务 ID。
func (snowflakeIDs) NextReviewTaskID() uint64 {
	id, err := idgen.Next()
	if err != nil {
		panic("article: 雪花 ID 生成失败: " + err.Error())
	}
	return id.Uint64()
}

// NextGroupID 返回下一个分组 ID（ADR-0019）。
func (snowflakeIDs) NextGroupID() grouping.ID {
	id, err := idgen.Next()
	if err != nil {
		panic("article: 雪花 ID 生成失败: " + err.Error())
	}
	return grouping.ID(id.String())
}

// NextGroupItemID 返回下一个条目归属行 ID。
func (snowflakeIDs) NextGroupItemID() ctype.ID {
	id, err := idgen.Next()
	if err != nil {
		panic("article: 雪花 ID 生成失败: " + err.Error())
	}
	return id
}

// NextCollectionID 返回下一个合集 ID。
func (snowflakeIDs) NextCollectionID() collection.ID {
	id, err := idgen.Next()
	if err != nil {
		panic("article: 雪花 ID 生成失败: " + err.Error())
	}
	return collection.ID(id)
}

// NextProjectID 返回下一个项目 ID。
//
// 与 NextCollectionID 同一做法：项目与合集是两套内容实体，ID 会出现在
// 读者站 URL 上，日志里要能分辨"这是项目还是合集"。
func (snowflakeIDs) NextProjectID() project.ID {
	id, err := idgen.Next()
	if err != nil {
		panic("article: 雪花 ID 生成失败: " + err.Error())
	}
	return project.ID(id)
}

// NextOutlineNodeID 返回下一个目录树节点 ID。
//
// 树节点 ID 会出现在后台树编辑器接口里（整树提交靠它做幂等更新），
// 与文章 / 合集 / 项目 ID 分开，日志里可分辨。
func (snowflakeIDs) NextOutlineNodeID() outline.ID {
	id, err := idgen.Next()
	if err != nil {
		panic("article: 雪花 ID 生成失败: " + err.Error())
	}
	return outline.ID(id)
}

// NextFolderID 返回下一个收藏夹 ID。
func (snowflakeIDs) NextFolderID() favorite.ID {
	id, err := idgen.Next()
	if err != nil {
		panic("article: 雪花 ID 生成失败: " + err.Error())
	}
	return favorite.ID(id)
}

// viewerFromContext 从中间件已解析的 claims 得到 Viewer。
//
// 不再自己解析 token：AuthMiddleware / OptionalAuthMiddleware 已经把 claims
// 放进上下文，重复解析既慢又容易与中间件的失败语义不一致。
func viewerFromContext(c *gin.Context) domain.Viewer {
	v, ok := c.Get("claims")
	if !ok {
		return domain.Anonymous()
	}
	claims, ok := v.(*jwts.MyClaims)
	if !ok || claims == nil {
		return domain.Anonymous()
	}
	if claims.Role == enum.RoleAdmin {
		return domain.AdminUser(domain.UserID(claims.UserID))
	}
	return domain.NormalUser(domain.UserID(claims.UserID))
}

// imageResolver 把图片模块包装成 HTTP 层需要的端口。
type imageResolver struct{ db *gorm.DB }

// FindImageIDByURL 反查图片 ID。
func (r *imageResolver) FindImageIDByURL(ctx context.Context, url string) (*ctype.ID, error) {
	_ = ctx
	id, ok, err := mediamysql.FindIDByURL(r.db, url)
	if err != nil || !ok {
		return nil, err
	}
	value := ctype.NewID(id)
	return &value, nil
}

// ResolveImageURLByID 取图片 URL。
func (r *imageResolver) ResolveImageURLByID(ctx context.Context, id ctype.ID) (string, error) {
	_ = ctx
	return mediamysql.ResolveURLByID(r.db, id.Uint64())
}

// qualityGatePort 是「发布质量闸门」的评分端口适配器（票 306 / ADR-0023）。
//
// 它把 article 用例层的窄口子接到 AI 模块：调 ScoreForGate 拿分数与问题列表，
// 并把"没有可用分数"的各种情形（未装配、AI 未开启、模型调用失败、返回非 JSON）
// 统一成 error —— 用例层据此失败关闭。
//
// 评分**落库**由 AI 侧负责：同一次提交若再被异步补评分评一次，就是一次提交烧
// 两次模型调用；写进记录后异步路径看到已有记录就会跳过。
type qualityGatePort struct{}

// Evaluate 实现 articleapp.QualityGatePort。
func (qualityGatePort) Evaluate(ctx context.Context, in articleapp.QualityGateInput) (articleapp.QualityGateScore, error) {
	if aiService == nil {
		// 闸门开着却没有 AI 装配：按"拿不到分数"处理，而不是静默放行。
		return articleapp.QualityGateScore{}, apperr.New(apperr.CodeAIServiceOff)
	}
	result, err := aiService.ScoreForGate(ctx, aiapp.GateScoreInput{
		ArticleID:     in.ArticleID,
		AuthorID:      in.AuthorID,
		Title:         in.Title,
		Content:       in.Content,
		UpdatedAt:     in.UpdatedAt,
		AuthorIsAdmin: in.AuthorIsAdmin,
	})
	if err != nil {
		return articleapp.QualityGateScore{}, err
	}

	issues := make([]articleapp.QualityIssue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		positions := make([]articleapp.QualityPosition, 0, len(issue.Positions))
		for _, pos := range issue.Positions {
			positions = append(positions, articleapp.QualityPosition{
				Paragraph: pos.Paragraph,
				Quote:     pos.Quote,
			})
		}
		issues = append(issues, articleapp.QualityIssue{
			Positions:  positions,
			Reason:     issue.Reason,
			Suggestion: issue.Suggestion,
		})
	}
	dimensions := make([]articleapp.QualityDimension, 0, len(result.Dimensions))
	for _, item := range result.Dimensions {
		dimensions = append(dimensions, articleapp.QualityDimension{Name: item.Name, Score: item.Score})
	}

	return articleapp.QualityGateScore{
		TotalScore: result.TotalScore,
		Dimensions: dimensions,
		Issues:     issues,
	}, nil
}

// upgradeLegacyDisposition 把改造前的取值映射到三档语义（2026-11-28）。
//
// 只有 mask → release 一条：mask 是「只打星、文本照常入库」，与 release 完全同义。
// 其余取值原样返回 —— 空串由调用方按 release 兜底，非法值在保存侧就被拒了。
func upgradeLegacyDisposition(action string) string {
	if action == "mask" {
		return articleapp.HitRelease
	}
	return action
}
