package http_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"myblogx/conf"
	mw "myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	sitehttp "myblogx/internal/site/adapter/http"
	sitetest "myblogx/internal/site/apptest"

	"github.com/gin-gonic/gin"
)

// ── 取景参数（焦点 + 缩放）在 HTTP 层的样子 ──────────────────────
//
// 这里装的是**真路由**（含绑定中间件）而不是手搓上下文：越界拒绝发生在前端
// 看得见的那一层，只测 handler 里转个结构体是测不出来的。

// newBannerEngine 装一台只含轮播图路由的引擎。
func newBannerEngine(env *sitetest.Env) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := sitehttp.NewHandler(env.Service, nil)

	g := r.Group("/api/banners")
	g.GET("", mw.BindQuery[sitehttp.BannerListRequest], h.BannerList)
	g.POST("", mw.BindJson[sitehttp.BannerCreateRequest], h.BannerCreate)
	g.PUT(":id", mw.BindUri[sitehttp.IDRequest], mw.BindJson[sitehttp.BannerCreateRequest], h.BannerUpdate)
	return r
}

// siteEnv 构造一套可用的站点替身，并登记一张封面图。
func siteEnv() *sitetest.Env {
	env := sitetest.NewEnv(conf.Site{}, conf.AI{}, "test", "")
	env.Images.Put(11, "/a.png")
	return env
}

// apiResponse 是统一响应壳里用例关心的三个字段。
type apiResponse struct {
	Code res.Code        `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

// doJSON 发一个 JSON 请求并解出统一响应壳。
func doJSON(t *testing.T, r *gin.Engine, method, path, body string) apiResponse {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s %s 的响应不是合法 JSON: %v（body=%s）", method, path, err, w.Body.String())
	}
	return out
}

// decodeData 把 data 解成目标结构。
func decodeData[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("data 解析失败: %v（raw=%s）", err, raw)
	}
	return out
}

// cropPayload 是契约里的取景参数形状（与生成的前端类型一一对应）。
type cropPayload struct {
	FocusX float64 `json:"focus_x"`
	FocusY float64 `json:"focus_y"`
	Zoom   float64 `json:"zoom"`
}

// TestBannerCreateEchoesCrop 钉住"创建响应回传取景参数"。
func TestBannerCreateEchoesCrop(t *testing.T) {
	env := siteEnv()
	r := newBannerEngine(env)

	got := doJSON(t, r, "POST", "/api/banners",
		`{"cover_image_id":"11","href":"/a","show":true,"crop":{"focus_x":0.2,"focus_y":0.8,"zoom":1.5}}`)
	if got.Code != res.SuccessCode {
		t.Fatalf("创建应成功，得到 code=%d msg=%s", got.Code, got.Msg)
	}
	created := decodeData[struct {
		Crop cropPayload `json:"crop"`
	}](t, got.Data)
	want := cropPayload{FocusX: 0.2, FocusY: 0.8, Zoom: 1.5}
	if created.Crop != want {
		t.Fatalf("创建响应应回传取景 %+v，得到 %+v", want, created.Crop)
	}

	// 列表是阅读站真正消费的那条路，它必须也带上取景。
	list := doJSON(t, r, "GET", "/api/banners?show=true", "")
	if list.Code != res.SuccessCode {
		t.Fatalf("列表应成功，得到 code=%d msg=%s", list.Code, list.Msg)
	}
	page := decodeData[struct {
		List []struct {
			Crop cropPayload `json:"crop"`
		} `json:"list"`
	}](t, list.Data)
	if len(page.List) != 1 {
		t.Fatalf("列表应有 1 条，得到 %d", len(page.List))
	}
	if page.List[0].Crop != want {
		t.Fatalf("列表应回传取景 %+v，得到 %+v", want, page.List[0].Crop)
	}
}

// TestBannerCreateWithoutCropDefaultsToCenter 钉住老客户端的兼容面：
// 不传 crop 也合法，回传的是"居中、缩放 1"。
func TestBannerCreateWithoutCropDefaultsToCenter(t *testing.T) {
	env := siteEnv()
	r := newBannerEngine(env)

	got := doJSON(t, r, "POST", "/api/banners", `{"cover_image_id":"11","href":"/a","show":true}`)
	if got.Code != res.SuccessCode {
		t.Fatalf("不传取景应成功，得到 code=%d msg=%s", got.Code, got.Msg)
	}
	created := decodeData[struct {
		Crop cropPayload `json:"crop"`
	}](t, got.Data)
	want := cropPayload{FocusX: 0.5, FocusY: 0.5, Zoom: 1}
	if created.Crop != want {
		t.Fatalf("缺省取景应为 %+v，得到 %+v", want, created.Crop)
	}

	list := doJSON(t, r, "GET", "/api/banners?show=true", "")
	page := decodeData[struct {
		List []struct {
			Crop cropPayload `json:"crop"`
		} `json:"list"`
	}](t, list.Data)
	if len(page.List) != 1 || page.List[0].Crop != want {
		t.Fatalf("列表也应回传缺省取景 %+v，得到 %+v", want, page.List)
	}
}

// TestBannerRejectsOutOfRangeCropOverHTTP 钉住越界在 HTTP 层被拒。
//
// 判据是**请求被拒且什么都没落库**，而不是"能返回一个错误提示"：
// 静默钳制与"先写后报错"都会让运营以为取景生效了。
func TestBannerRejectsOutOfRangeCropOverHTTP(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"焦点 x 越界", `{"cover_image_id":"11","href":"/a","show":true,"crop":{"focus_x":1.2,"focus_y":0.5,"zoom":1}}`},
		{"焦点 y 越界", `{"cover_image_id":"11","href":"/a","show":true,"crop":{"focus_x":0.5,"focus_y":-0.2,"zoom":1}}`},
		{"缩放越界", `{"cover_image_id":"11","href":"/a","show":true,"crop":{"focus_x":0.5,"focus_y":0.5,"zoom":3}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := siteEnv()
			r := newBannerEngine(env)

			got := doJSON(t, r, "POST", "/api/banners", c.body)
			// 全站统一响应壳：HTTP 永远 200，业务码 1001 表示"请求被拒绝"。
			// 这部分等价于接口层的 400 —— 前端只看得到这条业务码。
			if got.Code != res.FailValidCode {
				t.Fatalf("越界取景应被拒绝（业务码 %d），得到 code=%d msg=%s", res.FailValidCode, got.Code, got.Msg)
			}
			if got.Msg == "" {
				t.Fatal("拒绝时应给出可读原因")
			}
			if rows := env.Banners.All(); len(rows) != 0 {
				t.Fatalf("越界时不该落库，实际建出 %d 条", len(rows))
			}
		})
	}
}

