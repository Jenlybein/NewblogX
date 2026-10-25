package captcha_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"myblogx/internal/platform/captcha"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
)

// fakeSettings 是开关替身。
type fakeSettings struct{ enabled bool }

func (f fakeSettings) CaptchaEnabled() bool { return f.enabled }

// recordingStore 记录 Set 次数，用来证明"生成写进了传入的 store"。
//
// 这条断言守的是一个真实耦合：验证码的生成在这里、校验在
// middleware.CaptchaMiddleware，两边必须是同一个 store 实例。
// 如果将来有人在 Handler 内部自己 new 一个 store，前端拿到的 captcha_id
// 在校验时就查不到 —— 而那种错误在接口层面完全看不出来（都返回 200）。
type recordingStore struct {
	inner base64Captcha.Store
	sets  int
}

func (s *recordingStore) Set(id, value string) error {
	s.sets++
	return s.inner.Set(id, value)
}

func (s *recordingStore) Get(id string, clear bool) string { return s.inner.Get(id, clear) }

func (s *recordingStore) Verify(id, answer string, clear bool) bool {
	return s.inner.Verify(id, answer, clear)
}

type resp struct {
	Code int            `json:"code"`
	Data map[string]any `json:"data"`
	Msg  string         `json:"msg"`
}

// call 执行一次生成请求。
func call(t *testing.T, h *captcha.Handler) (resp, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.Image(c)

	var got resp
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
	}
	return got, w
}

func TestImageDisabled(t *testing.T) {
	store := &recordingStore{inner: base64Captcha.DefaultMemStore}
	h := captcha.NewHandler(fakeSettings{enabled: false}, store)

	got, w := call(t, h)
	if got.Code == 0 {
		t.Fatalf("站点关闭验证码时应返回失败, body=%s", w.Body.String())
	}
	if store.sets != 0 {
		t.Fatalf("开关关闭时不应写入 store，实际写入 %d 次", store.sets)
	}
}

func TestImageSuccess(t *testing.T) {
	store := &recordingStore{inner: base64Captcha.DefaultMemStore}
	h := captcha.NewHandler(fakeSettings{enabled: true}, store)

	got, w := call(t, h)
	if got.Code != 0 {
		t.Fatalf("验证码生成应成功, body=%s", w.Body.String())
	}

	id, ok := got.Data["captcha_id"].(string)
	if !ok || id == "" {
		t.Fatalf("captcha_id 为空或类型错误, data=%v", got.Data)
	}
	b64, ok := got.Data["base64"].(string)
	if !ok || len(b64) < 16 {
		t.Fatalf("base64 为空或类型错误, data=%v", got.Data)
	}
	if store.sets != 1 {
		t.Fatalf("生成应写入传入的 store 一次，实际 %d 次", store.sets)
	}
}

// TestImageStoreIsShared 是最关键的一条：证明 Handler 用的就是外部传入的 store，
// 而不是内部新建的默认实例。
func TestImageStoreIsShared(t *testing.T) {
	store := &recordingStore{inner: base64Captcha.DefaultMemStore}
	h := captcha.NewHandler(fakeSettings{enabled: true}, store)

	got, _ := call(t, h)
	id, _ := got.Data["captcha_id"].(string)
	if id == "" {
		t.Fatal("captcha_id 为空")
	}

	// 直接查外部 store：能查到，说明生成与校验面向同一份数据。
	if value := store.Get(id, false); value == "" {
		t.Fatal("外部 store 中查不到生成的 captcha_id，生成与校验的 store 不是同一个")
	}
}
