package ai_api_test

import (
	"encoding/json"
	"fmt"
	"myblogx/api/ai_api"
	"myblogx/conf"
	"myblogx/models/enum"
	"myblogx/service/ai_service"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIOverwriteView(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ai_service.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析 AI 请求失败: %v", err)
		}
		if !req.Stream {
			t.Fatalf("改写接口应使用流式 AI 请求: %+v", req)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"改写\"}}]}\n\n"+
				"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"后的\"}}]}\n\n"+
				"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"文本\"}}]}\n\n"+
				"data: [DONE]\n\n",
		)
	}))
	defer server.Close()

	testutil.SetConfig(&conf.Config{
		AI: conf.AI{
			Enable:    true,
			SecretKey: "test-key",
			BaseURL:   server.URL,
			ChatModel: "test-model",
		},
	})

	api := newAIApi(t)
	c, w := newAICtx()
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   1,
			Role:     enum.RoleUser,
			Username: "writer",
		},
	})
	c.Set("requestJson", ai_api.AIOverwriteRequest{
		Mode:          "polish",
		SelectionText: "这是一段足够长的原始文本，用来验证改写接口是否能够通过 SSE 连续返回文本片段。",
		PrefixText:    "前文内容",
		SuffixText:    "后文内容",
		ArticleTitle:  "测试标题",
	})

	api.AIOverwriteView(c)

	eventList := readSSEEvents(t, w.Body.String())
	if len(eventList) != 3 {
		t.Fatalf("改写接口应返回 3 个 SSE 文本片段, body=%s", w.Body.String())
	}

	result := ""
	for _, event := range eventList {
		if event.Code != 0 {
			t.Fatalf("改写 SSE 事件应成功: %+v", event)
		}
		var data ai_api.AIBaseResponse
		if err := json.Unmarshal(event.Data, &data); err != nil {
			t.Fatalf("解析改写 SSE 数据失败: %v", err)
		}
		result += data.Content
	}
	if result != "改写后的文本" {
		t.Fatalf("改写结果错误: %s", result)
	}
}

func TestAIDiagnoseView(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ai_service.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析 AI 请求失败: %v", err)
		}
		if req.Stream {
			t.Fatalf("诊断接口不应直接透传 token 流: %+v", req)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role": "assistant",
						"content": `{
							"summary":"该片段主要问题是句子过长且论证略显松散。",
							"issues":[
								{
									"type":"可读性",
									"severity":"中",
									"reason":"句子长度偏长，阅读负担较重。",
									"evidence":"同一句连续堆叠多个分句。",
									"suggestion":"拆成两到三句，并压缩重复表达。"
								}
							]
						}`,
					},
				},
			},
		})
	}))
	defer server.Close()

	testutil.SetConfig(&conf.Config{
		AI: conf.AI{
			Enable:    true,
			SecretKey: "test-key",
			BaseURL:   server.URL,
			ChatModel: "test-model",
		},
	})

	api := newAIApi(t)
	c, w := newAICtx()
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   1,
			Role:     enum.RoleUser,
			Username: "writer",
		},
	})
	c.Set("requestJson", ai_api.AIDiagnoseRequest{
		SelectionText: "这是一段足够长的诊断文本，它会用于验证结构化诊断结果是否能够通过 SSE 返回给前端。",
		PrefixText:    "前文内容",
		SuffixText:    "后文内容",
		ArticleTitle:  "测试标题",
	})

	api.AIDiagnoseView(c)

	eventList := readSSEEvents(t, w.Body.String())
	if len(eventList) != 1 {
		t.Fatalf("诊断接口应返回单个结构化 SSE 事件, body=%s", w.Body.String())
	}
	if eventList[0].Code != 0 {
		t.Fatalf("诊断 SSE 事件应成功: %+v", eventList[0])
	}

	var data ai_api.AIDiagnoseResponse
	if err := json.Unmarshal(eventList[0].Data, &data); err != nil {
		t.Fatalf("解析诊断 SSE 数据失败: %v", err)
	}
	if data.Summary == "" || len(data.Issues) != 1 {
		t.Fatalf("诊断返回结构错误: %+v", data)
	}
	if data.Issues[0].Type != "可读性" {
		t.Fatalf("诊断问题类型错误: %+v", data.Issues[0])
	}
}
