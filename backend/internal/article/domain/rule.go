package domain

// 本文件是"谁能对文章做什么"的全部答案。
//
// 全部写成纯函数：输入 (文章, 操作者)，输出布尔。
// 好处是穷举矩阵可以在毫秒级跑完，而不用起 DB 或 Gin。
//
// 规则来源：改造前散落在 article_detail.go / article_update.go /
// article_remove_user.go / article_remove.go / article_examine.go 里的 if 分支。
// 每一条都在 rule_test.go 里能对应回一个旧实现场景。

// CanBeViewedBy 判断 viewer 能否看到这篇文章。
//
// 与旧实现（article_detail.go 第 70–78 行）等价：
//   - 匿名：只能看"已发布 + visible"；
//   - 普通用户：不是作者时同样只能看"已发布 + visible"，是作者则全可见；
//   - 管理员：全可见。
//
// 已软删的文章一律不可见（旧实现里软删后 Take 就查不到，等价于此）。
// 注意错误语义：不可见对外统一返回"文章不存在"，见 ErrArticleNotFound 的说明。
func CanBeViewedBy(a *Article, viewer Viewer) bool {
	if a == nil || a.deleted {
		return false
	}
	if viewer.IsAdmin() {
		return true
	}
	if viewer.IsLoggedIn() && viewer.UserID == a.authorID {
		return true
	}
	return a.IsPubliclyVisible()
}

// CanBeEditedBy 判断 viewer 能否修改文章内容。
//
// 旧实现用 `id = ? AND author_id = ?` 直接查，所以只有作者本人；
// 管理员改内容走的是另一条管理路径（改可见性），不在本函数语义内。
func CanBeEditedBy(a *Article, viewer Viewer) bool {
	if a == nil || a.deleted || !viewer.IsLoggedIn() {
		return false
	}
	return viewer.UserID == a.authorID
}

// CanBeDeletedBy 判断 viewer 能否删除文章。
//
// 作者可删自己的；管理员可删任何人的（两条路由 authGroup / adminGroup）。
func CanBeDeletedBy(a *Article, viewer Viewer) bool {
	if a == nil || a.deleted || !viewer.IsLoggedIn() {
		return false
	}
	if viewer.IsAdmin() {
		return true
	}
	return viewer.UserID == a.authorID
}

// CanBeExaminedBy 判断 viewer 能否审核文章。
//
// 只有管理员，且文章必须处于"审核中"——否则 approve/reject 无意义。
func CanBeExaminedBy(a *Article, viewer Viewer) bool {
	if a == nil || a.deleted {
		return false
	}
	if !viewer.IsAdmin() {
		return false
	}
	return a.status == StatusExamining
}

// CanSetVisibility 判断 viewer 能否修改可见性。
//
// 作者可以设置 visible / user_hidden；admin_hidden 只允许管理员设置或解除，
// 否则作者可以自行撤销管理员的隐藏。
func CanSetVisibility(a *Article, viewer Viewer, next Visibility) bool {
	if a == nil || a.deleted || !viewer.IsLoggedIn() {
		return false
	}
	if viewer.IsAdmin() {
		return true
	}
	if viewer.UserID != a.authorID {
		return false
	}
	return next != VisibilityAdminHidden
}

// CanToggleComments 判断 viewer 能否开关评论区。仅作者。
func CanToggleComments(a *Article, viewer Viewer) bool {
	if a == nil || a.deleted || !viewer.IsLoggedIn() {
		return false
	}
	return viewer.UserID == a.authorID
}

// CanBeFavorited 判断 viewer 能否收藏。
//
// 规则：已登录、文章对本人可见、且不是自己的文章（旧实现在
// article_favorite.go 里禁止自收藏）。
func CanBeFavorited(a *Article, viewer Viewer) bool {
	if a == nil || !viewer.IsLoggedIn() {
		return false
	}
	if viewer.UserID == a.authorID {
		return false
	}
	return CanBeViewedBy(a, viewer)
}

// CanBeTopped 判断 viewer 能否把文章置顶到自己的主页。
//
// 只有作者本人可置顶自己的文章——置顶是"作者在个人页推荐作品"的功能。
func CanBeTopped(a *Article, viewer Viewer) bool {
	if a == nil || a.deleted || !viewer.IsLoggedIn() {
		return false
	}
	return viewer.UserID == a.authorID
}

