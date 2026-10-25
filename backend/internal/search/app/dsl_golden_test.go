package app

import (
	"encoding/json"
	"os"
	"testing"

	"myblogx/internal/search/domain"
	"myblogx/internal/testkit"
	"myblogx/models/ctype"
)

// dslGoldenDir 是请求体 golden 的目录（相对本包）。
const dslGoldenDir = "testdata/dsl"

// TestSearchDSLGolden 锁定"发给 ES 的请求体"的形状。
//
// # 为什么这条测试重要
//
// 搜索模块的错误几乎都不会报错：DSL 里少一个 filter，就是"搜出来的东西不对"；
// _source 少一个字段，就是"列表里某个字段永远是空"；highlight 配错，
// 就是"高亮没了但功能正常"。这些只有人肉点页面才可能发现。
//
// 而且 buildDSL 有 5 种 type × 6 种 sort × 标签/分类/关键词的组合，
// 改一处（比如给私有搜索加一个过滤）很容易把公共搜索也改坏 ——
// 逐字节比对是唯一能挡住这类回归的手段。
//
// 重新生成：BLOGX_UPDATE_GOLDEN=1 go test ./internal/search/app/ -run TestSearchDSLGolden
func TestSearchDSLGolden(t *testing.T) {
	t.Parallel()

	update := os.Getenv(testkit.UpdateGoldenEnv) == "1"
	user := &domain.Viewer{ID: 100}
	admin := &domain.Viewer{ID: 900, Admin: true}

	cases := []struct {
		name    string
		req     domain.Request
		viewer  *domain.Viewer
		likeTag []ctype.ID
	}{
		{
			name: "type1-公共搜索-默认排序",
			req:  domain.Request{Type: 1, Sort: 1, Key: "", PageMode: domain.PageModeCount},
		},
		{
			name: "type1-公共搜索-关键词+高亮",
			req:  domain.Request{Type: 1, Sort: 1, Key: "golang 并发", PageMode: domain.PageModeHasMore, PageInfo: domain.PageInfo{Page: 2, Limit: 20}},
		},
		{
			name: "type1-按最新排序",
			req:  domain.Request{Type: 1, Sort: 2, Key: "es", PageMode: domain.PageModeCount},
		},
		{
			name: "type1-按评论数排序",
			req:  domain.Request{Type: 1, Sort: 3, PageMode: domain.PageModeCount},
		},
		{
			name: "type1-标签过滤（新格式 tag_ids）",
			req:  domain.Request{Type: 1, Sort: 1, PageMode: domain.PageModeCount, TagIDs: []ctype.ID{1, 2}},
		},
		{
			name: "type1-标签过滤（旧格式 tag_list）",
			req:  domain.Request{Type: 1, Sort: 1, PageMode: domain.PageModeCount, LegacyTagList: []string{"Go", "MySQL"}},
		},
		{
			name: "type1-分类过滤+分页",
			req:  domain.Request{Type: 1, Sort: 1, PageMode: domain.PageModeCount, CategoryID: 7, PageInfo: domain.PageInfo{Page: 3, Limit: 15}},
		},
		{
			name:    "type2-猜你喜欢-带偏好标签",
			req:     domain.Request{Type: 2, Sort: 1, PageMode: domain.PageModeCount},
			viewer:  user,
			likeTag: []ctype.ID{3, 5},
		},
		{
			name:   "type2-猜你喜欢-无偏好标签（不能出现 tags filter）",
			req:    domain.Request{Type: 2, Sort: 1, PageMode: domain.PageModeCount},
			viewer: user,
		},
		{
			name:   "type3-作者文章",
			req:    domain.Request{Type: 3, Sort: 2, PageMode: domain.PageModeCount, AuthorID: 100},
			viewer: user,
		},
		{
			name:   "type4-自己的文章（含未发布）",
			req:    domain.Request{Type: 4, Sort: 6, PageMode: domain.PageModeCount, Status: domain.StatusExamining},
			viewer: user,
		},
		{
			name:   "type5-管理员后台（含全部状态）",
			req:    domain.Request{Type: 5, Sort: 1, PageMode: domain.PageModeCount},
			viewer: admin,
		},
		{
			name:   "type5-管理员按草稿筛",
			req:    domain.Request{Type: 5, Sort: 4, PageMode: domain.PageModeCount, Status: domain.StatusDraft},
			viewer: admin,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			req, err := normalize(c.req, c.viewer)
			if err != nil {
				t.Fatalf("请求应通过归一化: %v", err)
			}
			query, err := buildDSL(req, c.viewer, c.likeTag)
			if err != nil {
				t.Fatalf("编译 DSL 失败: %v", err)
			}
			body := buildBody(req, query, extraBodyBySort(req.Sort, req.Key))

			// json.Marshal 对 map 的键排序输出，所以同一份 DSL 的字节是稳定的。
			// 用缩进格式：golden 是给人看的，diff 时一眼能看出改了哪个字段。
			got, err := json.MarshalIndent(body, "", "  ")
			if err != nil {
				t.Fatalf("序列化请求体失败: %v", err)
			}
			got = append(got, '\n')

			testkit.CompareGolden(t, dslGoldenDir, c.name+".json", got, update)
		})
	}
}

