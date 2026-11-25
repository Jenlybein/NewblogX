package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	filterhttp "myblogx/internal/filter/adapter/http"
	"myblogx/internal/filter/app"
	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/contentfilter"
)

// 这一层钉的是"请求进去、契约形状出来"：筛选参数真的传到用例层、非法取值当场
// 报错（而不是静默退化成不筛选）、响应字段与前端从契约派生的形状一致。

// fakeRepo 是命中记录的仓储替身。
type fakeRepo struct {
	rows      []domain.Hit
	total     int64
	top       []domain.TopUser
	gotQuery  domain.Query
	gotSince  time.Time
	gotLimit  int
	createErr error
}

func (r *fakeRepo) Create(context.Context, *domain.Hit) error { return r.createErr }

func (r *fakeRepo) List(_ context.Context, q domain.Query) ([]domain.Hit, int64, error) {
	r.gotQuery = q
	return r.rows, r.total, nil
}

func (r *fakeRepo) TopUsers(_ context.Context, since time.Time, limit int) ([]domain.TopUser, error) {
	r.gotSince = since
	r.gotLimit = limit
	return r.top, nil
}

// newTestHandler 构造一个带一行命中的处理器。
func newTestHandler(t *testing.T) (*filterhttp.Handler, *fakeRepo) {
	t.Helper()
	engine, err := contentfilter.NewEngine(
		[]contentfilter.Word{{Word: "冰毒", Tier: contentfilter.TierBlock, Category: contentfilter.CategoryDrugs}},
		nil,
	)
	if err != nil {
		t.Fatalf("构建引擎失败: %v", err)
	}
	repo := &fakeRepo{
		rows: []domain.Hit{{
			ID:           ctype.NewID(11),
			CreatedAt:    time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC),
			Surface:      domain.SurfaceComment,
			ContentID:    ctype.NewID(9001),
			UserID:       ctype.NewID(7),
			UserNickname: "路人",
			Status:       domain.StatusPending,
			HitCount:     1,
			Words: []domain.Word{{
				Word:     "冰毒",
				Tier:     contentfilter.TierBlock,
				Category: contentfilter.CategoryDrugs,
				Snippets: []domain.Snippet{{Start: 2, End: 4, Text: "这里有冰毒"}},
			}},
		}},
		total: 3,
		top:   []domain.TopUser{{UserID: ctype.NewID(7), Nickname: "路人", HitCount: 9}},
	}
	return filterhttp.NewHandler(app.New(engine, repo, nil)), repo
}

// call 发一次 GET 并把响应解析成统一外壳。
func call(t *testing.T, h *filterhttp.Handler, target string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/content-filter/hits", h.ListHits)

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP 状态应为 200（业务码在包体里），实际 %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON: %v (%s)", err, rec.Body.String())
	}
	return body
}

