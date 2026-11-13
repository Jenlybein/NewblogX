package app_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// TestSitemapListsOnlyPublicArticles 钉住站点地图的文章集合（#314 交付 1）。
//
// 「只列已发布、可见、且通过作者可见性开关的文章」不是一条新规则，而是公开读面
// 已经生效的两条过滤的合成：
//   - 仓储层的 OnlyPublic（= 已发布 + visible，见 domain.Filter.OnlyPublic）；
//   - 开关层的 domain.CanBeSeenByReader（「显示非管理员发布文章」关闭时只出管理员作者）。
//
// 所以这里逐条摆出"该进"与"不该进"的样本，防止将来有人为了省事直接全表扫描，
// 把草稿 / 审核中 / 已隐藏的文章列进站点地图——那等于主动把私密内容交给爬虫。
func TestSitemapListsOnlyPublicArticles(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	ctx := context.Background()

	nonAdminAuthor := ctype.ID("300")
	adminAuthorID := ctype.ID("901")
	repos.FakeUsers().Seed(
		app.AuthorView{ID: nonAdminAuthor, Nickname: "普通作者", Username: "u300"},
		app.AuthorView{ID: adminAuthorID, Nickname: "管理员", Username: "u901", Admin: true},
	)

	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(ctype.NewID(11)).Author(adminAuthorID).Status(domain.StatusPublished).Build(),
		apptest.NewArticle().ID(ctype.NewID(12)).Author(nonAdminAuthor).Status(domain.StatusPublished).Build(),
		apptest.NewArticle().ID(ctype.NewID(13)).Author(adminAuthorID).Status(domain.StatusPublished).
			Visibility(domain.VisibilityUserHidden).Build(),
		apptest.NewArticle().ID(ctype.NewID(14)).Author(adminAuthorID).Status(domain.StatusDraft).Build(),
		apptest.NewArticle().ID(ctype.NewID(15)).Author(adminAuthorID).Status(domain.StatusExamining).Build(),
		apptest.NewArticle().ID(ctype.NewID(16)).Author(adminAuthorID).Status(domain.StatusPublished).Deleted().Build(),
	)

	entries, err := svc.Sitemap(ctx, false)
	if err != nil {
		t.Fatalf("生成站点地图失败: %v", err)
	}
	if got, want := sitemapPaths(entries), []string{"/article/11", "/article/12"}; !sameSet(got, want) {
		t.Fatalf("开关打开时应只含已发布且可见的文章，实际 %v，期望 %v", got, want)
	}

	// 开关关闭（只出管理员作者）时，非管理员作者的文章必须落榜——与详情页 404
	// 是同一条规则（ADR-0015 决策 4），不是第二套可见性判定。
	entries, err = svc.Sitemap(ctx, true)
	if err != nil {
		t.Fatalf("生成站点地图失败: %v", err)
	}
	if got, want := sitemapPaths(entries), []string{"/article/11"}; !sameSet(got, want) {
		t.Fatalf("「显示非管理员发布文章」关闭时应只含管理员作者的文章，实际 %v，期望 %v", got, want)
	}
}

// TestSitemapLastmodIsUpdatedAt 钉住 <lastmod> 取的是"最后修改"而不是创建时间。
//
// 取创建时间会让改过的老文章在爬虫眼里永远是旧的，站点地图就只剩"清单"作用，
// 失去了"我这儿有更新，来重抓"的意义。同时把时区固定成 UTC：sitemap 的
// lastmod 必须是可解析的 W3C 时间，本地时区会让输出随机器变化。
func TestSitemapLastmodIsUpdatedAt(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	createdAt := time.Date(2026, 3, 1, 8, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)

	repos.FakeArticles().Seed(domain.Restore(domain.RestoreParams{
		ID:         ctype.NewID(21),
		AuthorID:   author,
		Title:      "改过的老文章",
		Content:    "正文",
		Status:     domain.StatusPublished,
		Visibility: domain.VisibilityVisible,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}))

	entries, err := svc.Sitemap(context.Background(), false)
	if err != nil {
		t.Fatalf("生成站点地图失败: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("应只有一条文章条目，实际 %d 条", len(entries))
	}
	if !entries[0].Lastmod.Equal(updatedAt) {
		t.Fatalf("lastmod 应取最后修改时间 %s，实际 %s", updatedAt, entries[0].Lastmod)
	}
}

// TestSitemapIsNotTruncatedAtOnePage 钉住分页：仓储的 Page 上限是 100 条，
// 站点地图必须翻完所有页。
//
// 这是最容易写错也最难发现的一处：只查第一页时，第 101 篇文章起会静默缺席，
// 接口照样 200、XML 照样合法，只是新文章再也进不了索引。
func TestSitemapIsNotTruncatedAtOnePage(t *testing.T) {
	t.Parallel()

	svc, repos, _ := newService(t)
	const total = 150

	articles := make([]*domain.Article, 0, total)
	for i := 0; i < total; i++ {
		id := ctype.ID(strconv.Itoa(1000 + i))
		articles = append(articles, apptest.NewArticle().ID(id).Author(author).
			Status(domain.StatusPublished).Build())
	}
	repos.FakeArticles().Seed(articles...)

	entries, err := svc.Sitemap(context.Background(), false)
	if err != nil {
		t.Fatalf("生成站点地图失败: %v", err)
	}
	if len(entries) != total {
		t.Fatalf("站点地图应含全部 %d 篇文章，实际 %d 篇（分页没有翻完）", total, len(entries))
	}
	got := sitemapPaths(entries)
	if !containsString(got, "/article/1000") || !containsString(got, "/article/1149") {
		t.Fatalf("首末两篇文章都应出现，实际首条 %s、末条 %s", got[0], got[len(got)-1])
	}
}

// sitemapPaths 取条目路径，方便按集合比对。
func sitemapPaths(entries []app.SitemapEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

// sameSet 判断两个字符串切片是否同集合（忽略顺序）。
func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]int, len(got))
	for _, s := range got {
		seen[s]++
	}
	for _, s := range want {
		if seen[s] == 0 {
			return false
		}
		seen[s]--
	}
	return true
}

// containsString 判断切片里是否含某个值。
func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
