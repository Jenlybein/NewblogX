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

// ArticleOutlineContextRequest 是阅读页左栏的查询参数。
//
// from = `<kind>:<id>`（project / collection）；缺失或格式不合法时由用例层
// 回退到第一棵归属树，handler 不做第二份格式判定。
type ArticleOutlineContextRequest struct {
	From string `json:"from" form:"from"`
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

// OutlineArticleRefResponse 是文章上下文里"当前 / 上一篇 / 下一篇"的一行。
//
// 只带点进阅读页必需的字段（节点 ID、文章 ID、标题快照、封面快照），
// 不含 children：它不是一棵树，只是相邻文章的引用。
type OutlineArticleRefResponse struct {
	ID           ctype.ID `json:"id"`
	ArticleID    ctype.ID `json:"article_id"`
	ArticleTitle string   `json:"article_title"`
	ArticleCover string   `json:"article_cover"`
}

// ArticleOutlineContextResponse 是阅读页左栏的读模型。
//
// has_tree=false 表示这篇文章不属于任何读者可见的树：页面据此不渲染左栏
// （右栏标题大纲与阅读进度保持原样）。previous / next 可为 null（第一篇
// 没有上一篇，最后一篇没有下一篇）。
type ArticleOutlineContextResponse struct {
	HasTree   bool                       `json:"has_tree"`
	OwnerKind int                        `json:"owner_kind"`
	OwnerID   ctype.ID                   `json:"owner_id"`
	Nodes     []OutlineNodeResponse      `json:"nodes"`
	Current   *OutlineArticleRefResponse `json:"current"`
	Previous  *OutlineArticleRefResponse `json:"previous"`
	Next      *OutlineArticleRefResponse `json:"next"`
}
