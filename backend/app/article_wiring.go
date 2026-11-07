package app

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	articlehttp "myblogx/internal/article/adapter/http"
	"myblogx/internal/article/adapter/mysql"
	adapterredis "myblogx/internal/article/adapter/redis"
	articleapp "myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/article/domain/project"
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

	h := articlehttp.NewHandler(
		svc,
		infra.Logger,
		infra.Effective,
		viewerFromContext,
		&imageResolver{db: infra.DB},
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
	// 投稿配额（票 304 / ADR-0034）：普通用户每自然日可提交的次数上限。
	// 上限取生效值的**现读闭包**，所以后台改完下一次请求即生效；管理员豁免在
	// 用例层判定（这里不传角色）。Redis 不可用时适配器返回错误 —— 失败关闭。
	quota := adapterredis.NewSubmitQuota(counterOptions, infra.Effective.ArticleSubmitDailyMaxFunc())
	return articleapp.New(uow, realClock{}, snowflakeIDs{}, quota)
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
