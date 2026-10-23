package search_service

import (
	"encoding/json"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/repository/user_repo"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_article"
	"myblogx/test/testutil"
	"myblogx/utils/markdown"
	"testing"
	"time"
)

func TestBuildDefaultArticleSearchQueryOnlyPublished(t *testing.T) {
	query := buildDefaultArticleSearchQuery("golang")
	functionScore, ok := query["function_score"].(map[string]any)
	if !ok {
		t.Fatalf("function_score 查询结构错误: %#v", query)
	}
	queryBody, ok := functionScore["query"].(map[string]any)
	if !ok {
		t.Fatalf("function_score.query 结构错误: %#v", functionScore)
	}
	boolQuery, ok := queryBody["bool"].(map[string]any)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}

	filters, ok := boolQuery["filter"].([]any)
	if !ok || len(filters) != 2 {
		t.Fatalf("过滤条件异常: %#v", boolQuery["filter"])
	}
	publishFilter, ok := filters[0].(map[string]any)["term"].(map[string]any)
	if !ok || publishFilter["publish_status"] != enum.ArticleStatusPublished {
		t.Fatalf("发布状态过滤结构错误: %#v", filters[0])
	}
	visibleFilter, ok := filters[1].(map[string]any)["term"].(map[string]any)
	if !ok || visibleFilter["visibility_status"] != enum.ArticleVisibilityVisible {
		t.Fatalf("可见性过滤结构错误: %#v", filters[1])
	}

	functions, ok := functionScore["functions"].([]any)
	if !ok || len(functions) != 5 {
		t.Fatalf("综合评分函数异常: %#v", functionScore["functions"])
	}
	weights := []float64{0.22, 0.21, 0.20, 0.18, 0.12}
	for index, raw := range functions {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("评分函数结构错误: %#v", raw)
		}
		if item["weight"] != weights[index] {
			t.Fatalf("评分权重错误 index=%d weight=%#v", index, item["weight"])
		}
	}
}

func TestBuildLikeTagsQueryWithoutUserConf(t *testing.T) {
	query := buildDefaultArticleSearchQuery("")
	query = buildLikeTagsQuery(query, nil)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}
	if _, ok = boolQuery["should"]; ok {
		t.Fatalf("无有效用户配置时不应追加喜欢标签加权: %#v", boolQuery)
	}
}

func TestBuildLikeTagsQueryWithLikeTags(t *testing.T) {
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.TagModel{})

	user := models.UserModel{
		Username: "u1",
		Password: "x",
		Nickname: "nick",
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	var userConf models.UserConfModel
	if err := db.Take(&userConf, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("查询用户配置失败: %v", err)
	}
	if err := db.Model(&userConf).Updates(models.UserConfModel{
		LikeTags: []ctype.ID{3, 8},
	}).Error; err != nil {
		t.Fatalf("更新偏好标签失败: %v", err)
	}

	query := buildDefaultArticleSearchQuery("golang")
	likeTagIDs, err := user_repo.LoadLikeTagIDs(db, user.ID)
	if err != nil {
		t.Fatalf("加载偏好标签失败: %v", err)
	}
	query = buildLikeTagsQuery(query, likeTagIDs)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}
	if _, ok = boolQuery["must"]; !ok {
		t.Fatalf("有关键词时应带 must 查询: %#v", boolQuery)
	}

	should, ok := boolQuery["should"].([]any)
	if !ok || len(should) != 1 {
		t.Fatalf("有偏好标签时应追加 tag 加权: %#v", boolQuery["should"])
	}
}

