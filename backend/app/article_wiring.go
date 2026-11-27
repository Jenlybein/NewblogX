package app

import (
	"context"
	"fmt"
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
	notificationapp "myblogx/internal/notification/app"
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
		WithContentFilter(articleTextFilterBridge{svc: infra.Filter}).
		WithCollectionContentFilter(collectionTextFilterBridge{svc: infra.Filter}).
		WithActivity(&articleActivity{opts: analyticsLogOptions(infra)}).
		// 管理员强制退回草稿的站内信（运营纠错）：通知模块只有装配层认识。
		WithNotifier(&forceDemoteNotifier{db: infra.DB})
}

// forceDemoteNotifier 把"管理员强制退回草稿"的通知接成 article 模块的端口。
//
// 为什么实现在装配层：article 用例层不认识 notification 模块（垂直模块口径），
// 而这条通知是**同步**的（管理员当场要知道作者会不会收到），走不了发件箱。
// 与 moderationNotifier 同一形状：端口由使用方定义，实现放在唯一同时认识两边的包里。
type forceDemoteNotifier struct{ db *gorm.DB }

// NotifyForceDemoted 给作者发一条系统通知，正文里带管理员填的原因。
//
// 去重口径与 notifyQualityDemoted 一致（同接收者 + 同链接 + 同内容）：消息表不能
// 加去重列（零 DDL），而正文里就带着原因、链接里就带着文章 ID，所以"接收者 + 链接 +
// 正文"这个键在这里**等价于「文章 ID + 原因」**。去重不是为了拦顺序重试（那种情况
// 用例层已经按"已是草稿 → 目的已达成"提前返回），而是拦并发重复请求：两笔事务
// 可能都看到"已发布"，各自都会调到这里。
func (n *forceDemoteNotifier) NotifyForceDemoted(ctx context.Context, notice articleapp.ForceDemoteNotice) error {
	if n == nil || n.db == nil {
		return nil
	}
	content := fmt.Sprintf("您的文章《%s》已被管理员退回草稿：%s", notice.Title, notice.Reason)
	linkHref := fmt.Sprintf("/article/%s", notice.ArticleID)

	var exists int64
	if err := n.db.WithContext(ctx).Table(noticeMessageRow{}.TableName()).
		Where("receiver_id = ? AND link_herf = ? AND content = ?", notice.AuthorID, linkHref, content).
		Count(&exists).Error; err != nil {
		return err
	}
	if exists > 0 || notificationService == nil {
		return nil
	}
	// action_user 记发起人（管理员），作者看到的是"系统通知 + 是谁做的处置"；
	// 与"质量闸门退回"那条不同：那次是系统动作，这里是一次人工干预。
	return notificationService.NotifySystem(ctx, notificationapp.SystemNotice{
		Receiver:   notice.AuthorID.Uint64(),
		ActionUser: notice.OperatorID.Uint64(),
		Content:    content,
		LinkTitle:  notice.Title,
		LinkHref:   linkHref,
	})
}

// articleTextFilterBridge 把内容过滤服务接成 article 模块的文章面端口（票 397）。
//
// 与评论面同构：文章模块的端口不带 surface（它只知道"这是我的标题 / 正文"），
// 面枚举由装配层决定。分成文章与合集两个类型而不是一个带参数的实现，是为了让
// "把合集桥接给文章"变成编译错误，而不是一条悄悄写错面的命中记录。
type articleTextFilterBridge struct {
	svc *filterapp.Service
}

// FilterArticleText 替换文章文本里的命中词并记一条关键词风险。
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
	return articleapp.FilteredText{Masked: got.Masked, FilteredCount: got.FilteredCount}, nil
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
