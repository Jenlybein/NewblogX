package captcha_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"myblogx/conf"
	"myblogx/internal/platform/captcha"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
)

// fakeSettings 是配置替身：总开关 + 第十二段的图形/时效参数。
//
// 两个方法都必须实现：生成侧要同时回答"开不开"和"画成什么样"。只实现
// CaptchaEnabled 的话 Handler 根本构造不出来（编译期红），而这只在别人改接口时
// 才会暴露——所以这里有 CaptchaConfig，并且下面每条用例都从配置出发。
type fakeSettings struct {
	enabled bool
	cfg     conf.Captcha
}

func (f fakeSettings) CaptchaEnabled() bool { return f.enabled }

func (f fakeSettings) CaptchaConfig() conf.Captcha { return f.cfg }

// validCaptchaConfig 给一份能真正构建驱动的配置（字符型 + 合法背景色）。
//
// 生成成功类用例必须从一份合法配置出发：否则"配置无效"会先短路掉请求，
// 用例就变成了在测配置校验，而不是在测生成与回显。
func validCaptchaConfig() conf.Captcha {
	return conf.Captcha{
		Driver:         captcha.DriverString,
		Length:         4,
		Width:          200,
		Height:         60,
		ExpireSeconds:  600,
		Source:         "23456789ABCDEFGHJKMNPQRSTUVWXYZ",
		NoiseCount:     3,
		ShowHollowLine: true,
		ShowSlimeLine:  true,
		BgColor:        "#0A14320A",
	}
}

// recordingStore 记录写入了哪些答案，用来证明"生成写进了传入的 store"。
//
// 这条断言守的是一个真实耦合：验证码的生成在这里、校验在 middleware.CaptchaGate，
// 两边必须是同一个 store 实例。如果将来有人在 Handler 内部自己 new 一个 store，
// 前端拿到的 captcha_id 在校验时就查不到 —— 而那种错误在接口层面完全看不出来
// （两边都返回 200）。
//
// answers 存的是库写进来的正确答案（Set(id, answer)），于是测试可以拿它去
// 同一个 store 上 Verify，等价于"用户照着图填对了"这条真实路径。
type recordingStore struct {
	inner   base64Captcha.Store
	sets    int
	answers map[string]string
}