// TestPageModeFallsBackNotRejects 锁定"非法分页模式会回落，不报错"这条语义。
//
// 单独写出来是因为它反直觉：normalize 里确实有一句 PageMode 校验，
// 但前面 req.Normalize() 已经把非法值收敛成默认值了，那句校验永远不触发（死代码）。
// 也就是说非法分页模式**不会**被拒绝，而是按类型给默认：
// 自己/管理端默认 count，其余默认 has_more。
//
// 这条语义有实际影响：has_more 模式只多取一条、不算总数，返回的 total 是 0。
// 若哪天有人把回落改成拒绝，前端的"加载更多"会整片报错。
func TestPageModeFallsBackNotRejects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		req  domain.Request
		want domain.PageMode
	}{
		{name: "非法值+公共搜索 → has_more", req: domain.Request{Type: 1, PageMode: "whatever"}, want: domain.PageModeHasMore},
		{name: "非法值+自己的文章 → count", req: domain.Request{Type: 4, PageMode: "whatever"}, want: domain.PageModeCount},
		{name: "非法值+管理端 → count", req: domain.Request{Type: 5, PageMode: "whatever"}, want: domain.PageModeCount},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalize(c.req, &domain.Viewer{ID: 100, Admin: true})
			if err != nil {
				t.Fatalf("非法分页模式不该被拒绝: %v", err)
			}
			if got.PageMode != c.want {
				t.Fatalf("分页模式回落: got=%q want=%q", got.PageMode, c.want)
			}
		})
	}
}

// TestSearchDSLRejectsInvalid 确认非法参数在编译 DSL 之前就被挡住。
//
// 这条不是 golden 能覆盖的：非法参数不会产出请求体，
// 若校验被删掉，用户会看到一个"查不出东西"的空列表而不是明确报错。
func TestSearchDSLRejectsInvalid(t *testing.T) {
	t.Parallel()

	user := &domain.Viewer{ID: 100}
	cases := []struct {
		name    string
		req     domain.Request
		viewer  *domain.Viewer
		wantErr string
	}{
		{name: "type 越界", req: domain.Request{Type: 6, Sort: 1, PageMode: domain.PageModeCount}, wantErr: "类型"},
		{name: "sort 越界", req: domain.Request{Type: 1, Sort: 7, PageMode: domain.PageModeCount}, wantErr: "排序"},
		{name: "作者文章缺作者", req: domain.Request{Type: 3, Sort: 1, PageMode: domain.PageModeCount}, wantErr: "作者"},
		{name: "公共搜索不许查非发布态", req: domain.Request{Type: 1, Sort: 1, PageMode: domain.PageModeCount, Status: domain.StatusDraft}, wantErr: "状态"},
		{name: "自己的文章要登录", req: domain.Request{Type: 4, Sort: 1, PageMode: domain.PageModeCount}, wantErr: "登录"},
		{name: "管理端要管理员", req: domain.Request{Type: 5, Sort: 1, PageMode: domain.PageModeCount}, viewer: user, wantErr: "权限"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := normalize(c.req, c.viewer); err == nil {
				t.Fatalf("应当被拒绝（含 %q），实际通过", c.wantErr)
			}
		})
	}
}
