package collection_test

import (
	"errors"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/collection"
	"myblogx/internal/kernel/ctype"
)

// now 是测试统用的"现在"：领域不取时间，时间由调用方传入。
var now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func newParams() collection.NewParams {
	return collection.NewParams{
		ID:          ctype.NewID(1),
		Title:       "Nuxt 部署实战",
		Description: "从零到上线",
		Cover:       "https://img.example/cover.png",
		CreatorID:   ctype.NewID(100),
		Displayed:   true,
		SortOrder:   1,
		Now:         now,
	}
}

// TestNewRequiresTitle 标题是合集的唯一必填项。
func TestNewRequiresTitle(t *testing.T) {
	t.Parallel()

	for _, title := range []string{"", "   ", "\t\n"} {
		p := newParams()
		p.Title = title
		if _, err := collection.New(p); !apperr.IsCode(err, apperr.CodeCollectionTitleRequired) {
			t.Fatalf("标题 %q 应报标题必填，实际 %v", title, err)
		}
	}
}

// TestNewDefaultsToNotEmpty 创建时必须给出标题以外的字段都能落到聚合上。
func TestNewNormalizes(t *testing.T) {
	t.Parallel()

	c, err := collection.New(newParams())
	if err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	if c.ID() != ctype.NewID(1) {
		t.Fatalf("ID 不对: %s", c.ID())
	}
	if c.Title() != "Nuxt 部署实战" || c.Description() != "从零到上线" {
		t.Fatalf("标题/描述没有落到聚合上: %q / %q", c.Title(), c.Description())
	}
	if !c.Displayed() {
		t.Fatal("创建的合集应默认展示")
	}
	if c.CreatorID() != ctype.NewID(100) {
		t.Fatalf("创建者不对: %s", c.CreatorID())
	}
	if !c.CreatedAt().Equal(now) {
		t.Fatalf("创建时间应取调用方传入的 now，实际 %s", c.CreatedAt())
	}
}

// TestSetDisplay 展示开关是管理员可见性闸门：关掉后读者侧查不到。
func TestSetDisplay(t *testing.T) {
	t.Parallel()

	c, err := collection.New(newParams())
	if err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	c.SetDisplay(false, now)
	if c.Displayed() {
		t.Fatal("SetDisplay(false) 之后应为不展示")
	}
	c.SetDisplay(true, now)
	if !c.Displayed() {
		t.Fatal("SetDisplay(true) 之后应为展示")
	}
}

// TestRetitleRejectsEmpty 改标题也要过同一条不变量。
func TestRetitleRejectsEmpty(t *testing.T) {
	t.Parallel()

	c, err := collection.New(newParams())
	if err != nil {
		t.Fatalf("建合集失败: %v", err)
	}
	if err := c.Retitle("  ", now); !apperr.IsCode(err, apperr.CodeCollectionTitleRequired) {
		t.Fatalf("空标题应被拒绝，实际 %v", err)
	}
	if err := c.Retitle("新标题", now); err != nil {
		t.Fatalf("合法标题不应报错: %v", err)
	}
	if c.Title() != "新标题" {
		t.Fatalf("标题未更新: %q", c.Title())
	}
}

// TestCanBeIncluded 只有"已发布且可见"的文章能被收录。
//
// 复用文章聚合的 IsPubliclyVisible 而不是自己再判一次：两处一旦分叉，
// 读者侧就会出现一张点进去 404 的合集条目。
func TestCanBeIncluded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		build func() *domain.Article
		want  bool
	}{
		{"已发布且可见", func() *domain.Article {
			return domain.Restore(domain.RestoreParams{
				ID: ctype.NewID(1), AuthorID: ctype.NewID(100), Title: "t",
				Status: domain.StatusPublished, Visibility: domain.VisibilityVisible,
			})
		}, true},
		{"草稿", func() *domain.Article {
			return domain.Restore(domain.RestoreParams{
				ID: ctype.NewID(2), Title: "t",
				Status: domain.StatusDraft, Visibility: domain.VisibilityVisible,
			})
		}, false},
		{"作者隐藏", func() *domain.Article {
			return domain.Restore(domain.RestoreParams{
				ID: ctype.NewID(3), Title: "t",
				Status: domain.StatusPublished, Visibility: domain.VisibilityUserHidden,
			})
		}, false},
		{"管理员隐藏", func() *domain.Article {
			return domain.Restore(domain.RestoreParams{
				ID: ctype.NewID(4), Title: "t",
				Status: domain.StatusPublished, Visibility: domain.VisibilityAdminHidden,
			})
		}, false},
		{"已删除", func() *domain.Article {
			a := domain.Restore(domain.RestoreParams{
				ID: ctype.NewID(5), Title: "t",
				Status: domain.StatusPublished, Visibility: domain.VisibilityVisible,
			})
			a.SoftDelete(now)
			return a
		}, false},
		{"nil", func() *domain.Article { return nil }, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := collection.CanBeIncluded(c.build()); got != c.want {
				t.Fatalf("CanBeIncluded(%s) = %v，期望 %v", c.name, got, c.want)
			}
		})
	}
}

