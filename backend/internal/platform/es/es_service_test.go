package es_service

import (
	"encoding/json"
	"io"
	"myblogx/test/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
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

func TestIndexAndPipelineOps(t *testing.T) {
	indexExists := map[string]bool{"idx1": true}
	pipelineExists := map[string]bool{"p1": true}

	setupMockES(t, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			writeJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
			return
		}
		switch {
		case strings.HasPrefix(path, "/_ingest/pipeline/"):
			id := strings.TrimPrefix(path, "/_ingest/pipeline/")
			switch r.Method {
			case http.MethodGet:
				if pipelineExists[id] {
					writeJSON(w, 200, `{}`)
				} else {
					writeJSON(w, 404, `{"error":{"reason":"not found"}}`)
				}
			case http.MethodDelete:
				delete(pipelineExists, id)
				writeJSON(w, 200, `{}`)
			case http.MethodPut:
				pipelineExists[id] = true
				writeJSON(w, 200, `{}`)
			default:
				writeJSON(w, 500, `{"error":{"reason":"bad method"}}`)
			}
			return
		default:
			index := strings.TrimPrefix(path, "/")
			switch r.Method {
			case http.MethodHead:
				if indexExists[index] {
					w.WriteHeader(200)
				} else {
					w.WriteHeader(404)
				}
			case http.MethodDelete:
				delete(indexExists, index)
				writeJSON(w, 200, `{}`)
			case http.MethodPut:
				if strings.HasSuffix(path, "/_mapping") {
					writeJSON(w, 200, `{}`)
					return
				}
				indexExists[index] = true
				writeJSON(w, 200, `{}`)
			case http.MethodGet:
				if strings.HasSuffix(path, "/_mapping") {
					writeJSON(w, 200, `{"idx1":{"mappings":{"properties":{}}}}`)
					return
				}
				writeJSON(w, 500, `{"error":{"reason":"unexpected"}}`)
			default:
				writeJSON(w, 500, `{"error":{"reason":"bad method"}}`)
			}
		}
	})

	esClient := testutil.ESClient()
	if err := CreateIndexForce(esClient, "idx1", `{}`); err != nil {
		t.Fatalf("CreateIndexForce 失败: %v", err)
	}
	if exists, err := ExistsIndex(esClient, "idx1"); err != nil || !exists {
		t.Fatalf("ExistsIndex 结果异常: exists=%v err=%v", exists, err)
	}
	if err := DeleteIndex(esClient, "idx1"); err != nil {
		t.Fatalf("DeleteIndex 失败: %v", err)
	}
	if exists, err := ExistsIndex(esClient, "idx1"); err != nil || exists {
		t.Fatalf("Delete 后 ExistsIndex 异常: exists=%v err=%v", exists, err)
	}

	if err := CreatePipelineForce(esClient, "p1", `{}`); err != nil {
		t.Fatalf("CreatePipelineForce 失败: %v", err)
	}
	if exists, err := ExistsPipeline(esClient, "p1"); err != nil || !exists {
		t.Fatalf("ExistsPipeline 结果异常: exists=%v err=%v", exists, err)
	}
	if err := DeletePipeline(esClient, "p1"); err != nil {
		t.Fatalf("DeletePipeline 失败: %v", err)
	}
	if exists, err := ExistsPipeline(esClient, "p1"); err != nil || exists {
		t.Fatalf("Delete 后 ExistsPipeline 异常: exists=%v err=%v", exists, err)
	}
}

func TestDecodeResponseAndHandleErrorFallback(t *testing.T) {
	data, err := decodeResponse(io.NopCloser(strings.NewReader(`{"a":1}`)))
	if err != nil {
		t.Fatalf("decodeResponse 失败: %v", err)
	}
	if v, ok := data["a"].(json.Number); !ok || v.String() != "1" {
		t.Fatalf("decodeResponse 结果异常: %#v", data)
	}

	esRes := &esapi.Response{
		StatusCode: 403,
		Body:       io.NopCloser(strings.NewReader(`{"error":{}}`)),
	}
	err = handleError(esRes)
	if err == nil || !strings.Contains(err.Error(), "权限不足") {
		t.Fatalf("handleError 兜底信息异常: %v", err)
	}
}

func TestBuildBulkBody(t *testing.T) {
	items := []*BulkRequest{
		{
			Action: ActionIndex,
			Index:  "idx",
			ID:     "1",
			Data: map[string]interface{}{
				"title": "hello",
			},
		},
		{
			Action: ActionUpdate,
			Index:  "idx",
			ID:     "2",
			Data: map[string]interface{}{
				"title": "world",
			},
		},
		{
			Action: ActionDelete,
			Index:  "idx",
			ID:     "3",
		},
	}

	body, err := buildBulkBody(items)
	if err != nil {
		t.Fatalf("buildBulkBody 失败: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "\"index\"") || !strings.Contains(s, "\"update\"") || !strings.Contains(s, "\"delete\"") {
		t.Fatalf("bulk body 缺少 action: %s", s)
	}
	if !strings.Contains(s, "\"doc\"") {
		t.Fatalf("update 文档结构缺失: %s", s)
	}
}

func TestHandleError(t *testing.T) {
	res := &esapi.Response{
		StatusCode: 400,
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"reason":"bad request","caused_by":{"reason":"x"}}}`,
		)),
	}
	err := handleError(res)
	if err == nil {
		t.Fatal("handleError 应返回错误")
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("错误信息异常: %v", err)
	}
}

func TestCloseResponse(t *testing.T) {
	res := &esapi.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
	}
	closeResponse(res)
}
