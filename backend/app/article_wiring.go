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
	redis_service "myblogx/internal/platform/redis"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/router"
	"myblogx/service/db_service"
	"myblogx/service/image_service"
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

	ob := mysql.NewOutbox()
	registerArticleEventHandlers(ob, infra)

	counterDeps := redis_service.Deps{Client: infra.Redis, Logger: infra.Logger}
	uow := mysql.NewTxManager(infra.DB, counterDeps, ob)
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

// registerArticleEventHandlers 把领域事件接回既有的下游。
//
// 这些副作用改造前散在 handler 里（有的同步、有的是裸 goroutine）。
// 现在统一成"事务内发事件 → adapter 同步分发"，好处是：
//   - 下游失败会让整个事务回滚（不会出现"状态已发布但通知没发"）；
//   - 将来换成异步 outbox（S4.6）只需改 mysql.Outbox，用例与下游都不动。
func registerArticleEventHandlers(ob *mysql.Outbox, infra *Infra) {
	// 目前只做"记录 + 保底"，真正的下游订阅在 S4.6 的 outbox 消费者里补齐。
	// 保留这个函数是为了让装配点明确：新链路的事件出口在这里，不会悄悄丢弃。
	ob.On(articleapp.EventArticlePublished, func(ctx context.Context, e articleapp.Event) error {
		infra.Logger.WithField("event", e.Name).Debug("article 领域事件")
		return nil
	})
	ob.On(articleapp.EventArticleDeleted, func(ctx context.Context, e articleapp.Event) error {
		infra.Logger.WithField("event", e.Name).Debug("article 领域事件")
		return nil
	})
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
	id, err := db_service.NextSnowflakeID()
	if err != nil {
		panic("article: 雪花 ID 生成失败: " + err.Error())
	}
	return domain.ArticleID(id)
}

// NextReviewTaskID 返回下一个审核任务 ID。
func (snowflakeIDs) NextReviewTaskID() uint64 {
	id, err := db_service.NextSnowflakeID()
	if err != nil {
		panic("article: 雪花 ID 生成失败: " + err.Error())
	}
	return uint64(id)
}

// NextFolderID 返回下一个收藏夹 ID。
func (snowflakeIDs) NextFolderID() favorite.ID {
	id, err := db_service.NextSnowflakeID()
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
	return image_service.FindImageIDByURL(r.db, url)
}

// ResolveImageURLByID 取图片 URL。
func (r *imageResolver) ResolveImageURLByID(ctx context.Context, id ctype.ID) (string, error) {
	_ = ctx
	return image_service.ResolveImageURLByID(r.db, id)
}
