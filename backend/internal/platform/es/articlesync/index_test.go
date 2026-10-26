package articlesync_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/es/articlesync"

	"github.com/elastic/go-elasticsearch/v7"
)

// 写入必须打到注入的索引名上。
//
// 起因：读侧（搜索 / AI 检索）和 CLI（建索引、全量重建）一直取配置 `es.index`，
// 而增量写入曾经硬编码 DefaultArticleESIndex。两边默认值恰好都是 "article_index"，
// 所以配错时不会报错 —— 只会"建在新索引、搜在新索引、增量写进旧索引"。
//
// 这条用例盯的就是"注入到没到写入点"：给假 ES 发一次写，断言 URL 路径里的索引名。
func TestWritesGoToInjectedIndex(t *testing.T) {
	const custom = "blogx_articles_custom"

	var (
		mu    sync.Mutex
		paths []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"errors":false,"items":[]}`)
	}))
	t.Cleanup(srv.Close)

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{srv.URL}})
	if err != nil {
		t.Fatalf("创建假 ES 客户端失败: %v", err)
	}

	// 注入与还原：本包是包级状态，测试之间必须复位，否则顺序会影响结果。
	articlesync.SetIndexName(custom)
	t.Cleanup(func() { articlesync.SetIndexName(articlesync.DefaultArticleESIndex) })

	// 删除文档是最省的写入路径：不需要数据库，只用 ids 拼 bulk。
	if err := articlesync.DeleteESDocs(nil, client, []ctype.ID{ctype.NewID(1)}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// 客户端首次请求会先探一次 `GET /`（X-Elastic-Product 自检），那是噪音；
	// 只断言真正写数据的 _bulk 路径。
	bulks := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.Contains(path, "_bulk") {
			bulks = append(bulks, path)
		}
	}
	if len(bulks) == 0 {
		t.Fatalf("假 ES 没收到任何 _bulk 请求（收到的路径：%v）", paths)
	}
	for _, path := range bulks {
		if !strings.Contains(path, "/"+custom+"/") {
			t.Fatalf("写入应打到注入的索引 %q，实际路径 %q（说明 SetIndexName 没生效，"+
				"写死索引名的老问题又回来了）", custom, path)
		}
	}
}

// 空值必须被忽略：否则配置里漏写 es.index 会把索引名清成空串，
// 写入路径变成 "//_bulk" —— 那比用默认值难查得多。
func TestSetIndexNameIgnoresEmpty(t *testing.T) {
	articlesync.SetIndexName("first")
	t.Cleanup(func() { articlesync.SetIndexName(articlesync.DefaultArticleESIndex) })

	articlesync.SetIndexName("   ")
	if got := articlesync.IndexName(); got != "first" {
		t.Fatalf("空白值不应覆盖已有索引名，实际 %q", got)
	}

	articlesync.SetIndexName("second")
	if got := articlesync.IndexName(); got != "second" {
		t.Fatalf("非空值应生效，实际 %q", got)
	}
}
