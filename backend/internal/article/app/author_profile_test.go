package app_test

import (
	"context"
	"testing"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// TestAuthorProfileReadsVisitedTotalFromArticles 钉住作者卡「阅读」的口径：
// 不是 user_stat_models.article_visited_count（该列只有创建/删除时随文章数增删，
// 浏览增量从来没写进去，线上恒为 0），而是已发布文章的 SUM(view_count)。
//
// 用"统计列 0、文章汇总 571"这一对刻意矛盾的输入：如果实现又退回读统计列，
// 这里会立刻变红。
func TestAuthorProfileReadsVisitedTotalFromArticles(t *testing.T) {
	userID := domain.UserID(ctype.NewID(100))
	repos := apptest.NewRepos()
	tx := apptest.NewTx(repos)
	svc := app.New(tx, apptest.NewClock(apptest.DefaultTime), apptest.NewIDGen(1000), nil, nil, nil, nil)

	repos.FakeUsers().Seed(app.AuthorView{
		ID:       userID,
		Nickname: "作者甲",
		Username: "u100",
		// 统计列里的陈旧值：必须被文章汇总覆盖。
		ArticleVisitedCount: 0,
	})
	repos.FakeAuthorStats().SeedVisited(userID, 571)

	v, err := svc.AuthorProfile(context.Background(), userID)
	if err != nil {
		t.Fatalf("读取作者信息失败: %v", err)
	}
	if v.ArticleVisitedCount != 571 {
		t.Fatalf("阅读数应取已发布文章汇总 571，实际 %d", v.ArticleVisitedCount)
	}
	if v.Nickname != "作者甲" {
		t.Fatalf("昵称应保持用户资料里的值，实际 %q", v.Nickname)
	}
}
