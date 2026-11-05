package http

import "myblogx/internal/kernel/ctype"

// 本文件是 #274 expand 期两个**具名形状**及其请求 DTO。
//
// 合集与项目各有一份具名响应形状（契约里是两个独立的 schema），不复用旧的
// OutlineTreeResponse：两个实体从此各自有一个对外真相。节点 / 文章引用仍是
// 结构性的子形状，沿用共享定义（它们的字段口径没变）。
//
// owner_kind / owner_id **保留在具名形状里**：它们是读者侧 `?from=` 派生链
// （outlineKind.ts / outlineContext.ts / 折叠态 / 左栏 panelKey）的输入。
// 「去掉归属列」只针对两张新树表的列，不针对响应形状；保留它们不需要任何兼容层，
// 链接与来路判定逐字不变。表名已经表达归属，响应里带的是**呈现层要用的来源**。

// ── 请求 ──

// NamedOutlineTreeRequest 是具名树端点的查询参数。
//
// from = `<kind>:<ownerId>`，由阅读页原样透传；article_id 是阅读页当前文章，
// 为空时不回答上一章 / 下一章。格式校验与回退都在用例层一处。
type NamedOutlineTreeRequest struct {
	From      string   `json:"from" form:"from"`
	ArticleID ctype.ID `json:"article_id" form:"article_id"`
}

// CollectionOutlineSaveRequest 是合集具名树的整树提交。
type CollectionOutlineSaveRequest struct {
	Nodes []OutlineNodeRequest `json:"nodes"`
}

// ProjectOutlineSaveRequest 是项目具名树的整树提交。
type ProjectOutlineSaveRequest struct {
	Nodes []OutlineNodeRequest `json:"nodes"`
}

// ── 响应 ──

// CollectionOutlineTreeResponse 是合集具名端点的读模型。
//
// 后台整树提交（PUT）与读者读树（GET）共用这一个具名形状：
//   - has_tree / owner_kind / owner_id：来路与归属，读者侧据此生成 `?from=`；
//   - nodes：PUT 返回后台读面（含失效行与 stale_reason），GET 返回读者读面
//     （失效文章已过滤、空文件夹保留）；
//   - article_count / cover：只数仍公开的文章，封面取 DFS 第一个公开文章节点；
//   - current / previous / next：只在带 article_id 的 GET 上非空，可为 null。
type CollectionOutlineTreeResponse struct {
	HasTree      bool                       `json:"has_tree"`
	OwnerKind    int                        `json:"owner_kind"`
	OwnerID      ctype.ID                   `json:"owner_id"`
	Nodes        []OutlineNodeResponse      `json:"nodes"`
	ArticleCount int                        `json:"article_count"`
	Cover        string                     `json:"cover"`
	Current      *OutlineArticleRefResponse `json:"current"`
	Previous     *OutlineArticleRefResponse `json:"previous"`
	Next         *OutlineArticleRefResponse `json:"next"`
}

// ProjectOutlineTreeResponse 是项目具名端点的读模型。
//
// 与合集具名形状同构但**不共用数据**：一篇文章可以同时进两棵树，两个端点各读
// 各的表。字段口径与 CollectionOutlineTreeResponse 逐字一致。
type ProjectOutlineTreeResponse struct {
	HasTree      bool                       `json:"has_tree"`
	OwnerKind    int                        `json:"owner_kind"`
	OwnerID      ctype.ID                   `json:"owner_id"`
	Nodes        []OutlineNodeResponse      `json:"nodes"`
	ArticleCount int                        `json:"article_count"`
	Cover        string                     `json:"cover"`
	Current      *OutlineArticleRefResponse `json:"current"`
	Previous     *OutlineArticleRefResponse `json:"previous"`
	Next         *OutlineArticleRefResponse `json:"next"`
}
