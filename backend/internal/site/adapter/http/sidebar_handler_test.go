package http_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/res"
	sitehttp "myblogx/internal/site/adapter/http"
	"myblogx/internal/site/app"
	sitetest "myblogx/internal/site/apptest"
	"myblogx/internal/site/domain"

	"github.com/gin-gonic/gin"
)

// newSidebarHandler 构造带右栏卡片的 HTTP 处理器。
func newSidebarHandler(t *testing.T) (*sitehttp.Handler, *sitetest.SidebarCards, *sitetest.SidebarData) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cards := sitetest.NewSidebarCards()
	data := sitetest.NewSidebarData()
	svc := app.NewSidebar(cards, data, nil)
	h := sitehttp.NewHandler(sitetest.NewDefaultService(), nil, sitehttp.WithSidebar(svc))
	return h, cards, data
}

func seedSidebarCard(cards *sitetest.SidebarCards, id uint64, kind domain.SidebarCardKind, title string, order int, enabled bool) {
	cards.Seed(domain.SidebarCard{
		ID:        ctype.NewID(id),
		Kind:      kind,
		Title:     title,
		SortOrder: order,
		Enabled:   enabled,
	})
}

func readSidebarBody(t *testing.T, h *sitehttp.Handler, call func(*gin.Context)) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/site/sidebar_cards", nil)
	call(c)
	return recorder.Body.String()
}

// 读者侧接口：只返回启用中的卡、按顺序返回、各自带数据。
func TestSidebarCardsReaderEndpoint(t *testing.T) {
	h, cards, data := newSidebarHandler(t)
	seedSidebarCard(cards, 9, domain.CardKindTagCloud, "停用标签", 2, false)
	seedSidebarCard(cards, 7, domain.CardKindTagCloud, "常用标签", 2, true)
	seedSidebarCard(cards, 3, domain.CardKindHotArticles, "本周热读", 0, true)
	seedSidebarCard(cards, 5, domain.CardKindLatestComments, "大家怎么说", 1, true)
	data.PutArticles(domain.SidebarArticle{ID: ctype.NewID(1), Title: "热读一", DiggCount: 12})
	data.PutComments(domain.SidebarComment{ID: ctype.NewID(2), Content: "写得清楚", ArticleTitle: "文章甲"})
	data.PutTags(domain.SidebarTag{ID: ctype.NewID(3), Title: "Go", ArticleCount: 5})

	body := readSidebarBody(t, h, h.SidebarCards)

	var envelope struct {
		Code int                           `json:"code"`
		Msg  string                        `json:"msg"`
		Data sitehttp.SidebarCardsResponse `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("解析响应失败: %v，body=%s", err, body)
	}
	if envelope.Code != int(res.SuccessCode) {
		t.Fatalf("接口应成功返回，实际 code=%d msg=%s", envelope.Code, envelope.Msg)
	}
	list := envelope.Data.List
	if len(list) != 3 {
		t.Fatalf("停用卡不该返回：期望 3 张，实际 %d 张（%s）", len(list), body)
	}
	wantKinds := []string{"hot_articles", "latest_comments", "tag_cloud"}
	for i, kind := range wantKinds {
		if list[i].Kind != kind {
			t.Fatalf("第 %d 张卡片应为 %s，实际 %s（body=%s）", i, kind, list[i].Kind, body)
		}
	}
	if list[0].Title != "本周热读" || list[1].Title != "大家怎么说" || list[2].Title != "常用标签" {
		t.Fatalf("卡片标题应原样返回: %+v", list)
	}
	if len(list[0].HotArticles) != 1 || list[0].HotArticles[0].DiggCount != 12 {
		t.Fatalf("热门文章卡应带点赞数据: %+v", list[0].HotArticles)
	}
	if len(list[1].LatestComments) != 1 || list[1].LatestComments[0].ArticleTitle != "文章甲" {
		t.Fatalf("最新评论卡应带评论数据: %+v", list[1].LatestComments)
	}
	if len(list[2].Tags) != 1 || list[2].Tags[0].ArticleCount != 5 {
		t.Fatalf("标签云卡应带标签数据: %+v", list[2].Tags)
	}
}

// 管理侧接口：整表保存后，读者侧刷新即可看到新顺序与默认标题。
func TestSidebarCardsAdminSaveAffectsReader(t *testing.T) {
	h, _, data := newSidebarHandler(t)
	data.PutTags(domain.SidebarTag{ID: ctype.NewID(3), Title: "Go", ArticleCount: 5})

	// 模拟 BindJson 中间件：handler 从 requestJson 取已绑定的请求体。
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("PUT", "/site/admin/sidebar_cards", strings.NewReader(`{}`))
	c.Set("requestJson", sitehttp.SidebarCardsSaveRequest{
		List: []sitehttp.SidebarCardSaveItem{
			{Kind: "tag_cloud", Title: "   ", Enabled: true},
			{Kind: "hot_articles", Title: "本周热读", Enabled: true},
			{Kind: "latest_comments", Title: "大家怎么说", Enabled: false},
		},
	})
	h.UpdateSidebarCards(c)

	if !strings.Contains(recorder.Body.String(), "右栏卡片已保存") {
		t.Fatalf("保存应成功: %s", recorder.Body.String())
	}

	adminBody := readSidebarBody(t, h, h.AdminSidebarCards)
	if !strings.Contains(adminBody, "本周热读") || !strings.Contains(adminBody, "大家怎么说") {
		t.Fatalf("管理端应返回全部卡片（含停用）: %s", adminBody)
	}

	readerBody := readSidebarBody(t, h, h.SidebarCards)
	if strings.Contains(readerBody, "大家怎么说") {
		t.Fatalf("停用卡不该出现在读者侧: %s", readerBody)
	}
	// 提交顺序决定展示顺序：标签云排在第 1 位，热门文章第 2 位。
	// 空标题的标签云在保存时已回退成默认标题。
	tagTitle := domain.DefaultSidebarCardTitle(domain.CardKindTagCloud)
	tagIndex := strings.Index(readerBody, tagTitle)
	hotIndex := strings.Index(readerBody, "本周热读")
	if tagIndex < 0 || hotIndex < 0 {
		t.Fatalf("读者侧应渲染两张有数据的卡（标签云、热门文章）: %s", readerBody)
	}
	if tagIndex > hotIndex {
		t.Fatalf("提交顺序即展示顺序（标签云应在热门文章之前）: %s", readerBody)
	}
}

// 未知类型在落库前拒绝，响应给出可读的错误码文案。
func TestSidebarCardsAdminSaveRejectsUnknownKind(t *testing.T) {
	h, cards, _ := newSidebarHandler(t)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("PUT", "/site/admin/sidebar_cards", strings.NewReader(`{}`))
	c.Set("requestJson", sitehttp.SidebarCardsSaveRequest{
		List: []sitehttp.SidebarCardSaveItem{{Kind: "ai_card", Title: "AI", Enabled: true}},
	})
	h.UpdateSidebarCards(c)

	if !strings.Contains(recorder.Body.String(), "右栏卡片类型不存在") {
		t.Fatalf("未知类型应被拒绝: %s", recorder.Body.String())
	}
	if len(cards.All()) != 0 {
		t.Fatalf("被拒绝的提交不应落库: %+v", cards.All())
	}
}
