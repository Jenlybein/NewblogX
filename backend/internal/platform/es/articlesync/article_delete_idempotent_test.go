package articlesync_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/es/articlesync"

	"github.com/elastic/go-elasticsearch/v7"
)

// 删一个 ES 里本来就不存在的文档，必须是**成功**（目的已达成），不能重试。
//
// 这不是理论情况：2026-11 线上 8 条死信里有 4 条就是它 —— 文章创建后 70 毫秒被删掉，
// ES 索引还没写好；删除事件的 bulk 请求打到 ES，ES 回
//
//	{"delete":{"_id":"...","status":404,"result":"not_found"}}
//
// 注意**没有 `error` 对象**（对比 update 打不存在的文档会带 error.type=
// document_missing_exception）。当时豁免条件只认 error.type，于是这条被当成
// "删除失败、原因 unknown"，重试 5 次后进死信，死信告警一直亮着。

func TestDeleteESDocsTreatsMissingDocumentAsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"version":{"number":"7.12.0"}}`)
			return
		}
		// 真实 ES 对"删除不存在的文档"的响应形状：errors=true、status=404、
		// result=not_found、**没有 error 字段**。
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"took":1,"errors":true,"items":[{"delete":{`+
			`"_index":"article_index","_id":"7","status":404,"result":"not_found"}}]}`)
	}))
	t.Cleanup(srv.Close)

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{srv.URL}})
	if err != nil {
		t.Fatalf("创建假 ES 客户端失败: %v", err)
	}

	if err := articlesync.DeleteESDocs(nil, client, []ctype.ID{ctype.NewID(7)}); err != nil {
		t.Fatalf("删除一个 ES 里不存在的文档应视为成功（幂等），实际报错: %v", err)
	}
}

// 反向：真正的失败（429 限流 / 5xx）不能被上面那条豁免顺手吞掉。
//
// 两条一起钉住，才能区分"放宽了正确的边界"和"把错误一律当成功"。
func TestDeleteESDocsStillFailsOnRealError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"version":{"number":"7.12.0"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"took":1,"errors":true,"items":[{"delete":{`+
			`"_index":"article_index","_id":"7","status":429,`+
			`"error":{"type":"es_rejected_execution_exception","reason":"queue full"}}}]}`)
	}))
	t.Cleanup(srv.Close)

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{srv.URL}})
	if err != nil {
		t.Fatalf("创建假 ES 客户端失败: %v", err)
	}

	if err := articlesync.DeleteESDocs(nil, client, []ctype.ID{ctype.NewID(7)}); err == nil {
		t.Fatal("429 是真实失败，不该被当成删除成功")
	}
}
