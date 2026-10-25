// Package app 是搜索模块的用例层：把请求编译成 ES DSL、执行、解析成响应。
package app

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"myblogx/internal/apperr"
	es_service "myblogx/internal/platform/es"
	"myblogx/internal/search/domain"
	"myblogx/models/ctype"
	"myblogx/utils/markdown"

	"github.com/elastic/go-elasticsearch/v7"
)

const defaultArticleIndex = "article_index"

// Search 执行文章搜索。
//
// viewer 为 nil 表示匿名；counters 为 nil 时不做实时计数叠加。
func Search(req domain.Request, viewer *domain.Viewer, likeTagIDs []ctype.ID, counters domain.CounterReader, esClient *elasticsearch.Client, index string) (domain.Response, error) {
	normalized, err := normalize(req, viewer)
	if err != nil {
		return domain.Response{}, err
	}
	query, err := buildDSL(normalized, viewer, likeTagIDs)
	if err != nil {
		return domain.Response{}, err
	}
	extra := extraBodyBySort(normalized.Sort, normalized.Key)
	return execute(counters, esClient, normalized, query, extra, resolveIndex(index))
}

func normalize(req domain.Request, viewer *domain.Viewer) (domain.Request, error) {
	req = req.Normalize()
	if req.Type < 1 || req.Type > 5 {
		return req, apperr.New(apperr.CodeSearchTypeInvalid)
	}
	if req.Sort < 1 || req.Sort > 6 {
		return req, apperr.New(apperr.CodeSearchSortInvalid)
	}
	if req.PageMode != domain.PageModeHasMore && req.PageMode != domain.PageModeCount {
		return req, apperr.New(apperr.CodeSearchPageModeInvalid)
	}

	switch req.Type {
	case 1, 2:
		if req.Status == 0 {
			req.Status = domain.StatusPublished
		}
		if req.Status != domain.StatusPublished {
			return req, apperr.New(apperr.CodeSearchStatusRestrict)
		}
	case 3:
		if req.AuthorID == 0 {
			return req, apperr.New(apperr.CodeSearchAuthorRequired)
		}
		if req.Status == 0 {
			req.Status = domain.StatusPublished
		}
		if req.Status != domain.StatusPublished {
			return req, apperr.New(apperr.CodeSearchStatusRestrict)
		}
	case 4:
		if viewer == nil || viewer.IsZero() {
			return req, apperr.New(apperr.CodeSearchUnauthorized)
		}
		req.AuthorID = viewer.ID
	case 5:
		if viewer == nil || !viewer.Admin {
			return req, apperr.New(apperr.CodeSearchForbidden)
		}
	}
	return req, nil
}

func buildDSL(req domain.Request, viewer *domain.Viewer, likeTagIDs []ctype.ID) (map[string]any, error) {
	query := defaultQuery(req.Key)
	switch req.Type {
	case 1:
		query = publishedStatusQuery(query, req.Status)
	case 2:
		query = publishedStatusQuery(query, req.Status)
		if viewer != nil && len(likeTagIDs) > 0 {
			query = likeTagsQuery(query, likeTagIDs)
		}
	case 3:
		query = publishedStatusQuery(query, req.Status)
		query = authorQuery(query, req.AuthorID)
	case 4:
		query = selfQuery(req.Key, req.AuthorID, req.Status)
	case 5:
		query = adminQuery(req.Key, req.Status)
	default:
		return nil, apperr.New(apperr.CodeSearchTypeInvalid)
	}

	if len(req.TagIDs) > 0 {
		query = tagIDsQuery(query, req.TagIDs)
	} else if len(req.LegacyTagList) > 0 {
		query = tagListQuery(query, req.LegacyTagList)
	}
	if req.CategoryID != 0 {
		query = categoryQuery(query, req.CategoryID)
	}
	return query, nil
}

func defaultQuery(key string) map[string]any {
	return articleQuery(key, map[string]any{
		"filter": []any{publishedFilter(), visibleFilter()},
	})
}

func publishedFilter() map[string]any {
	return map[string]any{"term": map[string]any{"publish_status": domain.StatusPublished}}
}

func visibleFilter() map[string]any {
	return map[string]any{"term": map[string]any{"visibility_status": domain.VisibilityVisible}}
}

