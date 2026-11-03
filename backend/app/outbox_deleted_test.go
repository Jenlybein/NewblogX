package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	articleapp "myblogx/internal/article/app"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"

	"github.com/elastic/go-elasticsearch/v7"
)

// newMockESCollectingBulkLines 起一个记录 bulk 请求体原始行的 mock ES。
//
// 删除动作在 bulk 里只有一行元数据（没有文档体），所以不能"隔行取文档"。
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

// article.deleted 必须真的有订阅者：删除事件随业务事务落库，
// 中继把它投出去时要去删对应的索引文档。
//
// 断言方式是"看副作用"：同步驱动一次投递，mock ES 必须收到该文章 id 的 delete 动作。
// 没有订阅者时中继也会把这行标成 done（没人订阅不算失败），所以只断言状态
// 是测不出这个洞的 —— 必须断言索引侧真的收到了删除。
func TestOutboxArticleDeletedRemovesSearchDoc(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.OutboxRow{})
	client, bulkLines := newMockESCollectingBulkLines(t)

	infra := &Infra{DB: db, Logger: testutil.Logger(), ESClient: client}
	relay := outboxrelay.New(db, infra.Logger, outboxrelay.Options{})
	registerOutboxSubscribers(relay, infra)
	t.Cleanup(relay.Close)

	articleID := ctype.NewID(301900000000000021)
	row := &schema.OutboxRow{
		EventName:     articleapp.EventArticleDeleted,
		AggregateType: "article",
		AggregateID:   articleID.Uint64(),
		Payload:       `{}`,
		Status:        outboxrelay.OutboxStatusPending,
		AvailableAt:   time.Now().Add(-time.Second),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("写入发件箱事件失败: %v", err)
	}

	if err := relay.Dispatch(context.Background(), row.ID); err != nil {
		t.Fatalf("投递 article.deleted 失败: %v", err)
	}

	if len(*bulkLines) != 1 {
		t.Fatalf("删除事件应触发一次索引删除, got=%d 行: %v", len(*bulkLines), *bulkLines)
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal([]byte((*bulkLines)[0]), &meta); err != nil {
		t.Fatalf("解析 bulk 元数据失败: %v", err)
	}
	rawDelete, ok := meta["delete"]
	if !ok {
		t.Fatalf("删除事件应发 delete 动作，实际 bulk 行: %s", (*bulkLines)[0])
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

	var dispatched schema.OutboxRow
	if err := db.Where("id = ?", row.ID).Take(&dispatched).Error; err != nil {
		t.Fatalf("读取投递后的事件行失败: %v", err)
	}
	if dispatched.Status != outboxrelay.OutboxStatusDone {
		t.Fatalf("订阅者成功后事件应为 done，实际 %s", dispatched.Status)
	}
	if dispatched.LastError != "" {
		t.Fatalf("订阅者成功后不应留下失败原因，实际 %q", dispatched.LastError)
	}
}
