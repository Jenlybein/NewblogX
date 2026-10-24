package es_service

import (
	"encoding/json"
	"io"
	"myblogx/models/ctype"
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

func TestDocumentAndBulkOps(t *testing.T) {
	setupMockES(t, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			writeJSON(w, 200, `{"name":"mock","cluster_name":"mock","version":{"number":"7.17.10"},"tagline":"You Know, for Search"}`)
			return
		}
		switch {
		case strings.Contains(path, "/_search"):
			writeJSON(w, 200, `{"hits":{"total":{"value":2},"hits":[{"_source":{"id":1,"title":"a"}},{"_source":{"id":2,"title":"b"}}]}}`)
		case strings.Contains(path, "/_update/"):
			writeJSON(w, 200, `{"result":"updated"}`)
		case r.Method == http.MethodDelete && strings.Contains(path, "/_doc/"):
			writeJSON(w, 200, `{"result":"deleted"}`)
		case r.Method == http.MethodPost && strings.Contains(path, "/_doc"):
			writeJSON(w, 200, `{"result":"created"}`)
		case r.Method == http.MethodGet && strings.Contains(path, "/_doc/"):
			writeJSON(w, 200, `{"_id":"1","found":true}`)
		case r.Method == http.MethodHead && strings.Contains(path, "/_doc/"):
			w.WriteHeader(200)
		case path == "/_bulk" || strings.HasSuffix(path, "/_bulk"):
			writeJSON(w, 200, `{"errors":false,"items":[]}`)
		case strings.HasSuffix(path, "/_mapping") && r.Method == http.MethodGet:
			writeJSON(w, 200, `{"idx":{"mappings":{"properties":{}}}}`)
		case strings.HasSuffix(path, "/_mapping") && r.Method == http.MethodPut:
			writeJSON(w, 200, `{}`)
		case r.Method == http.MethodHead:
			w.WriteHeader(404)
		case r.Method == http.MethodPut:
			writeJSON(w, 200, `{}`)
		case r.Method == http.MethodDelete:
			writeJSON(w, 200, `{"acknowledged":true}`)
		default:
			writeJSON(w, 500, `{"error":{"reason":"unexpected"}}`)
		}
	})

	esClient := testutil.ESClient()
	if resp := CreateDocument(esClient, "idx", map[string]any{"title": "x"}); !resp.Success {
		t.Fatalf("CreateDocument 失败: %+v", resp)
	}
	if resp := Search[map[string]any](esClient, "idx", 1, 10, map[string]any{"match_all": map[string]any{}}); !resp.Success {
		t.Fatalf("Search 失败: %+v", resp)
	}
	if resp := UpdateDocument(esClient, "idx", "1", map[string]any{"title": "y"}); !resp.Success {
		t.Fatalf("UpdateDocument 失败: %+v", resp)
	}
	if resp := DeleteDocument(esClient, "idx", "1"); !resp.Success {
		t.Fatalf("DeleteDocument 失败: %+v", resp)
	}
	if resp := Get(esClient, "idx", "_doc", "1"); !resp.Success {
		t.Fatalf("Get 失败: %+v", resp)
	}
	if resp := Exists(esClient, "idx", "_doc", "1"); !resp.Success || resp.Data != true {
		t.Fatalf("Exists 结果异常: %+v", resp)
	}

	items := []*BulkRequest{
		{Action: ActionIndex, Index: "idx", ID: "1", Data: map[string]interface{}{"k": "v"}},
	}
	if resp := Bulk(esClient, items); !resp.Success {
		t.Fatalf("Bulk 失败: %+v", resp)
	}
	if resp := IndexBulk(esClient, "idx", items); !resp.Success {
		t.Fatalf("IndexBulk 失败: %+v", resp)
	}
	if resp := IndexTypeBulk(esClient, "idx", "_doc", items); !resp.Success {
		t.Fatalf("IndexTypeBulk 失败: %+v", resp)
	}

	if resp := CreateMapping(esClient, "idx", "_doc", map[string]interface{}{"title": map[string]any{"type": "text"}}); !resp.Success {
		t.Fatalf("CreateMapping 失败: %+v", resp)
	}
	if resp := GetMapping(esClient, "idx", "_doc"); !resp.Success {
		t.Fatalf("GetMapping 失败: %+v", resp)
	}
	if resp := DeleteIndexWithResponse(esClient, "idx"); !resp.Success {
		t.Fatalf("DeleteIndexWithResponse 失败: %+v", resp)
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

func TestExtractArticles(t *testing.T) {
	input := map[string]any{
		"hits": []any{
			map[string]any{
				"_source": map[string]any{
					"id":    1,
					"title": "title-1",
				},
			},
		},
	}

	articles := ExtractArticles(input)
	if len(articles) != 1 {
		t.Fatalf("数量错误: %d", len(articles))
	}
	if articles[0].ID != ctype.ID(1) || articles[0].Title != "title-1" {
		t.Fatalf("解析结果异常: %+v", articles[0])
	}
}

func TestExtractArticlesMoreFields(t *testing.T) {
	src := map[string]any{
		"hits": []any{
			map[string]any{
				"_source": map[string]any{
					"id":              3,
					"title":           "t3",
					"comments_toggle": true,
				},
			},
		},
	}
	arts := ExtractArticles(src)
	if len(arts) != 1 || arts[0].ID != 3 || arts[0].Title != "t3" || !arts[0].CommentsToggle {
		b, _ := json.Marshal(arts)
		t.Fatalf("ExtractArticles 结果异常: %s", string(b))
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
