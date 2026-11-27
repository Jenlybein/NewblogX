package http

import "myblogx/internal/kernel/ctype"

// 管理端评论管理的具名出入口（后台「文章管理 → 评论管理」第一期）。
//
// 响应形状一律具名、不内联 `object{}`：swag 只认具名类型，内联对象在契约里会
// 退化成 `object`，前端就只能手写视图模型（contracts.md §6.1 的老问题）。
//
// 校验不靠 binding tag：gin 的 binding 只能回一个笼统的 400，而这里要的是
// "带错误码 + 中文文案"的失败（状态筛选值写错、理由为空、天数不在四档之内），
// 所以形状在这里声明，取值校验统一在用例层做。

// AdminCommentListRequest 是管理端评论列表的查询参数。
//
// 每一项都允许留空（= 不按该项筛）；from / to 是 created_at 的日期范围，
// 按 YYYY-MM-DD 解析，to 含当天（右边界取次日零点、不含）。
type AdminCommentListRequest struct {
	Page        int      `form:"page"`
	Limit       int      `form:"limit"`
	Keyword     string   `form:"keyword"`
	UserID      ctype.ID `form:"user_id"`
	UserKeyword string   `form:"user_keyword"`
	ArticleID   ctype.ID `form:"article_id"`
	From        string   `form:"from"`
	To          string   `form:"to"`
	// Status 是状态筛选，默认 all（含已删除）；三个取值与契约里的 enums 对齐。
	Status string `form:"status" enums:"all,published,deleted"`
}

// AdminRemoveRequest 是批量删除的请求体。
type AdminRemoveRequest struct {
	// IDList 是 1–100 条评论 ID；条数、零值 ID 在用例层校验。
	IDList []ctype.ID `json:"id_list"`
}

// AdminMuteRequest 是禁言评论发布人的请求体。
type AdminMuteRequest struct {
	UserID ctype.ID `json:"user_id"`
	// Days 是禁言档位：1 / 7 / 30 天，0 表示永久；其它取值被拒绝。
	Days int `json:"days" enums:"1,7,30,0"`
	// Reason 是处置理由（必填、≤200 字）。
	Reason string `json:"reason"`
}

// AdminMuteResponse 是禁言结果的 data 载荷。
type AdminMuteResponse struct {
	// MutedUntil 是禁言到期时间（RFC3339）；"永久"时是远期时间点。
	MutedUntil string `json:"muted_until"`
}

// AdminBanRequest 是封禁评论发布人的请求体。
type AdminBanRequest struct {
	UserID ctype.ID `json:"user_id"`
	// Reason 是可选理由（≤200 字）：封禁没有到期日要向用户解释，留空也允许。
	Reason string `json:"reason"`
}