func TestBuildTagListQueryWithTags(t *testing.T) {
	query := buildDefaultArticleSearchQuery("golang")
	query = buildTagListQuery(query, []string{" Go ", "ES", "Go", ""})

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}

	filters, ok := boolQuery["filter"].([]any)
	if !ok || len(filters) != 3 {
		t.Fatalf("标签匹配过滤条件异常: %#v", boolQuery["filter"])
	}

	terms, ok := filters[2].(map[string]any)
	if !ok {
		t.Fatalf("标签 terms 结构错误: %#v", filters[2])
	}
	tagTerms, ok := terms["terms"].(map[string]any)
	if !ok {
		t.Fatalf("标签匹配结构错误: %#v", terms)
	}
	values, ok := tagTerms["tags.title"].([]string)
	if !ok || len(values) != 2 || values[0] != "Go" || values[1] != "ES" {
		t.Fatalf("标签名归一化异常: %#v", tagTerms["tags.title"])
	}
}

func TestBuildCategoryIDQuery(t *testing.T) {
	query := buildDefaultArticleSearchQuery("golang")
	query = buildCategoryIDQuery(query, 12)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}

	filters, ok := boolQuery["filter"].([]any)
	if !ok || len(filters) != 3 {
		t.Fatalf("分类过滤条件异常: %#v", boolQuery["filter"])
	}

	term, ok := filters[2].(map[string]any)
	if !ok {
		t.Fatalf("分类 term 结构错误: %#v", filters[2])
	}
	categoryTerm, ok := term["term"].(map[string]any)
	if !ok || categoryTerm["category_id"] != ctype.ID(12).String() {
		t.Fatalf("分类过滤条件错误: %#v", term)
	}
}

func TestBuildUserAndCategoryFilters(t *testing.T) {
	query := buildDefaultArticleSearchQuery("golang")
	query = buildUserIDQuery(query, 88)
	query = buildCategoryIDQuery(query, 12)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}

	filters, ok := boolQuery["filter"].([]any)
	if !ok || len(filters) != 4 {
		t.Fatalf("作者和分类联合过滤条件异常: %#v", boolQuery["filter"])
	}

	authorFilter, ok := filters[2].(map[string]any)
	if !ok {
		t.Fatalf("作者过滤结构错误: %#v", filters[2])
	}
	authorTerm, ok := authorFilter["term"].(map[string]any)
	if !ok || authorTerm["author_id"] != ctype.ID(88).String() {
		t.Fatalf("作者过滤条件错误: %#v", authorFilter)
	}

	categoryFilter, ok := filters[3].(map[string]any)
	if !ok {
		t.Fatalf("分类过滤结构错误: %#v", filters[3])
	}
	categoryTerm, ok := categoryFilter["term"].(map[string]any)
	if !ok || categoryTerm["category_id"] != ctype.ID(12).String() {
		t.Fatalf("分类过滤条件错误: %#v", categoryFilter)
	}
}

func TestBuildUserIDQuery(t *testing.T) {
	query := buildDefaultArticleSearchQuery("golang")
	query = buildUserIDQuery(query, 88)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}

	filters, ok := boolQuery["filter"].([]any)
	if !ok || len(filters) != 3 {
		t.Fatalf("作者过滤条件异常: %#v", boolQuery["filter"])
	}

	term, ok := filters[2].(map[string]any)
	if !ok {
		t.Fatalf("作者 term 结构错误: %#v", filters[2])
	}
	authorTerm, ok := term["term"].(map[string]any)
	if !ok || authorTerm["author_id"] != ctype.ID(88).String() {
		t.Fatalf("作者过滤条件错误: %#v", term)
	}
}

func TestBuildSelfArticleSearchQueryDefaultStatus(t *testing.T) {
	query := buildSelfArticleSearchQuery("golang", 99, 0)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}

	filters, ok := boolQuery["filter"].([]any)
	if !ok || len(filters) != 1 {
		t.Fatalf("我的文章过滤条件异常: %#v", boolQuery["filter"])
	}
	term, ok := filters[0].(map[string]any)
	if !ok {
		t.Fatalf("作者 term 结构错误: %#v", filters[0])
	}
	authorTerm, ok := term["term"].(map[string]any)
	if !ok || authorTerm["author_id"] != ctype.ID(99).String() {
		t.Fatalf("我的文章作者过滤错误: %#v", term)
	}

	if _, ok := boolQuery["must_not"]; ok {
		t.Fatalf("我的文章默认不应再回退 status 排除删除态: %#v", boolQuery["must_not"])
	}
}

