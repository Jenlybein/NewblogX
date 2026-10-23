package search_service

// buildAdminTopQuery 为搜索查询追加“管理员置顶优先”加权。
// 这里直接使用 ES 文档中的 admin_top 字段，不再额外查数据库拼文章 ID。
func buildAdminTopQuery(query map[string]any) map[string]any {
	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		return query
	}

	should, _ := boolQuery["should"].([]any)
	should = append(should, map[string]any{
		"term": map[string]any{
			"admin_top": map[string]any{
				"value": true,
				"boost": 100,
			},
		},
	})
	boolQuery["should"] = should
	return query
}

// buildAuthorAdminTopQuery 为作者相关文章追加“作者置顶 / 管理员置顶优先”加权。
// 作者筛选本身已经在主查询里限制，因此这里只需要基于 ES 里的布尔标记加权即可。
func buildAuthorAdminTopQuery(query map[string]any) map[string]any {
	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		return query
	}

	should, _ := boolQuery["should"].([]any)
	should = append(should,
		map[string]any{
			"term": map[string]any{
				"author_top": map[string]any{
					"value": true,
					"boost": 100,
				},
			},
		},
		map[string]any{
			"term": map[string]any{
				"admin_top": map[string]any{
					"value": true,
					"boost": 100,
				},
			},
		},
	)
	boolQuery["should"] = should
	return query
}
