// Package app 是搜索模块的用例层：把请求编译成 ES DSL、执行、解析成响应。
package app

import (
	"encoding/json"
	"errors"
	"math"
	"strings"

	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	es_service "myblogx/internal/platform/es"
	"myblogx/internal/search/domain"

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
		if req.AuthorID.IsZero() {
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

	// 「显示非管理员发布文章」开关关闭时，公开读面只保留管理员作者的文章
	// （ADR-0017 决策 4）。type 4（自己的文章）与 type 5（后台）刻意不过滤：
	// 作者要能继续编辑自己的稿子，管理员要能在后台看到全部内容。
	//
	// 这里必须处理返回的错误：这个过滤一旦被跳过，就是"开关关了但全量返回"，
	// 而查询本身仍然合法、ES 不会报错——只能靠失败关闭挡住。
	if req.OnlyAdminAuthors && isPublicReadType(req.Type) {
		filtered, err := adminAuthorQuery(query)
		if err != nil {
			return nil, err
		}
		query = filtered
	}

	if len(req.TagIDs) > 0 {
		query = tagIDsQuery(query, req.TagIDs)
	} else if len(req.LegacyTagList) > 0 {
		query = tagListQuery(query, req.LegacyTagList)
	}
	if !req.CategoryID.IsZero() {
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

// isPublicReadType 判断某个 type 是不是"公开读面"。
//
// 1 公共搜索（首页 / 搜索 / 标签 / 分类 / AI 搜索）、2 猜你喜欢、3 作者文章。
// 4 自己的文章与 5 管理员后台不是公开读面，不受显示开关影响。
func isPublicReadType(articleType int8) bool {
	return articleType == 1 || articleType == 2 || articleType == 3
}

// adminAuthorQuery 追加"作者是管理员"的过滤。
//
// 字段 author_is_admin 是作者角色在 ES 文档里的投影（由 CDC 按 user_models.role
// 重算），所以开关关掉之后不需要回库查作者角色，也不存在"文章数与实际不符"。
//
// **失败关闭**：取不到 bool 查询结构就返回错误，绝不原样放行。早先这里在
// 结构对不上时 `return query`，等于开关关闭却静默地全量返回——ES 不会报错，
// 只能靠这条错误把它暴露出来。错误码是内部不变量类的（不是调用方参数问题）。
func adminAuthorQuery(query map[string]any) (map[string]any, error) {
	boolQuery, ok := extractBool(query)
	if !ok {
		return nil, apperr.New(apperr.CodeSearchQueryInvalid)
	}
	filters, _ := boolQuery["filter"].([]any)
	boolQuery["filter"] = append(filters, adminAuthorFilter())
	return query, nil
}

// adminAuthorFilter 是"只出管理员作者"的那个过滤子句。
//
// 单独抽出来是为了让调用点能断言"过滤确实加上了"，而不是比对整棵查询树。
func adminAuthorFilter() map[string]any {
	return map[string]any{"term": map[string]any{"author_is_admin": true}}
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
	if authorID.IsZero() {
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
		if item.IsZero() {
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
	if categoryID.IsZero() {
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
		"tags", "author_id", "author", "author_is_admin", "category_id", "category",
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
			AuthorIsAdmin:    sourceBool(source, "author_is_admin"),
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
		if id.IsZero() {
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
