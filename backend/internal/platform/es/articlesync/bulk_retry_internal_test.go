package articlesync

// 批量写的重试行为：只重发该重发的，不整批重发、不对 4xx 白费力气。

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"myblogx/internal/platform/es"

	"github.com/elastic/go-elasticsearch/v7"
)

// scriptedBulkServer 按顺序回放响应体，并记录每次收到的请求体。
func scriptedBulkServer(t *testing.T, responses ...string) (*elasticsearch.Client, *[]string) {
	t.Helper()
	var (
		mu     sync.Mutex
		bodies []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" { // X-Elastic-Product 自检
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			_, _ = io.WriteString(w, `{}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(raw))
		index := len(bodies) - 1
		mu.Unlock()

		body := responses[len(responses)-1]
		if index < len(responses) {
			body = responses[index]
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{srv.URL}})
	if err != nil {
		t.Fatalf("创建假 ES 客户端失败: %v", err)
	}
	return client, &bodies
}

func bulkReqs(ids ...string) []*es_service.BulkRequest {
	reqs := make([]*es_service.BulkRequest, 0, len(ids))
	for _, id := range ids {
		reqs = append(reqs, &es_service.BulkRequest{
			Action: es_service.ActionIndex,
			ID:     id,
			Data:   map[string]any{"title": "t" + id},
		})
	}
	return reqs
}

// 429（限流）值得再发一次，而且**只发失败的那一条**。
func TestBulkRetryResendsOnlyFailedDocument(t *testing.T) {
	client, bodies := scriptedBulkServer(t,
		`{"errors":true,"items":[
			{"index":{"_id":"1","status":201}},
			{"index":{"_id":"2","status":429,"error":{"type":"es_rejected_execution_exception","reason":"queue full"}}},
			{"index":{"_id":"3","status":201}}
		]}`,
		`{"errors":false,"items":[{"index":{"_id":"2","status":201}}]}`,
	)

	out := bulkWriteWithRetry(client, bulkReqs("1", "2", "3"), 3, bulkRetryPolicy{MaxAttempts: 2})

	if len(out.failures) > 0 {
		t.Fatalf("第二次成功之后不该留失败: %+v", out.failures)
	}
	if !out.resp.Success {
		t.Fatalf("最终应成功，实际 %s", out.resp.Msg)
	}
	if out.attempts != 2 {
		t.Fatalf("应该正好重试一次（共 2 次请求），实际 %d 次", out.attempts)
	}
	if len(*bodies) != 2 {
		t.Fatalf("应发出 2 个 bulk 请求，实际 %d", len(*bodies))
	}

	// 第二次只带 id=2：整批重发会把已经成功的 1、3 再写一遍（幂等但浪费）。
	second := (*bodies)[1]
	if !strings.Contains(second, `"_id":"2"`) {
		t.Fatalf("第二次应重发失败的文档 2，实际请求体 %q", second)
	}
	if strings.Contains(second, `"_id":"1"`) || strings.Contains(second, `"_id":"3"`) {
		t.Fatalf("不该重发已成功的文档，实际请求体 %q", second)
	}
}

// 400（数据本身不行）不重试：重发一百次也是同一个错误，直接进死信让人看。
func TestBulkRetrySkipsNonRetryableStatus(t *testing.T) {
	client, bodies := scriptedBulkServer(t, `{"errors":true,"items":[
		{"index":{"_id":"1","status":400,"error":{"type":"mapper_parsing_exception","reason":"failed to parse field [title]"}}}
	]}`)

	out := bulkWriteWithRetry(client, bulkReqs("1"), 3, bulkRetryPolicy{MaxAttempts: 3})

	if out.attempts != 1 {
		t.Fatalf("不可重试的 400 不该发第二次，实际 %d 次", out.attempts)
	}
	if len(*bodies) != 1 {
		t.Fatalf("应只发 1 个请求，实际 %d", len(*bodies))
	}
	if len(out.failures) != 1 {
		t.Fatalf("应保留 1 条失败给调用方（进死信），实际 %+v", out.failures)
	}
	if !strings.Contains(out.failures[0].Reason, "mapper_parsing_exception") {
		t.Fatalf("失败原因应带 ES 的类型，实际 %q", out.failures[0].Reason)
	}
}

// 传输层失败（没有逐条信息）只能整批重试，且不会超过上限。
//
// 注意这里 DisableRetry=true：go-elasticsearch 客户端**自带**对 502/503/504 的重试
// （默认 3 次）。我们的重试就是为了"HTTP 200 但逐条 429"这种客户端看不见的失败，
// 不是为了替代它。测试里关掉客户端重试，才能准确断言"我们自己发了几次"。
func TestBulkRetryWholeBatchOnTransportFailure(t *testing.T) {
	var calls int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			_, _ = io.WriteString(w, `{}`)
			return
		}
		mu.Lock()
		calls++
		mu.Unlock()
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"unavailable"}`)
	}))
	t.Cleanup(srv.Close)

	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses:    []string{srv.URL},
		DisableRetry: true,
	})
	if err != nil {
		t.Fatalf("创建假 ES 客户端失败: %v", err)
	}

	out := bulkWriteWithRetry(client, bulkReqs("1", "2"), 3, bulkRetryPolicy{MaxAttempts: 2})

	if out.resp.Success {
		t.Fatal("整请求失败时不该报成功")
	}
	if out.attempts != 2 {
		t.Fatalf("整批失败应重试到上限（2 次），实际 %d 次", out.attempts)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("应发出 2 个请求，实际 %d", calls)
	}
}
