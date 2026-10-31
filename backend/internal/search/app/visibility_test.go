package app

import (
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/search/domain"
)

// TestOnlyAdminAuthorsFiltersPublicReadTypes 锁定「显示非管理员文章」开关关闭时的 DSL 形态。
//
// ADR-0017 决策 4：开关关闭时公开读面（type 1/2/3，含 AI 搜索复用的 type 1）
// 加 author_is_admin = true 过滤；type 4（自己的文章）与 type 5（后台）不受影响。
// 这条规则写在 DSL 里而不是 handler 里，所以这里直接对编译出的查询断言；
// 开关开着（零值 false）时必须**不出**现该过滤，否则等于悄悄隐藏内容。
func TestOnlyAdminAuthorsFiltersPublicReadTypes(t *testing.T) {
	t.Parallel()

	user := &domain.Viewer{ID: ctype.NewID(100)}
	admin := &domain.Viewer{ID: ctype.NewID(900), Admin: true}

	cases := []struct {
		name   string
		req    domain.Request
		viewer *domain.Viewer
		// want 开关关闭时公开读面是否应加过滤。
		want bool
	}{
		{
			name: "type1 公共搜索",
			req:  domain.Request{Type: 1, Sort: 1, PageMode: domain.PageModeCount},
			want: true,
		},
		{
			name:   "type2 猜你喜欢",
			req:    domain.Request{Type: 2, Sort: 1, PageMode: domain.PageModeCount},
			viewer: user,
			want:   true,
		},
		{
			name:   "type3 作者文章（作者页列表）",
			req:    domain.Request{Type: 3, Sort: 1, PageMode: domain.PageModeCount, AuthorID: ctype.NewID(100)},
			viewer: user,
			want:   true,
		},
		{
			name:   "type4 自己的文章不受影响",
			req:    domain.Request{Type: 4, Sort: 1, PageMode: domain.PageModeCount},
			viewer: user,
			want:   false,
		},
		{
			name:   "type5 管理员后台不受影响",
			req:    domain.Request{Type: 5, Sort: 1, PageMode: domain.PageModeCount},
			viewer: admin,
			want:   false,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			// 两种形态逐一对比：同一条请求，只有 OnlyAdminAuthors 不同。
			for _, onlyAdmin := range []bool{false, true} {
				req := c.req
				req.OnlyAdminAuthors = onlyAdmin

				normalized, err := normalize(req, c.viewer)
				if err != nil {
					t.Fatalf("请求应通过归一化: %v", err)
				}
				query, err := buildDSL(normalized, c.viewer, nil)
				if err != nil {
					t.Fatalf("编译 DSL 失败: %v", err)
				}

				got := hasTermFilter(query, "author_is_admin")
				want := onlyAdmin && c.want
				if got != want {
					t.Fatalf("OnlyAdminAuthors=%v 时 author_is_admin 过滤: got=%v want=%v\nDSL=%#v",
						onlyAdmin, got, want, query)
				}
			}
		})
	}
}

// TestOnlyAdminAuthorsFilterValueIsTrue 确认过滤是"只要管理员作者"，不是"只要非管理员"。
func TestOnlyAdminAuthorsFilterValueIsTrue(t *testing.T) {
	t.Parallel()

	req, err := normalize(domain.Request{Type: 1, Sort: 1, PageMode: domain.PageModeCount, OnlyAdminAuthors: true}, nil)
	if err != nil {
		t.Fatalf("请求应通过归一化: %v", err)
	}
	query, err := buildDSL(req, nil, nil)
	if err != nil {
		t.Fatalf("编译 DSL 失败: %v", err)
	}
	boolQuery, ok := extractBool(query)
	if !ok {
		t.Fatalf("DSL 结构不符合预期: %#v", query)
	}
	filters, _ := boolQuery["filter"].([]any)
	for _, raw := range filters {
		termMap, _ := raw.(map[string]any)
		terms, _ := termMap["term"].(map[string]any)
		if value, has := terms["author_is_admin"]; has {
			if value != true {
				t.Fatalf("author_is_admin 过滤值应为 true，实际 %#v", value)
			}
			return
		}
	}
	t.Fatalf("未找到 author_is_admin 过滤: %#v", query)
}

// hasTermFilter 判断 bool 查询的 filter 列表里是否有针对某字段的 term 过滤。
func hasTermFilter(query map[string]any, field string) bool {
	boolQuery, ok := extractBool(query)
	if !ok {
		return false
	}
	filters, _ := boolQuery["filter"].([]any)
	for _, raw := range filters {
		itemMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		terms, ok := itemMap["term"].(map[string]any)
		if !ok {
			continue
		}
		if _, has := terms[field]; has {
			return true
		}
	}
	return false
}
