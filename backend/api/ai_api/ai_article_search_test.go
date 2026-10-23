package ai_api_test

import (
	"encoding/json"
	"io"
	"myblogx/api/ai_api"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/enum"
	"myblogx/service/ai_service"
	"myblogx/service/search_service"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v7"
)

func TestAIArticleSearchView(t *testing.T) {
	db := testutil.SetupSQLite(
		t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.CategoryModel{},
		&models.ArticleModel{},
		&models.TagModel{},
	)

	user := models.UserModel{
		Username: "search_user",
		Password: "x",
		Nickname: "搜索用户",
		Avatar:   "/avatar.png",
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	category := models.CategoryModel{
		Title:  "Go 分类",
		UserID: user.ID,
	}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("创建分类失败: %v", err)
	}

	if err := db.Create(&models.TagModel{Title: "Go", IsEnabled: true}).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}
	if err := db.Create(&models.TagModel{Title: "Gin", IsEnabled: true}).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}

	article1 := models.ArticleModel{
		Model:         models.Model{ID: 1},
		Title:         "Gin 中间件实践",
		Abstract:      "讲 Gin 中间件",
		Content:       "Gin 中间件正文",
		CategoryID:    &category.ID,
		AuthorID:      user.ID,
		PublishStatus: enum.ArticleStatusPublished,
	}
	if err := db.Create(&article1).Error; err != nil {
		t.Fatalf("创建文章1失败: %v", err)
	}

	article2 := models.ArticleModel{
		Model:         models.Model{ID: 2},
		Title:         "Go Web 基础",
		Abstract:      "讲 Go Web",
		Content:       "Go Web 正文",
		CategoryID:    &category.ID,
		AuthorID:      user.ID,
		PublishStatus: enum.ArticleStatusPublished,
	}
	if err := db.Create(&article2).Error; err != nil {
		t.Fatalf("创建文章2失败: %v", err)
	}

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ai_service.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析 AI 请求失败: %v", err)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("AI 请求消息数量错误: %+v", req.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role": "assistant",
						"content": `{
							"intent":"search",
							"query":["Gin","中间件"],
							"tag_list":["Gin"],
							"sort":1
						}`,
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer aiServer.Close()

	esSearchIndex := 0
	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			writeMockESJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
			return
		}

		esSearchIndex++
		if esSearchIndex == 1 {
			writeMockESJSON(w, 200, `{
				"hits":{
					"total":{"value":1},
					"hits":[
						{"_source":{
							"id":"1",
							"title":"Gin 中间件实践",
							"abstract":"讲 Gin 中间件",
							"cover":"",
							"view_count":1,
							"digg_count":2,
							"comment_count":3,
							"favor_count":4,
							"comments_toggle":true,
							"status":2,
							"tags":[{"title":"Gin"}],
							"author_top":false,
							"admin_top":false
						}}
					]
				}
			}`)
			return
		}

		writeMockESJSON(w, 200, `{
			"hits":{
				"total":{"value":2},
				"hits":[
					{"_source":{
						"id":"1",
						"title":"Gin 中间件实践",
						"abstract":"讲 Gin 中间件",
						"cover":"",
						"view_count":1,
						"digg_count":2,
						"comment_count":3,
						"favor_count":4,
						"comments_toggle":true,
						"status":2,
						"tags":[{"title":"Gin"}],
						"author_top":false,
						"admin_top":false
					}},
					{"_source":{
						"id":"2",
						"title":"Go Web 基础",
						"abstract":"讲 Go Web",
						"cover":"",
						"view_count":5,
						"digg_count":6,
						"comment_count":7,
						"favor_count":8,
						"comments_toggle":true,
						"status":2,
						"tags":[{"title":"Go"}],
						"author_top":false,
						"admin_top":false
					}}
				]
			}
		}`)
	}))
	defer esServer.Close()

	esClient, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{esServer.URL},
	})
	if err != nil {
		t.Fatalf("创建 ES 客户端失败: %v", err)
	}
	testutil.SetESClient(esClient)

	testutil.SetConfig(&conf.Config{
		AI: conf.AI{
			Enable:        true,
			SecretKey:     "test-key",
			BaseURL:       aiServer.URL,
			ChatModel:     "test-model",
			MaxInputChars: 2048,
		},
		ES: conf.ES{
			Index: "article_index",
		},
	})

	api := newAIApi(t)
	c, w := newAICtx()
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     enum.RoleUser,
			Username: user.Username,
		},
	})
	c.Set("requestJson", ai_api.AIBaseRequest{
		Content: "帮我找讲 Gin 中间件的文章",
	})

	api.AIArticleSearchListView(c)

	if code := readAICode(t, w); code != 0 {
		t.Fatalf("AI 文章搜索应成功, body=%s", w.Body.String())
	}

	var body struct {
		Code int                                  `json:"code"`
		Data search_service.ArticleSearchResponse `json:"data"`
		Msg  string                               `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}

	if len(body.Data.List) != 2 {
		t.Fatalf("结果数量错误: %+v", body.Data)
	}
	if body.Data.List[0].ID != 1 || body.Data.List[1].ID != 2 {
		t.Fatalf("结果去重合并顺序错误: %+v", body.Data)
	}
	if body.Data.Pagination.Mode != search_service.PageModeHasMore || body.Data.Pagination.Page != 1 || body.Data.Pagination.Limit != 10 {
		t.Fatalf("分页信息应与 /api/search/articles 一致: %+v", body.Data.Pagination)
	}
	if body.Data.Pagination.HasMore {
		t.Fatalf("AI 搜索列表不支持继续翻页，has_more 应固定为 false: %+v", body.Data.Pagination)
	}
}

func TestAIArticleSearchLLMViewForOtherIntent(t *testing.T) {
	testutil.SetupSQLite(t, &models.TagModel{})

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role": "assistant",
						"content": `{
							"intent":"other",
							"content":"这看起来不是搜文章，你可以直接告诉我想找什么主题的文章。",
							"query":[],
							"tag_list":[],
							"sort":1
						}`,
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer aiServer.Close()

	testutil.SetConfig(&conf.Config{
		AI: conf.AI{
			Enable:    true,
			SecretKey: "test-key",
			BaseURL:   aiServer.URL,
			ChatModel: "test-model",
		},
	})

	api := newAIApi(t)
	c, w := newAICtx()
	c.Set("requestJson", ai_api.AIBaseRequest{
		Content: "你好呀",
	})

	api.AIArticleSearchLLMView(c)

	eventList := readSSEEvents(t, w.Body.String())
	if len(eventList) == 0 {
		t.Fatalf("非搜索意图应返回 SSE 数据, body=%s", w.Body.String())
	}
	last := eventList[len(eventList)-1]
	if last.Code != 0 {
		t.Fatalf("非搜索意图应成功返回普通回复, body=%s", w.Body.String())
	}
	var data ai_api.AIBaseResponse
	if err := json.Unmarshal(last.Data, &data); err != nil {
		t.Fatalf("解析 SSE data 失败: %v body=%s", err, w.Body.String())
	}
	if !strings.Contains(data.Content, "不是搜文章") {
		t.Fatalf("普通回复内容错误: %+v", data)
	}
}

func TestAIArticleSearchLLMViewForSearchIntent(t *testing.T) {
	db := testutil.SetupSQLite(
		t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.CategoryModel{},
		&models.ArticleModel{},
		&models.TagModel{},
	)

	user := models.UserModel{
		Username: "search_user",
		Password: "x",
		Nickname: "搜索用户",
		Avatar:   "/avatar.png",
		Role:     enum.RoleUser,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	category := models.CategoryModel{
		Title:  "Go 分类",
		UserID: user.ID,
	}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("创建分类失败: %v", err)
	}
	if err := db.Create(&models.TagModel{Title: "Gin", IsEnabled: true}).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}

	article := models.ArticleModel{
		Model:         models.Model{ID: 9},
		Title:         "Gin 中间件实践",
		Abstract:      "讲 Gin 中间件",
		Content:       "Gin 中间件正文",
		CategoryID:    &category.ID,
		AuthorID:      user.ID,
		PublishStatus: enum.ArticleStatusPublished,
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	callIndex := 0
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callIndex++
		var req ai_service.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析 AI 请求失败: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if callIndex == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"content": `{
								"intent":"search",
								"content":"",
								"query":["Gin","中间件"],
								"tag_list":["Gin"],
								"sort":1
							}`,
						},
						"finish_reason": "stop",
					},
				},
			})
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"以下是关于 Gin 中间件的文章：\\n\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"1. [Gin 中间件实践](/article/9)\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer aiServer.Close()

	esServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			writeMockESJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
			return
		}

		writeMockESJSON(w, 200, `{
			"hits":{
				"total":{"value":1},
				"hits":[
					{"_source":{
						"id":`+strconv.FormatUint(uint64(article.ID), 10)+`,
						"title":"Gin 中间件实践",
						"abstract":"讲 Gin 中间件",
						"cover":"",
						"view_count":1,
						"digg_count":2,
						"comment_count":3,
						"favor_count":4,
						"comments_toggle":true,
						"status":2,
						"tags":[{"title":"Gin"}],
						"author_top":false,
						"admin_top":false
					}}
				]
			}
		}`)
	}))
	defer esServer.Close()

	esClient, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{esServer.URL},
	})
	if err != nil {
		t.Fatalf("创建 ES 客户端失败: %v", err)
	}
	testutil.SetESClient(esClient)

	testutil.SetConfig(&conf.Config{
		AI: conf.AI{
			Enable:    true,
			SecretKey: "test-key",
			BaseURL:   aiServer.URL,
			ChatModel: "test-model",
		},
		ES: conf.ES{
			Index: "article_index",
		},
	})

	api := newAIApi(t)
	c, w := newAICtx()
	c.Set("requestJson", ai_api.AIBaseRequest{
		Content: "帮我找 Gin 中间件相关文章",
	})

	api.AIArticleSearchLLMView(c)

	eventList := readSSEEvents(t, w.Body.String())
	if len(eventList) == 0 {
		t.Fatalf("搜索意图应返回 SSE 数据, body=%s", w.Body.String())
	}
	last := eventList[len(eventList)-1]
	if last.Code != 0 {
		t.Fatalf("搜索意图 LLM 回复应成功, body=%s", w.Body.String())
	}
	var data ai_api.AIBaseResponse
	if err := json.Unmarshal(last.Data, &data); err != nil {
		t.Fatalf("解析 SSE data 失败: %v body=%s", err, w.Body.String())
	}
	if !strings.Contains(data.Content, `/article/9`) {
		t.Fatalf("LLM 搜索回复内容错误: %+v", data)
	}
	if strings.Contains(data.Content, "<a href=") {
		t.Fatalf("LLM 搜索回复不应包含 HTML 链接: %+v", data)
	}
	if !strings.Contains(data.Content, "[Gin 中间件实践](/article/9)") {
		t.Fatalf("LLM 搜索回复应包含 Markdown 链接: %+v", data)
	}
}

func writeMockESJSON(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Elastic-Product", "Elasticsearch")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, body)
}

type sseResponseBody struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

func readSSEEvents(t *testing.T, body string) []sseResponseBody {
	t.Helper()

	lines := strings.Split(body, "\n")
	result := make([]sseResponseBody, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}

		var item sseResponseBody
		if err := json.Unmarshal([]byte(payload), &item); err != nil {
			t.Fatalf("解析 SSE 事件失败: %v body=%s", err, body)
		}
		result = append(result, item)
	}
	return result
}