func selfQuery(key string, userID ctype.ID, status domain.ArticleStatus) map[string]any {
	boolQuery := map[string]any{
		"filter": []any{
			map[string]any{"term": map[string]any{"author_id": userID.String()}},
		},
	}
	if status != 0 {
		filters, _ := boolQuery["filter"].([]any)
		boolQuery["filter"] = append(filters, map[string]any{"term": map[string]any{"publish_status": status}})
	}
	return articleQuery(key, boolQuery)
}

func adminQuery(key string, status domain.ArticleStatus) map[string]any {
	boolQuery := map[string]any{}
	if status != 0 {
		boolQuery["filter"] = []any{
			map[string]any{"term": map[string]any{"publish_status": status}},
		}
	}
	return articleQuery(key, boolQuery)
}

func articleQuery(key string, boolQuery map[string]any) map[string]any {
	key = strings.TrimSpace(key)
	if key != "" {
		boolQuery["must"] = []any{
			map[string]any{"multi_match": map[string]any{
				"query":  key,
				"fields": []string{"title", "abstract", "content_parts.content"},
			}},
		}
	} else {
		boolQuery["must"] = []any{map[string]any{"match_all": map[string]any{}}}
	}

	return map[string]any{
		"function_score": map[string]any{
			"query": map[string]any{"bool": boolQuery},
			"functions": []any{
				gaussFunction("created_at", "now", "30d", "7d", 0.5, 0.22),
				fieldFunction("digg_count", 0.21),
				fieldFunction("comment_count", 0.20),
				fieldFunction("favor_count", 0.18),
				fieldFunction("view_count", 0.12),
			},
			"score_mode": "sum",
			"boost_mode": "sum",
		},
	}
}

func gaussFunction(field, origin, scale, offset string, decay, weight float64) map[string]any {
	return map[string]any{
		"gauss": map[string]any{
			field: map[string]any{"origin": origin, "scale": scale, "offset": offset, "decay": decay},
		},
		"weight": weight,
	}
}

func fieldFunction(field string, weight float64) map[string]any {
	return map[string]any{
		"field_value_factor": map[string]any{"field": field, "modifier": "log1p", "missing": 0},
		"weight":             weight,
	}
}

func likeTagsQuery(query map[string]any, tagIDs []ctype.ID) map[string]any {
	boolQuery, ok := extractBool(query)
	if !ok {
		return query
	}
	should, _ := boolQuery["should"].([]any)
	should = append(should, map[string]any{
		"terms": map[string]any{"tags.id": idValues(tagIDs), "boost": 2},
	})
	boolQuery["should"] = should
	return query
}

func authorQuery(query map[string]any, authorID ctype.ID) map[string]any {
	if authorID == 0 {
		return query
	}
	boolQuery, ok := extractBool(query)
	if !ok {
		return query
	}
	filters, _ := boolQuery["filter"].([]any)
	boolQuery["filter"] = append(filters, map[string]any{"term": map[string]any{"author_id": authorID.String()}})
	return query
}

func tagListQuery(query map[string]any, tagList []string) map[string]any {
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
	boolQuery, ok := extractBool(query)
	if !ok {
		return query
	}
	filters, _ := boolQuery["filter"].([]any)
	boolQuery["filter"] = append(filters, map[string]any{"terms": map[string]any{"tags.title": normalized}})
	return query
}

func tagIDsQuery(query map[string]any, tagIDs []ctype.ID) map[string]any {
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
	boolQuery, ok := extractBool(query)
	if !ok {
		return query
	}
	filters, _ := boolQuery["filter"].([]any)
	boolQuery["filter"] = append(filters, map[string]any{"terms": map[string]any{"tags.id": idValues(normalized)}})
	return query
}

func categoryQuery(query map[string]any, categoryID ctype.ID) map[string]any {
	if categoryID == 0 {
		return query
	}
	boolQuery, ok := extractBool(query)
	if !ok {
		return query
	}
	filters, _ := boolQuery["filter"].([]any)
	boolQuery["filter"] = append(filters, map[string]any{"term": map[string]any{"category_id": categoryID.String()}})
	return query
}

