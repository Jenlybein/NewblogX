package articlesync

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"myblogx/conf"
	confsite "myblogx/conf/site"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/test/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
)

func setupMockES(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{srv.URL},
	})
	if err != nil {
		t.Fatalf("创建 mock ES 客户端失败: %v", err)
	}

	old := testutil.ESClient()
	testutil.SetESClient(client)
	t.Cleanup(func() {
		testutil.SetESClient(old)
		srv.Close()
	})
}

func writeJSON(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Elastic-Product", "Elasticsearch")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, body)
}

func TestUpdateESDocsContent(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&UserConfRow{},
		&ArticleRow{},
		&TagRow{},
		&ArticleTagRow{},
		&UserTopArticleRow{},
	)
	testutil.SetConfig(&conf.Config{
		ES: conf.ES{Index: "article_index"},
		Site: conf.Site{
			SiteInfo: confsite.SiteInfo{Mode: enum.SiteModeCommunity},
		},
	})

	user := UserRow{Username: "author", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	article := ArticleRow{
		Title:         "t1",
		Abstract:      "摘要",
		Content:       "# 新标题\n新正文\n[错误链接](### 新标题)",
		AuthorID:      user.ID,
		PublishStatus: enum.ArticleStatusExamining,
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	var bulkDocs []map[string]any
	setupMockES(t, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/":
			writeJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case path == "/_bulk" || strings.HasSuffix(path, "/_bulk"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("读取 bulk body 失败: %v", err)
			}
			scanner := bufio.NewScanner(strings.NewReader(string(body)))
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				line := scanner.Bytes()
				if len(strings.TrimSpace(string(line))) == 0 {
					continue
				}
				if lineNo%2 == 0 {
					var doc map[string]any
					if err = json.Unmarshal(line, &doc); err != nil {
						t.Fatalf("解析 bulk 文档失败: %v", err)
					}
					bulkDocs = append(bulkDocs, doc)
				}
			}
			writeJSON(w, 200, `{"took":1,"errors":false,"items":[]}`)
		default:
			writeJSON(w, 404, `{"error":{"reason":"not found"}}`)
		}
	})

	if err := UpdateESDocsContent(db, testutil.ESClient(), []ctype.ID{article.ID}); err != nil {
		t.Fatalf("UpdateESDocsContent 失败: %v", err)
	}
	if len(bulkDocs) != 1 {
		t.Fatalf("应重建一篇 ES 文档, got=%d", len(bulkDocs))
	}

	docMap, ok := bulkDocs[0]["doc"].(map[string]any)
	if !ok {
		t.Fatalf("应使用 partial update 文档结构, got=%#v", bulkDocs[0])
	}
	parts, ok := docMap["content_parts"].([]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("content_parts 同步错误: %#v", docMap["content_parts"])
	}
	firstPart, ok := parts[0].(map[string]any)
	if !ok {
		t.Fatalf("content_parts 首段结构错误: %#v", parts[0])
	}
	content, _ := firstPart["content"].(string)
	if !strings.Contains(content, "新标题") || !strings.Contains(content, "新正文") {
		t.Fatalf("content_parts 未同步最新正文: %q", content)
	}
	if strings.Contains(content, "](### ") {
		t.Fatalf("content_parts 不应保留错误链接语法: %q", content)
	}
	if _, ok = docMap["tags"]; ok {
		t.Fatalf("UpdateESDocsContent 不应更新 tags: %#v", docMap)
	}
}

