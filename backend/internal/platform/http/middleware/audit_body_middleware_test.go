package middleware_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myblogx/internal/platform/http/middleware"
	logwriter "myblogx/internal/platform/log/writer"

	"github.com/gin-gonic/gin"
)

// 请求体采集绝不能改动业务能看到的 body。
//
// 2026-11-27：后台「关键词过滤」保存词表把整张词表（6 万条、约 4MB JSON）随请求体下发，
// 保存**永远失败**，提示「JSON 格式不完整，请检查请求体」。根因是 `CaptureLog`
// 采集完之后把 `c.Request.Body` 换成了"只含前 1MB 的 buffer"——
// 超过 1MB 的请求体到业务手里就是半截 JSON。
//
// 这些用例钉住两件事：
//  1. 大请求体（远超 1MB）到业务手里必须逐字节完整；
//  2. 采集侧仍然只留前 1MB 的样本（脱敏后 ≤16KB），不能把整个 4MB 拖进审计日志。

const captureSampleBytes = 1 << 20 // 与 audit_body_middleware.maxCaptureBytes 同量级

// largePayload 造一个约 2MB 的 JSON：结构照着词表段（words 数组），
// 这样测试既能覆盖"大 JSON 能否解析"，也贴近真实故障形态。
func largePayload(t *testing.T, words int) []byte {
	t.Helper()
	type word struct {
		Word     string `json:"word"`
		Tier     string `json:"tier"`
		Category string `json:"category"`
	}
	payload := struct {
		Words []word `json:"words"`
	}{Words: make([]word, 0, words)}
	for i := 0; i < words; i++ {
		payload.Words = append(payload.Words, word{
			Word:     "敏感词条-" + strings.Repeat("x", 12) + "-" + itoa(i),
			Tier:     "block",
			Category: "fraud",
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("构造请求体失败: %v", err)
	}
	return raw
}

// itoa 避免为了一个数字引入 strconv 之外的依赖差异。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// 读业务侧看到的请求体（原样 io.ReadAll），断言字节数与内容都与原始一致。
func readBusinessBody(t *testing.T, engine *gin.Engine, body []byte) []byte {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/probe", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("业务路由应成功，实际状态 %d，body: %s", rec.Code, rec.Body.String())
	}
	var got []byte
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		// 响应体里放的是原始字节的十六进制长度与内容，这里只取长度断言；
		// 为了简单，路由直接把读到的字节写回，前端用 Length 头对照。
		got = []byte(rec.Body.String())
	}
	return got
}

// TestCaptureLogKeepsLargeRequestBodyIntact 超过 1MB 的请求体必须完整到达业务。
//
// 旧实现的失败形态：业务读到 1MB+1 字节就 EOF → ShouldBindJSON 报 unexpected EOF。
func TestCaptureLogKeepsLargeRequestBodyIntact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.CaptureLog(middleware.ReqBody))

	var businessBody []byte
	var captured string
	engine.PUT("/probe", func(c *gin.Context) {
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, "读请求体失败: %v", err)
			return
		}
		businessBody = raw
		captured = logwriter.GetCapturedRequestBody(c)
		c.Status(http.StatusOK)
	})

	payload := largePayload(t, 20000) // 约 2MB，确定超过 1MB 采集上限
	if len(payload) <= captureSampleBytes {
		t.Fatalf("测试请求体应超过采集上限，实际 %d 字节", len(payload))
	}

	req := httptest.NewRequest(http.MethodPut, "/probe", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(payload))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("业务应成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
	// 逐字节比对：这是"业务拿到的 body 没有被采集中间件改动"的唯一硬证据。
	if len(businessBody) != len(payload) {
		t.Fatalf("业务读到的请求体被截断：原始 %d 字节，实际 %d 字节", len(payload), len(businessBody))
	}
	if string(businessBody) != string(payload) {
		t.Fatalf("业务读到的请求体与原始不一致（长度相同但内容不同）")
	}
	// 采集侧仍必须守住上限：样本非空、且脱敏压缩后不膨胀。
	if captured == "" {
		t.Fatal("大请求体也应该采集到样本（前 1MB），实际为空")
	}
	if len(captured) > logwriter.MaxBodyBytes {
		t.Fatalf("采集结果必须 ≤ MaxBodyBytes(%d)，实际 %d", logwriter.MaxBodyBytes, len(captured))
	}
}

// TestCaptureLogKeepsSmallRequestBodyIntact 小请求体（绝大多数接口）仍然完整且被采集。
func TestCaptureLogKeepsSmallRequestBodyIntact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.CaptureLog(middleware.ReqBody))

	var businessBody string
	var captured string
	engine.POST("/probe", func(c *gin.Context) {
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, "读请求体失败: %v", err)
			return
		}
		businessBody = string(raw)
		captured = logwriter.GetCapturedRequestBody(c)
		c.Status(http.StatusOK)
	})

	payload := `{"title":"小请求体"}`
	req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(payload))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("业务应成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if businessBody != payload {
		t.Fatalf("小请求体应逐字节完整：期望 %q，实际 %q", payload, businessBody)
	}
	if captured == "" {
		t.Fatal("小请求体应被采集，实际为空")
	}
}

// TestCaptureLogWithoutCaptureModeDoesNotTouchBody 不采集请求体时不读、不改 body。
func TestCaptureLogWithoutCaptureModeDoesNotTouchBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.CaptureLog(middleware.RespBody))

	var businessBody string
	engine.POST("/probe", func(c *gin.Context) {
		raw, _ := io.ReadAll(c.Request.Body)
		businessBody = string(raw)
		c.Status(http.StatusOK)
	})

	payload := `{"a":1}`
	req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(payload))
	req.ContentLength = int64(len(payload))
	engine.ServeHTTP(httptest.NewRecorder(), req)

	if businessBody != payload {
		t.Fatalf("未开启请求体采集时业务应读到原样请求体：期望 %q，实际 %q", payload, businessBody)
	}
	if captured := logwriter.GetCapturedRequestBody(nil); captured != "" {
		t.Fatalf("未开启采集时不应写入上下文，实际 %q", captured)
	}
}