func publishedStatusQuery(query map[string]any, status domain.ArticleStatus) map[string]any {
	boolQuery, ok := extractBool(query)
	if !ok {
		return query
	}
	filters, _ := boolQuery["filter"].([]any)
	filtered := make([]any, 0, len(filters))
	for _, item := range filters {
		termMap, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		termBody, ok := termMap["term"].(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		if _, has := termBody["publish_status"]; has {
			continue
		}
		filtered = append(filtered, item)
	}
	filtered = append(filtered, map[string]any{"term": map[string]any{"publish_status": status}})
	boolQuery["filter"] = filtered
	return query
}

func extractBool(query map[string]any) (map[string]any, bool) {
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

func extraBodyBySort(sort int8, key string) map[string]any {
	switch sort {
	case 2:
		return extraBody("created_at", key)
	case 3:
		return extraBody("comment_count", key)
	case 4:
		return extraBody("digg_count", key)
	case 5:
		return extraBody("favor_count", key)
	case 6:
		return extraBody("view_count", key)
	default:
		return extraBody("", key)
	}
}

func extraBody(sortField, key string) map[string]any {
	sortList := make([]any, 0, 2)
	if sortField != "" {
		sortList = append(sortList, map[string]any{sortField: map[string]any{"order": "desc"}})
	}
	sortList = append(sortList, map[string]any{"_score": map[string]any{"order": "desc"}})

	sourceFields := []string{
		"id", "created_at", "updated_at", "title", "abstract", "cover",
		"view_count", "digg_count", "comment_count", "favor_count",
		"comments_toggle", "publish_status", "visibility_status",
		"tags", "author_id", "author", "category_id", "category",
		"admin_top", "author_top",
	}
	highlightFields := map[string]any{}
	if strings.TrimSpace(key) != "" {
		sourceFields = append(sourceFields, "content_head", "content_parts")
		highlightFields["title"] = map[string]any{}
		highlightFields["abstract"] = map[string]any{"fragment_size": 120, "number_of_fragments": 1}
		highlightFields["content_parts.content"] = map[string]any{"fragment_size": 120, "number_of_fragments": 1}
	}
	return map[string]any{
		"_source": sourceFields,
		"sort":    sortList,
		"highlight": map[string]any{
			"pre_tags":  []string{"<em>"},
			"post_tags": []string{"</em>"},
			"fields":    highlightFields,
		},
	}
}

// buildBody 组装发给 ES 的请求体。
//
// 单独抽出来是为了能对"最终发出去的 JSON"做 golden：
// 分页、排序、_source 字段表、highlight 配置分散在几个函数里，
// 任何一处写错都不会报错，只表现为"某些字段搜不出来 / 高亮没了"。
// 只测 buildDSL 覆盖不到这些，测整个 body 才算守住契约。
func buildBody(req domain.Request, query, extra map[string]any) map[string]any {
	page := req.NormalizePage()
	limit := req.GetLimit()

	body := map[string]any{
		"from":             (page - 1) * limit,
		"size":             limit,
		"query":            query,
		"track_total_hits": req.PageMode == domain.PageModeCount,
	}
	for k, v := range extra {
		body[k] = v
	}
	if req.PageMode == domain.PageModeHasMore {
		// 多取一条：用来判断"还有没有下一页"，比再查一次总数便宜。
		body["size"] = limit + 1
	}
	return body
}

func execute(counters domain.CounterReader, esClient *elasticsearch.Client, req domain.Request, query map[string]any, extra map[string]any, index string) (domain.Response, error) {
	body := buildBody(req, query, extra)
	// 页面/条数在后面解析响应时还要用（算总页数、拼 has_more），这里再取一次。
	page, limit := req.NormalizePage(), req.GetLimit()

	resp := es_service.SearchBody(esClient, index, body)
	if !resp.Success {
		return domain.Response{}, errors.New(resp.Msg)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		return domain.Response{}, apperr.New(apperr.CodeSearchResultInvalid)
	}

	list := extract(counters, data)
	pagination := domain.Pagination{Mode: req.PageMode, Page: page, Limit: limit}
	if req.PageMode == domain.PageModeHasMore {
		if len(list) > limit {
			pagination.HasMore = true
			list = list[:limit]
		}
		return domain.Response{List: list, Pagination: pagination}, nil
	}

	total := 0
	switch value := data["value"].(type) {
	case float64:
		total = int(value)
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			total = int(parsed)
		}
	}
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(limit)))
	}
	pagination.HasMore = page*limit < total
	pagination.Total = &total
	pagination.TotalPages = &totalPages
	return domain.Response{List: list, Pagination: pagination}, nil
}

