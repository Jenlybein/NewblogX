package http_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"myblogx/conf"
	mw "myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	sitehttp "myblogx/internal/site/adapter/http"
	sitetest "myblogx/internal/site/apptest"

	"github.com/gin-gonic/gin"
)

// ── 轮播图契约（取景退场后）──────────────────────────────────────
//
// 这里装的是**真路由**（含绑定中间件）而不是手搓上下文：契约形状发生在前端
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

// TestBannerResponsesHaveNoCrop 钉住取景退场（#203）之后轮播图的契约形状：
// 创建响应、列表响应与更新响应都不再出现 crop。
//
// 判据是"响应 JSON 里没有这个键"，而不是"结构体里没有这个字段"——
// 前端消费的是序列化结果，空值对象（`"crop":{...}` 或 `"crop":null`）都算泄漏。
func TestBannerResponsesHaveNoCrop(t *testing.T) {
	env := siteEnv()
	r := newBannerEngine(env)

	created := doJSON(t, r, "POST", "/api/banners", `{"cover_image_id":"11","href":"/a","show":true}`)
	if created.Code != res.SuccessCode {
		t.Fatalf("创建应成功，得到 code=%d msg=%s", created.Code, created.Msg)
	}
	assertNoCropKey(t, "创建响应", created.Data)

	payload := decodeData[struct {
		ID string `json:"id"`
	}](t, created.Data)

	list := doJSON(t, r, "GET", "/api/banners?show=true", "")
	if list.Code != res.SuccessCode {
		t.Fatalf("列表应成功，得到 code=%d msg=%s", list.Code, list.Msg)
	}
	assertNoCropKey(t, "列表响应", list.Data)

	// 老调用方（还没删 crop 请求字段的后台）带着 crop 请求仍不该被拒：
	// 取景参数已不在契约里，多余字段按未知字段处理。
	updated := doJSON(t, r, "PUT", "/api/banners/"+payload.ID,
		`{"cover_image_id":"11","href":"/a2","show":true,"crop":{"focus_x":0.7,"focus_y":0.25,"zoom":9}}`)
	if updated.Code != res.SuccessCode {
		t.Fatalf("更新应成功，得到 code=%d msg=%s", updated.Code, updated.Msg)
	}
	assertNoCropKey(t, "更新响应", updated.Data)

	list = doJSON(t, r, "GET", "/api/banners?show=true", "")
	assertNoCropKey(t, "更新后的列表响应", list.Data)
}

// assertNoCropKey 递归检查响应 JSON 里任何一层都不出现 crop 键。
func assertNoCropKey(t *testing.T, where string, raw json.RawMessage) {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("%s 不是合法 JSON: %v", where, err)
	}
	if path, found := findKey(decoded, "crop", ""); found {
		t.Fatalf("%s 仍带着取景字段：%s", where, path)
	}
}

// findKey 深度优先找键，返回它在对象链里的路径。
func findKey(value any, key, prefix string) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for k, child := range typed {
			path := prefix + "." + k
			if k == key {
				return path, true
			}
			if found, ok := findKey(child, key, path); ok {
				return found, true
			}
		}
	case []any:
		for index, child := range typed {
			if found, ok := findKey(child, key, prefix+"["+strconv.Itoa(index)+"]"); ok {
				return found, true
			}
		}
	}
	return "", false
}

// TestBannerListOptionalShowQuery 钉住 GET 查询参数的三态绑定：
// 省略 show 不是 Go 零值 false，而是明确的“不按展示状态过滤”。
func TestBannerListOptionalShowQuery(t *testing.T) {
	env := siteEnv()
	env.Images.Put(12, "/b.png")
	r := newBannerEngine(env)

	for _, body := range []string{
		`{"cover_image_id":"11","href":"/shown","show":true}`,
		`{"cover_image_id":"12","href":"/hidden","show":false}`,
	} {
		got := doJSON(t, r, "POST", "/api/banners", body)
		if got.Code != res.SuccessCode {
			t.Fatalf("预置轮播图失败: code=%d msg=%s", got.Code, got.Msg)
		}
	}

	cases := []struct {
		name       string
		path       string
		wantShown  int
		wantHidden int
	}{
		{name: "不传时不过滤", path: "/api/banners", wantShown: 1, wantHidden: 1},
		{name: "传真时只看展示中", path: "/api/banners?show=true", wantShown: 1},
		{name: "传假时只看隐藏", path: "/api/banners?show=false", wantHidden: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := doJSON(t, r, "GET", tc.path, "")
			if got.Code != res.SuccessCode {
				t.Fatalf("列表应成功: code=%d msg=%s", got.Code, got.Msg)
			}
			page := decodeData[struct {
				List []struct {
					Show bool `json:"show"`
				} `json:"list"`
			}](t, got.Data)
			var gotShown, gotHidden int
			for _, row := range page.List {
				if row.Show {
					gotShown++
				} else {
					gotHidden++
				}
			}
			if gotShown != tc.wantShown || gotHidden != tc.wantHidden {
				t.Fatalf("筛选结果错误: 展示中=%d 隐藏=%d", gotShown, gotHidden)
			}
		})
	}
}
