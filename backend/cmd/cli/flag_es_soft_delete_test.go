package cli

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"myblogx/conf"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/es/articlesync"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
)

// 全量重建（既有的 CLI 入口 `-es -s article-sync`）必须能重建已软删的文章。
//
// 重建的 id 枚举用的是"只选 id"的投影，它不带 GORM 的软删作用域，所以已删文章
// 的 id 一直会进来；差别在回源装载。旧实现在这里按软删作用域过滤，于是整批
// "部分文章未加载到"直接失败 —— 也就是**库里有任何一篇删掉的文章，全量重建就跑不起来**。
//
// 修复后的约定是：索引是库的镜像，已软删的行也索引并写 is_deleted=true，
// 读侧再按标记过滤。这条用例钉住 CLI 这条入口上的完整行为，不另造重建流程。
func TestSyncArticleDocumentsRebuildsSoftDeletedArticles(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&schema.UserRow{},
		&schema.UserConfRow{},
		&schema.ArticleRow{},
		&schema.TagRow{},
		&schema.ArticleTagRow{},
	)
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		ES:    conf.ES{Index: "article_index"},
		River: conf.River{BulkSize: 1},
	})

	author := schema.UserRow{Username: "author", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}
	live := schema.ArticleRow{
		Title:          "活着的文章",
		Content:        "正文",
		AuthorID:       author.ID,
		CommentsToggle: true,
		PublishStatus:  enum.ArticleStatusPublished,
	}
	deleted := schema.ArticleRow{
		Title:          "已删的文章",
		Content:        "正文",
		AuthorID:       author.ID,
		CommentsToggle: true,
		PublishStatus:  enum.ArticleStatusPublished,
	}
	if err := db.Create(&live).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := db.Create(&deleted).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	if err := db.Delete(&deleted).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	bulkDocs := make([]map[string]any, 0, 2)
	setupMockESClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			writeESJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case r.Method == http.MethodHead && r.URL.Path == "/article_index":
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/article_index/_bulk":
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
					if err := json.Unmarshal(line, &doc); err != nil {
						t.Fatalf("解析 bulk 文档失败: %v", err)
					}
					bulkDocs = append(bulkDocs, doc)
				}
			}
			writeESJSON(w, 200, `{"took":1,"errors":false,"items":[]}`)
		default:
			writeESJSON(w, 404, `{"error":{"reason":"not found"}}`)
		}
	})

	total, err := syncArticleDocuments(db, testutil.ESClient(), "article_index", 1, nil)
	if err != nil {
		t.Fatalf("库里有已软删文章时全量重建不应失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("重建应覆盖已删文章, got=%d want=2", total)
	}
	if len(bulkDocs) != 2 {
		t.Fatalf("重建应写入两篇文档, got=%d", len(bulkDocs))
	}

	markerByID := make(map[string]any, len(bulkDocs))
	for _, doc := range bulkDocs {
		markerByID[doc["id"].(string)] = doc[articlesync.SoftDeleteMarkerField]
	}
	if got, ok := markerByID[live.ID.String()].(bool); !ok || got {
		t.Fatalf("活着的文章应写 is_deleted=false, got=%#v", got)
	}
	if got, ok := markerByID[deleted.ID.String()].(bool); !ok || !got {
		t.Fatalf("已删的文章应写 is_deleted=true, got=%#v", got)
	}
}