func extract(counters domain.CounterReader, data map[string]any) []domain.Item {
	hits, _ := data["hits"].([]any)
	list := make([]domain.Item, 0, len(hits))
	for _, raw := range hits {
		hit, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		source, ok := hit["_source"].(map[string]any)
		if !ok {
			continue
		}
		highlightMap, _ := hit["highlight"].(map[string]any)
		articleID := sourceID(source, "id")
		categoryMap, _ := source["category"].(map[string]any)
		authorMap, _ := source["author"].(map[string]any)
		topMap, _ := source["top"].(map[string]any)
		highlight := buildHighlight(highlightMap)
		contentHead := sourceString(source, "content_head")
		if values := highlightValues(highlightMap, "content_head"); len(values) > 0 {
			contentHead = values[0]
		} else if values := highlightValues(highlightMap, "content_parts.content"); len(values) > 0 {
			contentHead = values[0]
		}

		list = append(list, domain.Item{
			ID:               articleID,
			CreatedAt:        sourceTime(source, "created_at"),
			UpdatedAt:        sourceTime(source, "updated_at"),
			Title:            sourceString(source, "title"),
			Abstract:         sourceString(source, "abstract"),
			Content:          contentHead,
			Part:             sourceContentParts(source, "content_parts"),
			Cover:            sourceString(source, "cover"),
			ViewCount:        sourceInt(source, "view_count"),
			DiggCount:        sourceInt(source, "digg_count"),
			CommentCount:     sourceInt(source, "comment_count"),
			FavorCount:       sourceInt(source, "favor_count"),
			CommentsToggle:   sourceBool(source, "comments_toggle"),
			PublishStatus:    domain.ArticleStatus(sourceInt(source, "publish_status")),
			VisibilityStatus: domain.VisibilityStatus(sourceString(source, "visibility_status")),
			Tags:             sourceTags(source, "tags"),
			UserTop:          sourceBool(topMap, "user") || sourceBool(source, "author_top"),
			AdminTop:         sourceBool(topMap, "admin") || sourceBool(source, "admin_top"),
			Category: &domain.Category{
				ID:    sourceID(source, "category_id"),
				Title: sourceString(categoryMap, "title"),
			},
			Author: domain.Author{
				ID:       sourceID(source, "author_id"),
				Nickname: sourceString(authorMap, "nickname"),
				Avatar:   sourceString(authorMap, "avatar"),
			},
			CategoryTitle: sourceString(categoryMap, "title"),
			UserNickname:  sourceString(authorMap, "nickname"),
			UserAvatar:    sourceString(authorMap, "avatar"),
			Highlight:     highlight,
			Score:         sourceFloat(hit, "_score"),
		})
	}
	return normalizeMeta(counters, list)
}

func normalizeMeta(counters domain.CounterReader, list []domain.Item) []domain.Item {
	ids := make([]ctype.ID, 0, len(list))
	for _, item := range list {
		ids = append(ids, item.ID)
	}
	var counts domain.Counts
	if counters != nil {
		counts = counters.Batch(ids)
	}
	for i := range list {
		if counts.Favor != nil {
			list[i].FavorCount += counts.Favor[list[i].ID]
		}
		if counts.Digg != nil {
			list[i].DiggCount += counts.Digg[list[i].ID]
		}
		if counts.View != nil {
			list[i].ViewCount += counts.View[list[i].ID]
		}
		if counts.Comment != nil {
			list[i].CommentCount += counts.Comment[list[i].ID]
		}
		if list[i].Category == nil {
			list[i].Category = &domain.Category{}
		}
		if list[i].CategoryTitle == "" {
			list[i].CategoryTitle = list[i].Category.Title
		}
		if list[i].UserNickname == "" {
			list[i].UserNickname = list[i].Author.Nickname
		}
		if list[i].UserAvatar == "" {
			list[i].UserAvatar = list[i].Author.Avatar
		}
		if list[i].UserTop || list[i].AdminTop {
			list[i].Top = &domain.Top{User: list[i].UserTop, Admin: list[i].AdminTop}
		}
	}
	return list
}

