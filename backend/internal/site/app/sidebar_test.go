package app_test

import (
	"context"
	"strings"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/site/app"
	"myblogx/internal/site/apptest"
	"myblogx/internal/site/domain"
)

// newSidebar 构造一套右栏卡片替身。
func newSidebar() (*app.SidebarService, *apptest.SidebarCards, *apptest.SidebarData) {
	cards := apptest.NewSidebarCards()
	data := apptest.NewSidebarData()
	return app.NewSidebar(cards, data, nil), cards, data
}

func seedCard(id uint64, kind domain.SidebarCardKind, title string, sortOrder int, enabled bool) domain.SidebarCard {
	return domain.SidebarCard{
		ID:        ctype.NewID(id),
		Kind:      kind,
		Title:     title,
		SortOrder: sortOrder,
		Enabled:   enabled,
	}
}

// 读者侧只返回启用中的卡片，且按 sort_order 升序 —— 存进去的顺序不能决定返回顺序。
func TestPublicSidebarCardsEnabledAndOrdered(t *testing.T) {
	svc, cards, _ := newSidebar()
	cards.Seed(
		seedCard(7, domain.CardKindTagCloud, "标签", 2, true),
		seedCard(3, domain.CardKindHotArticles, "热门", 1, true),
		seedCard(9, domain.CardKindLatestComments, "停用卡", 3, false),
		seedCard(5, domain.CardKindLatestComments, "评论", 0, true),
	)

	got, err := svc.PublicCards(context.Background())
	if err != nil {
		t.Fatalf("读取读者侧卡片失败: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("停用卡不该返回：期望 3 张，实际 %d 张", len(got))
	}
	want := []domain.SidebarCardKind{
		domain.CardKindLatestComments,
		domain.CardKindHotArticles,
		domain.CardKindTagCloud,
	}
	for i, kind := range want {
		if got[i].Kind != kind {
			t.Fatalf("第 %d 张卡片类型应为 %s，实际 %s（顺序=%d）", i, kind, got[i].Kind, got[i].SortOrder)
		}
	}
}

// 每种卡只挂自己那一种数据，空数据原样返回（前端据此隐藏）。
func TestPublicSidebarCardsAttachKindData(t *testing.T) {
	svc, cards, data := newSidebar()
	data.PutArticles(domain.SidebarArticle{ID: ctype.NewID(1), Title: "点赞最多的文章", DiggCount: 42})
	// 标签云故意不给数据：它应回传空数组，而不是拿别的卡的数据顶上。
	data.PutComments(domain.SidebarComment{ID: ctype.NewID(2), Content: "写得清楚", ArticleTitle: "文章甲"})
	cards.Seed(
		seedCard(1, domain.CardKindHotArticles, "热门文章", 0, true),
		seedCard(2, domain.CardKindLatestComments, "最新评论", 1, true),
		seedCard(3, domain.CardKindTagCloud, "标签云", 2, true),
	)

	got, err := svc.PublicCards(context.Background())
	if err != nil {
		t.Fatalf("读取读者侧卡片失败: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("期望 3 张卡片，实际 %d", len(got))
	}
	if len(got[0].HotArticles) != 1 || got[0].HotArticles[0].DiggCount != 42 {
		t.Fatalf("热门文章卡应带点赞数据: %+v", got[0].HotArticles)
	}
	if len(got[0].LatestComments) != 0 || len(got[0].Tags) != 0 {
		t.Fatalf("热门文章卡不该混入别种数据: %+v", got[0])
	}
	if len(got[1].LatestComments) != 1 || got[1].LatestComments[0].ArticleTitle != "文章甲" {
		t.Fatalf("最新评论卡应带评论数据: %+v", got[1].LatestComments)
	}
	if len(got[2].Tags) != 0 {
		t.Fatalf("没有标签数据时应回传空（前端据此隐藏），实际 %+v", got[2].Tags)
	}
}

// 取数失败只影响那张卡的数据，不把整个接口带崩（ADR-0017 决策 5 的同一条口径）。
func TestPublicSidebarCardsDataFailureDegrades(t *testing.T) {
	svc, cards, data := newSidebar()
	data.FailWith(apperr.New(apperr.CodeCommonDBRequired))
	cards.Seed(
		seedCard(1, domain.CardKindHotArticles, "热门文章", 0, true),
		seedCard(2, domain.CardKindTagCloud, "标签云", 1, true),
	)

	got, err := svc.PublicCards(context.Background())
	if err != nil {
		t.Fatalf("取数失败不应让整个接口报错: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("配置本身仍应返回：期望 2 张，实际 %d", len(got))
	}
	if len(got[0].HotArticles) != 0 || len(got[1].Tags) != 0 {
		t.Fatalf("失败方应回传空数据: %+v", got)
	}
}

// 未知类型（库里历史脏数据）不给数据、不渲染；返回列表里保留原始 kind 供前端忽略。
func TestPublicSidebarCardsUnknownKind(t *testing.T) {
	svc, cards, _ := newSidebar()
	cards.Seed(seedCard(1, domain.SidebarCardKind("legacy_notice"), "旧公告", 0, true))

	got, err := svc.PublicCards(context.Background())
	if err != nil {
		t.Fatalf("未知类型不该让接口报错: %v", err)
	}
	if len(got) != 1 || got[0].Kind != "legacy_notice" {
		t.Fatalf("未知类型应原样回传、不带数据: %+v", got)
	}
}

// 管理端看到全部卡片（停用的也在），顺序与读者侧一致，方便后台直接按 returned.list 做排序。
func TestAdminSidebarCardsIncludesDisabled(t *testing.T) {
	svc, cards, _ := newSidebar()
	cards.Seed(
		seedCard(2, domain.CardKindLatestComments, "最新评论", 1, false),
		seedCard(1, domain.CardKindHotArticles, "热门文章", 0, true),
	)

	got, err := svc.AdminCards(context.Background())
	if err != nil {
		t.Fatalf("读取管理端卡片失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("管理端应包含停用卡：期望 2 张，实际 %d", len(got))
	}
	if got[0].Kind != domain.CardKindHotArticles || got[1].Kind != domain.CardKindLatestComments {
		t.Fatalf("管理端顺序应与读者侧一致: %+v", got)
	}
}

// 保存按"提交顺序"落排序号，空标题回退到该类型的默认标题。
func TestReplaceSidebarCardsNormalizesOrderAndTitle(t *testing.T) {
	svc, cards, _ := newSidebar()
	err := svc.ReplaceCards(context.Background(), []app.SidebarCardInput{
		{Kind: domain.CardKindTagCloud, Title: "", Enabled: true},
		{Kind: domain.CardKindHotArticles, Title: "今天读什么", Enabled: true},
		{Kind: domain.CardKindLatestComments, Title: "最新评论", Enabled: false},
	})
	if err != nil {
		t.Fatalf("保存卡片失败: %v", err)
	}

	got := cards.All()
	if len(got) != 3 {
		t.Fatalf("期望落 3 行，实际 %d", len(got))
	}
	if got[0].SortOrder != 0 || got[1].SortOrder != 1 || got[2].SortOrder != 2 {
		t.Fatalf("排序号应按提交顺序落 0/1/2: %+v", got)
	}
	if got[0].Title != domain.DefaultSidebarCardTitle(domain.CardKindTagCloud) {
		t.Fatalf("空标题应回退默认标题，实际 %q", got[0].Title)
	}
	if got[1].Title != "今天读什么" {
		t.Fatalf("显式标题应原样保留，实际 %q", got[1].Title)
	}
	if got[2].Enabled {
		t.Fatalf("停用状态应原样落库: %+v", got[2])
	}
}

// 未知类型与超量提交都拒绝，不能让脏类型落进表里。
func TestReplaceSidebarCardsRejectsInvalid(t *testing.T) {
	svc, cards, _ := newSidebar()

	err := svc.ReplaceCards(context.Background(), []app.SidebarCardInput{
		{Kind: "ai_card", Title: "AI", Enabled: true},
	})
	if err == nil || !apperr.IsCode(err, apperr.CodeSidebarCardKindInvalid) {
		t.Fatalf("未知类型应以 sidebar.card_kind_invalid 拒绝，实际 %v", err)
	}
	if cards.ReplaceCalls() != 0 {
		t.Fatal("校验失败时不应写库")
	}

	tooMany := make([]app.SidebarCardInput, domain.SidebarCardMaxCount+1)
	for i := range tooMany {
		tooMany[i] = app.SidebarCardInput{Kind: domain.CardKindHotArticles}
	}
	err = svc.ReplaceCards(context.Background(), tooMany)
	if err == nil || !apperr.IsCode(err, apperr.CodeSidebarCardLimitExceeded) {
		t.Fatalf("超量提交应以 sidebar.card_limit_exceeded 拒绝，实际 %v", err)
	}
}

// 推荐合集卡：新种类被分派到**合集**数据，别的种类数组一个都不沾。
//
// 分派是两端的唯一耦合点：后端按种类取数、前端按种类渲染，任何一端漏了这个
// 种类，那张卡要么没有数据要么不渲染 —— 不会画出一个空盒子。
func TestPublicSidebarCardsDispatchRecommendedCollections(t *testing.T) {
	svc, cards, data := newSidebar()
	data.PutCollections(
		domain.SidebarCollection{
			ID:           ctype.NewID(101),
			Title:        "从零搭一个博客",
			Description:  "一套完整的建站路径",
			Cover:        "https://cdn.example.com/collection.png",
			ArticleCount: 7,
		},
		domain.SidebarCollection{ID: ctype.NewID(102), Title: "工程效率", ArticleCount: 3},
	)
	// 脏数据：其它种类的数据源也有内容，不能串进合集卡。
	data.PutArticles(domain.SidebarArticle{ID: ctype.NewID(1), Title: "不该出现"})
	data.PutTags(domain.SidebarTag{ID: ctype.NewID(2), Title: "不该出现"})
	cards.Seed(seedCard(1, domain.CardKindRecommendedCollections, "推荐合集", 0, true))

	got, err := svc.PublicCards(context.Background())
	if err != nil {
		t.Fatalf("读取读者侧卡片失败: %v", err)
	}
	if len(got) != 1 || got[0].Kind != domain.CardKindRecommendedCollections {
		t.Fatalf("推荐合集卡应原样返回: %+v", got)
	}
	if len(got[0].Collections) != 2 {
		t.Fatalf("推荐合集卡应带 2 个合集，实际 %+v", got[0].Collections)
	}
	first := got[0].Collections[0]
	if first.Title != "从零搭一个博客" || first.Description != "一套完整的建站路径" ||
		first.Cover != "https://cdn.example.com/collection.png" || first.ArticleCount != 7 {
		t.Fatalf("合集条目字段应原样透传: %+v", first)
	}
	if len(got[0].HotArticles) != 0 || len(got[0].Tags) != 0 || len(got[0].LatestComments) != 0 {
		t.Fatalf("推荐合集卡不该混入别种数据: %+v", got[0])
	}
	if limit := data.LastLimit(string(domain.CardKindRecommendedCollections)); limit != domain.SidebarCardDataLimit {
		t.Fatalf("合集取数应受一张卡的条数上限约束，实际 limit=%d", limit)
	}
}

// 没有可推荐的合集时该卡回传空数组（读者站据此整块隐藏），接口本身照常成功。
func TestPublicSidebarCardsEmptyCollections(t *testing.T) {
	svc, cards, _ := newSidebar()
	cards.Seed(seedCard(1, domain.CardKindRecommendedCollections, "推荐合集", 0, true))

	got, err := svc.PublicCards(context.Background())
	if err != nil {
		t.Fatalf("没有合集数据不该让接口报错: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("配置本身仍应返回（是否渲染交给前端按空数组判断）: %+v", got)
	}
	if len(got[0].Collections) != 0 {
		t.Fatalf("没有可推荐合集时应回传空数组，实际 %+v", got[0].Collections)
	}
}

// 推荐合集是内置种类：管理端整表保存时合法，空标题回退默认标题。
func TestReplaceSidebarCardsRecommendedCollections(t *testing.T) {
	svc, cards, _ := newSidebar()

	err := svc.ReplaceCards(context.Background(), []app.SidebarCardInput{
		{Kind: domain.CardKindRecommendedCollections, Title: "   ", Enabled: true},
	})
	if err != nil {
		t.Fatalf("推荐合集应是合法内置种类: %v", err)
	}

	got := cards.All()
	if len(got) != 1 || got[0].Kind != domain.CardKindRecommendedCollections {
		t.Fatalf("应落一行推荐合集卡: %+v", got)
	}
	if got[0].Title != "推荐合集" {
		t.Fatalf("空标题应回退到「推荐合集」，实际 %q", got[0].Title)
	}
	if !got[0].Enabled {
		t.Fatalf("启停状态应原样落库: %+v", got[0])
	}
}

// 停用的推荐合集卡不进读者侧（排序/启停仍由既有配置控制）。
func TestPublicSidebarCardsSkipsDisabledRecommendedCollections(t *testing.T) {
	svc, cards, data := newSidebar()
	data.PutCollections(domain.SidebarCollection{ID: ctype.NewID(101), Title: "已关闭的合集"})
	cards.Seed(
		seedCard(1, domain.CardKindRecommendedCollections, "停用的合集卡", 0, false),
		seedCard(2, domain.CardKindHotArticles, "热门文章", 1, true),
	)

	got, err := svc.PublicCards(context.Background())
	if err != nil {
		t.Fatalf("读取读者侧卡片失败: %v", err)
	}
	if len(got) != 1 || got[0].Kind != domain.CardKindHotArticles {
		t.Fatalf("停用的推荐合集卡不该返回: %+v", got)
	}
}

// 空提交是合法的：运营把卡片全删了，读者侧回退到旧右栏（兼容口径）。
func TestReplaceSidebarCardsEmptyIsAllowed(t *testing.T) {
	svc, cards, _ := newSidebar()
	if err := svc.ReplaceCards(context.Background(), nil); err != nil {
		t.Fatalf("清空卡片应成功: %v", err)
	}
	if len(cards.All()) != 0 || cards.ReplaceCalls() != 1 {
		t.Fatalf("清空应落 0 行且只替换一次: rows=%+v calls=%d", cards.All(), cards.ReplaceCalls())
	}
}

// 富文本卡：保存时用既有用户内容白名单清洗，恶意输入不落库。
func TestReplaceSidebarCardsRichTextSanitized(t *testing.T) {
	svc, cards, _ := newSidebar()

	dirty := `<p>公告正文</p>` +
		`<script>alert(1)</script>` +
		`<img src="https://cdn.example.com/a.png" onerror="alert(1)">` +
		`<a href="javascript:alert(1)">点我</a>` +
		`<a href="https://example.com/post">合法链接</a>` +
		`<iframe src="https://evil.example.com"></iframe>`

	err := svc.ReplaceCards(context.Background(), []app.SidebarCardInput{
		{Kind: domain.CardKindRichText, Title: "  ", Enabled: true, Content: dirty},
	})
	if err != nil {
		t.Fatalf("富文本卡应是合法内置种类: %v", err)
	}

	got := cards.All()
	if len(got) != 1 || got[0].Kind != domain.CardKindRichText {
		t.Fatalf("应落一行富文本卡: %+v", got)
	}
	if got[0].Title != "公告" {
		t.Fatalf("空标题应回退到默认标题「公告」，实际 %q", got[0].Title)
	}
	lower := strings.ToLower(got[0].Content)
	for _, banned := range []string{"<script", "onerror", "javascript:", "iframe", "alert"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("恶意输入应被清洗掉 %q，实际 %q", banned, got[0].Content)
		}
	}
	if !strings.Contains(got[0].Content, "公告正文") || !strings.Contains(got[0].Content, "<a href=\"https://") {
		t.Fatalf("合法排版内容被误杀: %q", got[0].Content)
	}
}

// 非富文本卡不保存 content：脏字段不能跟着别的种类落库。
func TestReplaceSidebarCardsDropsContentForOtherKinds(t *testing.T) {
	svc, cards, _ := newSidebar()

	if err := svc.ReplaceCards(context.Background(), []app.SidebarCardInput{
		{Kind: domain.CardKindHotArticles, Title: "热门文章", Enabled: true, Content: "<p>不该落库</p>"},
	}); err != nil {
		t.Fatalf("保存热门文章卡失败: %v", err)
	}

	if got := cards.All(); got[0].Content != "" {
		t.Fatalf("非富文本卡不该保留 content，实际 %q", got[0].Content)
	}
}

// 读者侧富文本卡只带 content，别的数据数组保持空（前端按种类分派）。
func TestPublicSidebarCardsRichText(t *testing.T) {
	svc, cards, data := newSidebar()
	// 脏数据：其它种类的数据源有内容，不能串进富文本卡。
	data.PutArticles(domain.SidebarArticle{ID: ctype.NewID(1), Title: "不该出现"})
	data.PutTags(domain.SidebarTag{ID: ctype.NewID(2), Title: "不该出现"})
	cards.Seed(domain.SidebarCard{
		ID:      ctype.NewID(8),
		Kind:    domain.CardKindRichText,
		Title:   "站点公告",
		Enabled: true,
		Content: "<h2>更新日志</h2><p>本周 <strong>重点</strong></p>",
	})

	got, err := svc.PublicCards(context.Background())
	if err != nil {
		t.Fatalf("读取读者侧卡片失败: %v", err)
	}
	if len(got) != 1 || got[0].Kind != domain.CardKindRichText {
		t.Fatalf("富文本卡应原样返回: %+v", got)
	}
	if got[0].Content != "<h2>更新日志</h2><p>本周 <strong>重点</strong></p>" {
		t.Fatalf("富文本正文应原样透传: %q", got[0].Content)
	}
	if len(got[0].HotArticles) != 0 || len(got[0].Tags) != 0 || len(got[0].LatestComments) != 0 ||
		len(got[0].Collections) != 0 {
		t.Fatalf("富文本卡不该混入别种数据: %+v", got[0])
	}
}
