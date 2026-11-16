package http

import "myblogx/internal/kernel/ctype"

// 关键词风险页的具名出入口（ADR-0032 决策 7）。
//
// 响应形状全都自己具名、不内联 `object{}`：swag 只认具名类型，内联对象在契约里
// 会退化成 `object`，前端就只能手写视图模型（contracts.md §6.1 的老问题）。

// HitListRequest 是风险页列表的查询参数。
//
// 三个筛选项都允许留空（= 不按该项筛）；取值的合法性由 handler 校验，
// 这里只声明形状。
type HitListRequest struct {
	Page    int      `form:"page"`
	Limit   int      `form:"limit"`
	Surface string   `form:"surface" enums:"article,comment,nickname,abstract,chat,collection"`
	Status  string   `form:"status" enums:"pending,ignored,deleted"`
	UserID  ctype.ID `form:"user_id"`
}

// HitSnippet 是一条命中的上下文片段。
//
// Start / End 是命中词在 Text 内的 rune 下标（左闭右开），前端据此高亮；
// 后端不替前端插 `**` 或省略号 —— 那会让"多高亮一个字"变成契约变更。
type HitSnippet struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

// HitWord 是一个词在本条记录里的全部命中。
type HitWord struct {
	Word     string       `json:"word"`
	Tier     string       `json:"tier" enums:"block,watch"`
	Category string       `json:"category" enums:"drugs,gambling,fraud,porn,fake_docs,politics,violence"`
	Snippets []HitSnippet `json:"snippets"`
}

// HitItem 是风险页的一行。
type HitItem struct {
	ID        ctype.ID `json:"id"`
	CreatedAt string   `json:"created_at"`
	Surface   string   `json:"surface" enums:"article,comment,nickname,abstract,chat,collection"`
	ContentID ctype.ID `json:"content_id"`
	UserID    ctype.ID `json:"user_id"`
	// UserNickname 是读面现查的展示名，不落库（昵称会变）。
	UserNickname string `json:"user_nickname"`
	Status       string `json:"status" enums:"pending,ignored,deleted"`
	// HitCount 是命中处数（白名单命中计入，重叠命中按并集算一处）。
	HitCount int `json:"hit_count"`
	// Words 是命中明细：词 / 档位 / 类目 / ±20 字上下文。
	Words []HitWord `json:"words"`
	// HandlerID / HandledAt / Note 是处置痕迹（处置动作是 #398）。
	HandlerID ctype.ID `json:"handler_id"`
	HandledAt string   `json:"handled_at"`
	Note      string   `json:"note"`
}

// TopUserItem 是"命中用户 Top"的一行。
type TopUserItem struct {
	UserID       ctype.ID `json:"user_id"`
	UserNickname string   `json:"user_nickname"`
	// HitCount 是窗口内该用户的命中处数（按记录里的命中处数汇总）。
	HitCount int `json:"hit_count"`
}

// HitListResponse 是风险页的 data 载荷。
//
// Top 与列表同一次返回：页面打开时两者总是一起要，拆两个端点只是多一次往返。
type HitListResponse struct {
	List []HitItem `json:"list"`
	// Count 是满足筛选条件的总条数（不是本页条数）。
	Count int `json:"count"`
	// TopUsers 是最近 90 天命中处数最多的几个用户。
	TopUsers []TopUserItem `json:"top_users"`
}
