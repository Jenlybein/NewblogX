package hottop_test

import (
	"errors"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/hottop"
	"myblogx/internal/kernel/ctype"
)

var baseTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// newArticle 造一个测试用文章（走 Restore，绕开创建校验以便构造任意状态）。
func newArticle(status domain.Status, visibility domain.Visibility, deleted bool) *domain.Article {
	return domain.Restore(domain.RestoreParams{
		ID:         ctype.NewID(1),
		AuthorID:   ctype.NewID(100),
		Title:      "测试文章",
		Content:    "正文",
		Status:     status,
		Visibility: visibility,
		Deleted:    deleted,
		CreatedAt:  baseTime,
		UpdatedAt:  baseTime,
	})
}

// TestCanBePinned 是"什么样的文章能被设成热门置顶"的穷举矩阵。
//
// 规则来自 ADR-0017 决策 5：只有"已发布且可见"的文章可置顶。
// 期望值不是猜的——它等价于 domain.Article.IsPubliclyVisible，
// 也就是"匿名访客能不能在读者站看到这篇"，两者必须一致：
// 否则会出现"首页置顶了一张点进去 404 的卡片"。
func TestCanBePinned(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		status     domain.Status
		visibility domain.Visibility
		deleted    bool
		want       bool
	}{
		{"published+visible", domain.StatusPublished, domain.VisibilityVisible, false, true},
		{"published+user_hidden", domain.StatusPublished, domain.VisibilityUserHidden, false, false},
		{"published+admin_hidden", domain.StatusPublished, domain.VisibilityAdminHidden, false, false},
		{"draft+visible", domain.StatusDraft, domain.VisibilityVisible, false, false},
		{"examining+visible", domain.StatusExamining, domain.VisibilityVisible, false, false},
		{"rejected+visible", domain.StatusRejected, domain.VisibilityVisible, false, false},
		// 已软删的文章即便状态仍是"已发布"也不能置顶。
		{"published+visible+deleted", domain.StatusPublished, domain.VisibilityVisible, true, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := hottop.CanBePinned(newArticle(c.status, c.visibility, c.deleted)); got != c.want {
				t.Fatalf("CanBePinned(%s) = %v，期望 %v", c.name, got, c.want)
			}
		})
	}
}

// TestCanBePinnedNil 处理"文章查不到"的边界：置顶前必须已经读到聚合。
func TestCanBePinnedNil(t *testing.T) {
	t.Parallel()

	if hottop.CanBePinned(nil) {
		t.Fatal("空文章不应可置顶")
	}
}

// TestIsStale 是"已失效"的判定矩阵。
//
// 失效 = 文章已物理删除（置顶行还在，join 不到文章）、已软删、
// 不再是"已发布且可见"。置顶行本身**保留**（ADR-0017 决策 5），
// 首页靠 JOIN 过滤，后台靠这个判定标"已失效"。
func TestIsStale(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state *hottop.ArticleState
		want  bool
	}{
		{"文章已物理删除", nil, true},
		{"已发布+visible", &hottop.ArticleState{Status: domain.StatusPublished, Visibility: domain.VisibilityVisible}, false},
		{"已软删", &hottop.ArticleState{Deleted: true, Status: domain.StatusPublished, Visibility: domain.VisibilityVisible}, true},
		{"退回草稿", &hottop.ArticleState{Status: domain.StatusDraft, Visibility: domain.VisibilityVisible}, true},
		{"进入审核", &hottop.ArticleState{Status: domain.StatusExamining, Visibility: domain.VisibilityVisible}, true},
		{"作者隐藏", &hottop.ArticleState{Status: domain.StatusPublished, Visibility: domain.VisibilityUserHidden}, true},
		{"管理员隐藏", &hottop.ArticleState{Status: domain.StatusPublished, Visibility: domain.VisibilityAdminHidden}, true},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.state.IsStale(); got != c.want {
				t.Fatalf("IsStale(%s) = %v，期望 %v", c.name, got, c.want)
			}
		})
	}
}

// TestAdminItemStale 证明置顶行自身也能报出失效（admin 列表的最小单元）。
func TestAdminItemStale(t *testing.T) {
	t.Parallel()

	live := hottop.AdminItem{
		ArticleID: ctype.NewID(1),
		SortOrder: 1,
		Article:   &hottop.ArticleState{Status: domain.StatusPublished, Visibility: domain.VisibilityVisible},
	}
	if live.Stale() {
		t.Fatal("在架文章不应判为失效")
	}

	gone := hottop.AdminItem{ArticleID: ctype.NewID(2), SortOrder: 2}
	if !gone.Stale() {
		t.Fatal("文章已不存在的置顶行应判为失效")
	}
}

// TestCheckOrder 锁定"全量顺序替换"的输入校验。
//
// 之所以要求"与当前集合完全一致"而不是"按列表重排、缺的删掉"：
// PATCH 是后台排序按钮的产物，它拿到的是上一次读到的列表。
// 若允许静默删除，一个过期的后台页面点一次"上移"就会把别人刚加的一条置顶弄丢。
func TestCheckOrder(t *testing.T) {
	t.Parallel()

	ids := func(ns ...uint64) []domain.ArticleID {
		out := make([]domain.ArticleID, 0, len(ns))
		for _, n := range ns {
			out = append(out, ctype.NewID(n))
		}
		return out
	}

	cases := []struct {
		name    string
		current []domain.ArticleID
		next    []domain.ArticleID
		want    error
	}{
		{"同集合换序", ids(1, 2, 3), ids(3, 1, 2), nil},
		{"单条", ids(1), ids(1), nil},
		{"空对空", nil, nil, nil},
		{"少一条", ids(1, 2), ids(1), hottop.ErrOrderMismatch},
		{"多一条", ids(1), ids(1, 2), hottop.ErrOrderMismatch},
		{"换成别的文章", ids(1), ids(2), hottop.ErrOrderMismatch},
		{"有重复", ids(1, 2), ids(1, 1), hottop.ErrOrderMismatch},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := hottop.CheckOrder(c.current, c.next)
			if !errors.Is(err, c.want) {
				t.Fatalf("CheckOrder(%s) = %v，期望 %v", c.name, err, c.want)
			}
		})
	}
}

// TestCheckOrderRejectsTooMany 锁定上限：12 条是运营位的硬边界。
func TestCheckOrderRejectsTooMany(t *testing.T) {
	t.Parallel()

	ids := make([]domain.ArticleID, 0, hottop.MaxItems+1)
	for i := 1; i <= hottop.MaxItems+1; i++ {
		ids = append(ids, ctype.NewID(uint64(i)))
	}
	if err := hottop.CheckOrder(ids, ids); !apperr.IsCode(err, apperr.CodeHotTopLimitExceeded) {
		t.Fatalf("超过 %d 条应报上限错误，实际 %v", hottop.MaxItems, err)
	}
}
