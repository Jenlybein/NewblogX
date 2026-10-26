package es_service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elastic/go-elasticsearch/v7"
)

// fakeBulkServer 起一个只回固定响应体的假 ES，并记录收到的 bulk 请求次数。
func fakeBulkServer(t *testing.T, body string) (*elasticsearch.Client, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" { // 客户端首启的 X-Elastic-Product 自检
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			_, _ = io.WriteString(w, `{}`)
			return
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{srv.URL}})
	if err != nil {
		t.Fatalf("创建假 ES 客户端失败: %v", err)
	}
	return client, &calls
}

func bulkItems(ids ...string) []*BulkRequest {
	items := make([]*BulkRequest, 0, len(ids))
	for _, id := range ids {
		items = append(items, &BulkRequest{
			Action: ActionIndex,
			ID:     id,
			Data:   map[string]any{"title": "t" + id},
		})
	}
	return items
}

// HTTP 200 + errors:true 必须判成失败，并给出"哪一条、什么错"。
//
// 这条以前是漏的：IndexBulk 只看 HTTP 状态，个别文档因 mapping 冲突写失败
// 会被当成成功 —— 那几篇静默地不在索引里，不进死信、没有日志。
func TestIndexBulkReportsPerItemFailure(t *testing.T) {
	client, calls := fakeBulkServer(t, `{
		"took": 3,
		"errors": true,
		"items": [
			{"index": {"_id": "1", "status": 201}},
			{"index": {"_id": "2", "status": 400, "error": {"type": "mapper_parsing_exception", "reason": "failed to parse field [title]", "caused_by": {"reason": "expected text but got number"}}}},
			{"index": {"_id": "3", "status": 201}}
		]
	}`)

	resp := IndexBulk(client, "idx", bulkItems("1", "2", "3"))
	if resp.Success {
		t.Fatal("有一条失败时，Success 必须为 false —— 否则调用方会当成功放过")
	}
	result, ok := resp.Data.(BulkResult)
	if !ok {
		t.Fatalf("Data 应是 BulkResult，实际 %T", resp.Data)
	}
	if result.Total != 3 || len(result.Failures) != 1 {
		t.Fatalf("应 3 条里 1 条失败，实际 total=%d failures=%d", result.Total, len(result.Failures))
	}
	failure := result.Failures[0]
	if failure.Index != 1 || failure.ID != "2" || failure.Status != 400 {
		t.Fatalf("失败条目定位错误: %+v", failure)
	}
	if failure.Type != "mapper_parsing_exception" {
		t.Fatalf("应带出 error.type，实际 %q", failure.Type)
	}
	if failure.Reason == "" {
		t.Fatal("应带出人看的原因（含 caused_by），否则死信里等于没有信息")
	}
	if got := *calls; got != 1 {
		t.Fatalf("解析失败不该触发额外请求，实际 %d 次", got)
	}
}

// 全成功时 Success=true、Failures 为空。
func TestIndexBulkAllSuccess(t *testing.T) {
	client, _ := fakeBulkServer(t, `{"took":1,"errors":false,"items":[
		{"index":{"_id":"1","status":201}},
		{"index":{"_id":"2","status":201}}
	]}`)

	resp := IndexBulk(client, "idx", bulkItems("1", "2"))
	if !resp.Success {
		t.Fatalf("全成功却报失败: %s", resp.Msg)
	}
	result, _ := resp.Data.(BulkResult)
	if result.HasFailure() || result.Total != 2 {
		t.Fatalf("结果解析不对: %+v", result)
	}
}
