package http

import (
	"time"

	"myblogx/internal/kernel/ctype"
)

// 本文件是「项目」的请求与响应 DTO（#193：读者侧公开读）。
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