// ── 站点「发布与展示」开关（ADR-0017 决策 2）──

// WritePolicy 是 `site.article` 里与写作权限有关的两个开关的取值快照。
//
// 为什么是"快照"而不是直接读配置：domain 不认识配置服务（也不该认识），
// 判定保持纯函数，权限矩阵才能在毫秒级穷举；取值由 app 层的入参带进来，
// 读的是运行时配置的现读闭包，所以后台改完下一次请求即生效。
type WritePolicy struct {
	// AllowNonAdminPublish 允许非管理员发布文章。
	//
	// 关掉后非管理员不能"创建即发布"、不能提交审核、不能发布自己的草稿，
	// 也不能更新已发布（或审核中）的文章。
	AllowNonAdminPublish bool
	// AllowNonAdminDraft 允许非管理员创建草稿。
	//
	// 关掉后非管理员不能新建草稿、不能保存草稿；存量草稿仍可打开、可删除。
	AllowNonAdminDraft bool
}

// OpenWritePolicy 返回"不限制非管理员"的策略，与两个开关的业务默认值一致（全 true）。
//
// 用在两处：装配层读不到运行时配置时兜底（不能因为配置服务缺席就把全站作者锁死），
// 以及测试里显式表达"策略全开"。
func OpenWritePolicy() WritePolicy {
	return WritePolicy{AllowNonAdminPublish: true, AllowNonAdminDraft: true}
}

// writableRole 判断操作者是否有写权限的角色。
//
// 只有管理员（1）与注册用户（2）算"能写的人"。受限访客（3）与库里未入枚举的
// role=0 在鉴权层已经按未登录处理（ADR-0017 决策 3），这里再兜一层：
// 三个开关全开时也不能让它们借道写进来——它们连"非管理员作者"都不算。
func writableRole(v Viewer) bool {
	return v.IsLoggedIn() && (v.Role == RoleAdmin || v.Role == RoleUser)
}

// CheckWriteOnCreate 判断站点策略是否允许 viewer 以 target 状态新建文章。
//
// target 只可能是草稿或提交审核（其它值由调用方按非法迁移拒绝，这里不重复判断）。
func CheckWriteOnCreate(v Viewer, target Status, p WritePolicy) error {
	if v.IsAdmin() {
		return nil
	}
	if !writableRole(v) {
		return ErrForbidden
	}
	switch target {
	case StatusDraft:
		if !p.AllowNonAdminDraft {
			return ErrDraftDisabledBySite
		}
	case StatusExamining:
		if !p.AllowNonAdminPublish {
			return ErrPublishDisabledBySite
		}
	}
	return nil
}

// CheckWriteOnUpdate 判断站点策略是否允许 viewer 更新文章 a。
//
// target 为 nil 表示"只改内容、不改状态"，此时按文章当前所处阶段归属开关：
//   - 已发布 / 审核中：属于发布链路（编辑审核中的文章会重新送审），看发布开关；
//   - 草稿 / 已拒绝：属于准备阶段（存草稿），看草稿开关。
//
// 归属规则与 #64 验收标准逐条对应：关闭发布 → 不能更新已发布文章、不能发布草稿；
// 关闭草稿 → 存量草稿可打开可删除但不可更新（打开与删除不经过本函数）。
func CheckWriteOnUpdate(a *Article, v Viewer, target *Status, p WritePolicy) error {
	if v.IsAdmin() {
		return nil
	}
	if !writableRole(v) {
		return ErrForbidden
	}
	if a == nil {
		return ErrArticleNotFound
	}

	needsPublish := false
	switch {
	case target != nil && *target == StatusExamining:
		needsPublish = true
	case target != nil && *target == StatusDraft:
		needsPublish = false
	default:
		// 不显式改状态：按现有状态判断这次编辑落在哪条链路。
		needsPublish = a.status == StatusPublished || a.status == StatusExamining
	}

	if needsPublish {
		if !p.AllowNonAdminPublish {
			return ErrPublishDisabledBySite
		}
		return nil
	}
	if !p.AllowNonAdminDraft {
		return ErrDraftDisabledBySite
	}
	return nil
}
