package http

import (
	"os"

	"github.com/gin-gonic/gin"

	mw "myblogx/middleware"
	"myblogx/models"
)

// Enabled 判断是否启用新链路。
//
// 过渡期开关：M4 期间新旧两套路由并存，靠环境变量切换，
// golden 通过、灰度一周后再删旧路由与这个开关（S5.5）。
//
// 为什么用环境变量而不是配置文件：开关只影响路由装配，
// 不需要热更新；而且能避免为一个过渡期开关去改所有环境的 yaml。
func Enabled() bool {
	return os.Getenv("BLOGX_ARTICLE_NEW_PIPELINE") == "1"
}

// Register 把 article 模块的全部路由挂到 /api 下。
//
// 路径与改造前完全一致（含 article-review 与 article-review-tasks 两个独立前缀），
// 前端零改动。
//
// optionalMw 是"解析但不强制"的鉴权：公开详情页需要它来识别"作者本人看自己的草稿"。
// 改造前这一步是在 handler 内部手动解析 token 完成的，现在提到中间件，
// 行为一致但少一次重复解析。
func (h *Handler) Register(r *gin.RouterGroup, authMw, optionalMw gin.HandlerFunc) {
	if optionalMw != nil {
		r = r.Group("", optionalMw)
	}
	group := r.Group("articles")
	reviewGroup := r.Group("article-review")
	reviewTaskGroup := r.Group("article-review-tasks")

	authGroup := group.Group("", authMw)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)
	reviewAdminGroup := reviewGroup.Group("", authMw, mw.AdminMiddleware)
	reviewTaskAdminGroup := reviewTaskGroup.Group("", authMw, mw.AdminMiddleware)

	// 公开读
	group.GET("author_info", mw.BindQuery[ArticleAuthorInfoBindRequest], h.AuthorInfo)
	group.GET(":id", mw.BindUri[models.IDRequest], h.Detail)
	group.POST("view", mw.BindJson[ArticleViewCountRequest], h.Visit)
	group.GET("category", mw.BindQuery[CategoryListRequest], h.Categories)
	group.GET("category/options", mw.BindQuery[CategoryOptionsRequest], h.CategoryOptions)
	group.GET("tags", mw.BindQuery[TagListRequest], h.Tags)
	group.GET("tags/options", h.TagOptions)
	group.GET("favorite", mw.BindQuery[FavoriteListRequest], h.Folders)
	group.GET("favorite/contents", mw.BindQuery[FavoriteArticlesRequest], h.FavoriteItems)
	group.GET("top", mw.BindQuery[ArticleTopListRequest], h.Tops)
	reviewAdminGroup.GET("", mw.BindQuery[ArticleReviewTaskListRequest], h.ReviewTasks)

	// 登录用户写
	authGroup.POST("", mw.BindJson[ArticleCreateRequest], h.Create)
	authGroup.PUT(":id", mw.BindUri[models.IDRequest], mw.BindJson[ArticleUpdateRequest], h.Update)
	authGroup.DELETE(":id", mw.BindUri[models.IDRequest], h.Remove)
	authGroup.PUT(":id/digg", mw.BindUri[models.IDRequest], h.Digg)
	authGroup.GET("history", mw.BindQuery[ArticleViewHistoryRequest], h.History)
	authGroup.DELETE("history", mw.BindJson[models.IDListRequest], h.RemoveHistory)
	authGroup.PUT("favorite", mw.BindJson[FavoriteRequest], h.SaveFolder)
	authGroup.DELETE("favorite", mw.BindJson[models.IDListRequest], h.DeleteFolders)
	authGroup.DELETE("favorite/contents", mw.BindJson[FavoriteRemovePatchModel], h.RemoveFavoriteItems)
	authGroup.POST("favorite", mw.BindJson[ArticleFavoriteRequest], h.SaveFavorite)
	authGroup.POST("top", mw.BindJson[ArticleTopSetRequest], h.SetTop)
	authGroup.DELETE("top", mw.BindJson[ArticleTopSetRequest], h.RemoveTop)
	authGroup.POST("category", mw.BindJson[CategoryRequest], h.SaveCategory)
	authGroup.DELETE("category", mw.BindJson[models.IDListRequest], h.DeleteCategories)

	// 管理员
	adminGroup.DELETE("", mw.CaptureLog(mw.ReqBody), mw.BindJson[models.IDListRequest], h.RemoveBatch)
	adminGroup.POST(":id/admin/:visibility", mw.BindUri[ArticleAdminVisibilityURI], h.AdminVisibility)
	adminGroup.PUT("tags", mw.CaptureLog(mw.ReqBody), mw.BindJson[TagRequest], h.SaveTag)
	adminGroup.DELETE("tags", mw.CaptureLog(mw.ReqBody), mw.BindJson[models.IDListRequest], h.DeleteTags)
	reviewTaskAdminGroup.POST(":id/review", mw.CaptureLog(mw.ReqBody), mw.BindUri[models.IDRequest],
		mw.BindJson[ArticleReviewHandleRequest], h.HandleReviewTask)
}