func TestBuildSelfArticleSearchQueryWithStatus(t *testing.T) {
	query := buildSelfArticleSearchQuery("", 99, enum.ArticleStatusDraft)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}

	filters, ok := boolQuery["filter"].([]any)
	if !ok || len(filters) != 2 {
		t.Fatalf("我的文章指定状态过滤条件异常: %#v", boolQuery["filter"])
	}
	statusFilter, ok := filters[1].(map[string]any)
	if !ok {
		t.Fatalf("状态过滤结构错误: %#v", filters[1])
	}
	statusTerm, ok := statusFilter["term"].(map[string]any)
	if !ok || statusTerm["publish_status"] != enum.ArticleStatusDraft {
		t.Fatalf("我的文章指定状态过滤错误: %#v", statusFilter)
	}
	if _, ok = boolQuery["must_not"]; ok {
		t.Fatalf("指定状态后不应再保留默认排除条件: %#v", boolQuery["must_not"])
	}
}

func TestBuildAdminArticleSearchQueryDefaultStatus(t *testing.T) {
	query := buildAdminArticleSearchQuery("golang", 0)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}
	if _, ok = boolQuery["filter"]; ok {
		t.Fatalf("管理员默认搜索不应限制文章状态: %#v", boolQuery["filter"])
	}
	if _, ok = boolQuery["must_not"]; ok {
		t.Fatalf("管理员默认搜索不应排除文章状态: %#v", boolQuery["must_not"])
	}
}

func TestBuildAdminArticleSearchQueryWithStatus(t *testing.T) {
	query := buildAdminArticleSearchQuery("", enum.ArticleStatusRejected)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}

	filters, ok := boolQuery["filter"].([]any)
	if !ok || len(filters) != 1 {
		t.Fatalf("管理员指定状态过滤条件异常: %#v", boolQuery["filter"])
	}
	statusFilter, ok := filters[0].(map[string]any)
	if !ok {
		t.Fatalf("管理员状态过滤结构错误: %#v", filters[0])
	}
	statusTerm, ok := statusFilter["term"].(map[string]any)
	if !ok || statusTerm["publish_status"] != enum.ArticleStatusRejected {
		t.Fatalf("管理员指定状态过滤错误: %#v", statusFilter)
	}
}

func TestBuildAdminTopQuery(t *testing.T) {
	query := buildDefaultArticleSearchQuery("")
	query = buildAdminTopQuery(query)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}
	should, ok := boolQuery["should"].([]any)
	if !ok || len(should) != 1 {
		t.Fatalf("管理员置顶加权条件异常: %#v", boolQuery["should"])
	}

	term, ok := should[0].(map[string]any)
	if !ok {
		t.Fatalf("管理员置顶 term 结构错误: %#v", should[0])
	}
	adminTopTerm, ok := term["term"].(map[string]any)
	if !ok {
		t.Fatalf("管理员置顶条件异常: %#v", term)
	}
	adminTopValue, ok := adminTopTerm["admin_top"].(map[string]any)
	if !ok || adminTopValue["value"] != true {
		t.Fatalf("管理员置顶字段异常: %#v", adminTopTerm["admin_top"])
	}
	if adminTopValue["boost"] != 100 {
		t.Fatalf("管理员置顶 boost 异常: %#v", adminTopValue["boost"])
	}
}

func TestBuildAuthorAdminTopQuery(t *testing.T) {
	query := buildDefaultArticleSearchQuery("")
	query = buildUserIDQuery(query, 12)
	query = buildAuthorAdminTopQuery(query)

	boolQuery, ok := extractSearchBoolQuery(query)
	if !ok {
		t.Fatalf("bool 查询结构错误: %#v", query)
	}
	should, ok := boolQuery["should"].([]any)
	if !ok || len(should) != 2 {
		t.Fatalf("作者置顶加权条件异常: %#v", boolQuery["should"])
	}
}

