// Package article_review_enum 是审核任务的三组枚举。
//
// 从 models 搬来（M6）。它们本来就是"值"，不是行结构 ——
// 放在 kernel 里业务模块才能引用而不违反 R10。
//
// 三组都是 string 而不是数字：审核状态会进响应体与 ES 投影，
// 用字符串让"库里的值"和"日志/文档里看到的值"是同一个东西。
package article_review_enum

// Status 是审核任务的状态。
type Status string

// 状态取值。
//
//	pending ──审完──> approved / rejected
//	   └──作者撤回──> canceled
const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusCanceled Status = "canceled"
)

// Stage 是审核所处的阶段。
type Stage string

// 阶段取值。目前只有人工审核一种；自动审核的扩展点是新增取值，不是改结构。
const (
	StageManual Stage = "manual"
)

// Source 是任务被创建的原因。
//
// 它决定"驳回后作者能否直接改"，所以不能从状态反推。
type Source string

// 来源取值。
const (
	SourceCreate   Source = "create"
	SourceEdit     Source = "edit"
	SourceResubmit Source = "resubmit"
)
