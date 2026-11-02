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
}

// CollectionCreatedResponse 建合集的返回，只给新 ID。
//
// 只回 ID 而不是整张卡片：调用方（后台）接下来要跳去"编辑收录"，
// 需要的正是这个 ID；回整张卡片会多一份可能过期的读模型。
type CollectionCreatedResponse struct {
	ID ctype.ID `json:"id"`
}
