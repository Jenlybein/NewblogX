package app

import (
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/search/domain"
)

// 索引侧软删兜底（#199）。
//
// #196 之后，删除的主路径是投影直接把文档从索引里删掉。但只要中继位点落后、
// 投影进了死信，文档就会一直留着；索引文档上的软删标记就是给这种"删除没落地"
// 的情况兜底的第三层：标记为已删的文档，任何读面都不该再出现。
//
// 这里钉两件事：
//  1. 兜底过滤必须出现在每一条读面的查询里（1/2/3/4/5 同一套 DSL，口径一致）。
//  2. 过滤必须**容忍字段缺失**：索引里在标记字段引入之前写入的旧文档没有这个
//     字段，它们必须继续可见。写成 filter + term(is_deleted=false) 会把这些
//     历史文档从所有列表里悄悄抹掉 —— 这是本票最容易写错的地方。

// TestSoftDeleteFallbackFilterPresentForEveryReadType 每一条读面都带兜底过滤。
func TestSoftDeleteFallbackFilterPresentForEveryReadType(t *testing.T) {
	t.Parallel()

	user := &domain.Viewer{ID: ctype.NewID(100)}
	admin := &domain.Viewer{ID: ctype.NewID(900), Admin: true}

	cases := []struct {
		name   string
		req    domain.Request
		viewer *domain.Viewer
		like   []ctype.ID
	}{
		{name: "type1 公共搜索", req: domain.Request{Type: 1, Sort: 1, PageMode: domain.PageModeCount}},
		{name: "type1 公共搜索-关键词", req: domain.Request{Type: 1, Sort: 1, Key: "golang", PageMode: domain.PageModeCount}},
		{name: "type2 猜你喜欢", req: domain.Request{Type: 2, Sort: 1, PageMode: domain.PageModeCount}, viewer: user, like: []ctype.ID{ctype.NewID(3)}},
		{name: "type3 作者文章", req: domain.Request{Type: 3, Sort: 1, AuthorID: ctype.NewID(100), PageMode: domain.PageModeCount}, viewer: user},
		{name: "type4 自己的文章", req: domain.Request{Type: 4, Sort: 1, PageMode: domain.PageModeCount}, viewer: user},
		{name: "type5 管理员后台", req: domain.Request{Type: 5, Sort: 1, PageMode: domain.PageModeCount}, viewer: admin},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			normalized, err := normalize(c.req, c.viewer)
			if err != nil {
				t.Fatalf("请求应通过归一化: %v", err)
			}
			query, err := buildDSL(normalized, c.viewer, c.like)
			if err != nil {
				t.Fatalf("编译 DSL 失败: %v", err)
			}
			if !hasMustNotTerm(query, softDeleteMarkerField, true) {
				t.Fatalf("读面查询缺少软删兜底过滤（must_not is_deleted=true），实际 DSL=%#v", query)
			}
		})
	}
}

// TestSoftDeleteFallbackToleratesUnmarkedLegacyDocs 是这条兜底的核心行为矩阵。
//
// 同一份查询喂三种文档：
//   - is_deleted=true  ：已标记删除 → 不出现（兜底生效）；
//   - 没有 is_deleted  ：字段引入之前写入的旧文档 → 仍然出现（缺失容忍）；
//   - is_deleted=false ：新建文档 → 出现。
func TestSoftDeleteFallbackToleratesUnmarkedLegacyDocs(t *testing.T) {
	t.Parallel()

	// 用公共搜索（首页 / 搜索 / 标签 / 分类）与后台读面（type=5）两个口径验证，
	// 两者走的是同一套 bool 结构，兜底过滤只在构造处加一次。
	for _, tc := range []struct {
		name   string
		req    domain.Request
		viewer *domain.Viewer
	}{
		{name: "reader", req: domain.Request{Type: 1, Sort: 1, PageMode: domain.PageModeCount}},
		{name: "admin", req: domain.Request{Type: 5, Sort: 1, PageMode: domain.PageModeCount}, viewer: &domain.Viewer{ID: ctype.NewID(900), Admin: true}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			normalized, err := normalize(tc.req, tc.viewer)
			if err != nil {
				t.Fatalf("请求应通过归一化: %v", err)
			}
			query, err := buildDSL(normalized, tc.viewer, nil)
			if err != nil {
				t.Fatalf("编译 DSL 失败: %v", err)
			}

			// 文档的其余字段满足既有过滤条件（已发布 + 可见），
			// 这样"是否出现"只由软删标记决定。
			base := map[string]any{
				"publish_status":    domain.StatusPublished,
				"visibility_status": domain.VisibilityVisible,
			}
			docs := []struct {
				name string
				doc  map[string]any
				want bool
			}{
				{
					name: "已标记软删的文档",
					doc:  mergeDoc(base, map[string]any{softDeleteMarkerField: true}),
					want: false,
				},
				{
					name: "没有标记字段的旧文档",
					doc:  mergeDoc(base, nil),
					want: true,
				},
				{
					name: "标记为未删除的文档",
					doc:  mergeDoc(base, map[string]any{softDeleteMarkerField: false}),
					want: true,
				},
			}

			for _, d := range docs {
				if got := docMatchesQuery(query, d.doc); got != d.want {
					t.Fatalf("%s：可见性 got=%v want=%v\n文档=%#v\nDSL=%#v", d.name, got, d.want, d.doc, query)
				}
			}
		})
	}
}

