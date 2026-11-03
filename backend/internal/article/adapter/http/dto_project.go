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

// ProjectDetailResponse 是读者站项目详情（项目 + 嵌套目录树）。
//
// 与合集共用 OutlineTreeResponse 形状（owner_kind 区分），扁平 articles[] 已退役。
type ProjectDetailResponse struct {
	ID           ctype.ID            `json:"id"`
	Title        string              `json:"title"`
	Description  string              `json:"description"`
	Cover        string              `json:"cover"`
	CreatedAt    time.Time           `json:"created_at"`
	Outline      OutlineTreeResponse `json:"outline"`
	ArticleCount int                 `json:"article_count"`
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
