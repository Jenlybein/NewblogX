package router

import (
	"myblogx/api"
	"myblogx/api/article_api"
	"myblogx/api/article_api/category"
	"myblogx/api/article_api/favorite"
	"myblogx/api/article_api/tags"
	"myblogx/api/article_api/top"
	"myblogx/api/article_api/view_history"
	mw "myblogx/middleware"
	"myblogx/models"

	"github.com/gin-gonic/gin"
)

func ArticleRouter(r *gin.RouterGroup, appContainer api.Api, runtimeMw mw.Runtime) {
	group := r.Group("articles")
	reviewGroup := r.Group("article-review")
	reviewTaskGroup := r.Group("article-review-tasks")
	authGroup := group.Group("", runtimeMw.AuthMiddleware)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)
	reviewAdminGroup := reviewGroup.Group("", runtimeMw.AuthMiddleware, mw.AdminMiddleware)
	reviewTaskAdminGroup := reviewTaskGroup.Group("", runtimeMw.AuthMiddleware, mw.AdminMiddleware)

	app := appContainer.ArticleApi

	// 文章操作
	group.GET("author_info", mw.BindQuery[article_api.ArticleAuthorInfoBindRequest], app.ArticleAuthorInfoView)
	group.GET(":id", mw.BindUri[models.IDRequest], app.ArticleDetailView)
	authGroup.POST("", mw.BindJson[article_api.ArticleCreateRequest], app.ArticleCreateView)
	authGroup.PUT(":id", mw.BindUri[models.IDRequest], mw.BindJson[article_api.ArticleUpdateRequest], app.ArticleUpdateView)
	authGroup.DELETE(":id", mw.BindUri[models.IDRequest], app.ArticleRemoveUserView)
	adminGroup.DELETE("", mw.CaptureLog(mw.ReqBody|mw.ReqHeader), mw.BindJson[models.IDListRequest], app.ArticleRemoveView)
	adminGroup.POST(":id/admin/:visibility", mw.CaptureLog(mw.ReqHeader), mw.BindUri[article_api.ArticleAdminVisibilityURI], app.ArticleAdminVisibilityView)

	group.POST("view", mw.BindJson[article_api.ArticleViewCountRequest], app.ArticleVisitView)
	authGroup.PUT(":id/digg", mw.BindUri[models.IDRequest], app.ArticleDiggView)
	reviewAdminGroup.GET("", mw.BindQuery[article_api.ArticleReviewTaskListRequest], app.ArticleReviewTaskListView)
	reviewTaskAdminGroup.POST(":id/review", mw.CaptureLog(mw.ReqBody|mw.ReqHeader), mw.BindUri[models.IDRequest], mw.BindJson[article_api.ArticleReviewHandleRequest], app.ArticleReviewTaskHandleView)

	// 收藏
	group.GET("favorite", mw.BindQuery[favorite.FavoriteListRequest], app.FavoriteListView)
	group.GET("favorite/contents", mw.BindQuery[favorite.FavoriteArticlesRequest], app.FavoriteArticlesView)
	authGroup.PUT("favorite", mw.BindJson[favorite.FavoriteRequest], app.FavoriteCreateUpdateView)
	authGroup.DELETE("favorite/contents", mw.BindJson[favorite.FavoriteRemovePatchModel], app.FavoriteRemovePatchView)
	authGroup.DELETE("favorite", mw.BindJson[models.IDListRequest], app.FavoriteDeleteView)
	authGroup.POST("favorite", mw.BindJson[article_api.ArticleFavoriteRequest], app.ArticleFavoriteSaveView)

	// 置顶
	group.GET("top", mw.BindQuery[top.ArticleTopListRequest], app.ArticleTopListView)
	authGroup.POST("top", mw.BindJson[top.ArticleTopSetRequest], app.ArticleTopSetView)
	authGroup.DELETE("top", mw.BindJson[top.ArticleTopSetRequest], app.ArticleTopRemoveView)

	// 浏览历史
	authGroup.GET("history", mw.BindQuery[view_history.ArticleViewHistoryRequest], app.ArticleViewHistoryView)
	authGroup.DELETE("history", mw.BindJson[models.IDListRequest], app.ArticleViewHistoryRemoveView)

	// 分类
	group.GET("category", mw.BindQuery[category.CategoryListRequest], app.CategoryListView)
	authGroup.POST("category", mw.CaptureLog(mw.ReqBody), mw.BindJson[category.CategoryRequest], app.CategoryCreateUpdateView)
	authGroup.DELETE("category", mw.CaptureLog(mw.ReqBody|mw.ReqHeader), mw.BindJson[models.IDListRequest], app.CategoryDeleteView)
	group.GET("category/options", mw.BindQuery[category.CategoryOptionsRequest], app.CategoryOptionsView)

	// 标签
	group.GET("tags", mw.BindQuery[tags.TagListRequest], app.TagListView)
	group.GET("tags/options", app.ArticleTagOptionsView)
	adminGroup.PUT("tags", mw.CaptureLog(mw.ReqBody), mw.BindJson[tags.TagRequest], app.TagCreateUpdateView)
	adminGroup.DELETE("tags", mw.CaptureLog(mw.ReqBody|mw.ReqHeader), mw.BindJson[models.IDListRequest], app.TagDeleteView)
}
