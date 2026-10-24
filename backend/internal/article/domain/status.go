package domain

// Status 是文章的发布状态。
//
// 取值必须与数据库现有值一致（models/enum/article_status.go）：
// 草稿=1、审核中=2、已发布=3、已拒绝=4。没有"已删除"这一状态——
// 删除走软删标记（见 Article.deleted），所以它是独立维度而不是第五个状态。
type Status int8

const (
	// StatusDraft 草稿，仅作者与管理员可见。
	StatusDraft Status = 1
	// StatusExamining 审核中，等待管理员处理。
	StatusExamining Status = 2
	// StatusPublished 已发布。
	StatusPublished Status = 3
	// StatusRejected 审核被拒，作者可修改后重新提交。
	StatusRejected Status = 4
)

// Valid 判断状态值是否合法。
func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusExamining, StatusPublished, StatusRejected:
		return true
	}
	return false
}

// String 返回中文名称，用于错误信息与审计日志。
func (s Status) String() string {
	switch s {
	case StatusDraft:
		return "草稿"
	case StatusExamining:
		return "审核中"
	case StatusPublished:
		return "已发布"
	case StatusRejected:
		return "已拒绝"
	}
	return "未知状态"
}

// Visibility 是文章的可见性设置。
//
// 取值与数据库一致（models/enum/article_visibility_status.go）。
type Visibility string

const (
	// VisibilityVisible 正常可见。
	VisibilityVisible Visibility = "visible"
	// VisibilityUserHidden 作者主动隐藏。
	VisibilityUserHidden Visibility = "user_hidden"
	// VisibilityAdminHidden 管理员隐藏。
	VisibilityAdminHidden Visibility = "admin_hidden"
)

// Valid 判断可见性取值是否合法。
func (v Visibility) Valid() bool {
	switch v {
	case VisibilityVisible, VisibilityUserHidden, VisibilityAdminHidden:
		return true
	}
	return false
}

// NormalizeVisibility 把非法/空值归一到 visible。
//
// 与旧实现 normalizeArticleVisibilityStatus 行为一致：只认三种合法值，其余一律 visible。
func NormalizeVisibility(v Visibility) Visibility {
	if v.Valid() {
		return v
	}
	return VisibilityVisible
}

// ResolveSubmitStatus 计算"提交"之后的落库状态。
//
// 站点配置跳过审核时，提交即发布；否则进审核队列。
// 与旧实现 resolvePublishStatus 行为一致。
func ResolveSubmitStatus(skipExamining bool) Status {
	if skipExamining {
		return StatusPublished
	}
	return StatusExamining
}