func mergeDoc(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// hasMustNotTerm 判断 bool 查询里是否有 must_not 形式的 term 过滤。
func hasMustNotTerm(query map[string]any, field string, value any) bool {
	boolQuery, ok := extractBool(query)
	if !ok {
		return false
	}
	return boolHasMustNotTerm(boolQuery, field, value)
}

func boolHasMustNotTerm(boolQuery map[string]any, field string, value any) bool {
	for _, raw := range asAnyList(boolQuery["must_not"]) {
		clause, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if inner, ok := clause["bool"].(map[string]any); ok {
			if boolHasMustNotTerm(inner, field, value) {
				return true
			}
			continue
		}
		terms, ok := clause["term"].(map[string]any)
		if !ok {
			continue
		}
		if got, exists := terms[field]; exists && got == value {
			return true
		}
	}
	// 兜底过滤也可以挂在 filter 里（嵌套 bool），这里一并看。
	for _, raw := range asAnyList(boolQuery["filter"]) {
		clause, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if inner, ok := clause["bool"].(map[string]any); ok {
			if boolHasMustNotTerm(inner, field, value) {
				return true
			}
		}
	}
	return false
}

// docMatchesQuery 是 ES bool 查询语义的一个极小子集，专门用来回答
// "这份文档会不会出现在结果里"：function_score → bool → filter/must_not → term/bool。
//
// 刻意不复用生产代码，期望值来自 ES 的过滤语义本身 —— 复用就等于让代码
// 自己验证自己。没实现的子句（multi_match / terms / gauss 等）一律视为匹配，
// 用例里的文档与查询不依赖它们。
func docMatchesQuery(query, doc map[string]any) bool {
	functionScore, ok := query["function_score"].(map[string]any)
	if !ok {
		return false
	}
	queryBody, ok := functionScore["query"].(map[string]any)
	if !ok {
		return false
	}
	boolQuery, ok := queryBody["bool"].(map[string]any)
	if !ok {
		return false
	}
	return docMatchesBool(boolQuery, doc)
}

func docMatchesBool(boolQuery, doc map[string]any) bool {
	for _, raw := range asAnyList(boolQuery["filter"]) {
		if !docMatchesClause(raw, doc) {
			return false
		}
	}
	for _, raw := range asAnyList(boolQuery["must"]) {
		if !docMatchesClause(raw, doc) {
			return false
		}
	}
	for _, raw := range asAnyList(boolQuery["must_not"]) {
		if docMatchesClause(raw, doc) {
			return false
		}
	}
	return true
}

func docMatchesClause(raw any, doc map[string]any) bool {
	clause, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	if inner, ok := clause["bool"].(map[string]any); ok {
		return docMatchesBool(inner, doc)
	}
	if _, ok := clause["match_all"]; ok {
		return true
	}
	terms, ok := clause["term"].(map[string]any)
	if !ok {
		// 未实现的子句：视为匹配（用例不依赖）。
		return true
	}
	for field, want := range terms {
		got, exists := doc[field]
		// ES 的 term 对缺失字段不匹配，且类型必须一致。
		if !exists || got != want {
			return false
		}
	}
	return true
}

func asAnyList(raw any) []any {
	list, _ := raw.([]any)
	return list
}
