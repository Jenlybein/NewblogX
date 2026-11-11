package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 个人数据概览端点（票 354）。
//
// 这层要钉的是**跨过 HTTP 边界之后**仍然成立的语义，不是仓储细节：
//  1. 活跃度按当前登录用户取（别人再活跃也不该出现在我的热力图里）；
//  2. 载荷里的数字与 fake 里的数据一致（状态数量、四个和、最近 / Top 的顺序）；
//  3. 未登录被拒 —— 不是返回空数据（那会被前端画成"你还没有内容"）。

// fakeActivity 是活动端口的替身：按 userID 给不同的每日计数。
type fakeActivity struct {
	byUser map[domain.UserID]map[string]int
	// lastUser 记录最后一次被问到的用户，用来断言"问的是当前登录用户"。
	lastUser domain.UserID
	lastDays int
}

func (f *fakeActivity) DailyActivity(
	_ context.Context, userID domain.UserID, days int,
) (map[string]int, error) {
	f.lastUser = userID
	f.lastDays = days
	return f.byUser[userID], nil
}

func newStudioStatsTestHandler(t *testing.T) (*Handler, *apptest.Repos, *fakeActivity) {
	t.Helper()

	repos := apptest.NewRepos()
	activity := &fakeActivity{byUser: map[domain.UserID]map[string]int{}}
	svc := app.New(apptest.NewTx(repos), apptest.NewClock(apptest.DefaultTime), apptest.NewIDGen(1000), apptest.NewSubmitQuota(15), nil, nil, nil).
		WithActivity(activity)
	return NewHandler(svc, nil, nil, nil, nil), repos, activity
}

// getStudioStats 发一次请求并解出响应体。
func getStudioStats(t *testing.T, h *Handler, viewer domain.Viewer) (*httptest.ResponseRecorder, StudioStatsResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// 操作者由装配层注入的解析器给出（与生产同一条路径）：测试里用闭包直接
	// 返回目标 viewer，这样"端点按登录态取人"这件事才真的被跨过。
	h.viewer = func(*gin.Context) domain.Viewer { return viewer }

	r := gin.New()
	r.GET("/articles/studio-stats", h.StudioStats)

	req := httptest.NewRequest(http.MethodGet, "/articles/studio-stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 响应是 Envelope：业务数据在 data 里，code 非 0 表示业务失败
	// （共享响应层把可预期的失败也包在 200 里，靠 code 区分）。
	var envelope struct {
		Code int                `json:"code"`
		Data StudioStatsResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("响应不是合法 JSON: %v（body=%s）", err, w.Body.String())
	}
	return w, envelope.Data
}

// TestStudioStatsScopesActivityToViewer 是这票最关键的一条：
// 热力图只能反映**当前登录用户**的创作动作。
func TestStudioStatsScopesActivityToViewer(t *testing.T) {
	h, repos, activity := newStudioStatsTestHandler(t)

	viewer := domain.NormalUser(ctype.NewID(77))
	other := ctype.NewID(99)

	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(ctype.NewID(11)).Author(viewer.UserID).Status(domain.StatusPublished).Build(),
	)
	activity.byUser[viewer.UserID] = map[string]int{"2026-11-10": 3}
	activity.byUser[domain.UserID(other)] = map[string]int{"2026-11-10": 42}

	w, body := getStudioStats(t, h, viewer)
	if w.Code != http.StatusOK {
		t.Fatalf("应返回 200，实际 %d（body=%s）", w.Code, w.Body.String())
	}

	// 端口被问的是当前登录用户，不是别人。
	if activity.lastUser != viewer.UserID {
		t.Fatalf("活跃度应按当前登录用户取，实际问了 %s", activity.lastUser)
	}
	// 天数口径与热力图一致。
	if activity.lastDays != app.StudioStatsDays {
		t.Fatalf("天数应为 %d，实际 %d", app.StudioStatsDays, activity.lastDays)
	}

	// 载荷里只有我的数字。
	if len(body.Activity) != 1 || body.Activity[0].Count != 3 {
		t.Fatalf("热力图应只含当前用户的 3 次，实际 %+v", body.Activity)
	}
	if body.ActivityDays != app.StudioStatsDays {
		t.Fatalf("activity_days 应为 %d，实际 %d", app.StudioStatsDays, body.ActivityDays)
	}
}

// TestStudioStatsPayloadMatchesData 钉住载荷里的数字与数据一致。
func TestStudioStatsPayloadMatchesData(t *testing.T) {
	h, repos, _ := newStudioStatsTestHandler(t)

	viewer := domain.NormalUser(ctype.NewID(78))
	repos.FakeArticles().Seed(
		apptest.NewArticle().ID(ctype.NewID(21)).Author(viewer.UserID).Status(domain.StatusPublished).Counters(100, 0, 0, 0).Build(),
		apptest.NewArticle().ID(ctype.NewID(22)).Author(viewer.UserID).Status(domain.StatusPublished).Counters(50, 0, 0, 0).Build(),
		apptest.NewArticle().ID(ctype.NewID(23)).Author(viewer.UserID).Status(domain.StatusDraft).Counters(0, 0, 0, 0).Build(),
		// 别人的文章不该进我的统计。
		apptest.NewArticle().ID(ctype.NewID(24)).Author(ctype.NewID(999)).Status(domain.StatusPublished).Counters(999, 0, 0, 0).Build(),
	)

	w, body := getStudioStats(t, h, viewer)
	if w.Code != http.StatusOK {
		t.Fatalf("应返回 200，实际 %d", w.Code)
	}

	// 状态数量：两篇已发布 + 一篇草稿，不含别人的。
	counts := map[int]int{}
	for _, item := range body.StatusCounts {
		counts[item.Status] = item.Count
	}
	if counts[int(domain.StatusPublished)] != 2 {
		t.Fatalf("已发布应为 2 篇，实际 %d", counts[int(domain.StatusPublished)])
	}
	if counts[int(domain.StatusDraft)] != 1 {
		t.Fatalf("草稿应为 1 篇，实际 %d", counts[int(domain.StatusDraft)])
	}

	// 四个和只累加我的文章（100 + 50）。
	if body.SumView != 150 {
		t.Fatalf("阅读总数应为 150，实际 %d", body.SumView)
	}

	// Top 按阅读量倒序：100 在前。
	if len(body.Top) != 3 || body.Top[0].ViewCount != 100 {
		t.Fatalf("Top 应按阅读量倒序，实际 %+v", body.Top)
	}
	// 最近 5 篇：条数不超过 5。
	if len(body.Recent) > 5 {
		t.Fatalf("最近文章最多 5 条，实际 %d", len(body.Recent))
	}
}

// TestStudioStatsRequiresLogin 未登录被拒 —— 不是返回空数据。
func TestStudioStatsRequiresLogin(t *testing.T) {
	h, _, _ := newStudioStatsTestHandler(t)

	w, body := getStudioStats(t, h, domain.Anonymous())
	// 共享响应层把可预期的失败包在 200 里、用 code 区分，所以这里看 code：
	// 未登录必须是一个明确的失败码，而不是"空数据"（那会被前端画成「你还没有内容」）。
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if envelope.Code == 0 {
		t.Fatalf("未登录应返回失败码，实际 code=0（body=%s）", w.Body.String())
	}
	if len(body.Recent) != 0 || len(body.Activity) != 0 {
		t.Fatalf("未登录不该带回任何数据，实际 %+v", body)
	}
}
