package river

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/test/testutil"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/go-mysql-org/go-mysql/canal"
	mysqlschema "github.com/go-mysql-org/go-mysql/schema"
)

// newMockESCollectingBulkLines 起一个记录 bulk 请求体原始行的 mock ES。
//
// 删除动作在 bulk 里只有一行元数据（没有文档体），所以不能像既有用例那样
// 隔行取文档 —— 那种读法对 delete 请求一条也收不到。
func newMockESCollectingBulkLines(t *testing.T) (*elasticsearch.Client, *[]string) {
	t.Helper()

	lines := make([]string, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/":
			_, _ = io.WriteString(w, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
		case strings.HasSuffix(r.URL.Path, "/_bulk"):
			body, _ := io.ReadAll(r.Body)
			for _, line := range strings.Split(string(body), "\n") {
				if strings.TrimSpace(line) != "" {
					lines = append(lines, line)
				}
			}
			_, _ = io.WriteString(w, `{"took":1,"errors":false,"items":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"reason":"not found"}}`)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{srv.URL}})
	if err != nil {
		t.Fatalf("创建 mock ES 客户端失败: %v", err)
	}
	return client, &lines
}

// 绕过接口直接改库软删的旁路也要收敛。
//
// article_models 的删除是 GORM 软删：binlog 里是 UPDATE deleted_at，不是 DELETE 行。
// 这条用例从一条真实的"before=NULL / after=时间"行事件驱动投影，
// 断言 ES 收到该文章 id 的 delete 动作 —— 旧实现会因变更列不可投影而零写入。
func TestSoftDeleteArticleRowRemovesESDoc(t *testing.T) {
	db := testutil.SetupSQLite(t)
	client, bulkLines := newMockESCollectingBulkLines(t)

	articleID := ctype.NewID(301900000000000009)
	deletedAt := time.Date(2026, 11, 3, 9, 0, 0, 0, time.Local)
	e := &canal.RowsEvent{
		Action: canal.UpdateAction,
		Table: &mysqlschema.Table{
			Name: "article_models",
			Columns: []mysqlschema.TableColumn{
				{Name: "id"},
				{Name: "title"},
				{Name: "deleted_at"},
			},
		},
		// update 事件的行是成对出现的：before 在前、after 在后。
		Rows: [][]any{
			{articleID.Uint64(), "测试文章", nil},
			{articleID.Uint64(), "测试文章", deletedAt},
		},
	}

	r := &River{db: db, es: client}
	if err := r.syncArticleDocsByArticleRows(e); err != nil {
		t.Fatalf("软删行事件不应报错: %v", err)
	}

	if len(*bulkLines) != 1 {
		t.Fatalf("软删应产生一次索引删除, got=%d 行: %v", len(*bulkLines), *bulkLines)
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal([]byte((*bulkLines)[0]), &meta); err != nil {
		t.Fatalf("解析 bulk 元数据失败: %v", err)
	}
	rawDelete, ok := meta["delete"]
	if !ok {
		t.Fatalf("软删应发 delete 动作，实际 bulk 行: %s", (*bulkLines)[0])
	}
	var detail struct {
		ID string `json:"_id"`
	}
	if err := json.Unmarshal(rawDelete, &detail); err != nil {
		t.Fatalf("解析 delete 动作失败: %v", err)
	}
	if detail.ID != articleID.String() {
		t.Fatalf("delete 的 _id 应为 %s，实际 %s", articleID.String(), detail.ID)
	}
}