// TestCheckOrder 全量顺序替换只接受"同一批文章的一个排列"。
//
// 不允许"缺的删掉"：后台排序按钮拿到的是上一次读到的列表，一个过期页面
// 点一次「上移」就会把别人刚收录的那篇静默删掉。
func TestCheckOrder(t *testing.T) {
	t.Parallel()

	a, b, c := ctype.NewID(1), ctype.NewID(2), ctype.NewID(3)

	t.Run("同一集合换序通过", func(t *testing.T) {
		t.Parallel()
		if err := collection.CheckOrder([]collection.ArticleID{a, b, c},
			[]collection.ArticleID{c, a, b}); err != nil {
			t.Fatalf("合法换序应通过，实际 %v", err)
		}
	})

	t.Run("数量不等被拒", func(t *testing.T) {
		t.Parallel()
		if err := collection.CheckOrder([]collection.ArticleID{a, b},
			[]collection.ArticleID{a}); !apperr.IsCode(err, apperr.CodeCollectionOrderMismatch) {
			t.Fatalf("缺一条应被拒，实际 %v", err)
		}
	})

	t.Run("多出一条被拒", func(t *testing.T) {
		t.Parallel()
		if err := collection.CheckOrder([]collection.ArticleID{a},
			[]collection.ArticleID{a, b}); !apperr.IsCode(err, apperr.CodeCollectionOrderMismatch) {
			t.Fatalf("多一条应被拒，实际 %v", err)
		}
	})

	t.Run("外来文章被拒", func(t *testing.T) {
		t.Parallel()
		if err := collection.CheckOrder([]collection.ArticleID{a, b},
			[]collection.ArticleID{a, c}); !apperr.IsCode(err, apperr.CodeCollectionOrderMismatch) {
			t.Fatalf("替换成别的文章应被拒，实际 %v", err)
		}
	})

	t.Run("重复项被拒", func(t *testing.T) {
		t.Parallel()
		if err := collection.CheckOrder([]collection.ArticleID{a, b},
			[]collection.ArticleID{a, a}); !apperr.IsCode(err, apperr.CodeCollectionOrderMismatch) {
			t.Fatalf("重复项应被拒，实际 %v", err)
		}
	})
}

// TestIsStale 收录行是否已失效：文章没了 / 被删 / 不再公开。
//
// 失效行**保留**在表里（后台要能看见并清理），读者侧靠 JOIN 过滤掉。
func TestIsStale(t *testing.T) {
	t.Parallel()

	// nil 接收者代表"文章已经被物理删除"：行还在，join 不到文章。
	var missing *collection.ArticleState
	if !missing.IsStale() {
		t.Fatal("文章已物理删除时应判为失效")
	}

	state := &collection.ArticleState{Deleted: true}
	if !state.IsStale() {
		t.Fatal("软删的文章应判为失效")
	}
	state = &collection.ArticleState{Status: domain.StatusDraft, Visibility: domain.VisibilityVisible}
	if !state.IsStale() {
		t.Fatal("退回草稿的文章应判为失效")
	}
	state = &collection.ArticleState{Status: domain.StatusPublished, Visibility: domain.VisibilityVisible}
	if state.IsStale() {
		t.Fatal("仍公开的文章不应判为失效")
	}
}

// TestErrorsCarryCodes 领域错误必须是错误码，不是中文文案（archcheck R8）。
func TestDomainErrorsAreCodes(t *testing.T) {
	t.Parallel()

	for name, err := range map[string]error{
		"标题必填": collection.ErrTitleRequired,
		"不存在": collection.ErrNotFound,
		"重复收录": collection.ErrDuplicateArticle,
		"文章不可收录": collection.ErrNotIncludable,
		"顺序不一致": collection.ErrOrderMismatch,
	} {
		if err == nil {
			t.Fatalf("%s 错误不应为 nil", name)
		}
		if _, ok := apperr.CodeOf(err); !ok {
			t.Fatalf("%s 错误没有携带错误码: %v", name, err)
		}
	}
	// errors.Is 按错误码比较，跨实例也成立。
	if !errors.Is(collection.ErrNotFound, apperr.New(apperr.CodeCollectionNotFound)) {
		t.Fatal("同码不同实例应被判为同一错误")
	}
}