func TestUpdateESDocsTags(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&UserConfRow{},
		&ArticleRow{},
		&TagRow{},
		&ArticleTagRow{},
	)
	testutil.SetConfig(&conf.Config{ES: conf.ES{Index: "article_index"}})

	user := UserRow{Username: "author", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	tagGo := TagRow{Title: "Go", IsEnabled: true}
	tagES := TagRow{Title: "ES", IsEnabled: true}
	if err := db.Create(&tagGo).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}
	if err := db.Create(&tagES).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}
	article := ArticleRow{Title: "t1", Content: "正文", AuthorID: user.ID, PublishStatus: enum.ArticleStatusExamining}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := db.Model(&article).Association("Tags").Replace([]TagRow{tagGo, tagES}); err != nil {
		t.Fatalf("写入文章标签失败: %v", err)
	}

	var bulkDocs []map[string]any
	setupMockES(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			writeJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case r.URL.Path == "/_bulk" || strings.HasSuffix(r.URL.Path, "/_bulk"):
			body, _ := io.ReadAll(r.Body)
			scanner := bufio.NewScanner(strings.NewReader(string(body)))
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				line := scanner.Bytes()
				if len(strings.TrimSpace(string(line))) == 0 {
					continue
				}
				if lineNo%2 == 0 {
					var doc map[string]any
					if err := json.Unmarshal(line, &doc); err != nil {
						t.Fatalf("解析 bulk 文档失败: %v", err)
					}
					bulkDocs = append(bulkDocs, doc)
				}
			}
			writeJSON(w, 200, `{"took":1,"errors":false,"items":[]}`)
		default:
			writeJSON(w, 404, `{"error":{"reason":"not found"}}`)
		}
	})

	if err := UpdateESDocsTags(db, testutil.ESClient(), []ctype.ID{article.ID}); err != nil {
		t.Fatalf("UpdateESDocsTags 失败: %v", err)
	}
	if len(bulkDocs) != 1 {
		t.Fatalf("应更新一篇 ES 文档, got=%d", len(bulkDocs))
	}
	docMap, ok := bulkDocs[0]["doc"].(map[string]any)
	if !ok {
		t.Fatalf("应使用 partial update 文档结构, got=%#v", bulkDocs[0])
	}
	tags, ok := docMap["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Fatalf("tags 更新错误: %#v", docMap["tags"])
	}
	if _, ok = docMap["content_parts"]; ok {
		t.Fatalf("UpdateESDocsTags 不应更新正文字段: %#v", docMap)
	}
}

func TestUpdateESDocsTop(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&UserConfRow{},
		&ArticleRow{},
		&UserTopArticleRow{},
	)
	testutil.SetConfig(&conf.Config{ES: conf.ES{Index: "article_index"}})

	admin := UserRow{Username: "admin", Password: "x", Role: enum.RoleAdmin}
	author := UserRow{Username: "author", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}
	article := ArticleRow{Title: "t1", Content: "正文", AuthorID: author.ID, PublishStatus: enum.ArticleStatusExamining}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := db.Create(&UserTopArticleRow{UserID: admin.ID, ArticleID: article.ID}).Error; err != nil {
		t.Fatalf("创建管理员置顶失败: %v", err)
	}
	if err := db.Create(&UserTopArticleRow{UserID: author.ID, ArticleID: article.ID}).Error; err != nil {
		t.Fatalf("创建作者置顶失败: %v", err)
	}

	var bulkDocs []map[string]any
	setupMockES(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			writeJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case r.URL.Path == "/_bulk" || strings.HasSuffix(r.URL.Path, "/_bulk"):
			body, _ := io.ReadAll(r.Body)
			scanner := bufio.NewScanner(strings.NewReader(string(body)))
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				line := scanner.Bytes()
				if len(strings.TrimSpace(string(line))) == 0 {
					continue
				}
				if lineNo%2 == 0 {
					var doc map[string]any
					if err := json.Unmarshal(line, &doc); err != nil {
						t.Fatalf("解析 bulk 文档失败: %v", err)
					}
					bulkDocs = append(bulkDocs, doc)
				}
			}
			writeJSON(w, 200, `{"took":1,"errors":false,"items":[]}`)
		default:
			writeJSON(w, 404, `{"error":{"reason":"not found"}}`)
		}
	})

	if err := UpdateESDocsTop(db, testutil.ESClient(), []ctype.ID{article.ID}); err != nil {
		t.Fatalf("UpdateESDocsTop 失败: %v", err)
	}
	if len(bulkDocs) != 1 {
		t.Fatalf("应更新一篇 ES 文档, got=%d", len(bulkDocs))
	}
	docMap, ok := bulkDocs[0]["doc"].(map[string]any)
	if !ok {
		t.Fatalf("应使用 partial update 文档结构, got=%#v", bulkDocs[0])
	}
	if docMap["admin_top"] != true || docMap["author_top"] != true {
		t.Fatalf("置顶字段更新错误: %#v", docMap)
	}
	if _, ok = docMap["tags"]; ok {
		t.Fatalf("UpdateESDocsTop 不应更新其他字段: %#v", docMap)
	}
}

