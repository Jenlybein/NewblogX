package http

import (
	"github.com/gin-gonic/gin"
	"myblogx/internal/article/domain/outline"
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

	// 站点地图（#314）：挂在 /api 下而不是 /api/articles 下——它不是"文章详情"，
	// 而且 /api/articles/sitemap.xml 会被 /articles/:id 抢走（:id 会拿到 "sitemap.xml"）。
	// nginx 只把 /api/ 与 /uploads/ 转给后端，所以它必须落在这个前缀内；
	// 前端 robots.txt 的 Sitemap: 行指向的就是这里。
	r.GET("sitemap.xml", h.Sitemap)

	authGroup := group.Group("", authMw)
	// 个人数据概览（票 354）：登录用户读自己的，不是管理员专属。
	// 路径里带连字符前缀，避免被 /articles/:id 抢走。
	authGroup.GET("studio-stats", h.StudioStats)
	adminGroup := authGroup.Group("", mw.AdminMiddleware)
	reviewAdminGroup := reviewGroup.Group("", authMw, mw.AdminMiddleware)
	collectionAdminGroup := collectionGroup.Group("", authMw, mw.AdminMiddleware)
	projectAdminGroup := projectGroup.Group("", authMw, mw.AdminMiddleware)
	reviewTaskAdminGroup := reviewTaskGroup.Group("", authMw, mw.AdminMiddleware)

	// 公开读
	group.GET("author_info", mw.BindQuery[ArticleAuthorInfoBindRequest], h.AuthorInfo)
	group.GET(":id", mw.BindUri[kernel.IDRequest], h.Detail)
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
	// 合集（读者侧只读：列表页 + 详情页 + 合集目录树）。
	collectionGroup.GET("", mw.BindQuery[CollectionListRequest], h.Collections)
	collectionGroup.GET(":id", mw.BindUri[kernel.IDRequest], h.CollectionDetail)
	collectionGroup.GET(":id/outline-tree", mw.BindUri[kernel.IDRequest],
		mw.BindQuery[NamedOutlineTreeRequest], h.CollectionOutlineTree)
	// 读者侧分组读面（票 353）：只给"谁和谁在一起、谁在前"，
	// 没有「未分组」标题，也没有后台那些编排动作。
	collectionGroup.GET("grouped", BindKind(outline.OwnerCollection, h.PublicGroupTreeView))
	// 项目（与合集并列的独立内容实体；读者侧同样只读：列表页 + 详情页 + 项目目录树）。
	projectGroup.GET("", mw.BindQuery[ProjectListRequest], h.Projects)
	projectGroup.GET(":id", mw.BindUri[kernel.IDRequest], h.ProjectDetail)
	projectGroup.GET(":id/outline-tree", mw.BindUri[kernel.IDRequest],
		mw.BindQuery[NamedOutlineTreeRequest], h.ProjectOutlineTree)
	projectGroup.GET("grouped", BindKind(outline.OwnerProject, h.PublicGroupTreeView))
	reviewAdminGroup.GET("", mw.BindQuery[ArticleReviewTaskListRequest], h.ReviewTasks)
	// 驳回原因的固定列表：值与文案都从后端唯一词表出（#308），前端据此渲染下拉。
	reviewAdminGroup.GET("reasons", h.ReviewReasons)

	// 登录用户写
	authGroup.POST("", mw.BindJson[ArticleCreateRequest], h.Create)
	authGroup.PUT(":id", mw.BindUri[kernel.IDRequest], mw.BindJson[ArticleUpdateRequest], h.Update)
	authGroup.DELETE(":id", mw.BindUri[kernel.IDRequest], h.Remove)
	authGroup.PUT(":id/digg", mw.BindUri[kernel.IDRequest], mw.BindJson[ArticleDiggRequest], h.Digg)
	authGroup.GET("history", mw.BindQuery[ArticleViewHistoryRequest], h.History)
	authGroup.DELETE("history", mw.BindJson[kernel.IDListRequest], h.RemoveHistory)
	// 清空历史：单独的路径而不是"id_list 为空即清空"——后者会把一个漏传/空数组的
	// 请求变成不可逆的全量删除，误伤面太大（前端另有二次确认）。
	authGroup.DELETE("history/all", h.ClearHistory)
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
	// 合集目录树：后台读回整棵树 + 整棵树一次性提交，写入
	// collection_outline_node_models。这是合集内容编排的唯一入口。
	collectionAdminGroup.PUT(":id/outline-tree", mw.CaptureLog(mw.ReqBody),
		mw.BindUri[kernel.IDRequest], mw.BindJson[CollectionOutlineSaveRequest],
		h.SaveCollectionOutlineTree)
	// 合集分组（ADR-0019）：读面 / 建改名 / 删除 / 整树换序。
	//
	// 分组 CRUD 立即生效，不参与拖拽的 10 秒防抖；整树换序是幂等整份覆盖。
	// 路径里带 :kind 是为了让两个实体共用同一组 handler —— 语义完全一致，
	// 写两份只会让"删分组时把条目退回未分组"这种差异只修在一边。
	collectionAdminGroup.GET("groups", BindKind(outline.OwnerCollection, h.GroupTreeView))
	collectionAdminGroup.PUT("groups", mw.CaptureLog(mw.ReqBody), mw.BindJson[GroupSaveRequest],
		BindKind(outline.OwnerCollection, h.GroupSaveView))
	collectionAdminGroup.DELETE("groups/:id", mw.BindUri[kernel.IDRequest],
		BindKind(outline.OwnerCollection, h.GroupDeleteView))
	collectionAdminGroup.PUT("groups/order", mw.CaptureLog(mw.ReqBody), mw.BindJson[GroupOrderRequest],
		BindKind(outline.OwnerCollection, h.GroupOrderView))
	// 字段 + 目录树一次原子提交：编辑弹窗里"改标题 + 拖完树"是一次保存动作。
	collectionAdminGroup.PUT(":id/compose", mw.CaptureLog(mw.ReqBody),
		mw.BindUri[kernel.IDRequest], mw.BindJson[CollectionComposeRequest], h.CollectionComposeView)
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
	// 项目目录树（与合集同一套读写形状，路径换成 /projects），写入
	// project_outline_node_models。
	projectAdminGroup.PUT(":id/outline-tree", mw.CaptureLog(mw.ReqBody),
		mw.BindUri[kernel.IDRequest], mw.BindJson[ProjectOutlineSaveRequest],
		h.SaveProjectOutlineTree)
	// 项目分组（与合集逐条对齐，路径换成 /projects）。
	projectAdminGroup.GET("groups", BindKind(outline.OwnerProject, h.GroupTreeView))
	projectAdminGroup.PUT("groups", mw.CaptureLog(mw.ReqBody), mw.BindJson[GroupSaveRequest],
		BindKind(outline.OwnerProject, h.GroupSaveView))
	projectAdminGroup.DELETE("groups/:id", mw.BindUri[kernel.IDRequest],
		BindKind(outline.OwnerProject, h.GroupDeleteView))
	projectAdminGroup.PUT("groups/order", mw.CaptureLog(mw.ReqBody), mw.BindJson[GroupOrderRequest],
		BindKind(outline.OwnerProject, h.GroupOrderView))
	projectAdminGroup.PUT(":id/compose", mw.CaptureLog(mw.ReqBody),
		mw.BindUri[kernel.IDRequest], mw.BindJson[ProjectComposeRequest], h.ProjectComposeView)

	adminGroup.PUT("tags", mw.CaptureLog(mw.ReqBody), mw.BindJson[TagRequest], h.SaveTag)
	adminGroup.DELETE("tags", mw.CaptureLog(mw.ReqBody), mw.BindJson[kernel.IDListRequest], h.DeleteTags)
	// 标签之间的拖拽排序（票 346）：落既有 tag_models.sort，是全量顺序替换。
	adminGroup.PATCH("tags/order", mw.CaptureLog(mw.ReqBody),
		mw.BindJson[TagOrderRequest], h.ReplaceTagSortOrder)
	reviewTaskAdminGroup.POST(":id/review", mw.CaptureLog(mw.ReqBody), mw.BindUri[kernel.IDRequest],
		mw.BindJson[ArticleReviewHandleRequest], h.HandleReviewTask)
	// 正文按需读（2026-11-13）：列表不再带正文，审核员要看全文时点一次这里。
	reviewTaskAdminGroup.GET(":id/content", mw.BindUri[kernel.IDRequest], h.ReviewTaskContent)
	// 批量：与单条同一份用例、同一套原因校验，只是把同一个决定应用到多个任务
	// （spec 决策 32）。挂在 tasks 前缀下而不是 review 下，因为入参是任务 ID。
	reviewTaskAdminGroup.POST("batch-review", mw.CaptureLog(mw.ReqBody),
		mw.BindJson[ArticleReviewBatchRequest], h.BatchHandleReviewTasks)
}
