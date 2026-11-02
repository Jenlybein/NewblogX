package domain

// 本文件是运营关系行（热门置顶 / 合集收录）所指文章的"此刻状态"。
//
// 两个运营位都保留关系行、读者侧靠 JOIN 过滤、后台标「已失效」并允许清理。
// 失效口径必须逐字一致，所以状态字段与判定函数只在这里定义一份；
// domain/hottop 与 domain/collection 各自用类型别名引用它，不要另起一套。

// StaleReason 是失效原因的可读分类。
//
// 判定只说明"失效了"，原因用于后台让运营看懂为什么失效；文案在 apperr /
// 前端映射，domain 只给稳定的枚举值。
type StaleReason string

const (
	// StaleReasonNone 文章此刻仍公开，关系行有效。
	StaleReasonNone StaleReason = ""
	// StaleReasonMissing 文章已被物理删除（关系行还在，join 不到文章）。
	StaleReasonMissing StaleReason = "missing"
	// StaleReasonDeleted 文章已被软删（行还在，deleted_at 非空）。
	StaleReasonDeleted StaleReason = "deleted"
	// StaleReasonNotPublished 文章不再是"已发布"（草稿 / 审核中 / 被拒等）。
	StaleReasonNotPublished StaleReason = "not_published"
	// StaleReasonNotVisible 文章已发布但不可见（作者隐藏 / 管理员隐藏）。
	StaleReasonNotVisible StaleReason = "not_visible"
)

// ArticleState 是运营关系行所指文章**此刻**的状态。
//
// 与读模型卡片分开是因为职责不同：卡片是"还能展示的样子"，ArticleState
// 是"它怎么样了"——后台的「已失效」标记与失效原因只读后者。
// nil 表示文章已被物理删除（关系行还在，join 不到文章）。
//
// 同一套判定服务「热门置顶」与「合集收录」：两处一旦各自实现，同一个文章
// 状态就会在不同页面得到不同答案。
type ArticleState struct {
	// Deleted 文章已被软删（行还在，deleted_at 非空）。
	Deleted bool
	// Status 发布状态。
	Status Status
	// Visibility 可见性。
	Visibility Visibility
}

// IsStale 判断关系行是否已失效。
//
// 失效 = 文章没了 / 被删 / 不再"已发布且可见"。关系行本身**保留**：
// 读者侧靠 JOIN 过滤掉，后台标「已失效」并允许一键清理。
// 方法可以在 nil 接收者上调用，所以"文章已物理删除"也走同一条判定。
func (s *ArticleState) IsStale() bool {
	if s == nil {
		return true
	}
	return s.Deleted || s.Status != StatusPublished || s.Visibility != VisibilityVisible
}

// StaleReason 返回失效原因；仍有效时返回 StaleReasonNone。
//
// 优先级与 IsStale 的判定顺序一致：先"没了"，再"被删"，再状态，最后可见性。
// 反过来先看可见性会在多因同时成立时给出误导性的原因。
func (s *ArticleState) StaleReason() StaleReason {
	switch {
	case s == nil:
		return StaleReasonMissing
	case s.Deleted:
		return StaleReasonDeleted
	case s.Status != StatusPublished:
		return StaleReasonNotPublished
	case s.Visibility != VisibilityVisible:
		return StaleReasonNotVisible
	default:
		return StaleReasonNone
	}
}
