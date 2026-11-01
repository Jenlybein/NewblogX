package mysql

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/kernel/ctype"
	"myblogx/test/testutil"
)

// TestSaveCategoryDuplicateTitleReturnsReadableError 钉住唯一键冲突的翻译。
//
// 场景：应用层先做 ExistsTitle 查重，但两次请求并发时仍可能同时通过查重、
// 最后由表上的 uk_category_user_title 拒绝后一次。如果适配层把
// gorm.ErrDuplicatedKey 原样返出去，handler 会把它当普通失败透传，
// 用户看到的就是 "UNIQUE constraint failed..." / MySQL 1062，而不是
// "名称重复"。
//
// 这里用与生产一致的表结构（composite unique index + TranslateError）复现冲突，
// 断言仓储把底层错误翻译成 taxonomy 的领域错误 —— 文案由 apperr 消息表提供。
func TestSaveCategoryDuplicateTitleReturnsReadableError(t *testing.T) {
	db := testutil.SetupSQLite(t, &categoryRow{})
	if err := db.Exec(
		"CREATE UNIQUE INDEX uk_category_user_title ON category_models(user_id, title)",
	).Error; err != nil {
		t.Fatalf("建唯一索引失败: %v", err)
	}

	repo := NewCategoryRepo(db)
	ctx := context.Background()
	userID := domain.UserID(ctype.NewID(100))

	first, err := taxonomy.NewCategory(domain.CategoryID(ctype.NewID(1)), userID, "前端")
	if err != nil {
		t.Fatalf("构造第一个分类失败: %v", err)
	}
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("保存第一个分类失败: %v", err)
	}

	second, err := taxonomy.NewCategory(domain.CategoryID(ctype.NewID(2)), userID, "前端")
	if err != nil {
		t.Fatalf("构造第二个分类失败: %v", err)
	}
	if err := repo.Save(ctx, second); err != nil {
		if !errors.Is(err, taxonomy.ErrTitleDuplicated) {
			t.Fatalf("唯一键冲突必须翻译成领域错误，实际: %v", err)
		}
		if err.Error() != "名称重复" {
			t.Fatalf("面向用户的文案应为「名称重复」，实际: %q", err.Error())
		}
		return
	}
	t.Fatal("同名分类第二次保存应失败，实际成功了")
}

// TestSaveCategoryDuplicateTitleOnRenameReturnsReadableError 覆盖更新路径：
// 把已有分类改成一个同用户下已存在的标题，撞的也是同一个唯一索引。
func TestSaveCategoryDuplicateTitleOnRenameReturnsReadableError(t *testing.T) {
	db := testutil.SetupSQLite(t, &categoryRow{})
	if err := db.Exec(
		"CREATE UNIQUE INDEX uk_category_user_title ON category_models(user_id, title)",
	).Error; err != nil {
		t.Fatalf("建唯一索引失败: %v", err)
	}

	repo := NewCategoryRepo(db)
	ctx := context.Background()
	userID := domain.UserID(ctype.NewID(100))

	first, _ := taxonomy.NewCategory(domain.CategoryID(ctype.NewID(1)), userID, "前端")
	second, _ := taxonomy.NewCategory(domain.CategoryID(ctype.NewID(2)), userID, "后端")
	for _, c := range []*taxonomy.Category{first, second} {
		if err := repo.Save(ctx, c); err != nil {
			t.Fatalf("预置分类 %q 失败: %v", c.Title(), err)
		}
	}

	// 把 second 改名成 first 的标题。
	if err := second.Rename("前端"); err != nil {
		t.Fatalf("改名失败: %v", err)
	}
	if err := repo.Save(ctx, second); err != nil {
		if !errors.Is(err, taxonomy.ErrTitleDuplicated) {
			t.Fatalf("改名撞唯一键必须翻译成领域错误，实际: %v", err)
		}
		return
	}
	t.Fatal("改名成同名分类应失败，实际成功了")
}
