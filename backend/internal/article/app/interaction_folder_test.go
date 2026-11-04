package app_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/favorite"
	"myblogx/internal/kernel/ctype"
)

// TestUnfavoriteOnlyRemovesTargetFolder 钉住「移出 A 不能删掉 B 的那条」。
//
// 病根：旧实现用 FindItemAnyFolder 取任意一条，再按 item.FolderID() 删除，
// 完全忽略调用方传入的 folder。文章同时在 A/B 两个收藏夹时，点「移出 A」
// 可能删掉 B 的那条：A 的勾还在、toast 却说「已从 A 移除」，而 B 的收藏凭空消失。
func TestUnfavoriteOnlyRemovesTargetFolder(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	id := ctype.NewID(1)
	seedArticle(t, repos, id, author, domain.StatusPublished)

	folderA := favorite.RestoreFolder(favorite.RestoreFolderParams{
		ID: ctype.NewID(11), UserID: strangerViewer().UserID, Title: "A",
	})
	folderB := favorite.RestoreFolder(favorite.RestoreFolderParams{
		ID: ctype.NewID(12), UserID: strangerViewer().UserID, Title: "B",
	})
	repos.FakeFavorites().SeedFolder(folderA).SeedFolder(folderB)

	// 同一篇文章收进 A 与 B。
	for _, folderID := range []favorite.ID{folderA.ID(), folderB.ID()} {
		if _, err := svc.Favorite(ctx, app.FavoriteCmd{
			Actor: strangerViewer(), ArticleID: id, FolderID: folderID,
		}); err != nil {
			t.Fatalf("收藏到 %s 失败: %v", folderID, err)
		}
	}
	if got := repos.FakeCounters().Get(id).Favor; got != 2 {
		t.Fatalf("两个收藏夹应有 2 次收藏增量，实际 %d", got)
	}

	res, err := svc.Unfavorite(ctx, app.FavoriteCmd{
		Actor: strangerViewer(), ArticleID: id, FolderID: folderA.ID(),
	})
	if err != nil {
		t.Fatalf("移出 A 失败: %v", err)
	}

	// A 的关系行必须真的消失。
	if _, err := repos.FakeFavorites().FindItem(ctx, id, strangerViewer().UserID, folderA.ID()); !errors.Is(err, favorite.ErrItemNotFound) {
		t.Fatalf("A 的关系行应已删除，实际 err=%v", err)
	}
	// B 的关系行必须原样保留：旧实现会误删这一条。
	if _, err := repos.FakeFavorites().FindItem(ctx, id, strangerViewer().UserID, folderB.ID()); err != nil {
		t.Fatalf("B 的关系行不应受影响，实际 err=%v", err)
	}
	if !res.IsFavor {
		t.Fatal("还在 B 里，is_favor 应为 true（语义是「是否仍在任一收藏夹」）")
	}
	if got := repos.FakeCounters().Get(id).Favor; got != 1 {
		t.Fatalf("移出 A 后收藏增量应为 1，实际 %d", got)
	}

	// 再移出 B：两夹都空了，is_favor 才是 false。
	res, err = svc.Unfavorite(ctx, app.FavoriteCmd{
		Actor: strangerViewer(), ArticleID: id, FolderID: folderB.ID(),
	})
	if err != nil {
		t.Fatalf("移出 B 失败: %v", err)
	}
	if res.IsFavor {
		t.Fatal("两个收藏夹都移出后 is_favor 应为 false")
	}
	if got := repos.FakeCounters().Get(id).Favor; got != 0 {
		t.Fatalf("两个收藏夹都移出后收藏增量应回到 0，实际 %d", got)
	}
}

// TestUnfavoriteMissingTargetFolderIsNotFound 钉住「找不到该夹的记录按不存在处理」。
func TestUnfavoriteMissingTargetFolderIsNotFound(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	id := ctype.NewID(1)
	seedArticle(t, repos, id, author, domain.StatusPublished)

	folderA := favorite.RestoreFolder(favorite.RestoreFolderParams{
		ID: ctype.NewID(11), UserID: strangerViewer().UserID, Title: "A",
	})
	folderB := favorite.RestoreFolder(favorite.RestoreFolderParams{
		ID: ctype.NewID(12), UserID: strangerViewer().UserID, Title: "B",
	})
	repos.FakeFavorites().SeedFolder(folderA).SeedFolder(folderB)

	// 只收进 B，却点名从 A 移除。
	if _, err := svc.Favorite(ctx, app.FavoriteCmd{
		Actor: strangerViewer(), ArticleID: id, FolderID: folderB.ID(),
	}); err != nil {
		t.Fatalf("收藏到 B 失败: %v", err)
	}

	if _, err := svc.Unfavorite(ctx, app.FavoriteCmd{
		Actor: strangerViewer(), ArticleID: id, FolderID: folderA.ID(),
	}); !errors.Is(err, favorite.ErrItemNotFound) {
		t.Fatalf("目标夹里没有记录应报 ErrItemNotFound，实际 %v", err)
	}
	// 失败时不得顺手删掉 B 的那条、也不得扣减计数。
	if _, err := repos.FakeFavorites().FindItem(ctx, id, strangerViewer().UserID, folderB.ID()); err != nil {
		t.Fatalf("B 的关系行不应被误删: %v", err)
	}
	if got := repos.FakeCounters().Get(id).Favor; got != 1 {
		t.Fatalf("失败不应扣减计数，实际 %d", got)
	}
}

// TestUnfavoriteZeroFolderUsesDefault 钉住未指定收藏夹时的口径。
//
// `FavoriteCmd.FolderID == 0` 的既有语义是「落到默认收藏夹」，取消收藏必须
// 用同一口径：否则不传 favorite_id 的旧调用会退化成「随便删一条」。
func TestUnfavoriteZeroFolderUsesDefault(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()
	id := ctype.NewID(1)
	seedArticle(t, repos, id, author, domain.StatusPublished)

	if _, err := svc.Favorite(ctx, app.FavoriteCmd{Actor: strangerViewer(), ArticleID: id}); err != nil {
		t.Fatalf("收藏失败: %v", err)
	}
	res, err := svc.Unfavorite(ctx, app.FavoriteCmd{Actor: strangerViewer(), ArticleID: id})
	if err != nil {
		t.Fatalf("默认收藏夹取消收藏失败: %v", err)
	}
	if res.IsFavor {
		t.Fatalf("默认收藏夹移出后 is_favor 应为 false，实际 %v", res)
	}
}
