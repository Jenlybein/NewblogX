package http

// 本文件复制了 models/enum 与 models 里的文章相关枚举。
//
// 为什么复制而不是 import：架构守卫（R10）禁止业务模块 import models，
// 因为这些类型在 HTTP 层只承担"JSON 序列化 + 取值校验"的角色，
// 与数据库 row 无关；让 adapter/http 依赖 models 会让"删 models"变成永久阻塞。
//
// 取值必须与数据库、与 domain 层完全一致，改一个数字/字符串都会让老数据读不出来。

import (
	"myblogx/internal/article/domain/review"
)

// ArticleStatus 是文章发布状态。数值与库中一致：草稿=1、审核中=2、已发布=3、已拒绝=4。
type ArticleStatus int8

const (
	ArticleStatusDraft     ArticleStatus = 1
	ArticleStatusExamining ArticleStatus = 2
	ArticleStatusPublished ArticleStatus = 3
	ArticleStatusRejected  ArticleStatus = 4
)

// ArticleVisibility 是文章可见性。字符串与库中一致。
type ArticleVisibility string

const (
	ArticleVisibilityVisible       ArticleVisibility = "visible"
	ArticleVisibilityUserHidden    ArticleVisibility = "user_hidden"
	ArticleVisibilityAdminHidden   ArticleVisibility = "admin_hidden"
	// ArticleVisibilityPendingReview 是"已发布、等模型合规判定通过才可见"的
	// 系统中间态。本文件按文件头约定复制枚举字面量（不 import 业务包），
	// 唯一来源在 kernel/enum/article_visibility_status.go；domain 侧另有一份
	// 同值常量。它不出现在任何写接口的入参枚举里（作者不能自己设），
	// 只作为读响应里可能出现的值，所以这里的 oneof 校验刻意不含它。
	ArticleVisibilityPendingReview ArticleVisibility = "pending_review"
)

// ReviewTaskStatus 是审核任务状态。
type ReviewTaskStatus string

const (
	ReviewTaskPending  ReviewTaskStatus = "pending"
	ReviewTaskApproved ReviewTaskStatus = "approved"
	ReviewTaskRejected ReviewTaskStatus = "rejected"
	ReviewTaskCanceled ReviewTaskStatus = "canceled"
)

// ReviewTaskStage 是审核阶段。
type ReviewTaskStage string

const (
	ReviewTaskStageManual ReviewTaskStage = "manual"
)

// ReviewTaskSource 是审核任务的来源。
type ReviewTaskSource string

const (
	ReviewTaskSourceCreate   ReviewTaskSource = "create"
	ReviewTaskSourceEdit     ReviewTaskSource = "edit"
	ReviewTaskSourceResubmit ReviewTaskSource = "resubmit"
	// ReviewTaskSourceCompliance 是合规判定命中的自动拒绝（票 313 / ADR-0025）。
	//
	// 单列一个来源：复盘时要能一眼分清"人拒的"与"系统拒的"。
	ReviewTaskSourceCompliance ReviewTaskSource = "compliance"
	// 票 302 新规则：同步风险扫描直接拒、模型判存疑转人工、模型判定失败转人工。
	ReviewTaskSourceRiskCheck   ReviewTaskSource = "risk_check"
	ReviewTaskSourceModelSuspect ReviewTaskSource = "model_suspect"
	ReviewTaskSourceModelFailure ReviewTaskSource = "model_failure"
)

// ReviewTaskSort 是审核队列的排序口径（票 311）。
type ReviewTaskSort string

const (
	// ReviewTaskSortCreatedAt 提交时间升序（默认）。
	ReviewTaskSortCreatedAt ReviewTaskSort = "created_at"
	// ReviewTaskSortRisk 风险等级降序、同等级仍按提交时间升序。
	ReviewTaskSortRisk ReviewTaskSort = "risk"
)

// ReviewRiskLevel 是机审前两层给出的风险等级（票 311）。
//
// 与 domain/moderation 的 Level 取值一致；这里复制一份是 HTTP 层的惯例
// （见本文件顶部说明），转换只在 handler 里做一次。
type ReviewRiskLevel string

const (
	ReviewRiskLevelNone   ReviewRiskLevel = "none"
	ReviewRiskLevelLow    ReviewRiskLevel = "low"
	ReviewRiskLevelMedium ReviewRiskLevel = "medium"
	ReviewRiskLevelHigh   ReviewRiskLevel = "high"
)

// ReviewReasonCode 是驳回原因的固定枚举值。
//
// 这里**别名**而不是像上面那几组一样重抄一份取值：驳回原因的词表只能有一处
// （kernel/enum/article_review_enum，见 GLOSSARY「枚举别名」），因为它是 #308
// 的核心——契约里的 enum、管理后台的下拉与作者收到的文案全都从那一处生成，
// 再抄一份取值就会出现"契约允许的值代码不认"。
type ReviewReasonCode = review.ReasonCode
