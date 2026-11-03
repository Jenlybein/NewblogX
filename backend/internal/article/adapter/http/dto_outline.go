package http

import "myblogx/internal/kernel/ctype"

// 本文件是「目录树」的请求与响应 DTO。
//
// 一棵树服务合集与项目：路径前缀不同（/collections 与 /projects），
// 请求 / 响应形状完全相同，不按实体复制两套字段。

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

// OutlineSaveRequest 是整棵树一次性提交。
//
// 兄弟顺序就是数组顺序（服务端按 DFS 顺序落 sort_order）；
// 不带 deleted / 失效字段：移除节点 = 不在这次提交里出现。
type OutlineSaveRequest struct {
	Nodes []OutlineNodeRequest `json:"nodes"`
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