func (s *recordingStore) Set(id, value string) error {
	s.sets++
	if s.answers == nil {
		s.answers = make(map[string]string, 1)
	}
	s.answers[id] = value
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

// 总开关关闭时生成接口必须拒绝，而且不能写 store。
//
// 不写 store 这一条和"返回失败"同等重要：总开关关掉后如果还继续写，Redis 里会
// 堆一批永远不会被校验的验证码，且没人会注意到。
func TestImageDisabled(t *testing.T) {
	settings := fakeSettings{enabled: false, cfg: validCaptchaConfig()}
	store := &recordingStore{inner: base64Captcha.DefaultMemStore}
	h := captcha.NewHandler(settings, store)

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
	h := captcha.NewHandler(fakeSettings{enabled: true, cfg: validCaptchaConfig()}, store)

	got, w := call(t, h)
	if got.Code != 0 {
		t.Fatalf("验证码生成应成功, body=%s", w.Body.String())
	}

	id, ok := got.Data["captcha_id"].(string)
	if !ok || id == "" {
		t.Fatalf("captcha_id 为空或类型错误, data=%v", got.Data)
	}
	b64, ok := got.Data["base64"].(string)
	if !ok || !strings.HasPrefix(b64, "data:image/png;base64,") {
		t.Fatalf("base64 应是可直接塞进 <img src> 的 PNG data URL, data=%v", got.Data)
	}
	if store.sets != 1 {
		t.Fatalf("生成应写入传入的 store 一次，实际 %d 次", store.sets)
	}
}

// expire_seconds 必须回显**当前配置值**，不是写死的常量。
//
// 前端靠这个字段决定"多久没操作就主动换一张"，运营在后台把有效期从 600 改成 900
// 之后，前端必须立刻按 900 走。如果回显写死（例如 600），运营改完配置看到接口
// 还是 600，会以为保存没生效，而实际图的有效期已经是 900——两套时钟互相打架。
func TestImageExpireSecondsEchoesConfiguredValue(t *testing.T) {
	cfg := validCaptchaConfig()
	cfg.ExpireSeconds = 900

	store := &recordingStore{inner: base64Captcha.DefaultMemStore}
	h := captcha.NewHandler(fakeSettings{enabled: true, cfg: cfg}, store)

	got, w := call(t, h)
	if got.Code != 0 {
		t.Fatalf("验证码生成应成功, body=%s", w.Body.String())
	}
	gotSeconds, ok := got.Data["expire_seconds"].(float64)
	if !ok {
		t.Fatalf("响应应带 expire_seconds 数字字段, data=%v", got.Data)
	}
	if int(gotSeconds) != 900 {
		t.Fatalf("expire_seconds 应回显配置值 900，实际 %d", int(gotSeconds))
	}
}

// 配置写坏（驱动名不认识 / 背景色不是十六进制）时必须是一条**可读的失败**，
// 而不是 500。
//
// 这两种输入都来自后台：运营把 bg_color 填成"深蓝"、或从旧备份里恢复了一份
// driver=wechat 的配置。500 会被前端当成"后端坏了"并触发重试风暴，用户也只会
// 看到通用错误；这里要求文案明确指向"配置无效"，让人去后台改配置而不是怀疑网络。
func TestImageInvalidConfigIsRejectedNot500(t *testing.T) {
	cases := map[string]func(conf.Captcha) conf.Captcha{
		"未知驱动名": func(cfg conf.Captcha) conf.Captcha {
			cfg.Driver = "wechat"
			return cfg
		},
		"背景色非法": func(cfg conf.Captcha) conf.Captcha {
			cfg.BgColor = "深蓝"
			return cfg
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			store := &recordingStore{inner: base64Captcha.DefaultMemStore}
			h := captcha.NewHandler(fakeSettings{enabled: true, cfg: mutate(validCaptchaConfig())}, store)

			got, w := call(t, h)
			// 这个仓库的成功/失败一律走 HTTP 200 + 业务码（res.Response.Json），
			// 所以"不是 500"的判据就是状态码是 200 且业务码是参数类失败 1001。
			if w.Code != 200 {
				t.Fatalf("配置错误不该是 5xx，实际 status=%d body=%s", w.Code, w.Body.String())
			}
			if got.Code != int(res.FailValidCode) {
				t.Fatalf("配置错误应返回参数类失败码 %d，实际 code=%d body=%s",
					int(res.FailValidCode), got.Code, w.Body.String())
			}
			if !strings.Contains(got.Msg, "配置无效") {
				t.Fatalf("失败文案应点明是配置无效（去后台改配置），实际: %q", got.Msg)
			}
			if store.sets != 0 {
				t.Fatalf("配置无效时不该生成图片，实际写入 store %d 次", store.sets)
			}
		})
	}
}

// 生成用哪个 store，校验就必须是哪个 store —— 拿生成的 id 与答案在**同一个**
// store 上 Verify 必须通过，且只通过一次。
//
// 这是整条验证码链路唯一无法从接口层看出来的耦合：两个实例各自 new 一个 store
// 时，生成接口照样 200、图片照样能显示，只是永远"验证码错误"。一次性的断言也
// 在这里：答对之后必须立刻作废，否则同一张图可以被反复提交（重放）。
func TestImageCaptchaIDVerifiesWithSameStore(t *testing.T) {
	store := &recordingStore{inner: base64Captcha.DefaultMemStore}
	h := captcha.NewHandler(fakeSettings{enabled: true, cfg: validCaptchaConfig()}, store)

	got, w := call(t, h)
	if got.Code != 0 {
		t.Fatalf("验证码生成应成功, body=%s", w.Body.String())
	}
	id, _ := got.Data["captcha_id"].(string)
	if id == "" {
		t.Fatal("captcha_id 为空")
	}
	answer := store.answers[id]
	if answer == "" {
		t.Fatalf("store 里没有 captcha_id=%s 的答案，生成没有写入传入的 store", id)
	}

	if !store.Verify(id, answer, true) {
		t.Fatal("用生成的 id + 库里的答案应能通过同一个 store 校验，实际失败")
	}
	// clear=true 是一次性语义：同一张图第二次提交必须失败。
	if store.Verify(id, answer, true) {
		t.Fatal("验证码校验成功后必须作废（不能重复提交同一张图）")
	}
}

// TestImageStoreIsShared 证明 Handler 用的就是外部传入的 store，
// 而不是内部新建的默认实例（生成与校验指向同一份数据的最直接证据）。
func TestImageStoreIsShared(t *testing.T) {
	store := &recordingStore{inner: base64Captcha.DefaultMemStore}
	h := captcha.NewHandler(fakeSettings{enabled: true, cfg: validCaptchaConfig()}, store)

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
