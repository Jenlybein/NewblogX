package http

import (
	"myblogx/internal/article/domain/outline"
	"myblogx/internal/kernel/ctype"
)

// 本文件是「合集目录树」「项目目录树」两个具名形状**复用**的叶子 DTO。
//
// 两个端点各有自己的请求 / 响应外壳（见 dto_named_outline.go），但节点行与
// 文章引用是同一份字段；这里不按实体复制两套，也不留旧共享端点的外壳。

// ── 请求 ──

// OutlineNodeRequest 是整树提交里的一个节点。
//
// 嵌套表达父子关系：Children 非空时本节点必须是文件夹（文章节点不能有子节点，
// 由 app/domain 拒绝）。已有节点带 id；新节点留空，服务端新建。
type OutlineNodeRequest struct {
	ID        ctype.ID             `json:"id"`
	Kind      int                  `json:"kind" binding:"required,oneof=1 2" swaggertype:"integer" enums:"1,2"`
	Title     string               `json:"title"`
	ArticleID ctype.ID             `json:"article_id"`
	Children  []OutlineNodeRequest `json:"children"`
}

// ── 响应 ──

// OutlineNodeResponse 是后台树编辑器读回的一行（嵌套）。
//
// 文件夹节点：kind=1、article_id 为空、title 是文件夹标题。
// 文章节点：kind=2、title 为空、article_title / article_cover 是文章快照。
// stale 只对文章节点有意义：文章后来被删 / 退回草稿 / 隐藏时节点保留并标失效。
type OutlineNodeResponse struct {
	ID           ctype.ID              `json:"id"`
	ParentID     ctype.ID              `json:"parent_id"`
	Kind         int                   `json:"kind"`
	Title        string                `json:"title"`
	ArticleID    ctype.ID              `json:"article_id"`
	SortOrder    int                   `json:"sort_order"`
	ArticleTitle string                `json:"article_title"`
	ArticleCover string                `json:"article_cover"`
	Stale        bool                  `json:"stale"`
	StaleReason  string                `json:"stale_reason"`
	Children     []OutlineNodeResponse `json:"children"`
}

// OutlineTreeResponse 是后台读回的整棵嵌套树（含空文件夹）。
type OutlineTreeResponse struct {
	OwnerKind int                   `json:"owner_kind"`
	OwnerID   ctype.ID              `json:"owner_id"`
	Nodes     []OutlineNodeResponse `json:"nodes"`
}

// OutlineArticleRefResponse 是具名树读模型里"当前 / 上一篇 / 下一篇"的一行。
//
// 只带点进阅读页必需的字段（节点 ID、文章 ID、标题快照、封面快照），
// 不含 children：它不是一棵树，只是相邻文章的引用。
type OutlineArticleRefResponse struct {
	ID           ctype.ID `json:"id"`
	ArticleID    ctype.ID `json:"article_id"`
	ArticleTitle string   `json:"article_title"`
	ArticleCover string   `json:"article_cover"`
}

// ── 内部读模型 → DTO（两个具名端点与两个详情响应共用同一份组装） ──

func toOutlineTreeResponse(detail *outline.Detail) OutlineTreeResponse {
	return OutlineTreeResponse{
		OwnerKind: int(detail.OwnerKind),
		OwnerID:   ctype.ID(detail.OwnerID),
		Nodes:     toOutlineNodeResponses(detail.Tree),
	}
}

func toOutlineNodeResponses(items []outline.ViewNode) []OutlineNodeResponse {
	out := make([]OutlineNodeResponse, 0, len(items))
	for _, item := range items {
		row := OutlineNodeResponse{
			ID:           ctype.ID(item.Node.ID()),
			ParentID:     ctype.ID(item.Node.ParentID()),
			Kind:         int(item.Node.Kind()),
			Title:        item.Node.Title(),
			ArticleID:    ctype.ID(item.Node.ArticleID()),
			SortOrder:    item.Node.SortOrder(),
			ArticleTitle: item.ArticleTitle,
			ArticleCover: item.ArticleCover,
			Stale:        item.Stale(),
			StaleReason:  string(item.StaleReason()),
			Children:     toOutlineNodeResponses(item.Children),
		}
		out = append(out, row)
	}
	return out
}

// toOutlineArticleRefResponse 把树上的文章节点压成"相邻文章"引用。
func toOutlineArticleRefResponse(item *outline.ViewNode) *OutlineArticleRefResponse {
	if item == nil || !item.Node.IsArticle() {
		return nil
	}
	return &OutlineArticleRefResponse{
		ID:           ctype.ID(item.Node.ID()),
		ArticleID:    ctype.ID(item.Node.ArticleID()),
		ArticleTitle: item.ArticleTitle,
		ArticleCover: item.ArticleCover,
	}
}
