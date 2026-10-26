package domain_test

import (
	"myblogx/internal/kernel/ctype"
	"testing"

	"myblogx/internal/article/domain"
)

const (
	authorID   = ctype.ID("1001")
	strangerID = ctype.ID("2002")
	adminID    = ctype.ID("9001")
)

// newArticle 造一个测试用文章（走 Restore，绕开创建校验以便构造任意状态）。
func newArticle(t *testing.T, status domain.Status, visibility domain.Visibility, deleted bool) *domain.Article {
	t.Helper()
	return domain.Restore(domain.RestoreParams{
		ID:         ctype.NewID(1),
		AuthorID:   authorID,
		Title:      "测试文章",
		Content:    "正文",
		Status:     status,
		Visibility: visibility,
		Deleted:    deleted,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	})
}

// TestArticleVisibility 是可见性不变量矩阵：状态 × 角色穷举。
//
// 期望值全部来自改造前的 if 分支（article_detail.go 第 70–78 行），
// 不是"看起来合理"的猜测。
func TestArticleVisibility(t *testing.T) {
	t.Parallel()

	author := domain.NormalUser(authorID)
	stranger := domain.NormalUser(strangerID)
	admin := domain.AdminUser(adminID)
	anon := domain.Anonymous()

	cases := []struct {
		name       string
		status     domain.Status
		visibility domain.Visibility
		deleted    bool
		viewer     domain.Viewer
		want       bool
	}{
		// 草稿：只有作者与管理员
		{"draft/author", domain.StatusDraft, domain.VisibilityVisible, false, author, true},
		{"draft/stranger", domain.StatusDraft, domain.VisibilityVisible, false, stranger, false},
		{"draft/admin", domain.StatusDraft, domain.VisibilityVisible, false, admin, true},
		{"draft/anonymous", domain.StatusDraft, domain.VisibilityVisible, false, anon, false},

		// 审核中：同上
		{"examining/author", domain.StatusExamining, domain.VisibilityVisible, false, author, true},
		{"examining/stranger", domain.StatusExamining, domain.VisibilityVisible, false, stranger, false},
		{"examining/admin", domain.StatusExamining, domain.VisibilityVisible, false, admin, true},
		{"examining/anonymous", domain.StatusExamining, domain.VisibilityVisible, false, anon, false},

		// 已发布 + visible：所有人都能看
		{"published/author", domain.StatusPublished, domain.VisibilityVisible, false, author, true},
		{"published/stranger", domain.StatusPublished, domain.VisibilityVisible, false, stranger, true},
		{"published/admin", domain.StatusPublished, domain.VisibilityVisible, false, admin, true},
		{"published/anonymous", domain.StatusPublished, domain.VisibilityVisible, false, anon, true},

		// 已拒绝：只有作者与管理员
		{"rejected/author", domain.StatusRejected, domain.VisibilityVisible, false, author, true},
		{"rejected/stranger", domain.StatusRejected, domain.VisibilityVisible, false, stranger, false},
		{"rejected/admin", domain.StatusRejected, domain.VisibilityVisible, false, admin, true},
		{"rejected/anonymous", domain.StatusRejected, domain.VisibilityVisible, false, anon, false},

		// 已删除：谁都看不到（软删后旧实现连 Take 都查不到）
		{"deleted/author", domain.StatusPublished, domain.VisibilityVisible, true, author, false},
		{"deleted/stranger", domain.StatusPublished, domain.VisibilityVisible, true, stranger, false},
		{"deleted/admin", domain.StatusPublished, domain.VisibilityVisible, true, admin, false},
		{"deleted/anonymous", domain.StatusPublished, domain.VisibilityVisible, true, anon, false},

		// 已发布但被作者隐藏：路人不可见，作者与管理员可见
		{"user_hidden/author", domain.StatusPublished, domain.VisibilityUserHidden, false, author, true},
		{"user_hidden/stranger", domain.StatusPublished, domain.VisibilityUserHidden, false, stranger, false},
		{"user_hidden/admin", domain.StatusPublished, domain.VisibilityUserHidden, false, admin, true},
		{"user_hidden/anonymous", domain.StatusPublished, domain.VisibilityUserHidden, false, anon, false},

		// 被管理员隐藏：同上，但这是管理员操作的结果
		{"admin_hidden/author", domain.StatusPublished, domain.VisibilityAdminHidden, false, author, true},
		{"admin_hidden/stranger", domain.StatusPublished, domain.VisibilityAdminHidden, false, stranger, false},
		{"admin_hidden/admin", domain.StatusPublished, domain.VisibilityAdminHidden, false, admin, true},
		{"admin_hidden/anonymous", domain.StatusPublished, domain.VisibilityAdminHidden, false, anon, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := newArticle(t, c.status, c.visibility, c.deleted)
			if got := domain.CanBeViewedBy(a, c.viewer); got != c.want {
				t.Fatalf("CanBeViewedBy = %v，期望 %v（状态=%s 可见性=%s 已删=%v）",
					got, c.want, c.status, c.visibility, c.deleted)
			}
		})
	}
}

