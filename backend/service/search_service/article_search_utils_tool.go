package search_service

import (
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_article"
)

// extractHighlightValues 提取高亮值
func extractHighlightValues(highlightMap map[string]any, field string) []string {
	rawList, ok := highlightMap[field].([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(rawList))
	for _, rawValue := range rawList {
		value, ok := rawValue.(string)
		if !ok {
			continue
		}
		result = append(result, value)
	}
	return result
}

// extractSearchBoolQuery 提取搜索 bool 查询
func extractSearchBoolQuery(query map[string]any) (map[string]any, bool) {
	functionScore, ok := query["function_score"].(map[string]any)
	if !ok {
		return nil, false
	}
	queryBody, ok := functionScore["query"].(map[string]any)
	if !ok {
		return nil, false
	}
	boolQuery, ok := queryBody["bool"].(map[string]any)
	return boolQuery, ok
}

// loadSearchArticleCounterMaps 批量读取 Redis 中的文章计数增量。
// 搜索结果里的计数字段以 ES 文档为基础值，再叠加 Redis 中尚未落库的实时增量。
func loadSearchArticleCounterMaps(redisDeps redis_service.Deps, articleIDs []ctype.ID) (favorMap, diggMap, viewMap, commentMap map[ctype.ID]int) {
	favorMap = make(map[ctype.ID]int)
	diggMap = make(map[ctype.ID]int)
	viewMap = make(map[ctype.ID]int)
	commentMap = make(map[ctype.ID]int)
	if len(articleIDs) == 0 {
		return favorMap, diggMap, viewMap, commentMap
	}

	counters := redis_article.GetBatchCounters(redisDeps, articleIDs)
	favorMap = counters.FavorMap
	diggMap = counters.DiggMap
	viewMap = counters.ViewMap
	commentMap = counters.CommentMap
	return favorMap, diggMap, viewMap, commentMap
}

func buildSearchHighlight(highlightMap map[string]any) *SearchHighlight {
	if len(highlightMap) == 0 {
		return nil
	}

	highlight := &SearchHighlight{}
	if values := extractHighlightValues(highlightMap, "title"); len(values) > 0 {
		highlight.Title = values[0]
	}
	if values := extractHighlightValues(highlightMap, "abstract"); len(values) > 0 {
		highlight.Abstract = values[0]
	} else if values := extractHighlightValues(highlightMap, "content_head"); len(values) > 0 {
		highlight.Abstract = values[0]
	} else if values := extractHighlightValues(highlightMap, "content_parts.content"); len(values) > 0 {
		highlight.Abstract = values[0]
	}
	if highlight.Title == "" && highlight.Abstract == "" {
		return nil
	}
	return highlight
}

func normalizeSearchResponseMeta(redisDeps redis_service.Deps, list []SearchListResponse) []SearchListResponse {
	articleIDs := make([]ctype.ID, 0, len(list))
	for _, item := range list {
		articleIDs = append(articleIDs, item.ID)
	}
	favorMap, diggMap, viewMap, commentMap := loadSearchArticleCounterMaps(redisDeps, articleIDs)
	for index := range list {
		list[index].FavorCount += favorMap[list[index].ID]
		list[index].DiggCount += diggMap[list[index].ID]
		list[index].ViewCount += viewMap[list[index].ID]
		list[index].CommentCount += commentMap[list[index].ID]

		if list[index].Category == nil {
			list[index].Category = &SearchCategory{}
		}
		if list[index].CategoryTitle == "" {
			list[index].CategoryTitle = list[index].Category.Title
		}
		if list[index].UserNickname == "" {
			list[index].UserNickname = list[index].Author.Nickname
		}
		if list[index].UserAvatar == "" {
			list[index].UserAvatar = list[index].Author.Avatar
		}

		if list[index].UserTop || list[index].AdminTop {
			list[index].Top = &SearchTop{
				User:  list[index].UserTop,
				Admin: list[index].AdminTop,
			}
		}
	}
	return list
}

// extractArticleSearchResults 提取文章搜索结果
func extractArticleSearchResults(redisDeps redis_service.Deps, data map[string]any) (list []SearchListResponse) {
	hits, _ := data["hits"].([]any)
	list = make([]SearchListResponse, 0, len(hits))

	for _, hit := range hits {
		item, ok := hit.(map[string]any)
		if !ok {
			continue
		}

		sourceMap, ok := item["_source"].(map[string]any)
		if !ok {
			continue
		}

		highlightMap, _ := item["highlight"].(map[string]any)
		articleID := sourceIDValue(sourceMap, "id")
		title := sourceStringValue(sourceMap, "title")
		abstract := sourceStringValue(sourceMap, "abstract")
		contentHead := sourceStringValue(sourceMap, "content_head")
		partList := sourceContentPartsValue(sourceMap, "content_parts")
		categoryID := sourceIDValue(sourceMap, "category_id")
		authorID := sourceIDValue(sourceMap, "author_id")
		categoryMap, _ := sourceMap["category"].(map[string]any)
		authorMap, _ := sourceMap["author"].(map[string]any)
		topMap, _ := sourceMap["top"].(map[string]any)
		highlight := buildSearchHighlight(highlightMap)
		if values := extractHighlightValues(highlightMap, "content_head"); len(values) > 0 {
			contentHead = values[0]
		} else if values := extractHighlightValues(highlightMap, "content_parts.content"); len(values) > 0 {
			contentHead = values[0]
		} else if highlight != nil && highlight.Abstract != "" {
			contentHead = highlight.Abstract
		}

		list = append(list, SearchListResponse{
			ID:               articleID,
			CreatedAt:        sourceTimeValue(sourceMap, "created_at"),
			UpdatedAt:        sourceTimeValue(sourceMap, "updated_at"),
			Title:            title,
			Abstract:         abstract,
			Content:          contentHead,
			Part:             partList,
			Cover:            sourceStringValue(sourceMap, "cover"),
			ViewCount:        sourceIntValue(sourceMap, "view_count"),
			DiggCount:        sourceIntValue(sourceMap, "digg_count"),
			CommentCount:     sourceIntValue(sourceMap, "comment_count"),
			FavorCount:       sourceIntValue(sourceMap, "favor_count"),
			CommentsToggle:   sourceBoolValue(sourceMap, "comments_toggle"),
			PublishStatus:    sourceArticleStatusValue(sourceMap, "publish_status"),
			VisibilityStatus: sourceArticleVisibilityStatusValue(sourceMap, "visibility_status"),
			Tags:             sourceTagItemsValue(sourceMap, "tags"),
			UserTop:          sourceBoolValue(topMap, "user") || sourceBoolValue(sourceMap, "author_top"),
			AdminTop:         sourceBoolValue(topMap, "admin") || sourceBoolValue(sourceMap, "admin_top"),
			Category: &SearchCategory{
				ID:    categoryID,
				Title: sourceStringValue(categoryMap, "title"),
			},
			Author: SearchAuthor{
				ID:       authorID,
				Nickname: sourceStringValue(authorMap, "nickname"),
				Avatar:   sourceStringValue(authorMap, "avatar"),
			},
			CategoryTitle: sourceStringValue(categoryMap, "title"),
			UserNickname:  sourceStringValue(authorMap, "nickname"),
			UserAvatar:    sourceStringValue(authorMap, "avatar"),
			Highlight:     highlight,
			Score:         sourceFloatValue(item, "_score"),
		})
	}
	return normalizeSearchResponseMeta(redisDeps, list)
}
