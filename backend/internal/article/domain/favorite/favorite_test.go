package favorite_test

import (
	"errors"
	"testing"
	"time"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
)

func TestFolderRules(t *testing.T) {
	t.Parallel()

	if _, err := favorite.NewFolder(1, domain.UserID(1), " "); !errors.Is(err, favorite.ErrTitleRequired) {
		t.Fatalf("空名应报错，实际 %v", err)
	}
	if _, err := favorite.NewFolder(1, 0, "默认"); !errors.Is(err, favorite.ErrFolderForbidden) {
		t.Fatalf("无用户应报错，实际 %v", err)
	}

	normal, _ := favorite.NewFolder(1, domain.UserID(1), "我的收藏")
	if !normal.CanDelete() {
		t.Fatal("普通收藏夹可删")
	}
	if err := normal.Rename("", "", ""); !errors.Is(err, favorite.ErrTitleRequired) {
		t.Fatalf("改名为空应报错，实际 %v", err)
	}

	def := favorite.RestoreFolder(favorite.RestoreFolderParams{ID: 2, UserID: 1, Title: "默认收藏", IsDefault: true})
	if def.CanDelete() {
		t.Fatal("默认收藏夹不可删")
	}
}

// TestNewItemFromArticleSnapshots 确认收藏项把文章字段拷进快照，
// 收藏列表因此不用 join 文章表（这是既有设计，必须保持）。
func TestNewItemFromArticleSnapshots(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a, err := domain.NewArticle(domain.NewArticleParams{
		ID:       domain.ArticleID(77),
		AuthorID: domain.UserID(5),
		Title:    "被收藏的文章",
		Abstract: "摘要",
		Content:  "正文",
		Cover:    "http://img/1.png",
		Status:   domain.StatusPublished,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("构造文章失败: %v", err)
	}

	item := favorite.NewItemFromArticle(favorite.ID(3), domain.UserID(9), a)

	if item.UserID() != 9 {
		t.Fatalf("收藏者应为 9，实际 %d", item.UserID())
	}
	if item.FolderID() != 3 {
		t.Fatalf("收藏夹应为 3，实际 %d", item.FolderID())
	}
	if item.ArticleID() != 77 {
		t.Fatalf("文章应为 77，实际 %d", item.ArticleID())
	}
}
