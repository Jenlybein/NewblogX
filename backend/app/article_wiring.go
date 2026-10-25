package app

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	articlehttp "myblogx/internal/article/adapter/http"
	"myblogx/internal/article/adapter/mysql"
	articleapp "myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	mediamysql "myblogx/internal/media/adapter/mysql"
	"myblogx/internal/platform/idgen"
	redis_service "myblogx/internal/platform/redis"
	usermysql "myblogx/internal/user/adapter/mysql"
	"myblogx/router"
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
	counterOptions := redis_service.Options{Client: infra.Redis, Logger: infra.Logger}
	// 作者统计的写入逻辑归 user 模块，这里把函数交给 article 的 UoW，
	// 由它在自己的事务里调用 —— 跨模块写仍然同事务，但不再互相 import。
	uow := mysql.NewTxManager(infra.DB, counterOptions, usermysql.ApplyArticleDelta)
	svc := articleapp.New(uow, realClock{}, snowflakeIDs{})

	h := articlehttp.NewHandler(
		svc,
		infra.Logger,
		infra.RuntimeSite,
		viewerFromContext,
		&imageResolver{db: infra.DB},
	)
	router.SetArticleHandler(h)
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
	return uint64(id)
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
	value := ctype.ID(id)
	return &value, nil
}

// ResolveImageURLByID 取图片 URL。
func (r *imageResolver) ResolveImageURLByID(ctx context.Context, id ctype.ID) (string, error) {
	_ = ctx
	return mediamysql.ResolveURLByID(r.db, uint64(id))
}
