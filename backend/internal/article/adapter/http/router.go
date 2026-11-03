package http

import (
	"github.com/gin-gonic/gin"
	"myblogx/internal/kernel"

	mw "myblogx/internal/platform/http/middleware"
)

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
	collectionGroup := r.Group("collections")
	projectGroup := r.Group("projects")
	reviewGroup := r.Group("article-review")
	reviewTaskGroup := r.Group("article-review-tasks")

	authGroup := group.Group("", authMw)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)
	reviewAdminGroup := reviewGroup.Group("", authMw, mw.AdminMiddleware)
	collectionAdminGroup := collectionGroup.Group("", authMw, mw.AdminMiddleware)
	projectAdminGroup := projectGroup.Group("", authMw, mw.AdminMiddleware)
	reviewTaskAdminGroup := reviewTaskGroup.Group("", authMw, mw.AdminMiddleware)

	// 公开读
	group.GET("author_info", mw.BindQuery[ArticleAuthorInfoBindRequest], h.AuthorInfo)
	group.GET(":id", mw.BindUri[kernel.IDRequest], h.Detail)
	// 文章上下文（阅读页左栏目录树）：公开读；`from` 缺失时回退到第一棵归属树。
	group.GET(":id/outline-context", mw.BindUri[kernel.IDRequest],
		mw.BindQuery[ArticleOutlineContextRequest], h.ArticleOutlineContext)
	group.POST("view", mw.BindJson[ArticleViewCountRequest], h.Visit)
	group.GET("category", mw.BindQuery[CategoryListRequest], h.Categories)
	group.GET("category/options", mw.BindQuery[CategoryOptionsRequest], h.CategoryOptions)
	group.GET("tags", mw.BindQuery[TagListRequest], h.Tags)
	group.GET("tags/options", h.TagOptions)
	group.GET("favorite", mw.BindQuery[FavoriteListRequest], h.Folders)
	group.GET("favorite/contents", mw.BindQuery[FavoriteArticlesRequest], h.FavoriteItems)
	group.GET("top", mw.BindQuery[ArticleTopListRequest], h.Tops)
	// 热门置顶运营位（与上面的作者置顶是两套数据）。公开读：首页区块。
	group.GET("hot-top", h.HotTops)
	// 合集（读者侧只读：列表页 + 详情页）。
	collectionGroup.GET("", mw.BindQuery[CollectionListRequest], h.Collections)
	collectionGroup.GET(":id", mw.BindUri[kernel.IDRequest], h.CollectionDetail)
	// 项目（与合集并列的独立内容实体；读者侧同样只读：列表页 + 详情页）。
	projectGroup.GET("", mw.BindQuery[ProjectListRequest], h.Projects)
	projectGroup.GET(":id", mw.BindUri[kernel.IDRequest], h.ProjectDetail)
	reviewAdminGroup.GET("", mw.BindQuery[ArticleReviewTaskListRequest], h.ReviewTasks)

	// 登录用户写
	authGroup.POST("", mw.BindJson[ArticleCreateRequest], h.Create)
	authGroup.PUT(":id", mw.BindUri[kernel.IDRequest], mw.BindJson[ArticleUpdateRequest], h.Update)
	authGroup.DELETE(":id", mw.BindUri[kernel.IDRequest], h.Remove)
	authGroup.PUT(":id/digg", mw.BindUri[kernel.IDRequest], mw.BindJson[ArticleDiggRequest], h.Digg)
	authGroup.GET("history", mw.BindQuery[ArticleViewHistoryRequest], h.History)
	authGroup.DELETE("history", mw.BindJson[kernel.IDListRequest], h.RemoveHistory)
	authGroup.PUT("favorite", mw.BindJson[FavoriteRequest], h.SaveFolder)
	authGroup.DELETE("favorite", mw.BindJson[kernel.IDListRequest], h.DeleteFolders)
	authGroup.DELETE("favorite/contents", mw.BindJson[FavoriteRemovePatchModel], h.RemoveFavoriteItems)
	authGroup.POST("favorite", mw.BindJson[ArticleFavoriteRequest], h.SaveFavorite)
	authGroup.POST("top", mw.BindJson[ArticleTopSetRequest], h.SetTop)
	authGroup.DELETE("top", mw.BindJson[ArticleTopSetRequest], h.RemoveTop)
	authGroup.POST("category", mw.BindJson[CategoryRequest], h.SaveCategory)
	authGroup.DELETE("category", mw.BindJson[kernel.IDListRequest], h.DeleteCategories)

	// 管理员
	adminGroup.DELETE("", mw.CaptureLog(mw.ReqBody), mw.BindJson[kernel.IDListRequest], h.RemoveBatch)
	adminGroup.POST(":id/admin/:visibility", mw.BindUri[ArticleAdminVisibilityURI], h.AdminVisibility)
	// 热门置顶的运营动作：设置/取消（幂等）与整表排序。
	adminGroup.GET("hot-top/admin", h.AdminHotTops)
	adminGroup.POST(":id/admin/hot-top", mw.BindUri[kernel.IDRequest], h.SetHotTop)
	adminGroup.DELETE(":id/admin/hot-top", mw.BindUri[kernel.IDRequest], h.RemoveHotTop)
	adminGroup.PATCH("hot-top/order", mw.CaptureLog(mw.ReqBody),
		mw.BindJson[ArticleHotTopOrderRequest], h.ReplaceHotTopOrder)
	// 合集管理（全部在管理员分组下）。
	collectionAdminGroup.GET("admin", h.AdminCollections)
	collectionAdminGroup.POST("", mw.CaptureLog(mw.ReqBody), mw.BindJson[CollectionCreateRequest], h.CreateCollection)
	collectionAdminGroup.PUT(":id", mw.CaptureLog(mw.ReqBody),
		mw.BindUri[kernel.IDRequest], mw.BindJson[CollectionUpdateRequest], h.UpdateCollection)
	collectionAdminGroup.DELETE(":id", mw.BindUri[kernel.IDRequest], h.DeleteCollection)
	collectionAdminGroup.PATCH(":id/display", mw.CaptureLog(mw.ReqBody),
		mw.BindUri[kernel.IDRequest], mw.BindJson[CollectionDisplayRequest], h.SetCollectionDisplay)
	// 目录树：后台读回整棵树 + 整棵树一次性提交。
	// 这是合集内容编排的**唯一**入口：旧的扁平收录 / 排序 / 失效清理接口已随
	// 旧中间表一起退役（#225），不在这里留任何单步写入口。
	collectionAdminGroup.GET(":id/outline/admin", mw.BindUri[kernel.IDRequest], h.AdminCollectionOutline)
	collectionAdminGroup.PUT(":id/outline", mw.CaptureLog(mw.ReqBody),
		mw.BindUri[kernel.IDRequest], mw.BindJson[OutlineSaveRequest], h.SaveCollectionOutline)
	// 项目管理（全部在管理员分组下）：与合集逐条对齐，路径换成 /projects。
	projectAdminGroup.GET("admin", h.AdminProjects)
	projectAdminGroup.POST("", mw.CaptureLog(mw.ReqBody), mw.BindJson[ProjectCreateRequest], h.CreateProject)
	projectAdminGroup.PUT(":id", mw.CaptureLog(mw.ReqBody),
		mw.BindUri[kernel.IDRequest], mw.BindJson[ProjectUpdateRequest], h.UpdateProject)
	projectAdminGroup.DELETE(":id", mw.BindUri[kernel.IDRequest], h.DeleteProject)
	projectAdminGroup.PATCH(":id/display", mw.CaptureLog(mw.ReqBody),
		mw.BindUri[kernel.IDRequest], mw.BindJson[ProjectDisplayRequest], h.SetProjectDisplay)
	// 项目之间排序：落 project_models.sort_order，是现行功能（不随旧表退役）。
	projectAdminGroup.PATCH("order", mw.CaptureLog(mw.ReqBody),
		mw.BindJson[ProjectOrderRequest], h.ReplaceProjectSortOrder)
	// 目录树（与合集同一套读写形状，路径换成 /projects）。
	projectAdminGroup.GET(":id/outline/admin", mw.BindUri[kernel.IDRequest], h.AdminProjectOutline)
	projectAdminGroup.PUT(":id/outline", mw.CaptureLog(mw.ReqBody),
		mw.BindUri[kernel.IDRequest], mw.BindJson[OutlineSaveRequest], h.SaveProjectOutline)

	adminGroup.PUT("tags", mw.CaptureLog(mw.ReqBody), mw.BindJson[TagRequest], h.SaveTag)
	adminGroup.DELETE("tags", mw.CaptureLog(mw.ReqBody), mw.BindJson[kernel.IDListRequest], h.DeleteTags)
	reviewTaskAdminGroup.POST(":id/review", mw.CaptureLog(mw.ReqBody), mw.BindUri[kernel.IDRequest],
		mw.BindJson[ArticleReviewHandleRequest], h.HandleReviewTask)
}
