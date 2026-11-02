package collection_test

import (
	"testing"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/article/domain/hottop"
)

// 本文件把「合集收录行」与「热门置顶」的失效口径钉成同一份。
//
// 两个运营位各自保留失效行、读者侧靠 JOIN 过滤、后台标失效并可清理，这套做法
// 必须逐字一致：两边的「已失效」判定一旦分叉，同一个文章状态在不同页面就会
// 得到不同答案，运营看到的和读者看到的互相矛盾。
// 共用点是 domain.ArticleState（状态字段 + IsStale + StaleReason）。

// TestStaleStateTypeIsShared 在**编译期**证明两边用的是同一个类型。
//
// 若有人在任一包里重新声明一个同名字段的结构体，下面的赋值就会编译失败：
// 这是比断言更硬的约束，也是"不要自创第二套判定"的落地方式。
func TestStaleStateTypeIsShared(t *testing.T) {
	t.Parallel()

	shared := &domain.ArticleState{
		Status:     domain.StatusPublished,
		Visibility: domain.VisibilityVisible,
	}

	var collectionState *collection.ArticleState = shared
	var hotTopState *hottop.ArticleState = shared
	var _ *domain.ArticleState = collectionState
	var _ *domain.ArticleState = hotTopState
}

// TestStaleVerdictMatchesHotTop 是两边的行为一致性矩阵。
//
// 同一批文章状态分别喂给合集与热门置顶的判定入口，失效与否、失效原因都必须
// 完全相同。nil 是"文章已物理删除"：关系行还在，join 不到文章。
func TestStaleVerdictMatchesHotTop(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		state  *domain.ArticleState
		stale  bool
		reason domain.StaleReason
	}{
		{"文章已物理删除", nil, true, domain.StaleReasonMissing},
		{"已发布且可见", &domain.ArticleState{
			Status: domain.StatusPublished, Visibility: domain.VisibilityVisible,
		}, false, domain.StaleReasonNone},
		{"已软删", &domain.ArticleState{
			Deleted: true, Status: domain.StatusPublished, Visibility: domain.VisibilityVisible,
		}, true, domain.StaleReasonDeleted},
		{"退回草稿", &domain.ArticleState{
			Status: domain.StatusDraft, Visibility: domain.VisibilityVisible,
		}, true, domain.StaleReasonNotPublished},
		{"进入审核", &domain.ArticleState{
			Status: domain.StatusExamining, Visibility: domain.VisibilityVisible,
		}, true, domain.StaleReasonNotPublished},
		{"作者隐藏", &domain.ArticleState{
			Status: domain.StatusPublished, Visibility: domain.VisibilityUserHidden,
		}, true, domain.StaleReasonNotVisible},
		{"管理员隐藏", &domain.ArticleState{
			Status: domain.StatusPublished, Visibility: domain.VisibilityAdminHidden,
		}, true, domain.StaleReasonNotVisible},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			collectionItem := collection.AdminItem{Article: c.state}
			hotTopItem := hottop.AdminItem{Article: c.state}

			if got, want := collectionItem.Stale(), c.stale; got != want {
				t.Fatalf("合集判定 IsStale = %v，期望 %v", got, want)
			}
			if got, want := hotTopItem.Stale(), c.stale; got != want {
				t.Fatalf("热门置顶判定 IsStale = %v，期望 %v", got, want)
			}
			if got := collectionItem.StaleReason(); got != c.reason {
				t.Fatalf("合集失效原因 = %q，期望 %q", got, c.reason)
			}
			if got := hotTopItem.StaleReason(); got != c.reason {
				t.Fatalf("热门置顶失效原因 = %q，期望 %q", got, c.reason)
			}
		})
	}
}