func TestBuildArticleSearchExtraBody(t *testing.T) {
	defaultExtraBody := buildArticleSearchExtraBody("", "go")
	sourceFields, ok := defaultExtraBody["_source"].([]string)
	if !ok || len(sourceFields) == 0 {
		t.Fatalf("_source 白名单异常: %#v", defaultExtraBody["_source"])
	}
	hasContentParts := false
	for _, field := range sourceFields {
		if field == "content_parts" {
			hasContentParts = true
			break
		}
	}
	if !hasContentParts {
		t.Fatalf("_source 白名单缺少 content_parts: %#v", sourceFields)
	}
	highlightMap, ok := defaultExtraBody["highlight"].(map[string]any)
	if !ok {
		t.Fatalf("highlight 结构错误: %#v", defaultExtraBody["highlight"])
	}
	highlightFields, ok := highlightMap["fields"].(map[string]any)
	if !ok {
		t.Fatalf("highlight.fields 结构错误: %#v", highlightMap["fields"])
	}
	if _, ok = highlightFields["content_parts.content"]; !ok {
		t.Fatalf("有关键词时应高亮 content_parts.content: %#v", highlightFields)
	}
	defaultSortList, ok := defaultExtraBody["sort"].([]any)
	if !ok || len(defaultSortList) != 1 {
		t.Fatalf("默认排序条件异常: %#v", defaultExtraBody["sort"])
	}

	noKeyExtraBody := buildArticleSearchExtraBody("", "")
	noKeySourceFields, ok := noKeyExtraBody["_source"].([]string)
	if !ok {
		t.Fatalf("空关键词 _source 结构错误: %#v", noKeyExtraBody["_source"])
	}
	for _, field := range noKeySourceFields {
		if field == "content_head" || field == "content_parts" {
			t.Fatalf("空关键词时不应请求正文相关字段: %#v", noKeySourceFields)
		}
	}
	noKeyHighlightMap, ok := noKeyExtraBody["highlight"].(map[string]any)
	if !ok {
		t.Fatalf("空关键词 highlight 结构错误: %#v", noKeyExtraBody["highlight"])
	}
	noKeyHighlightFields, ok := noKeyHighlightMap["fields"].(map[string]any)
	if !ok {
		t.Fatalf("空关键词 highlight.fields 结构错误: %#v", noKeyHighlightMap["fields"])
	}
	if _, ok = noKeyHighlightFields["content_head"]; ok {
		t.Fatalf("空关键词时不应高亮 content_head: %#v", noKeyHighlightFields)
	}
	if _, ok = noKeyHighlightFields["content_parts.content"]; ok {
		t.Fatalf("空关键词时不应高亮 content_parts.content: %#v", noKeyHighlightFields)
	}

	extraBody := buildArticleSearchExtraBody("view_count", "go")
	sortList, ok := extraBody["sort"].([]any)
	if !ok || len(sortList) != 2 {
		t.Fatalf("排序条件异常: %#v", extraBody["sort"])
	}
}