func buildHighlight(highlightMap map[string]any) *domain.Highlight {
	if len(highlightMap) == 0 {
		return nil
	}
	highlight := &domain.Highlight{}
	if values := highlightValues(highlightMap, "title"); len(values) > 0 {
		highlight.Title = values[0]
	}
	if values := highlightValues(highlightMap, "abstract"); len(values) > 0 {
		highlight.Abstract = values[0]
	} else if values := highlightValues(highlightMap, "content_head"); len(values) > 0 {
		highlight.Abstract = values[0]
	} else if values := highlightValues(highlightMap, "content_parts.content"); len(values) > 0 {
		highlight.Abstract = values[0]
	}
	if highlight.Title == "" && highlight.Abstract == "" {
		return nil
	}
	return highlight
}

func highlightValues(highlightMap map[string]any, field string) []string {
	rawList, ok := highlightMap[field].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(rawList))
	for _, raw := range rawList {
		if value, ok := raw.(string); ok {
			out = append(out, value)
		}
	}
	return out
}

func idValues(ids []ctype.ID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		out = append(out, id.String())
	}
	return out
}

func resolveIndex(index string) string {
	clean := strings.TrimSpace(index)
	if clean == "" {
		return defaultArticleIndex
	}
	return clean
}

// ── ES _source 取值（容忍 ES 返回的数字/字符串类型差异） ──

func sourceID(source map[string]any, key string) ctype.ID {
	switch value := source[key].(type) {
	case ctype.ID:
		return value
	case string:
		var id ctype.ID
		if err := id.UnmarshalText([]byte(value)); err != nil {
			return 0
		}
		return id
	default:
		return 0
	}
}

func sourceInt(source map[string]any, key string) int {
	switch value := source[key].(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
		return 0
	default:
		return 0
	}
}

func sourceString(source map[string]any, key string) string {
	value, _ := source[key].(string)
	return value
}

func sourceBool(source map[string]any, key string) bool {
	switch value := source[key].(type) {
	case bool:
		return value
	case int:
		return value != 0
	case int8:
		return value != 0
	case int16:
		return value != 0
	case int32:
		return value != 0
	case int64:
		return value != 0
	case uint:
		return value != 0
	case uint8:
		return value != 0
	case uint16:
		return value != 0
	case uint32:
		return value != 0
	case uint64:
		return value != 0
	case float32:
		return value != 0
	case float64:
		return value != 0
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return parsed != 0
		}
		return false
	default:
		return false
	}
}

func sourceTime(source map[string]any, key string) time.Time {
	switch value := source[key].(type) {
	case time.Time:
		return value
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func sourceFloat(source map[string]any, key string) float64 {
	switch value := source[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		if parsed, err := value.Float64(); err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func sourceStringSlice(source map[string]any, key string) []string {
	switch value := source[key].(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func sourceContentParts(source map[string]any, key string) []markdown.ContentPart {
	rawList, ok := source[key].([]any)
	if !ok {
		return nil
	}
	out := make([]markdown.ContentPart, 0, len(rawList))
	for _, raw := range rawList {
		itemMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		part := markdown.ContentPart{
			Level: sourceInt(itemMap, "level"),
			Title: sourceString(itemMap, "title"),
			Path:  sourceStringSlice(itemMap, "path"),
		}
		if part.Title == "" && part.Content == "" && len(part.Path) == 0 && part.Order == 0 && part.Level == 0 {
			continue
		}
		out = append(out, part)
	}
	return out
}

func sourceTags(source map[string]any, key string) []domain.Tag {
	rawList, ok := source[key].([]any)
	if !ok {
		return nil
	}
	out := make([]domain.Tag, 0, len(rawList))
	for _, raw := range rawList {
		itemMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		title := sourceString(itemMap, "title")
		id := sourceID(itemMap, "id")
		if title == "" && id == 0 {
			continue
		}
		out = append(out, domain.Tag{ID: id, Title: title})
	}
	return out
}