// TestBannerUpdateCropOverHTTP 钉住 PUT 这条路：重新取景立即生效。
func TestBannerUpdateCropOverHTTP(t *testing.T) {
	env := siteEnv()
	r := newBannerEngine(env)

	created := doJSON(t, r, "POST", "/api/banners", `{"cover_image_id":"11","href":"/a","show":true}`)
	payload := decodeData[struct {
		ID string `json:"id"`
	}](t, created.Data)

	updated := doJSON(t, r, "PUT", "/api/banners/"+payload.ID,
		`{"cover_image_id":"11","href":"/a","show":true,"crop":{"focus_x":0.7,"focus_y":0.25,"zoom":1.75}}`)
	if updated.Code != res.SuccessCode {
		t.Fatalf("更新应成功，得到 code=%d msg=%s", updated.Code, updated.Msg)
	}

	list := doJSON(t, r, "GET", "/api/banners?show=true", "")
	page := decodeData[struct {
		List []struct {
			Crop cropPayload `json:"crop"`
		} `json:"list"`
	}](t, list.Data)
	want := cropPayload{FocusX: 0.7, FocusY: 0.25, Zoom: 1.75}
	if len(page.List) != 1 || page.List[0].Crop != want {
		t.Fatalf("更新后列表应回传取景 %+v，得到 %+v", want, page.List)
	}

	// 更新时越界同样拒绝，且不该把已有取景改坏。
	rejected := doJSON(t, r, "PUT", "/api/banners/"+payload.ID,
		`{"cover_image_id":"11","href":"/a","show":true,"crop":{"focus_x":0.7,"focus_y":0.25,"zoom":9}}`)
	if rejected.Code != res.FailValidCode {
		t.Fatalf("更新时越界应被拒绝，得到 code=%d msg=%s", rejected.Code, rejected.Msg)
	}
	list = doJSON(t, r, "GET", "/api/banners?show=true", "")
	page = decodeData[struct {
		List []struct {
			Crop cropPayload `json:"crop"`
		} `json:"list"`
	}](t, list.Data)
	if page.List[0].Crop != want {
		t.Fatalf("被拒绝的更新不该改动取景，期望 %+v，得到 %+v", want, page.List[0].Crop)
	}
}
