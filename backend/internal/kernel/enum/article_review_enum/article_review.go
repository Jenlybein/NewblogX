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
	// SourceCompliance 是模型合规判定命中的自动拒绝（票 313 / ADR-0025）。
	//
	// 单开一个来源而不是复用 create：复盘时要能一眼分清"人拒的"与"系统拒的"，
	// 否则自动拒的误判会被当成审核员判断失误。
	SourceCompliance Source = "compliance"
	// SourceRiskCheck 是提交时同步风险扫描命中 medium 及以上的自动拒绝（票 302 新规则）。
	//
	// 与 SourceCompliance 分开：前者是模型异步判定，这里是纯字符串/评分派生
	// 的同步判定，失败率、误判类型与复盘方式都不同。
	SourceRiskCheck Source = "risk_check"
	// SourceModelSuspect / SourceModelFailure 是"已发布待可见"文章被异步改回
	// 人工审核的两种原因（票 302 新规则）：模型判存疑、或判定重试一次后仍失败。
	SourceModelSuspect Source = "model_suspect"
	SourceModelFailure Source = "model_failure"
	// SourceQuotaDeferred 是「合规判定因为作者当日额度用尽而推迟，兜底扫描把它
	// 转回人工」（ADR-0028）。正常路径下这类文章次日会自动重审，不会走到人工；
	// 这条来源只在延迟事件丢失、被兜底扫描捞回来时出现。
	SourceQuotaDeferred Source = "quota_deferred"
)

// ReasonCode 是驳回原因的固定枚举。
//
// 为什么是固定枚举而不是自由文本：自由文本让同一件事在校验口径上互不相认，
// 也没法让前端"按值渲染"——改一个字，按中文字符串匹配的代码就断（apperr 的教训）。
// 取值是**稳定值**：进契约、落库、出通知都只认它；中文文案只在这里出现一次。
//
// 词表只在这一处声明（GLOSSARY「枚举别名」），domain / adapter 一律别名引用；
// 加一条原因只改本文件，契约里的枚举与前端下拉都跟着生成。
//
// 空串不是枚举值，它表示"没有枚举"——#303 之前的历史行与撤销原因用纯自由文本。
type ReasonCode string

// 驳回原因取值。
const (
	// ReasonQuality 内容质量不达标。
	ReasonQuality ReasonCode = "quality"
	// ReasonCompliance 内容不合规。
	ReasonCompliance ReasonCode = "compliance"
	// ReasonIncomplete 内容不完整。
	ReasonIncomplete ReasonCode = "incomplete"
	// ReasonFormat 排版或格式问题。
	ReasonFormat ReasonCode = "format"
	// ReasonOther 其他原因（说明必须补一句，否则作者只看到「其他原因」）。
	ReasonOther ReasonCode = "other"
	// ReasonRiskCheck 是提交时同步内容检查未通过（票 302 新规则）。
	// 作者只看到这条固定文案，命中的词、维度、等级一律不外泄。
	ReasonRiskCheck ReasonCode = "risk_check"
)

// reasonLabels 是驳回原因的中文文案。
//
// 与取值放同一文件：这就是"错误码 + 中文文案集中一处"的同源做法——
// 改一句话只动这里，作者通知、管理后台下拉与列表都跟着变。
var reasonLabels = map[ReasonCode]string{
	ReasonQuality:    "内容质量不达标",
	ReasonCompliance: "内容不合规",
	ReasonIncomplete: "内容不完整",
	ReasonFormat:     "排版格式问题",
	ReasonOther:      "其他原因",
	ReasonRiskCheck:  "内容检查未通过",
}

// ReasonCodes 返回**人工审核员可选**的全部取值。
//
// 顺序就是下拉框顺序（先质量问题、再合规、再格式、最后其他）。
// 显式列表而不是遍历 map：map 顺序随机，下拉框每次刷新换一个顺序会让人以为选项变了。
//
// 系统专用的取值（ReasonRiskCheck 等）刻意不在这里：它们只能由自动判定写入
// 任务行供复盘，不能出现在管理员的下拉里 —— 否则人会替系统背一次误判。
func ReasonCodes() []ReasonCode {
	return []ReasonCode{ReasonQuality, ReasonCompliance, ReasonIncomplete, ReasonFormat, ReasonOther}
}

// Label 返回该取值的中文文案。未知取值返回空串（调用方据此判断"不是固定枚举"）。
func (c ReasonCode) Label() string { return reasonLabels[c] }
