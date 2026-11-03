package http

import (
	"time"

	"myblogx/internal/kernel/ctype"
)

// 本文件是「项目」的请求与响应 DTO（#193 读者侧公开读 + #197 管理端）。
//
// 契约唯一来源：字段名先在这里定，swagger.json 由注解生成，两端
// openapi.d.ts 从它再生成。不要手写 TS 类型。
//
// 项目与合集的契约形状相同但**是两套定义**：前端按项目入口的命名消费，
// 后端改名/加字段时两边的编译期报错各自独立。

// ── 请求 ──

// ProjectListRequest 项目列表分页。
//
// 与其它列表接口一致用 1-based 页码（page/limit 的旧口径由 pageOffset 收敛）。
type ProjectListRequest struct {
	Page  int `form:"page"`
	Limit int `form:"limit"`
}

// ProjectCreateRequest 建项目。
//
// Displayed 显式传：项目允许"先建好、不展示"，展示开关是运营节奏的一部分。
type ProjectCreateRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Cover       string `json:"cover"`
	Displayed   bool   `json:"displayed"`
}

// ProjectDisplayRequest 切换项目的展示开关。
type ProjectDisplayRequest struct {
	Displayed bool `json:"displayed"`
}

// ProjectUpdateRequest 改项目内容（标题 / 描述 / 封面）。
//
// 与创建同步但**没有 displayed**：展示开关是独立动作（PATCH /display）。
// 改一次标题不该顺手覆盖别人刚切换的展示状态。
// cover 传空串表示回退到首篇文章的封面。
type ProjectUpdateRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Cover       string `json:"cover"`
}

// ProjectArticleRequest 收录 / 移除一篇文章。
//
// 收录与移除共用一个请求体：两者都只认 (项目, 文章) 这一对，动作由 HTTP 方法区分。
type ProjectArticleRequest struct {
	ArticleID ctype.ID `json:"article_id" binding:"required"`
}

// ProjectArticleOrderRequest 项目内的全量顺序替换。
//
// 传的是"完整的有序列表"而不是"把 A 移到 B 前面"：整表替换让服务端不必推断
// 意图，也让并发下的结果可预测（与合集 / 热门置顶同一口径）。
type ProjectArticleOrderRequest struct {
	ArticleIDs []ctype.ID `json:"article_ids" binding:"required"`
}

// ProjectBatchIncludeRequest 批量收录：一次提交多篇。
//
// 与单篇收录共用同一个路径前缀、但走独立的 /batch：单篇入口"重复即报错"，
// 批量入口逐篇返回结局（一次勾十几篇时不该因为一篇重复整单失败）。
type ProjectBatchIncludeRequest struct {
	ArticleIDs []ctype.ID `json:"article_ids" binding:"required"`
}

// ProjectOrderRequest 项目之间的全量顺序替换。
//
// 传**完整**的项目列表（含未展示的项目）：后端在集合不一致时整单拒绝，
// 所以后台提交的是自己刚读到的完整序列。
type ProjectOrderRequest struct {
	ProjectIDs []ctype.ID `json:"project_ids" binding:"required"`
}

// ── 响应 ──

// ProjectCardResponse 是读者站项目列表页的一张卡片。
//
// ArticleCount 只数**仍公开**的文章：卡片写着"20 篇"点进去只有 18 篇是坏体验。
type ProjectCardResponse struct {
	ID           ctype.ID  `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Cover        string    `json:"cover"`
	ArticleCount int       `json:"article_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// ProjectDetailResponse 是读者站项目详情（项目 + 按顺序排列的文章）。
type ProjectDetailResponse struct {
	ID          ctype.ID                     `json:"id"`
	Title       string                       `json:"title"`
	Description string                       `json:"description"`
	Cover       string                       `json:"cover"`
	CreatedAt   time.Time                    `json:"created_at"`
	Articles    []ProjectArticleItemResponse `json:"articles"`
}

// ProjectArticleItemResponse 是项目详情里的一篇文章。
//
// SortOrder 是"我在项目里的位置"：读者要能看出从哪篇开始读。
type ProjectArticleItemResponse struct {
	ArticleID      ctype.ID `json:"article_id"`
	SortOrder      int      `json:"sort_order"`
	Title          string   `json:"title"`
	Abstract       string   `json:"abstract"`
	Cover          string   `json:"cover"`
	ViewCount      int      `json:"view_count"`
	AuthorNickname string   `json:"author_nickname"`
	CategoryTitle  string   `json:"category_title"`
}

// ── 管理端响应（#197） ──

// ProjectAdminCard 是后台项目列表的一行（含未展示的项目）。
type ProjectAdminCard struct {
	ID           ctype.ID `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Cover        string   `json:"cover"`
	Displayed    bool     `json:"displayed"`
	ArticleCount int      `json:"article_count"`
	SortOrder    int      `json:"sort_order"`
	// FallbackCover 是没上传封面时读者看到的封面（首篇公开文章的封面）。
	//
	// 与 cover 分开：后台列表要按读者的样子预览，但编辑表单必须能看出
	// "这张是回退来的"，否则一保存就把回退值固化成上传值。
	FallbackCover string `json:"fallback_cover"`
}

// ProjectCreatedResponse 建项目的返回，只给新 ID。
type ProjectCreatedResponse struct {
	ID ctype.ID `json:"id"`
}

// ProjectAdminItem 是后台「项目编排」页的一行（含已失效占位）。
//
// 与读者侧 ProjectArticleItemResponse 的差别：这一行**保留**了失效条目，
// 且带 stale / stale_reason，后台据此标记与清理。失效行标题为空时（文章已被
// 物理删除）article_id 仍在，页面不能因此丢掉这一行。
//
// 另外，全集换序必须提交**完整**集合（失效行也在内），否则会被服务端的
// project.CheckOrder 整单拒绝。
type ProjectAdminItem struct {
	ArticleID ctype.ID `json:"article_id"`
	SortOrder int      `json:"sort_order"`
	Title     string   `json:"title"`
	Cover     string   `json:"cover"`
	// Stale 文章是否已失效（被删 / 退回草稿 / 不再可见）。判定与热门置顶 / 合集共用。
	Stale bool `json:"stale"`
	// StaleReason 失效原因枚举；仍有效时为空串。枚举值由后端 domain 定义，
	// 前端只做文案映射，不据此另判一次失效。
	StaleReason string `json:"stale_reason"`
}

// ProjectStaleCleanupResponse 一键清理的结果：本次删掉的失效行数。
//
// 返回数量而不是只有"成功"：幂等清理第二次会返回 0，运营据此知道已经清空，
// 而不是怀疑按钮没生效。
type ProjectStaleCleanupResponse struct {
	Removed int `json:"removed"`
}

// ProjectBatchIncludeItem 是批量收录里一篇文章的结局。
//
// outcome 取值：included / duplicated / not_includable / missing。
type ProjectBatchIncludeItem struct {
	ArticleID ctype.ID `json:"article_id"`
	Outcome   string   `json:"outcome"`
}

// ProjectBatchIncludeResponse 是批量收录的汇总。
//
// 三个计数与 results 是同一份事实的两种视图：前端直接用计数拼提示，
// 需要逐篇标记时读 results。
type ProjectBatchIncludeResponse struct {
	Results    []ProjectBatchIncludeItem `json:"results"`
	Included   int                       `json:"included"`
	Duplicated int                       `json:"duplicated"`
	Rejected   int                       `json:"rejected"`
}
