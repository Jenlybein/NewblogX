package http

import (
	"myblogx/internal/article/app"
	"myblogx/internal/kernel/ctype"
)

// 本文件是合集 / 项目分组的请求与响应形状（ADR-0019）。
//
// 两个实体各有一份具名请求体，但**读面形状共用**：分组是两个实体共用的编排
// 语义，字段一模一样；分成两个 schema 只会让前端多维护一份同样的映射。
// 归属由端点身份给出（/collections/... 还是 /projects/...），不在请求体里传。

// GroupItemResponse 是分组里的一个条目（只有一个 ID：顺序由数组位置表达）。
type GroupItemResponse struct {
	OwnerID ctype.ID `json:"owner_id"`
}

// GroupNodeResponse 是一个分组及其条目顺序。
type GroupNodeResponse struct {
	ID    ctype.ID            `json:"id"`
	Title string              `json:"title"`
	Items []GroupItemResponse `json:"items"`
}

// GroupTreeResponse 是后台分组读面。
//
// Ungrouped 单独一段而不是一个"分组"：未分组固定排在最前，且在没有未分组条目
// 时整段消失 —— 它不是一种分组，不需要一条数据行来代表它。
type GroupTreeResponse struct {
	Groups    []GroupNodeResponse `json:"groups"`
	Ungrouped []GroupItemResponse `json:"ungrouped"`
}

// GroupSaveRequest 是新建 / 改名分组的请求体。
//
// ID 为空表示新建；给了就是改名。与标签的写法一致：一个端点两种语义，
// 因为"建"与"改名"在界面上是同一个弹窗。
type GroupSaveRequest struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title" binding:"required"`
}

// GroupOrderRequest 是整树顺序的整份覆盖请求体。
//
// 语义是幂等整份覆盖：提交的一定是界面上最终看到的排列。后端在集合与当前库
// 不一致时整单拒绝（有人并发建 / 删了分组或条目），前端据此刷新后重排。
type GroupOrderRequest struct {
	Groups    []GroupNodeRequest `json:"groups"`
	Ungrouped []ctype.ID         `json:"ungrouped"`
}

// GroupNodeRequest 是提交里的一个分组：ID + 组内条目顺序。
type GroupNodeRequest struct {
	ID    ctype.ID   `json:"id" binding:"required"`
	Items []ctype.ID `json:"items"`
}

// CollectionComposeRequest 是合集"字段 + 目录树"的一次原子提交。
type CollectionComposeRequest struct {
	Title       string               `json:"title" binding:"required"`
	Description string               `json:"description"`
	Cover       string               `json:"cover"`
	Nodes       []OutlineNodeRequest `json:"nodes"`
}

// ProjectComposeRequest 是项目"字段 + 目录树"的一次原子提交。
type ProjectComposeRequest struct {
	Title       string               `json:"title" binding:"required"`
	Description string               `json:"description"`
	Cover       string               `json:"cover"`
	Nodes       []OutlineNodeRequest `json:"nodes"`
}

// toGroupTreeResponse 把领域读面翻译成响应。
func toGroupTreeResponse(tree *app.GroupTree) GroupTreeResponse {
	out := GroupTreeResponse{Groups: make([]GroupNodeResponse, 0), Ungrouped: make([]GroupItemResponse, 0)}
	if tree == nil {
		return out
	}
	for _, g := range tree.Groups {
		node := GroupNodeResponse{ID: ctype.ID(g.ID), Title: g.Title, Items: make([]GroupItemResponse, 0, len(g.Items))}
		for _, owner := range g.Items {
			node.Items = append(node.Items, GroupItemResponse{OwnerID: owner})
		}
		out.Groups = append(out.Groups, node)
	}
	for _, owner := range tree.Ungrouped {
		out.Ungrouped = append(out.Ungrouped, GroupItemResponse{OwnerID: owner})
	}
	return out
}


