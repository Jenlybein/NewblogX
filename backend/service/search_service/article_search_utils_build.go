package search_service

import (
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/repository/user_repo"
	"strings"
)

func idTermsValues(ids []ctype.ID) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		result = append(result, id.String())
	}
	return result
}

// buildDefaultArticleSearchQuery 构建默认文章搜索查询
func buildDefaultArticleSearchQuery(key string) map[string]any {
	return buildArticleSearchQuery(key, map[string]any{
		"filter": []any{
			buildPublishedArticleFilter(),
			buildVisibleArticleFilter(),
		},
	})
}

func buildPublishedArticleFilter() map[string]any {
	return map[string]any{
		"term": map[string]any{
			"publish_status": enum.ArticleStatusPublished,
		},
	}
}

func buildVisibleArticleFilter() map[string]any {
	return map[string]any{
		"term": map[string]any{
			"visibility_status": enum.ArticleVisibilityVisible,
		},
	}
}

// buildSelfArticleSearchQuery 构建“我的文章”搜索查询。
// 默认查询当前用户除已删除外的全部文章；如果显式传入状态，则按指定状态精确筛选。
func buildSelfArticleSearchQuery(key string, userID ctype.ID, status enum.ArticleStatus) map[string]any {
	boolQuery := map[string]any{
		"filter": []any{
			map[string]any{
				"term": map[string]any{
					"author_id": userID.String(),
				},
			},
		},
	}

	if status != 0 {
		filters, _ := boolQuery["filter"].([]any)
		boolQuery["filter"] = append(filters, map[string]any{
			"term": map[string]any{
				"publish_status": status,
			},
		})
	}

	return buildArticleSearchQuery(key, boolQuery)
}

// buildAdminArticleSearchQuery 构建管理员文章搜索查询。
// 管理员默认可以搜索任意状态的文章；如果显式传入状态，则按指定状态精确筛选。
func buildAdminArticleSearchQuery(key string, status enum.ArticleStatus) map[string]any {
	boolQuery := map[string]any{}
	if status != 0 {
		boolQuery["filter"] = []any{
			map[string]any{
				"term": map[string]any{
					"publish_status": status,
				},
			},
		}
	}
	return buildArticleSearchQuery(key, boolQuery)
}

// buildArticleSearchQuery 构建文章搜索查询。
// boolQuery 负责业务筛选条件，关键词匹配和综合评分在这里统一追加。
func buildArticleSearchQuery(key string, boolQuery map[string]any) map[string]any {
	key = strings.TrimSpace(key)

	if key != "" {
		boolQuery["must"] = []any{
			map[string]any{
				"multi_match": map[string]any{
					"query":  key,
					"fields": []string{"title", "abstract", "content_parts.content"},
				},
			},
		}
	} else {
		boolQuery["must"] = []any{
			map[string]any{
				"match_all": map[string]any{},
			},
		}
	}

	return map[string]any{
		"function_score": map[string]any{
			"query": map[string]any{
				"bool": boolQuery,
			},
			"functions": []any{
				map[string]any{
					"gauss": map[string]any{
						"created_at": map[string]any{
							"origin": "now",
							"scale":  "30d",
							"offset": "7d",
							"decay":  0.5,
						},
					},
					"weight": 0.22,
				},
				map[string]any{
					"field_value_factor": map[string]any{
						"field":    "digg_count",
						"modifier": "log1p",
						"missing":  0,
					},
					"weight": 0.21,
				},
				map[string]any{
					"field_value_factor": map[string]any{
						"field":    "comment_count",
						"modifier": "log1p",
						"missing":  0,
					},
					"weight": 0.20,
				},
				map[string]any{
					"field_value_factor": map[string]any{
						"field":    "favor_count",
						"modifier": "log1p",
						"missing":  0,
					},
					"weight": 0.18,
				},
				map[string]any{
					"field_value_factor": map[string]any{
						"field":    "view_count",
						"modifier": "log1p",
						"missing":  0,
					},
					"weight": 0.12,
				},
			},
			"score_mode": "sum",
			"boost_mode": "sum",
		},
	}
}

