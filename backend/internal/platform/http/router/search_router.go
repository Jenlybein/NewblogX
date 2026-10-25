package router

import (
	mw "myblogx/internal/platform/http/middleware"
	searchhttp "myblogx/internal/search/adapter/http"

	"github.com/gin-gonic/gin"
)

// searchHandler 由 app.WireSearch 在启动时注入。
var searchHandler *searchhttp.Handler

// SetSearchHandler 注入搜索处理器。
func SetSearchHandler(h *searchhttp.Handler) { searchHandler = h }

// SearchRouter 注册搜索路由。
func SearchRouter(r *gin.RouterGroup, runtimeMw mw.Runtime) {
	if searchHandler == nil {
		panic("router: 搜索处理器未注入（应在 app.WireSearch 中调用 SetSearchHandler）")
	}
	_ = runtimeMw
	group := r.Group("search")
	group.GET("articles", searchHandler.ArticleSearch)
}