func TestListHitsReturnsPageAndTopUsers(t *testing.T) {
	h, repo := newTestHandler(t)

	body := call(t, h, "/content-filter/hits?page=2&limit=5&surface=comment&status=pending&user_id=7")
	if code := body["code"].(float64); code != 0 {
		t.Fatalf("业务码应为 0，实际 %v（msg=%v）", code, body["msg"])
	}
	if repo.gotQuery.Surface != domain.SurfaceComment || repo.gotQuery.Status != domain.StatusPending {
		t.Fatalf("筛选参数没传到用例层: %+v", repo.gotQuery)
	}
	if repo.gotQuery.UserID != ctype.NewID(7) {
		t.Fatalf("用户筛选没传下去: %+v", repo.gotQuery)
	}
	if repo.gotQuery.Page.Page != 2 || repo.gotQuery.Page.Limit != 5 {
		t.Fatalf("分页参数没传下去: %+v", repo.gotQuery.Page)
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("响应缺少 data: %v", body)
	}
	if count := data["count"].(float64); count != 3 {
		t.Fatalf("count 应是满足条件的总条数 3，实际 %v", count)
	}
	list, ok := data["list"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("list 形状不对: %v", data["list"])
	}
	item := list[0].(map[string]any)
	if item["surface"] != "comment" || item["status"] != "pending" {
		t.Fatalf("行字段不对: %v", item)
	}
	if item["user_nickname"] != "路人" {
		t.Fatalf("昵称没带出来: %v", item["user_nickname"])
	}
	words := item["words"].([]any)
	word := words[0].(map[string]any)
	if word["word"] != "冰毒" || word["tier"] != "block" || word["category"] != "drugs" {
		t.Fatalf("命中明细不对: %v", word)
	}
	snippet := word["snippets"].([]any)[0].(map[string]any)
	if snippet["start"].(float64) != 2 || snippet["end"].(float64) != 4 || snippet["text"] != "这里有冰毒" {
		t.Fatalf("上下文片段不对（高亮下标必须原样给前端）: %v", snippet)
	}

	top := data["top_users"].([]any)
	if len(top) != 1 {
		t.Fatalf("top_users 应为 1 条，实际 %d", len(top))
	}
	if topUser := top[0].(map[string]any); topUser["hit_count"].(float64) != 9 {
		t.Fatalf("Top 汇总数不对: %v", topUser)
	}
}

func TestListHitsRejectsUnknownSurface(t *testing.T) {
	h, _ := newTestHandler(t)

	body := call(t, h, "/content-filter/hits?surface=bogus")
	if code := body["code"].(float64); code != 1001 {
		t.Fatalf("非法文本面应是校验失败 1001，实际 %v（msg=%v）", code, body["msg"])
	}
	if msg, _ := body["msg"].(string); msg == "" {
		t.Fatal("非法文本面必须给出可读的中文提示，而不是静默不筛选")
	}
}

func TestListHitsRejectsUnknownStatus(t *testing.T) {
	h, _ := newTestHandler(t)

	body := call(t, h, "/content-filter/hits?status=bogus")
	if code := body["code"].(float64); code != 1001 {
		t.Fatalf("非法状态应是校验失败 1001，实际 %v（msg=%v）", code, body["msg"])
	}
}

// TestListHitsWithoutFiltersReturnsEverything：三个筛选项留空 = 不筛。
func TestListHitsWithoutFiltersReturnsEverything(t *testing.T) {
	h, repo := newTestHandler(t)

	body := call(t, h, "/content-filter/hits")
	if code := body["code"].(float64); code != 0 {
		t.Fatalf("业务码应为 0，实际 %v", code)
	}
	if repo.gotQuery.Surface != "" || repo.gotQuery.Status != "" || !repo.gotQuery.UserID.IsZero() {
		t.Fatalf("没传筛选项时不该拼条件: %+v", repo.gotQuery)
	}
}

// FindByID / MarkHandled 是处置能力（票 398）加进仓储端口的两个方法。
// 本文件的用例只关心"过滤与记录"，所以这里给出"查不到 / 不更新"的最简实现；
// 处置行为由 handle_test.go 的 handleRepo 覆盖。
func (r *fakeRepo) FindByID(_ context.Context, _ ctype.ID) (domain.Hit, error) {
	return domain.Hit{}, apperr.New(apperr.CodeFilterHitNotFound)
}

func (r *fakeRepo) MarkHandled(_ context.Context, _ ctype.ID, _ domain.Handle) (bool, error) {
	return false, nil
}

// PurgeBefore 是保留期清理（ADR-0032 决策 10）加进仓储端口的方法。
// 本文件关心的是读面 / 处置，清理行为由 cron 侧用例覆盖，这里给"什么都不删"的最简实现。
func (r *fakeRepo) PurgeBefore(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// WordStats 是词云统计（ADR-0032 决策 9）加进仓储端口的方法。
// 本文件不验证词云口径，给"空集"最简实现；口径由 adapter 与装配层用例覆盖。
func (r *fakeRepo) WordStats(_ context.Context, _ time.Time) ([]domain.WordStat, error) {
	return nil, nil
}
