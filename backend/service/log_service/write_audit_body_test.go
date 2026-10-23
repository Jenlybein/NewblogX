package log_service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrepareCapturedBodyMasksSensitiveFields(t *testing.T) {
	raw := []byte(`{"username":"alice","password":"secret","nested":{"token":"abc123","secret_key":"xyz"}}`)

	body := PrepareCapturedBody(raw, "application/json")

	if strings.Contains(body, `"password":"secret"`) || strings.Contains(body, "abc123") || strings.Contains(body, "xyz") {
		t.Fatalf("脱敏后的原始 body 不应包含敏感明文: %s", body)
	}
	if !strings.Contains(body, `"password":"***"`) {
		t.Fatalf("password 字段应被脱敏: %s", body)
	}
	if !strings.Contains(body, `"token":"***"`) {
		t.Fatalf("token 字段应被脱敏: %s", body)
	}
	if !strings.Contains(body, `"secret_key":"***"`) {
		t.Fatalf("secret_key 字段应被脱敏: %s", body)
	}
}

func TestPrepareCapturedBodyTruncatesLargeJSONAndKeepsKeys(t *testing.T) {
	value := map[string]any{
		"title":   "test",
		"content": strings.Repeat("a", MaxBodyBytes*2),
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("构造测试 JSON 失败: %v", err)
	}

	body := PrepareCapturedBody(raw, "application/json")

	if len(body) > MaxBodyBytes {
		t.Fatalf("截断后的 JSON 不应超过限制: got=%d limit=%d", len(body), MaxBodyBytes)
	}
	if !strings.Contains(body, `"title":"test"`) {
		t.Fatalf("截断后仍应保留 title 字段: %s", body)
	}
	if !strings.Contains(body, `"content":"`) {
		t.Fatalf("截断后仍应保留 content 字段名: %s", body)
	}
}

func TestPrepareCapturedHeadersMasksSensitiveFields(t *testing.T) {
	headers := map[string][]string{
		"Authorization": {"Bearer token-123"},
		"Cookie":        {"sid=abc"},
		"X-Trace-Id":    {"trace-1"},
	}

	body := PrepareCapturedHeaders(headers)

	if strings.Contains(body, "token-123") || strings.Contains(body, "sid=abc") {
		t.Fatalf("脱敏后的请求头不应包含敏感明文: %s", body)
	}
	if !strings.Contains(body, `"Authorization":"***"`) {
		t.Fatalf("Authorization 头应被脱敏: %s", body)
	}
	if !strings.Contains(body, `"Cookie":"***"`) {
		t.Fatalf("Cookie 头应被脱敏: %s", body)
	}
	if !strings.Contains(body, `"X-Trace-Id":"trace-1"`) {
		t.Fatalf("普通请求头应被保留: %s", body)
	}
}