func TestSyncESDocsByArticleSnapshots(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&UserConfRow{},
		&CategoryRow{},
		&TagRow{},
		&ArticleTagRow{},
		&UserTopArticleRow{},
	)
	testutil.SetConfig(&conf.Config{
		ES: conf.ES{Index: "article_index"},
		Site: conf.Site{
			SiteInfo: confsite.SiteInfo{Mode: enum.SiteModeCommunity},
		},
	})

	admin := UserRow{Username: "admin", Password: "x", Role: enum.RoleAdmin, Nickname: "管理员"}
	author := UserRow{Username: "author", Password: "x", Role: enum.RoleUser, Nickname: "作者", Avatar: "/avatar.png"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("创建管理员失败: %v", err)
	}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}
	category := CategoryRow{Title: "后端", UserID: author.ID}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("创建分类失败: %v", err)
	}
	tag := TagRow{Title: "Go", IsEnabled: true}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("创建标签失败: %v", err)
	}

	articleID := ctype.NewID(301900000000000001)
	if err := db.Exec("INSERT INTO article_tag_models(article_id, tag_id) VALUES (?, ?)", articleID, tag.ID).Error; err != nil {
		t.Fatalf("创建文章标签关系失败: %v", err)
	}
	if err := db.Create(&UserTopArticleRow{UserID: admin.ID, ArticleID: articleID}).Error; err != nil {
		t.Fatalf("创建管理员置顶失败: %v", err)
	}
	if err := db.Create(&UserTopArticleRow{UserID: author.ID, ArticleID: articleID}).Error; err != nil {
		t.Fatalf("创建作者置顶失败: %v", err)
	}

	var bulkDocs []map[string]any
	setupMockES(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			writeJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case r.URL.Path == "/_bulk" || strings.HasSuffix(r.URL.Path, "/_bulk"):
			body, _ := io.ReadAll(r.Body)
			scanner := bufio.NewScanner(strings.NewReader(string(body)))
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				line := scanner.Bytes()
				if len(strings.TrimSpace(string(line))) == 0 {
					continue
				}
				if lineNo%2 == 0 {
					var doc map[string]any
					if err := json.Unmarshal(line, &doc); err != nil {
						t.Fatalf("解析 bulk 文档失败: %v", err)
					}
					bulkDocs = append(bulkDocs, doc)
				}
			}
			writeJSON(w, 200, `{"took":1,"errors":false,"items":[]}`)
		default:
			writeJSON(w, 404, `{"error":{"reason":"not found"}}`)
		}
	})

	snapshots := []ArticleRowSnapshot{{
		ID:               articleID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Title:            "新文章",
		Abstract:         "摘要",
		Content:          "# 标题\n正文",
		CategoryID:       &category.ID,
		Cover:            "/cover.png",
		AuthorID:         author.ID,
		ViewCount:        1,
		DiggCount:        2,
		CommentCount:     3,
		FavorCount:       4,
		CommentsToggle:   true,
		PublishStatus:    enum.ArticleStatusPublished,
		VisibilityStatus: enum.ArticleVisibilityVisible,
	}}
	if err := SyncESDocsByArticleSnapshots(db, testutil.ESClient(), snapshots); err != nil {
		t.Fatalf("SyncESDocsByArticleSnapshots 失败: %v", err)
	}
	if len(bulkDocs) != 1 {
		t.Fatalf("应写入一篇 ES 文档, got=%d", len(bulkDocs))
	}

	doc := bulkDocs[0]
	if doc["title"] != "新文章" {
		t.Fatalf("文章标题应直接来自快照, got=%#v", doc["title"])
	}
	categoryDoc, ok := doc["category"].(map[string]any)
	if !ok || categoryDoc["title"] != "后端" {
		t.Fatalf("分类快照补齐失败: %#v", doc["category"])
	}
	authorDoc, ok := doc["author"].(map[string]any)
	if !ok || authorDoc["nickname"] != "作者" {
		t.Fatalf("作者快照补齐失败: %#v", doc["author"])
	}
	tags, ok := doc["tags"].([]any)
	if !ok || len(tags) != 1 {
		t.Fatalf("标签补齐失败: %#v", doc["tags"])
	}
	if doc["admin_top"] != true || doc["author_top"] != true {
		t.Fatalf("置顶补齐失败: admin=%#v author=%#v", doc["admin_top"], doc["author_top"])
	}
}

