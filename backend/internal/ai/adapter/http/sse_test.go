package http

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// newSSEContext 造一个能记录 SSE 输出的 gin 上下文。
//
// httptest.ResponseRecorder 实现了 http.Flusher，所以 gin 的
// c.SSEvent + c.Writer.Flush 能正常工作 —— 不需要真起一个 HTTP 服务。
func newSSEContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/ai/overwrite", nil)
	return c, w
}

// sseFrames 把 SSE 响应体切成帧（每帧以空行结束）。
func sseFrames(body string) []string {
	raw := strings.Split(strings.TrimSpace(body), "\n\n")
	out := make([]string, 0, len(raw))
	for _, f := range raw {
		if strings.TrimSpace(f) == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// framePayload 取一帧里的 JSON（去掉 "data:" 前缀）。
func framePayload(t *testing.T, frame string) map[string]any {
	t.Helper()
	idx := strings.Index(frame, "data:")
	if idx < 0 {
		t.Fatalf("帧里没有 data: 前缀: %q", frame)
	}
	raw := strings.TrimSpace(frame[idx+len("data:"):])
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("帧内容不是合法 JSON: %q (%v)", raw, err)
	}
	return body
}

// TestPrepareSSESetsNoBufferingHeader 校验 SSE 关键响应头。
//
// X-Accel-Buffering: no 尤其重要：漏了它 nginx 会把整条流缓冲起来，
// 表现是"AI 一直在转圈，最后一次性蹦出全文"——功能没坏，体验废了，
// 而且本地直连后端根本复现不出来。
func TestPrepareSSESetsNoBufferingHeader(t *testing.T) {
	c, w := newSSEContext()
	prepareSSE(c)

	if got := w.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type: got=%q want=text/event-stream", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control: got=%q want=no-cache", got)
	}
	if got := w.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering: got=%q want=no（否则 nginx 会缓冲整条流）", got)
	}
}

// TestDrainStreamEmitsAllChunksInOrder 内容按到达顺序逐帧下发。
//
// 顺序错的表现是"AI 改写的文字前后颠倒"，而语言模型输出一旦乱序就完全不可读。
func TestDrainStreamEmitsAllChunksInOrder(t *testing.T) {
	c, w := newSSEContext()

	content := make(chan string, 3)
	errs := make(chan error, 1)
	content <- "第一段"
	content <- "第二段"
	content <- "第三段"
	close(content)
	close(errs)

	drainStream(c, content, errs)

	frames := sseFrames(w.Body.String())
	if len(frames) != 3 {
		t.Fatalf("应有 3 帧，实际 %d 帧: %q", len(frames), w.Body.String())
	}
	wantOrder := []string{"第一段", "第二段", "第三段"}
	for i, want := range wantOrder {
		body := framePayload(t, frames[i])
		if body["msg"] != "成功" {
			t.Fatalf("第 %d 帧不是成功响应: %+v", i, body)
		}
		data, ok := body["data"].(map[string]any)
		if !ok {
			t.Fatalf("第 %d 帧缺少 data 对象: %+v", i, body)
		}
		if data["content"] != want {
			t.Fatalf("第 %d 帧内容应=%q，实际=%v", i, want, data["content"])
		}
	}
}

// TestDrainStreamStopsOnError 上游报错时下发失败帧并立刻收尾。
//
// 两条断言缺一不可：
//   - 失败帧必须发出去，否则前端看到连接关闭却不知道原因，只能显示"网络错误"；
//   - 之后必须 return，不能继续把残留内容写进已经声明失败的流。
//
// 这里用**无缓冲** channel 来固定顺序：只有 drainStream 真正收下一条，
// 发送方才会继续发下一条。若改成带缓冲，两个 case 同时就绪时
// select 是随机选的，测试就会时绿时红（第一次写这条测试就踩了）。
func TestDrainStreamStopsOnError(t *testing.T) {
	c, w := newSSEContext()

	content := make(chan string)
	errs := make(chan error)
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		content <- "写了一半"          // 等 drainStream 收下
		errs <- errors.New("上游超时") // 收下这条后 drainStream 立即返回
		close(content)
		close(errs)
	}()

	drainStream(c, content, errs)
	<-sent

	body := w.Body.String()
	frames := sseFrames(body)
	if len(frames) != 2 {
		t.Fatalf("应有 2 帧（一条内容 + 一条错误），实际 %d 帧: %q", len(frames), body)
	}
	if data, ok := framePayload(t, frames[0])["data"].(map[string]any); !ok || data["content"] != "写了一半" {
		t.Fatalf("第一帧应是报错前的内容，实际 %+v", framePayload(t, frames[0]))
	}
	last := framePayload(t, frames[1])
	if last["msg"] != "上游超时" {
		t.Fatalf("最后一帧应是失败消息，实际 %+v", last)
	}
	if code, ok := last["code"].(float64); !ok || code == 0 {
		t.Fatalf("失败帧的 code 不应为 0，实际 %+v", last)
	}
}

// TestDrainStreamExitsWhenBothClosed 两个 channel 都关闭时正常退出。
//
// 这是最容易写出死循环的地方：只把 channel 置 nil 而不改循环条件，
// 或者先关 err 再关 content，都可能让 handler 永远挂在那里，
// 表现是"这条连接一直不断开，连接池慢慢被耗尽"。
func TestDrainStreamExitsWhenBothClosed(t *testing.T) {
	c, w := newSSEContext()

	content := make(chan string)
	errs := make(chan error)
	close(content)
	close(errs)

	done := make(chan struct{})
	go func() {
		drainStream(c, content, errs)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("两个 channel 都关闭后 drainStream 没有退出（死循环）")
	}
	if body := strings.TrimSpace(w.Body.String()); body != "" {
		t.Fatalf("没有任何内容时不该写帧，实际 %q", body)
	}
}

// TestDrainStreamHandlesNilErrValue 收到 nil 错误时继续等下一条内容。
//
// 上游实现常见写法是 errChan <- nil 表示"本段没有错误"。
// 若这里把它当成失败，正常输出会被截断。
func TestDrainStreamHandlesNilErrValue(t *testing.T) {
	c, w := newSSEContext()

	content := make(chan string, 1)
	errs := make(chan error, 1)
	errs <- nil
	content <- "正常内容"
	close(content)
	close(errs)

	drainStream(c, content, errs)

	frames := sseFrames(w.Body.String())
	if len(frames) != 1 {
		t.Fatalf("nil 错误不该产生帧，实际 %d 帧: %q", len(frames), w.Body.String())
	}
	body := framePayload(t, frames[0])
	data, _ := body["data"].(map[string]any)
	if data["content"] != "正常内容" {
		t.Fatalf("内容应正常下发，实际 %+v", body)
	}
}