func TestEditAndDeletePermission(t *testing.T) {
	t.Parallel()

	a := newArticle(t, domain.StatusPublished, domain.VisibilityVisible, false)

	cases := []struct {
		name      string
		viewer    domain.Viewer
		canEdit   bool
		canDelete bool
	}{
		{"author", domain.NormalUser(authorID), true, true},
		{"stranger", domain.NormalUser(strangerID), false, false},
		{"admin", domain.AdminUser(adminID), false, true},
		{"anonymous", domain.Anonymous(), false, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.CanBeEditedBy(a, c.viewer); got != c.canEdit {
				t.Fatalf("CanBeEditedBy = %v，期望 %v", got, c.canEdit)
			}
			if got := domain.CanBeDeletedBy(a, c.viewer); got != c.canDelete {
				t.Fatalf("CanBeDeletedBy = %v，期望 %v", got, c.canDelete)
			}
		})
	}
}

func TestExaminePermission(t *testing.T) {
	t.Parallel()

	examining := newArticle(t, domain.StatusExamining, domain.VisibilityVisible, false)
	draft := newArticle(t, domain.StatusDraft, domain.VisibilityVisible, false)

	if !domain.CanBeExaminedBy(examining, domain.AdminUser(adminID)) {
		t.Fatal("管理员应能审核审核中的文章")
	}
	if domain.CanBeExaminedBy(draft, domain.AdminUser(adminID)) {
		t.Fatal("草稿不该进入审核动作")
	}
	if domain.CanBeExaminedBy(examining, domain.NormalUser(authorID)) {
		t.Fatal("普通用户不能审核")
	}
}

func TestSetVisibilityPermission(t *testing.T) {
	t.Parallel()

	a := newArticle(t, domain.StatusPublished, domain.VisibilityVisible, false)

	if !domain.CanSetVisibility(a, domain.NormalUser(authorID), domain.VisibilityUserHidden) {
		t.Fatal("作者应能隐藏自己的文章")
	}
	if domain.CanSetVisibility(a, domain.NormalUser(authorID), domain.VisibilityAdminHidden) {
		t.Fatal("作者不得自行设置管理员隐藏（否则能撤销管理员的处置）")
	}
	if !domain.CanSetVisibility(a, domain.AdminUser(adminID), domain.VisibilityAdminHidden) {
		t.Fatal("管理员应能设置管理员隐藏")
	}
	if domain.CanSetVisibility(a, domain.NormalUser(strangerID), domain.VisibilityUserHidden) {
		t.Fatal("他人不能改可见性")
	}
}

func TestFavoriteAndTopPermission(t *testing.T) {
	t.Parallel()

	published := newArticle(t, domain.StatusPublished, domain.VisibilityVisible, false)
	draft := newArticle(t, domain.StatusDraft, domain.VisibilityVisible, false)

	if domain.CanBeFavorited(published, domain.NormalUser(authorID)) {
		t.Fatal("不能收藏自己的文章")
	}
	if !domain.CanBeFavorited(published, domain.NormalUser(strangerID)) {
		t.Fatal("他人应能收藏可见文章")
	}
	if domain.CanBeFavorited(draft, domain.NormalUser(strangerID)) {
		t.Fatal("不可见文章不能被收藏")
	}
	if domain.CanBeFavorited(published, domain.Anonymous()) {
		t.Fatal("匿名不能收藏")
	}

	if !domain.CanBeTopped(published, domain.NormalUser(authorID)) {
		t.Fatal("作者应能置顶自己的文章")
	}
	if domain.CanBeTopped(published, domain.NormalUser(strangerID)) {
		t.Fatal("不能置顶他人文章")
	}
}

func TestToggleCommentsPermission(t *testing.T) {
	t.Parallel()

	a := newArticle(t, domain.StatusPublished, domain.VisibilityVisible, false)
	if !domain.CanToggleComments(a, domain.NormalUser(authorID)) {
		t.Fatal("作者应能开关评论")
	}
	if domain.CanToggleComments(a, domain.NormalUser(strangerID)) {
		t.Fatal("他人不能开关评论")
	}
}