func TestExtractArticleSearchResults(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	redisDeps := redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}
	db := testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.CategoryModel{}, &models.ArticleModel{})
	createdAt := time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)
	if err := redis_article.SetCacheView(redisDeps, 1, 3); err != nil {
		t.Fatalf("设置浏览增量失败: %v", err)
	}
	if err := redis_article.SetCacheDigg(redisDeps, 1, 2); err != nil {
		t.Fatalf("设置点赞增量失败: %v", err)
	}
	if err := redis_article.SetCacheFavorite(redisDeps, 1, 4); err != nil {
		t.Fatalf("设置收藏增量失败: %v", err)
	}
	if err := redis_article.SetCacheComment(redisDeps, 1, 5); err != nil {
		t.Fatalf("设置评论增量失败: %v", err)
	}

	user := models.UserModel{
		Username: "search_author",
		Password: "x",
		Nickname: "作者昵称",
		Avatar:   "/avatar.png",
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}

	category := models.CategoryModel{
		Title:  "Go 分类",
		UserID: user.ID,
	}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("创建分类失败: %v", err)
	}

	article := models.ArticleModel{
		Model:         models.Model{ID: 1},
		Title:         "db article",
		Content:       "db markdown content",
		CategoryID:    &category.ID,
		AuthorID:      user.ID,
		PublishStatus: enum.ArticleStatusPublished,
		Abstract:      "db abstract",
		ContentHead:   "db content head",
		ViewCount:     10,
		DiggCount:     20,
		FavorCount:    30,
		CommentCount:  40,
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	data := map[string]any{
		"hits": []any{
			map[string]any{
				"_source": map[string]any{
					"id":           "1",
					"created_at":   createdAt.Format(time.RFC3339Nano),
					"updated_at":   updatedAt.Format(time.RFC3339Nano),
					"title":        "go search article",
					"abstract":     "hello world",
					"content_head": "origin content head",
					"content_parts": []any{
						map[string]any{
							"order":   0,
							"level":   1,
							"title":   "一级标题",
							"path":    []any{"一级标题"},
							"content": "一级标题\n正文一",
						},
						map[string]any{
							"order":   1,
							"level":   2,
							"title":   "二级标题",
							"path":    []any{"一级标题", "二级标题"},
							"content": "二级标题\n正文二",
						},
					},
					"cover":             "/cover.png",
					"view_count":        10,
					"digg_count":        20,
					"favor_count":       30,
					"comment_count":     40,
					"comments_toggle":   true,
					"publish_status":    int(enum.ArticleStatusPublished),
					"visibility_status": string(enum.ArticleVisibilityVisible),
					"category": map[string]any{
						"id":    "9",
						"title": "Go 分类",
					},
					"author": map[string]any{
						"id":       "8",
						"nickname": "作者昵称",
						"avatar":   "/avatar.png",
					},
					"tags": []any{
						map[string]any{"id": "1", "title": "Go"},
						map[string]any{"id": "2", "title": "ES"},
					},
					"author_top": true,
					"admin_top":  true,
				},
				"highlight": map[string]any{
					"title":                 []any{"<em>go</em> search"},
					"abstract":              []any{"<em>hello</em> world", "another <em>piece</em>"},
					"content_parts.content": []any{"prefix <em>go</em> suffix", "second <em>segment</em>"},
				},
			},
		},
	}

	list := extractArticleSearchResults(redisDeps, data)
	if len(list) != 1 {
		t.Fatalf("结果数量错误: %d", len(list))
	}
	if list[0].ID != 1 || list[0].Title != "go search article" {
		t.Fatalf("文章解析错误: %+v", list[0])
	}
	if !list[0].CreatedAt.Equal(createdAt) || !list[0].UpdatedAt.Equal(updatedAt) {
		t.Fatalf("时间字段解析错误: %+v", list[0])
	}
	if list[0].Abstract != "hello world" {
		t.Fatalf("摘要字段解析错误: %+v", list[0])
	}
	if list[0].Content != "prefix <em>go</em> suffix" {
		t.Fatalf("正文摘要回填错误: %+v", list[0])
	}
	if list[0].Highlight == nil || list[0].Highlight.Title != "<em>go</em> search" || list[0].Highlight.Abstract != "<em>hello</em> world" {
		t.Fatalf("高亮字段解析错误: %+v", list[0].Highlight)
	}
	expectedParts := []markdown.ContentPart{
		{
			Level: 1,
			Title: "一级标题",
			Path:  []string{"一级标题"},
		},
		{
			Level: 2,
			Title: "二级标题",
			Path:  []string{"一级标题", "二级标题"},
		},
	}
	if len(list[0].Part) != len(expectedParts) {
		t.Fatalf("正文分段数量错误: %+v", list[0].Part)
	}
	for index, expected := range expectedParts {
		if list[0].Part[index].Level != expected.Level ||
			list[0].Part[index].Title != expected.Title {
			t.Fatalf("正文分段解析错误 index=%d got=%+v expected=%+v", index, list[0].Part[index], expected)
		}
		if len(list[0].Part[index].Path) != len(expected.Path) {
			t.Fatalf("正文分段路径长度错误 index=%d got=%+v expected=%+v", index, list[0].Part[index].Path, expected.Path)
		}
		for pathIndex, pathItem := range expected.Path {
			if list[0].Part[index].Path[pathIndex] != pathItem {
				t.Fatalf("正文分段路径解析错误 index=%d got=%+v expected=%+v", index, list[0].Part[index].Path, expected.Path)
			}
		}
	}
	partRaw, err := json.Marshal(list[0].Part[0])
	if err != nil {
		t.Fatalf("正文分段序列化失败: %v", err)
	}
	if string(partRaw) != `{"level":1,"title":"一级标题","path":["一级标题"]}` {
		t.Fatalf("正文分段返回字段错误: %s", string(partRaw))
	}
	if list[0].Cover != "/cover.png" || !list[0].CommentsToggle {
		t.Fatalf("基础字段解析错误: %+v", list[0])
	}
	if list[0].PublishStatus != enum.ArticleStatusPublished || list[0].VisibilityStatus != enum.ArticleVisibilityVisible {
		t.Fatalf("发布/可见状态解析错误: %+v", list[0])
	}
	if len(list[0].Tags) != 2 || list[0].Tags[0].Title != "Go" {
		t.Fatalf("标签解析错误: %+v", list[0].Tags)
	}
	if list[0].ViewCount != 13 || list[0].DiggCount != 22 || list[0].FavorCount != 34 || list[0].CommentCount != 45 {
		t.Fatalf("Redis 增量叠加错误: %+v", list[0])
	}
	if !list[0].UserTop || !list[0].AdminTop {
		t.Fatalf("置顶标记解析错误: %+v", list[0])
	}
	if list[0].CategoryTitle != "Go 分类" {
		t.Fatalf("分类标题回填错误: %+v", list[0])
	}
	if list[0].UserNickname != "作者昵称" || list[0].UserAvatar != "/avatar.png" {
		t.Fatalf("作者信息回填错误: %+v", list[0])
	}
	if list[0].Category == nil || list[0].Category.Title != "Go 分类" {
		t.Fatalf("分类对象回填错误: %+v", list[0].Category)
	}
	if list[0].Author.Nickname != "作者昵称" || list[0].Author.Avatar != "/avatar.png" {
		t.Fatalf("作者对象回填错误: %+v", list[0].Author)
	}
	if list[0].Top == nil || !list[0].Top.User || !list[0].Top.Admin {
		t.Fatalf("置顶对象解析错误: %+v", list[0].Top)
	}
}

