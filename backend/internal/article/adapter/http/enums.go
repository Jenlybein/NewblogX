package http

// 本文件复制了 models/enum 与 models 里的文章相关枚举。
//
// 为什么复制而不是 import：架构守卫（R10）禁止业务模块 import models，
// 因为这些类型在 HTTP 层只承担"JSON 序列化 + 取值校验"的角色，
// 与数据库 row 无关；让 adapter/http 依赖 models 会让"删 models"变成永久阻塞。
//
// 取值必须与数据库、与 domain 层完全一致，改一个数字/字符串都会让老数据读不出来。

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
	ArticleVisibilityVisible     ArticleVisibility = "visible"
	ArticleVisibilityUserHidden  ArticleVisibility = "user_hidden"
	ArticleVisibilityAdminHidden ArticleVisibility = "admin_hidden"
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
)
