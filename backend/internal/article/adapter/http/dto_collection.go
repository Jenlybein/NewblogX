package http

import (
	"time"

	"myblogx/internal/kernel/ctype"
)

// 本文件是「合集」的请求与响应 DTO。
//
// 契约唯一来源：字段名先在这里定，swagger.json 由注解生成，两端
// openapi.d.ts 从它再生成。不要手写 TS 类型。

// ── 请求 ──

// CollectionListRequest 合集列表分页。
//
// 与其它列表接口一致用 1-based 页码（page/pageSize 的旧口径由 pageOffset 收敛）。
type CollectionListRequest struct {
	Page  int `form:"page"`
	Limit int `form:"limit"`
}

// CollectionCreateRequest 建合集。
//
// Displayed 显式传：合集允许"先建好、不展示"，展示开关是运营节奏的一部分。
type CollectionCreateRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Cover       string `json:"cover"`
	Displayed   bool   `json:"displayed"`
}

// CollectionDisplayRequest 切换合集的展示开关。
type CollectionDisplayRequest struct {
	Displayed bool `json:"displayed"`
}

// CollectionUpdateRequest 改合集内容（标题 / 描述 / 封面）。
//
// 与创建同步但**没有 displayed**：展示开关是独立动作（PATCH /display）。
// 改一次标题不该顺手覆盖别人刚切换的展示状态。
// cover 传空串表示回退到首篇文章的封面。
type CollectionUpdateRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Cover       string `json:"cover"`
}

// CollectionArticleRequest 收录 / 移除一篇文章。
//
// 收录与移除共用一个请求体：两者都只认 (合集, 文章) 这一对，动作由 HTTP 方法区分。
type CollectionArticleRequest struct {
	ArticleID ctype.ID `json:"article_id" binding:"required"`
}

// CollectionArticleOrderRequest 合集内的全量顺序替换。
//
// 传的是"完整的有序列表"而不是"把 A 移到 B 前面"：整表替换让服务端不必推断
// 意图，也让并发下的结果可预测（与热门置顶同一口径）。
type CollectionArticleOrderRequest struct {
	ArticleIDs []ctype.ID `json:"article_ids" binding:"required"`
}

// CollectionBatchIncludeRequest 批量收录：一次提交多篇。
//
// 与单篇收录共用同一个路径前缀、但走独立的 /batch：单篇入口"重复即报错"，
// 批量入口逐篇返回结局（一次勾十几篇时不该因为一篇重复整单失败）。
type CollectionBatchIncludeRequest struct {
	ArticleIDs []ctype.ID `json:"article_ids" binding:"required"`
}

// CollectionOrderRequest 合集之间的全量顺序替换。
//
// 传**完整**的合集列表（含未展示的合集）：后端在集合不一致时整单拒绝，
// 所以后台提交的是自己刚读到的完整序列。
type CollectionOrderRequest struct {
	CollectionIDs []ctype.ID `json:"collection_ids" binding:"required"`
}

// ── 响应 ──

// CollectionCardResponse 是读者站合集列表页的一张卡片。
//
// ArticleCount 只数**仍公开**的文章：卡片写着"20 篇"点进去只有 18 篇是坏体验。
type CollectionCardResponse struct {
	ID           ctype.ID  `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Cover        string    `json:"cover"`
	ArticleCount int       `json:"article_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// CollectionDetailResponse 是读者站合集详情（合集 + 按顺序排列的文章）。
type CollectionDetailResponse struct {
	ID          ctype.ID                        `json:"id"`
	Title       string                          `json:"title"`
	Description string                          `json:"description"`
	Cover       string                          `json:"cover"`
	CreatedAt   time.Time                       `json:"created_at"`
	Articles    []CollectionArticleItemResponse `json:"articles"`
}

// CollectionArticleItemResponse 是合集详情里的一篇文章。
//
// SortOrder 是"我在系列里的位置"：读者要能看出从哪篇开始读。
type CollectionArticleItemResponse struct {
	ArticleID      ctype.ID `json:"article_id"`
	SortOrder      int      `json:"sort_order"`
	Title          string   `json:"title"`
	Abstract       string   `json:"abstract"`
	Cover          string   `json:"cover"`
	ViewCount      int      `json:"view_count"`
	AuthorNickname string   `json:"author_nickname"`
	CategoryTitle  string   `json:"category_title"`
}

// CollectionAdminCard 是后台合集列表的一行（含未展示的合集）。
type CollectionAdminCard struct {
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

// CollectionCreatedResponse 建合集的返回，只给新 ID。
//
// 只回 ID 而不是整张卡片：调用方（后台）接下来要跳去"编辑收录"，
// 需要的正是这个 ID；回整张卡片会多一份可能过期的读模型。
type CollectionCreatedResponse struct {
	ID ctype.ID `json:"id"`
}

// CollectionAdminItem 是后台「合集收录」页的一行（含已失效占位）。
//
// 与读者侧 CollectionArticleItemResponse 的差别：这一行**保留**了失效条目，
// 且带 stale / stale_reason，后台据此标记与清理。失效行标题为空时（文章已被
// 物理删除）article_id 仍在，页面不能因此丢掉这一行。
//
// 另外，全集换序必须提交**完整**集合（失效行也在内），否则会被服务端的
// CheckOrder 整单拒绝。
type CollectionAdminItem struct {
	ArticleID ctype.ID `json:"article_id"`
	SortOrder int      `json:"sort_order"`
	Title     string   `json:"title"`
	Cover     string   `json:"cover"`
	// Stale 文章是否已失效（被删 / 退回草稿 / 不再可见）。判定与热门置顶共用。
	Stale bool `json:"stale"`
	// StaleReason 失效原因枚举；仍有效时为空串。枚举值由后端 domain 定义，
	// 前端只做文案映射，不据此另判一次失效。
	StaleReason string `json:"stale_reason"`
}

// CollectionStaleCleanupResponse 一键清理的结果：本次删掉的失效行数。
//
// 返回数量而不是只有"成功"：幂等清理第二次会返回 0，运营据此知道已经清空，
// 而不是怀疑按钮没生效。
type CollectionStaleCleanupResponse struct {
	Removed int `json:"removed"`
}

// CollectionBatchIncludeItem 是批量收录里一篇文章的结局。
//
// outcome 取值：included / duplicated / not_includable / missing。
// 逐篇返回而不是只给总数，后台才能告诉运营"哪几篇重复了"。
type CollectionBatchIncludeItem struct {
	ArticleID ctype.ID `json:"article_id"`
	Outcome   string   `json:"outcome"`
}

// CollectionBatchIncludeResponse 是批量收录的汇总。
//
// 三个计数与 results 是同一份事实的两种视图：前端直接用计数拼提示，
// 需要逐篇标记时读 results。
type CollectionBatchIncludeResponse struct {
	Results    []CollectionBatchIncludeItem `json:"results"`
	Included   int                          `json:"included"`
	Duplicated int                          `json:"duplicated"`
	Rejected   int                          `json:"rejected"`
}