func TestExtractArticleSearchResultsKeepsSnowflakeIDPrecision(t *testing.T) {
	_ = testutil.SetupMiniRedis(t)
	redisDeps := redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}
	data := map[string]any{
		"hits": []any{
			map[string]any{
				"_source": map[string]any{
					"id":          "301850494807052288",
					"title":       "precision test",
					"abstract":    "abstract",
					"author_id":   "301850494807052289",
					"category_id": "301850494807052290",
					"category": map[string]any{
						"id":    "301850494807052290",
						"title": "分类",
					},
					"author": map[string]any{
						"id":       "301850494807052289",
						"nickname": "作者",
						"avatar":   "/avatar.png",
					},
					"publish_status":    json.Number("3"),
					"visibility_status": "visible",
				},
			},
		},
	}

	list := extractArticleSearchResults(redisDeps, data)
	if len(list) != 1 {
		t.Fatalf("结果数量错误: %d", len(list))
	}
	if got := list[0].ID.String(); got != "301850494807052288" {
		t.Fatalf("文章 ID 不应丢失精度, got=%s", got)
	}
	if got := list[0].Author.ID.String(); got != "301850494807052289" {
		t.Fatalf("作者 ID 不应丢失精度, got=%s", got)
	}
	if list[0].Category == nil || list[0].Category.ID.String() != "301850494807052290" {
		t.Fatalf("分类 ID 不应丢失精度, got=%+v", list[0].Category)
	}
}
