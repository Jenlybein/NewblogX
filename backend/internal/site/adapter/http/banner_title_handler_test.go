package http_test

import (
	"encoding/json"
	"testing"

	"myblogx/internal/platform/http/res"
)

// ── 轮播标题在 HTTP 契约里的样子 ─────────────────────────────────
//
// 契约要同时覆盖三处：列表条目、创建请求、创建响应。只测 handler 转结构体
// 测不出"字段有没有进 JSON"，所以这里装真路由、按最终响应体断言。

// bannerTitlePayload 只取 title 一个字段：别的字段由各自的专项用例钉。
type bannerTitlePayload struct {
	Title string `json:"title"`
}

// bannersOf 解出列表里的全部条目。
func bannersOf(t *testing.T, raw json.RawMessage) []bannerTitlePayload {
	t.Helper()
	page := decodeData[struct {
		List []bannerTitlePayload `json:"list"`
	}](t, raw)
	return page.List
}

// TestBannerCreateAndListCarryTitle 钉住"创建请求带标题、创建响应回传标题、
// 列表条目也带标题"这三处同名字段。
func TestBannerCreateAndListCarryTitle(t *testing.T) {
	env := siteEnv()
	r := newBannerEngine(env)

	got := doJSON(t, r, "POST", "/api/banners",
		`{"cover_image_id":"11","title":"春季上新","href":"/a","show":true}`)
	if got.Code != res.SuccessCode {
		t.Fatalf("带标题创建应成功，得到 code=%d msg=%s", got.Code, got.Msg)
	}
	created := decodeData[bannerTitlePayload](t, got.Data)
	if created.Title != "春季上新" {
		t.Fatalf("创建响应应回传标题“春季上新”，得到 %+v", created)
	}

	list := doJSON(t, r, "GET", "/api/banners?show=true", "")
	if list.Code != res.SuccessCode {
		t.Fatalf("列表应成功，得到 code=%d msg=%s", list.Code, list.Msg)
	}
	rows := bannersOf(t, list.Data)
	if len(rows) != 1 || rows[0].Title != "春季上新" {
		t.Fatalf("列表应回传标题“春季上新”，得到 %+v", rows)
	}
}

