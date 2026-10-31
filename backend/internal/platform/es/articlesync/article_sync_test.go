package articlesync

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"myblogx/conf"
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

func TestUpdateESDocsTags(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
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

// setupMockESCollectingBulkDocs 起一个只负责记录 bulk 文档体的 mock ES。
//
// 与各用例里手写的那份相比，它只做"把发出去的文档收下来"这一件事，
// 让新增用例的正文只剩下断言。
func setupMockESCollectingBulkDocs(t *testing.T) *[]map[string]any {
	t.Helper()

	docs := make([]map[string]any, 0, 2)
	setupMockES(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			writeJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case strings.HasSuffix(r.URL.Path, "/_bulk"):
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
					docs = append(docs, doc)
				}
			}
			writeJSON(w, 200, `{"took":1,"errors":false,"items":[]}`)
		default:
			writeJSON(w, 404, `{"error":{"reason":"not found"}}`)
		}
	})
	return &docs
}

// 用户角色变更必须让该作者的文章文档重算 author_is_admin。
//
// 走的是已有分支：river 收到 user_models 行变更 → ArticleSearchProjectionAuthorSnapshot
// → SyncESDocsByAuthorIDs（ADR-0017 决策 4 明确要复用这条路径，不新增事件）。
func TestSyncArticleSearchProjectionRefreshesAuthorIsAdmin(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&ArticleRow{},
		&TagRow{},
		&ArticleTagRow{},
	)
	testutil.SetConfig(&conf.Config{ES: conf.ES{Index: "article_index"}})

	author := UserRow{Username: "author", Password: "x", Role: enum.RoleUser, Nickname: "作者", Avatar: "/avatar.png"}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}
	article := ArticleRow{Title: "t1", Content: "正文", AuthorID: author.ID, PublishStatus: enum.ArticleStatusPublished}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	// 角色从注册用户升到管理员：文档里的 author_is_admin 必须跟着变。
	if err := db.Model(&author).Update("role", enum.RoleAdmin).Error; err != nil {
		t.Fatalf("升级作者为管理员失败: %v", err)
	}

	bulkDocs := setupMockESCollectingBulkDocs(t)

	if err := SyncArticleSearchProjection(db, testutil.ESClient(), ArticleSearchProjectionEvent{
		Type: ArticleSearchProjectionAuthorSnapshot,
		IDs:  []ctype.ID{author.ID},
	}); err != nil {
		t.Fatalf("按用户行重算文章文档失败: %v", err)
	}
	if len(*bulkDocs) != 1 {
		t.Fatalf("应更新一篇 ES 文档, got=%d", len(*bulkDocs))
	}
	docMap, ok := (*bulkDocs)[0]["doc"].(map[string]any)
	if !ok {
		t.Fatalf("应使用 partial update 文档结构, got=%#v", (*bulkDocs)[0])
	}
	if got, ok := docMap["author_is_admin"].(bool); !ok || !got {
		t.Fatalf("角色变更后 author_is_admin 应重算为 true, got=%#v", docMap["author_is_admin"])
	}
	authorDoc, ok := docMap["author"].(map[string]any)
	if !ok || authorDoc["nickname"] != "作者" {
		t.Fatalf("作者快照应一并刷新: %#v", docMap["author"])
	}
	for _, key := range []string{"top", "admin_top", "author_top"} {
		if _, ok := docMap[key]; ok {
			t.Fatalf("角色变更不应再写置顶字段 %q: %#v", key, docMap)
		}
	}
}

// 文章换作者时（增量路径）要重算 author_is_admin —— 以前这里重算的是 author_top。
func TestUpdateESDocsByArticleDeltasAuthorChangeRefreshesAuthorIsAdmin(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&ArticleRow{},
		&TagRow{},
		&ArticleTagRow{},
	)
	testutil.SetConfig(&conf.Config{ES: conf.ES{Index: "article_index"}})

	oldAuthor := UserRow{Username: "old", Password: "x", Role: enum.RoleUser}
	newAuthor := UserRow{Username: "new", Password: "x", Role: enum.RoleAdmin, Nickname: "新作者", Avatar: "/new.png"}
	if err := db.Create(&oldAuthor).Error; err != nil {
		t.Fatalf("创建原作者失败: %v", err)
	}
	if err := db.Create(&newAuthor).Error; err != nil {
		t.Fatalf("创建新作者失败: %v", err)
	}
	article := ArticleRow{Title: "t1", Content: "正文", AuthorID: oldAuthor.ID, PublishStatus: enum.ArticleStatusPublished}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	bulkDocs := setupMockESCollectingBulkDocs(t)

	if err := UpdateESDocsByArticleDeltas(db, testutil.ESClient(), []ArticleModelDelta{{
		ArticleID: article.ID,
		Changed:   map[string]any{"author_id": newAuthor.ID.String()},
	}}); err != nil {
		t.Fatalf("增量更新失败: %v", err)
	}
	if len(*bulkDocs) != 1 {
		t.Fatalf("应更新一篇 ES 文档, got=%d", len(*bulkDocs))
	}
	docMap, ok := (*bulkDocs)[0]["doc"].(map[string]any)
	if !ok {
		t.Fatalf("应使用 partial update 文档结构, got=%#v", (*bulkDocs)[0])
	}
	if docMap["author_id"] != newAuthor.ID.String() {
		t.Fatalf("作者 ID 应更新为新作者, got=%#v", docMap["author_id"])
	}
	if got, ok := docMap["author_is_admin"].(bool); !ok || !got {
		t.Fatalf("换作者后 author_is_admin 应为 true, got=%#v", docMap["author_is_admin"])
	}
	for _, key := range []string{"top", "admin_top", "author_top"} {
		if _, ok := docMap[key]; ok {
			t.Fatalf("换作者不应再写置顶字段 %q: %#v", key, docMap)
		}
	}
	if _, ok := docMap["tags"]; ok {
		t.Fatalf("换作者不应更新其他字段: %#v", docMap)
	}
}

func TestSyncESDocsByArticleSnapshots(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&CategoryRow{},
		&TagRow{},
		&ArticleTagRow{},
	)
	testutil.SetConfig(&conf.Config{
		ES: conf.ES{Index: "article_index"},
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
	if got, ok := doc["author_is_admin"].(bool); !ok || got {
		t.Fatalf("注册用户作者的 author_is_admin 应为 false, got=%#v", doc["author_is_admin"])
	}
	for _, key := range []string{"top", "admin_top", "author_top"} {
		if _, ok := doc[key]; ok {
			t.Fatalf("ES 文档不应再出现置顶字段 %q, got=%#v", key, doc[key])
		}
	}
}

func TestUpdateESDocsByArticleDeltasRebuildsMissingDocs(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&UserRow{},
		&ArticleRow{},
		&TagRow{},
		&ArticleTagRow{},
	)
	testutil.SetConfig(&conf.Config{
		ES: conf.ES{Index: "article_index"},
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
		&ArticleRow{},
		&TagRow{},
		&ArticleTagRow{},
	)
	testutil.SetConfig(&conf.Config{
		ES: conf.ES{Index: "article_index"},
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
