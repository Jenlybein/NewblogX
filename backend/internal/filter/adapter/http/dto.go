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
	Surface string   `form:"surface" enums:"article,comment,nickname,username,abstract,chat,collection"`
	Status  string   `form:"status" enums:"pending,ignored,deleted,account"`
	UserID  ctype.ID `form:"user_id"`
	// Word 按命中词筛选；StartAt / EndAt 是命中时间范围（YYYY-MM-DD 或 RFC3339）。
	Word    string `form:"word"`
	StartAt string `form:"start_at"`
	EndAt   string `form:"end_at"`
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
	Surface   string   `json:"surface" enums:"article,comment,nickname,username,abstract,chat,collection"`
	ContentID ctype.ID `json:"content_id"`
	UserID    ctype.ID `json:"user_id"`
	// UserNickname 是读面现查的展示名，不落库（昵称会变）。
	UserNickname string `json:"user_nickname"`
	Status       string `json:"status" enums:"pending,ignored,deleted,account"`
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

// HandleHitRequest 是单条处置的请求体。
//
// 动作与备注都在服务端校验（不靠 binding tag）：非法动作、超长备注要给出
// 明确的中文提示，而 gin 的 binding 只能回一个笼统的 400。
//
// 四个动作（票 398 的内容动作 + 票 399 的账号动作）：
//   - delete：删除命中的内容；
//   - ignore：忽略误报；
//   - ban：封禁账号（账号禁用，登录与发布都被拦）；
//   - mute：禁言账号，时长见 MuteDays。
type HandleHitRequest struct {
	Action string `json:"action" enums:"delete,ignore,ban,mute"`
	// MuteDays 是禁言天数，只有 mute 动作读它：1–365，缺省（0）按 7 天。
	// 越界由服务端拒绝（user.mute_days_invalid）。
	MuteDays int `json:"mute_days"`
	// Note 是可选备注（≤200 字，服务端按 rune 计数）。
	Note string `json:"note"`
}

// HandleHitResponse 是单条处置的 data 载荷。
type HandleHitResponse struct {
	Hit HitItem `json:"hit"`
	// AlreadyHandled 表示这条记录之前已被处置，本次只是返回当前状态（幂等）。
	AlreadyHandled bool `json:"already_handled"`
}

// HandleBatchRequest 是批量处置的请求体。
type HandleBatchRequest struct {
	IDs    []ctype.ID `json:"ids"`
	Action string     `json:"action" enums:"delete,ignore,ban,mute"`
	// MuteDays 与单条处置同义（只有 mute 动作读它）。
	MuteDays int    `json:"mute_days"`
	Note     string `json:"note"`
}

// HandleBatchItem 是批量处置里的一条结果。
//
// 用"逐条结果"而不是"整批失败"：一批里可能混着文章与私信面，私信面没有
// 可删内容、某条记录可能刚被别人处置，这些都是单条失败而整批照常可行的结局。
type HandleBatchItem struct {
	ID ctype.ID `json:"id"`
	// Ok 表示这一条是否处置成功（或本来就已处置、幂等返回）。
	Ok bool `json:"ok"`
	// Status 是处置后的状态；失败且拿不到记录时为空串。
	Status string `json:"status" enums:"pending,ignored,deleted,account"`
	// Message 是失败原因的中文提示（成功时为空串）。
	Message string `json:"message"`
	// AlreadyHandled 与单条处置同义。
	AlreadyHandled bool `json:"already_handled"`
}

// HandleBatchResponse 是批量处置的 data 载荷。
type HandleBatchResponse struct {
	Results []HandleBatchItem `json:"results"`
}

// WordStatItem 是词云里的一格：词 + 窗口内出现处数 + 展示用的档位与类目。
type WordStatItem struct {
	Word string `json:"word"`
	// Count 是最近 90 天的出现处数（词云字号的数据源）。
	Count    int    `json:"count"`
	Tier     string `json:"tier" enums:"block,watch"`
	Category string `json:"category" enums:"drugs,gambling,fraud,porn,fake_docs,politics,violence"`
}

// WordStatsResponse 是词云数据的 data 载荷。
//
// 带 days 而不是让前端硬编码 90：窗口是后端口径（与风险页 Top 共用），
// 前端只负责展示，不该自己再记一个数字。
type WordStatsResponse struct {
	Days  int            `json:"days"`
	Items []WordStatItem `json:"items"`
}

