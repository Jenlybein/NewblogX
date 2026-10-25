package router

import (
	articlehttp "myblogx/internal/article/adapter/http"
	mw "myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// articleHTTPHandler 是 article 模块的处理器，由装配层注入。
//
// 不放进 api.Api 容器：那个容器是旧结构的产物，M5 会整体删除；
// 把新链路塞进去等于让新代码依赖即将消失的壳。
var articleHTTPHandler *articlehttp.Handler

// SetArticleHandler 注入 article 模块的处理器。
//
// 装配层在启动时调用一次。旧实现（api/article_api）已删除，
// article 路由只有新链路一条路径，不再有开关切换。
func SetArticleHandler(h *articlehttp.Handler) { articleHTTPHandler = h }

// ArticleRouter 注册 article 模块路由。
//
// 处理器必须由装配层在启动时注入；未注入说明装配顺序有误，直接 panic 早暴露。
func ArticleRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	if articleHTTPHandler == nil {
		panic("article: 处理器未注入，请确认 app.WireArticle 在 router.Run 之前调用")
	}
	articleHTTPHandler.Register(r, runtimeMw.AuthMiddleware, runtimeMw.OptionalAuthMiddleware)
}