// buildLikeTagsQuery 构建喜欢标签查询
func buildLikeTagsQuery(query map[string]any, likeTagIDs []ctype.ID) map[string]any {
	return user_repo.BuildLikeTagsQuery(query, likeTagIDs)
}

// buildAuthorIDQuery 构建作者 ID 查询。
func buildAuthorIDQuery(query map[string]any, authorID ctype.ID) map[string]any {
	if authorID == 0 {
		return query
	}

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		return query
	}

	filters, _ := boolQuery["filter"].([]any)
	boolQuery["filter"] = append(filters, map[string]any{
		"term": map[string]any{
			"author_id": authorID.String(),
		},
	})
	return query
}

// buildUserIDQuery 兼容旧调用，内部转给 buildAuthorIDQuery。
func buildUserIDQuery(query map[string]any, userID ctype.ID) map[string]any {
	return buildAuthorIDQuery(query, userID)
}

// buildTagListQuery 构建标签标题查询，兼容旧 AI 搜索调用。
func buildTagListQuery(query map[string]any, tagList []string) map[string]any {
	normalized := make([]string, 0, len(tagList))
	seen := make(map[string]struct{}, len(tagList))
	for _, item := range tagList {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		return query
	}

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		return query
	}

	filters, _ := boolQuery["filter"].([]any)
	boolQuery["filter"] = append(filters, map[string]any{
		"terms": map[string]any{
			"tags.title": normalized,
		},
	})

	return query
}

// buildTagIDsQuery 构建标签 ID 查询。
func buildTagIDsQuery(query map[string]any, tagIDs []ctype.ID) map[string]any {
	normalized := make([]ctype.ID, 0, len(tagIDs))
	seen := make(map[ctype.ID]struct{}, len(tagIDs))
	for _, item := range tagIDs {
		if item == 0 {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	if len(normalized) == 0 {
		return query
	}

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		return query
	}

	filters, _ := boolQuery["filter"].([]any)
	boolQuery["filter"] = append(filters, map[string]any{
		"terms": map[string]any{
			"tags.id": idTermsValues(normalized),
		},
	})

	return query
}

// buildCategoryIDQuery 构建分类查询
func buildCategoryIDQuery(query map[string]any, categoryID ctype.ID) map[string]any {
	if categoryID == 0 {
		return query
	}

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		return query
	}

	filters, _ := boolQuery["filter"].([]any)
	boolQuery["filter"] = append(filters, map[string]any{
		"term": map[string]any{
			"category_id": categoryID.String(),
		},
	})
	return query
}

// buildArticleSearchExtraBody 构建文章搜索额外参数。
// 只有在存在关键词时，才额外请求正文摘要和正文分段，避免空关键词列表返回冗余大字段。
func buildArticleSearchExtraBody(sortField, key string) map[string]any {
	sortList := make([]any, 0, 2)
	if sortField != "" {
		sortList = append(sortList, map[string]any{
			sortField: map[string]any{
				"order": "desc",
			},
		})
	}
	sortList = append(sortList, map[string]any{
		"_score": map[string]any{
			"order": "desc",
		},
	})

	sourceFields := []string{
		"id",
		"created_at",
		"updated_at",
		"title",
		"abstract",
		"cover",
		"view_count",
		"digg_count",
		"comment_count",
		"favor_count",
		"comments_toggle",
		"publish_status",
		"visibility_status",
		"tags",
		"author_id",
		"author",
		"category_id",
		"category",
		"admin_top",
		"author_top",
	}
	highlightFields := map[string]any{}
	if strings.TrimSpace(key) != "" {
		sourceFields = append(sourceFields, "content_head", "content_parts")
		highlightFields["title"] = map[string]any{}
		highlightFields["abstract"] = map[string]any{
			"fragment_size":       120,
			"number_of_fragments": 1,
		}
		highlightFields["content_parts.content"] = map[string]any{
			"fragment_size":       120,
			"number_of_fragments": 1,
		}
	}

	return map[string]any{
		"_source": sourceFields,
		"sort":    sortList,
		// 高亮，用<em>标签包裹
		"highlight": map[string]any{
			"pre_tags":  []string{"<em>"},
			"post_tags": []string{"</em>"},
			"fields":    highlightFields,
		},
	}
}
