package app_test

import (
	"context"
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
