package collection_test

import (
	"errors"
	"testing"
	"time"

	"myblogx/internal/apperr"
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

// TestNewRejectsTooLongTitleWithDistinctCode 超长标题必须报"超长"码，
// 不能复用"必填"码——否则返回的文案会变成"合集标题不能为空"。
func TestNewRejectsTooLongTitleWithDistinctCode(t *testing.T) {
	t.Parallel()

	long := make([]rune, collection.MaxTitleLen+1)
	for i := range long {
		long[i] = '题'
	}
	p := newParams()
	p.Title = string(long)
	_, err := collection.New(p)
	if !errors.Is(err, collection.ErrTitleTooLong) {
		t.Fatalf("超长标题应报 ErrTitleTooLong，实际 %v", err)
	}
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeCollectionTitleTooLong {
		t.Fatalf("错误码应为 %q，实际 %v/%v", apperr.CodeCollectionTitleTooLong, code, ok)
	}
	if apperr.IsCode(err, apperr.CodeCollectionTitleRequired) {
		t.Fatalf("超长标题不能落到必填码，实际 %v", err)
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

// TestErrorsCarryCodes 领域错误必须是错误码，不是中文文案（archcheck R8）。
func TestDomainErrorsAreCodes(t *testing.T) {
	t.Parallel()

	for name, err := range map[string]error{
		"标题必填": collection.ErrTitleRequired,
		"不存在":  collection.ErrNotFound,
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
