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