// TestBannerCreateWithoutTitleIsAccepted 钉住"标题可空、缺省不报错"。
//
// 断言的是**字段存在且为空**，不是"解出来恰好是空串"：缺字段也会解成空串，
// 那样契约三处是否真的带上了标题就测不出来。
func TestBannerCreateWithoutTitleIsAccepted(t *testing.T) {
	env := siteEnv()
	r := newBannerEngine(env)

	got := doJSON(t, r, "POST", "/api/banners", `{"cover_image_id":"11","href":"/a","show":true}`)
	if got.Code != res.SuccessCode {
		t.Fatalf("不带标题创建应成功，得到 code=%d msg=%s", got.Code, got.Msg)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(got.Data, &fields); err != nil {
		t.Fatalf("创建响应 data 解析失败: %v", err)
	}
	rawTitle, ok := fields["title"]
	if !ok {
		t.Fatalf("创建响应缺 title 字段：%s", got.Data)
	}
	var title string
	if err := json.Unmarshal(rawTitle, &title); err != nil {
		t.Fatalf("创建响应 title 不是字符串: %v", err)
	}
	if title != "" {
		t.Fatalf("不带标题时应回传空串，得到 %q", title)
	}

	list := doJSON(t, r, "GET", "/api/banners", "")
	if list.Code != res.SuccessCode {
		t.Fatalf("列表应成功，得到 code=%d msg=%s", list.Code, list.Msg)
	}
	page := decodeData[struct {
		List []map[string]json.RawMessage `json:"list"`
	}](t, list.Data)
	if len(page.List) != 1 {
		t.Fatalf("列表应有 1 条，得到 %d", len(page.List))
	}
	if _, ok := page.List[0]["title"]; !ok {
		t.Fatalf("列表条目缺 title 字段：%s", list.Data)
	}
}

// TestBannerUpdateTitle 钉住"可编辑已有轮播的标题"：更新后列表立即反映新标题；
// 再传空串则清空（后台要能把误填的标题改掉）。
func TestBannerUpdateTitle(t *testing.T) {
	env := siteEnv()
	r := newBannerEngine(env)

	created := doJSON(t, r, "POST", "/api/banners",
		`{"cover_image_id":"11","title":"旧标题","href":"/a","show":true}`)
	if created.Code != res.SuccessCode {
		t.Fatalf("创建应成功，得到 code=%d msg=%s", created.Code, created.Msg)
	}
	id := decodeData[struct {
		ID string `json:"id"`
	}](t, created.Data).ID

	updated := doJSON(t, r, "PUT", "/api/banners/"+id,
		`{"cover_image_id":"11","title":"新标题","href":"/a","show":true}`)
	if updated.Code != res.SuccessCode {
		t.Fatalf("改标题应成功，得到 code=%d msg=%s", updated.Code, updated.Msg)
	}
	list := doJSON(t, r, "GET", "/api/banners", "")
	if rows := bannersOf(t, list.Data); len(rows) != 1 || rows[0].Title != "新标题" {
		t.Fatalf("改标题后列表应回传“新标题”，得到 %+v", rows)
	}

	cleared := doJSON(t, r, "PUT", "/api/banners/"+id,
		`{"cover_image_id":"11","title":"","href":"/a","show":true}`)
	if cleared.Code != res.SuccessCode {
		t.Fatalf("清空标题应成功，得到 code=%d msg=%s", cleared.Code, cleared.Msg)
	}
	list = doJSON(t, r, "GET", "/api/banners", "")
	if rows := bannersOf(t, list.Data); len(rows) != 1 || rows[0].Title != "" {
		t.Fatalf("清空后列表应回传空串，得到 %+v", rows)
	}
}

// TestBannerTitleStaysPlainText 钉住"标题只存纯文本"：HTML 标签与实体符号
// 原样进出，不被解析、不被剥离、不被转义成另一种写法。
func TestBannerTitleStaysPlainText(t *testing.T) {
	env := siteEnv()
	r := newBannerEngine(env)

	title := `<b>粗体</b> & "引号" 与 <script>alert(1)</script>`
	body, err := json.Marshal(map[string]any{
		"cover_image_id": "11",
		"title":          title,
		"href":           "/a",
		"show":           true,
	})
	if err != nil {
		t.Fatalf("构造请求体失败: %v", err)
	}

	got := doJSON(t, r, "POST", "/api/banners", string(body))
	if got.Code != res.SuccessCode {
		t.Fatalf("带 HTML 标签的标题应原样接受，得到 code=%d msg=%s", got.Code, got.Msg)
	}

	list := doJSON(t, r, "GET", "/api/banners", "")
	rows := bannersOf(t, list.Data)
	if len(rows) != 1 || rows[0].Title != title {
		t.Fatalf("标题应原样存回 %q，得到 %+v", title, rows)
	}
}

// TestBannerTitleWhitespaceCollapsed 钉住首尾空白收敛：
// 运营在输入框里多打几个空格不应变成"渲染出空白标题块"。
func TestBannerTitleWhitespaceCollapsed(t *testing.T) {
	env := siteEnv()
	r := newBannerEngine(env)

	got := doJSON(t, r, "POST", "/api/banners",
		`{"cover_image_id":"11","title":"  \u3000 ","href":"/a","show":true}`)
	if got.Code != res.SuccessCode {
		t.Fatalf("纯空白标题不应报错，得到 code=%d msg=%s", got.Code, got.Msg)
	}
	list := doJSON(t, r, "GET", "/api/banners", "")
	rows := bannersOf(t, list.Data)
	if len(rows) != 1 || rows[0].Title != "" {
		t.Fatalf("纯空白标题应收敛为空串，得到 %+v", rows)
	}

	trimmed := doJSON(t, r, "POST", "/api/banners",
		`{"cover_image_id":"11","title":"  带空白标题  ","href":"/b","show":true}`)
	if trimmed.Code != res.SuccessCode {
		t.Fatalf("带空白的标题应成功，得到 code=%d msg=%s", trimmed.Code, trimmed.Msg)
	}
	list = doJSON(t, r, "GET", "/api/banners", "")
	titles := map[string]int{}
	for _, row := range bannersOf(t, list.Data) {
		titles[row.Title]++
	}
	if titles["带空白标题"] != 1 {
		t.Fatalf("标题首尾空白应被去掉，列表标题分布 %+v", titles)
	}
}