func TestUpdateESDocsByArticleDeltasRebuildsMissingDocs(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&UserConfRow{},
		&ArticleRow{},
		&TagRow{},
		&ArticleTagRow{},
		&UserTopArticleRow{},
	)
	testutil.SetConfig(&conf.Config{
		ES: conf.ES{Index: "article_index"},
		Site: conf.Site{
			SiteInfo: confsite.SiteInfo{Mode: enum.SiteModeCommunity},
		},
	})

	user := UserRow{Username: "author", Password: "x", Role: enum.RoleUser, Nickname: "作者", Avatar: "/avatar.png"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	article := ArticleRow{
		Title:            "旧标题",
		Abstract:         "旧摘要",
		Content:          "旧内容",
		AuthorID:         user.ID,
		PublishStatus:    enum.ArticleStatusPublished,
		VisibilityStatus: enum.ArticleVisibilityVisible,
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := db.Model(&article).Updates(map[string]any{
		"title":    "新标题",
		"abstract": "新摘要",
		"content":  "新内容",
	}).Error; err != nil {
		t.Fatalf("更新文章失败: %v", err)
	}

	bulkBodies := make([]string, 0, 2)
	setupMockES(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			writeJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case r.URL.Path == "/_bulk" || strings.HasSuffix(r.URL.Path, "/_bulk"):
			body, _ := io.ReadAll(r.Body)
			payload := string(body)
			bulkBodies = append(bulkBodies, payload)
			if strings.Contains(payload, "\"update\"") {
				writeJSON(w, 200, fmt.Sprintf(`{"took":1,"errors":true,"items":[{"update":{"_id":"%s","status":404,"error":{"type":"document_missing_exception","reason":"missing"}}}]}`, article.ID.String()))
				return
			}
			writeJSON(w, 200, `{"took":1,"errors":false,"items":[{"index":{"_id":"ok","status":201}}]}`)
		default:
			writeJSON(w, 404, `{"error":{"reason":"not found"}}`)
		}
	})

	err := UpdateESDocsByArticleDeltas(db, testutil.ESClient(), []ArticleModelDelta{{
		ArticleID: article.ID,
		Changed: map[string]any{
			"title":    "新标题",
			"abstract": "新摘要",
			"content":  "新内容",
		},
	}})
	if err != nil {
		t.Fatalf("缺文档时应自动回源重建, got err=%v", err)
	}
	if len(bulkBodies) != 2 {
		t.Fatalf("应先局部更新再回源重建, got=%d", len(bulkBodies))
	}
	if !strings.Contains(bulkBodies[0], "\"update\"") {
		t.Fatalf("第一轮应为 update 请求: %s", bulkBodies[0])
	}
	if !strings.Contains(bulkBodies[1], "\"index\"") || !strings.Contains(bulkBodies[1], "新标题") {
		t.Fatalf("第二轮应为完整 index 重建: %s", bulkBodies[1])
	}
}

func TestSyncESDocsReturnsErrorWhenArticlesMissing(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&UserConfRow{},
		&ArticleRow{},
		&TagRow{},
		&ArticleTagRow{},
		&UserTopArticleRow{},
	)
	testutil.SetConfig(&conf.Config{
		ES: conf.ES{Index: "article_index"},
		Site: conf.Site{
			SiteInfo: confsite.SiteInfo{Mode: enum.SiteModeCommunity},
		},
	})

	setupMockES(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			writeJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case r.URL.Path == "/_bulk" || strings.HasSuffix(r.URL.Path, "/_bulk"):
			writeJSON(w, 200, `{"took":1,"errors":false,"items":[]}`)
		default:
			writeJSON(w, 404, `{"error":{"reason":"not found"}}`)
		}
	})

	missingID := ctype.NewID(301829212698841088)
	err := SyncESDocs(db, testutil.ESClient(), []ctype.ID{missingID})
	if err == nil {
		t.Fatal("文章不存在时应返回错误，避免 river 静默吞掉 upsert 事件")
	}
	if !strings.Contains(err.Error(), "未加载到任何文章") {
		t.Fatalf("错误信息应指向缺失文章，got=%v", err)
	}
}
