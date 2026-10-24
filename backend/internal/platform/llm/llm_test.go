package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(baseURL string) *Client {
	return NewClient(Config{
		Enable:     true,
		BaseURL:    baseURL,
		SecretKey:  "test-key",
		ChatModel:  "test-model",
		Timeout:    2 * time.Second,
		MaxRetries: 2,
	})
}

// TestChat_Success 验证正常非流式响应能解析出正文，且带上了鉴权头。
func TestChat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"你好"}}]}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat err: %v", err)
	}
	if got != "你好" {
		t.Fatalf("got %q, want 你好", got)
	}
}

// TestChat_RetriesOn5xx 验证 5xx 会重试到上限：1 次首试 + 2 次重试 = 3 次。
func TestChat_RetriesOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"boom"}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("calls = %d, want 3 (1 + 2 retries)", got)
	}
}

// TestChat_NoRetryOn4xx 验证 4xx 不重试，只请求一次。
func TestChat_NoRetryOn4xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"bad request"}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1 (4xx 不重试)", got)
	}
}

// TestChat_ContextCanceled 验证 ctx 取消能及时中断，而不是等连接超时。
func TestChat_ContextCanceled(t *testing.T) {
	// handler 必须能被测试确定地放行：仅靠 r.Context() 在 keep-alive
	// 连接上不一定触发，会导致 srv.Close() 卡住。
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	c := newTestClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.Chat(ctx, []Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected ctx error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("取消后耗时 %v，应远小于 1s", elapsed)
	}
}

// TestChatStream 验证 SSE 分片能按序拼出完整内容。
func TestChatStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	content, errs := c.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}})

	var sb strings.Builder
	for s := range content {
		sb.WriteString(s)
	}
	for e := range errs {
		t.Fatalf("stream err: %v", e)
	}
	if sb.String() != "你好" {
		t.Fatalf("got %q, want 你好", sb.String())
	}
}

// TestChat_NotEnabled 验证未开启时直接报错，不发请求。
func TestChat_NotEnabled(t *testing.T) {
	c := NewClient(Config{Enable: false})
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("expected disabled error")
	}
}
