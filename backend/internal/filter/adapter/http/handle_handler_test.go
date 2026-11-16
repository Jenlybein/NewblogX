package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	filterhttp "myblogx/internal/filter/adapter/http"
	"myblogx/internal/apperr"
	"myblogx/internal/filter/app"
	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/utils/jwts"
)

// 处置端点的 HTTP 层（票 398）：只钉两件路由层的事 ——
//   - 没有登录主体时拒绝（AdminMiddleware 漏挂也不该被当成匿名管理员）；
//   - 请求级非法（动作写错 / 一条没选）在动手之前整体拒绝。

// handleRepo 是"查得到一条待处理记录、能改状态"的仓储替身。
type handleRepo struct {
	fakeRepo
	row     domain.Hit
	found   bool
	updated bool
}

func (r *handleRepo) FindByID(_ context.Context, id ctype.ID) (domain.Hit, error) {
	if !r.found || r.row.ID != id {
		return domain.Hit{}, apperr.New(apperr.CodeFilterHitNotFound)
	}
	return r.row, nil
}

func (r *handleRepo) MarkHandled(_ context.Context, _ ctype.ID, h domain.Handle) (bool, error) {
	if !r.updated {
		return false, nil
	}
	r.row.Status = h.Status
	return true, nil
}

// handleDisposer 只记录"点了删除有没有真的删"。
type handleDisposer struct{ deleted []ctype.ID }

func (d *handleDisposer) DeleteArticle(_ context.Context, _, id ctype.ID) error {
	d.deleted = append(d.deleted, id)
	return nil
}
func (d *handleDisposer) DeleteComment(_ context.Context, _, _ ctype.ID) error   { return nil }
func (d *handleDisposer) DeleteCollection(_ context.Context, _, _ ctype.ID) error { return nil }
func (d *handleDisposer) ResetProfile(_ context.Context, _, _ ctype.ID, _ string) error {
	return nil
}

// newHandleHandler 构造一个带处置端口的处理器。
func newHandleHandler(t *testing.T) (*filterhttp.Handler, *handleRepo) {
	t.Helper()
	repo := &handleRepo{
		fakeRepo: fakeRepo{},
		row: domain.Hit{
			ID:        ctype.NewID(11),
			Surface:   domain.SurfaceArticle,
			ContentID: ctype.NewID(9001),
			UserID:    ctype.NewID(7),
			Status:    domain.StatusPending,
		},
		found:   true,
		updated: true,
	}
	svc := app.New(nil, repo, nil).WithDisposer(&handleDisposer{})
	return filterhttp.NewHandler(svc), repo
}

// post 发一次 POST 并解析成统一外壳；adminID 为零表示不带登录主体。
//
// single 决定挂哪个端点：单条处置的路径带 `:id` 参数段，批量处置是静态段。
// 两个端点各自要按自己的路由形状注册，否则 gin 会把请求交给另一条。
func post(t *testing.T, h *filterhttp.Handler, single bool, route string, payload any, adminID uint64) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	pattern := "/content-filter/hits/handle-batch"
	if single {
		pattern = "/content-filter/hits/:id/handle"
	}
	router.POST(pattern, func(c *gin.Context) {
		if adminID != 0 {
			claims := &jwts.MyClaims{}
			claims.UserID = ctype.NewID(adminID)
			c.Set("claims", claims)
		}
		c.Next()
	}, func(c *gin.Context) {
		if single {
			h.HandleHit(c)
			return
		}
		h.HandleBatchHits(c)
	})

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, route, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应不是 JSON: %v (%s)", err, rec.Body.String())
	}
	return got
}

// TestHandleHitRequiresAdminPrincipal 没有登录主体时拒绝。
func TestHandleHitRequiresAdminPrincipal(t *testing.T) {
	t.Parallel()

	h, _ := newHandleHandler(t)
	body := post(t, h, true, "/content-filter/hits/11/handle",
		map[string]any{"action": "ignore"}, 0)
	if code := body["code"].(float64); code == 0 {
		t.Fatalf("无主体时不该成功: %v", body)
	}
}

// TestHandleHitRejectsUnknownAction 动作写错时按参数错误拒绝。
func TestHandleHitRejectsUnknownAction(t *testing.T) {
	t.Parallel()

	h, _ := newHandleHandler(t)
	body := post(t, h, true, "/content-filter/hits/11/handle",
		map[string]any{"action": "ban"}, 1)
	if code := body["code"].(float64); code == 0 {
		t.Fatalf("未知动作应被拒: %v", body)
	}
}

// TestHandleHitSucceeds 正常处置返回处置后的记录。
func TestHandleHitSucceeds(t *testing.T) {
	t.Parallel()

	h, _ := newHandleHandler(t)
	body := post(t, h, true, "/content-filter/hits/11/handle",
		map[string]any{"action": "ignore", "note": "误报"}, 1)
	if code := body["code"].(float64); code != 0 {
		t.Fatalf("处置应成功: %v", body)
	}
}

// TestHandleBatchRejectsEmptySelection 一条没选时整体拒绝。
func TestHandleBatchRejectsEmptySelection(t *testing.T) {
	t.Parallel()

	h, _ := newHandleHandler(t)
	body := post(t, h, false, "/content-filter/hits/handle-batch",
		map[string]any{"ids": []string{}, "action": "ignore"}, 1)
	if code := body["code"].(float64); code == 0 {
		t.Fatalf("空选择应被拒: %v", body)
	}
}

// TestHandleBatchReturnsPerItemResults 批量处置返回逐条结果，HTTP 恒 200。
func TestHandleBatchReturnsPerItemResults(t *testing.T) {
	t.Parallel()

	h, _ := newHandleHandler(t)
	body := post(t, h, false, "/content-filter/hits/handle-batch",
		map[string]any{"ids": []string{"11"}, "action": "ignore"}, 1)
	if code := body["code"].(float64); code != 0 {
		t.Fatalf("批量处置本身应成功: %v", body)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("响应缺少 data: %v", body)
	}
	if results, ok := data["results"].([]any); !ok || len(results) != 1 {
		t.Fatalf("应返回逐条结果: %v", data)
	}
}


