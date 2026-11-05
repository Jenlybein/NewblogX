package app_test

import (
	"context"
	"testing"

	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 列表读面与详情读面必须给出同一个权威计数（#279「读路径只认一个权威值入口」）。
//
// 这两个读面过去各自把 Redis 增量加到聚合上：详情走 enrichCounters，列表在自己的
// 事务体里再拼一遍。两处任何一处漏掉"叠增量"，同一篇文章在同一时刻就会有两个数。
// 现在两处都经 CountLedger，所以这条用例拿详情做对照即可。
func TestListAndDetailAgreeOnAuthoritativeCounters(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	articleID := ctype.NewID(1)

	svc, repos, _ := newService(t)

	columns := domain.Counters{View: 6, Digg: 5, Comment: 4, Favor: 3}
	delta := domain.Counters{View: 1, Digg: 2, Comment: 1, Favor: 3}
	want := domain.Counters{View: 7, Digg: 7, Comment: 5, Favor: 6}

	article := apptest.NewArticle().ID(articleID).Author(author).Status(domain.StatusPublished).Build()
	article.SetCounters(columns)
	repos.FakeArticles().Seed(article)
	if err := repos.FakeCounters().Add(ctx, domain.ArticleID(articleID), delta); err != nil {
		t.Fatalf("预置增量失败: %v", err)
	}

	views, total, err := svc.List(ctx, domain.Filter{}, domain.Page{Limit: 10}, authorViewer())
	if err != nil {
		t.Fatalf("列文章失败: %v", err)
	}
	if total != 1 || len(views) != 1 {
		t.Fatalf("应恰好 1 篇，实际 total=%d len=%d", total, len(views))
	}
	if got := views[0].Article.Counters(); got != want {
		t.Fatalf("列表读面计数应为「列 + 增量」%+v，实际 %+v", want, got)
	}

	detail, err := svc.Detail(ctx, domain.ArticleID(articleID), authorViewer(), false)
	if err != nil {
		t.Fatalf("查详情失败: %v", err)
	}
	if got := detail.Article.Counters(); got != want {
		t.Fatalf("详情读面必须与列表一致 %+v，实际 %+v", want, got)
	}
}
