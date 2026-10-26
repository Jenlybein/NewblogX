package taxonomy_test

import (
	"errors"
	"myblogx/internal/kernel/ctype"
	"testing"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/taxonomy"
)

func TestNewCategoryRequiresTitle(t *testing.T) {
	t.Parallel()

	if _, err := taxonomy.NewCategory(ctype.NewID(1), domain.UserID(ctype.NewID(1)), "   "); !errors.Is(err, taxonomy.ErrTitleRequired) {
		t.Fatalf("空标题应报 ErrTitleRequired，实际 %v", err)
	}

	c, err := taxonomy.NewCategory(ctype.NewID(1), domain.UserID(ctype.NewID(1)), "  技术  ")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if c.Title() != "技术" {
		t.Fatalf("标题应去首尾空白，实际 %q", c.Title())
	}
	if err := c.Rename(""); !errors.Is(err, taxonomy.ErrTitleRequired) {
		t.Fatalf("改名为空应报错，实际 %v", err)
	}
}

// TestFilterUsable 锁住"请求带停用标签就整单失败"的语义。
//
// 旧实现 loadEnabledTagsByIDs 就是这么做的：静默丢弃会让用户以为标签加上了。
func TestFilterUsable(t *testing.T) {
	t.Parallel()

	ok1, _ := taxonomy.NewTag(ctype.NewID(1), "Go", ctype.NewID(1))
	ok2, _ := taxonomy.NewTag(ctype.NewID(2), "Vue", ctype.NewID(1))
	disabled, _ := taxonomy.NewTag(ctype.NewID(3), "废弃", ctype.NewID(1))
	disabled.Disable()

	got, err := taxonomy.FilterUsable([]*taxonomy.Tag{ok1, ok2})
	if err != nil {
		t.Fatalf("全部可用不应报错: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应返回 2 个标签，实际 %d", len(got))
	}

	if _, err := taxonomy.FilterUsable([]*taxonomy.Tag{ok1, disabled}); !errors.Is(err, taxonomy.ErrTagDisabled) {
		t.Fatalf("含停用标签应报 ErrTagDisabled，实际 %v", err)
	}
}

func TestTagToggle(t *testing.T) {
	t.Parallel()

	tag, _ := taxonomy.NewTag(ctype.NewID(1), "Go", ctype.NewID(1))
	if !tag.Usable() {
		t.Fatal("新建标签默认可用")
	}
	tag.Disable()
	if tag.Usable() {
		t.Fatal("停用后不可用")
	}
	tag.Enable()
	if !tag.Usable() {
		t.Fatal("重新启用后可用")
	}
	if err := tag.Rename("  ", 0, ""); !errors.Is(err, taxonomy.ErrTitleRequired) {
		t.Fatalf("改名为空应报错，实际 %v", err)
	}
}
