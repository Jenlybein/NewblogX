package log_service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"unicode/utf8"

	"myblogx/utils/logsafe"
)

// CaptureLogMode 定义请求/响应 body 与 header 的按位采集模式。
type CaptureLogMode uint8

const (
	// None 表示不采集任何原始请求/响应信息。
	None CaptureLogMode = 0
	// ReqBody 表示采集原始请求体。
	ReqBody CaptureLogMode = 1 << iota
	// RespBody 表示采集原始响应体。
	RespBody
	// ReqHeader 表示采集原始请求头。
	ReqHeader
	// RespHeader 表示采集原始响应头。
	RespHeader
)

const (
	// BothBody 表示同时采集原始请求体和响应体。
	BothBody = ReqBody | RespBody
	// BothHeader 表示同时采集原始请求头和响应头。
	BothHeader = ReqHeader | RespHeader
	// All 表示同时采集 body 和 header。
	All = BothBody | BothHeader
)

const (
	// MaxBodyBytes 控制写入日志前单个原始 body/header 的最大长度。
	MaxBodyBytes = 16 * 1024

	contextKeyRawRequestBody  = "_audit_raw_request_body"
	contextKeyRawResponseBody = "_audit_raw_response_body"
	contextKeyRawRequestHead  = "_audit_raw_request_header"
	contextKeyRawResponseHead = "_audit_raw_response_header"
	truncationSuffix          = "...(已截断)"
)

var sensitiveKeys = map[string]struct{}{
	"access_token":     {},
	"api_key":          {},
	"authorization":    {},
	"captcha_code":     {},
	"confirm_password": {},
	"cookie":           {},
	"email_code":       {},
	"new_password":     {},
	"old_password":     {},
	"password":         {},
	"refresh_token":    {},
	"secret":           {},
	"secret_key":       {},
	"set-cookie":       {},
	"token":            {},
}

type contextValueStore interface {
	Set(any, any)
	Get(any) (any, bool)
}

// NeedRequestBody 返回当前模式是否需要采集原始请求体。
func (mode CaptureLogMode) NeedRequestBody() bool {
	return mode&ReqBody != 0
}

// NeedResponseBody 返回当前模式是否需要采集原始响应体。
func (mode CaptureLogMode) NeedResponseBody() bool {
	return mode&RespBody != 0
}

// NeedRequestHeader 返回当前模式是否需要采集原始请求头。
func (mode CaptureLogMode) NeedRequestHeader() bool {
	return mode&ReqHeader != 0
}

// NeedResponseHeader 返回当前模式是否需要采集原始响应头。
func (mode CaptureLogMode) NeedResponseHeader() bool {
	return mode&RespHeader != 0
}

// SetRawRequestBody 将处理后的原始请求体缓存到 Gin 上下文。
func SetRawRequestBody(c contextValueStore, body string) {
	if c == nil || body == "" {
		return
	}
	c.Set(contextKeyRawRequestBody, body)
}

// GetRawRequestBody 从 Gin 上下文读取处理后的原始请求体。
func GetRawRequestBody(c contextValueStore) string {
	if c == nil {
		return ""
	}
	value, ok := c.Get(contextKeyRawRequestBody)
	if !ok {
		return ""
	}
	body, _ := value.(string)
	return body
}

// SetRawResponseBody 将处理后的原始响应体缓存到 Gin 上下文。
func SetRawResponseBody(c contextValueStore, body string) {
	if c == nil || body == "" {
		return
	}
	c.Set(contextKeyRawResponseBody, body)
}

// GetRawResponseBody 从 Gin 上下文读取处理后的原始响应体。
func GetRawResponseBody(c contextValueStore) string {
	if c == nil {
		return ""
	}
	value, ok := c.Get(contextKeyRawResponseBody)
	if !ok {
		return ""
	}
	body, _ := value.(string)
	return body
}

// SetRawRequestHeader 将处理后的原始请求头缓存到 Gin 上下文。
func SetRawRequestHeader(c contextValueStore, header string) {
	if c == nil || header == "" {
		return
	}
	c.Set(contextKeyRawRequestHead, header)
}

// GetRawRequestHeader 从 Gin 上下文读取处理后的原始请求头。
func GetRawRequestHeader(c contextValueStore) string {
	if c == nil {
		return ""
	}
	value, ok := c.Get(contextKeyRawRequestHead)
	if !ok {
		return ""
	}
	header, _ := value.(string)
	return header
}

// SetRawResponseHeader 将处理后的原始响应头缓存到 Gin 上下文。
func SetRawResponseHeader(c contextValueStore, header string) {
	if c == nil || header == "" {
		return
	}
	c.Set(contextKeyRawResponseHead, header)
}

// GetRawResponseHeader 从 Gin 上下文读取处理后的原始响应头。
func GetRawResponseHeader(c contextValueStore) string {
	if c == nil {
		return ""
	}
	value, ok := c.Get(contextKeyRawResponseHead)
	if !ok {
		return ""
	}
	header, _ := value.(string)
	return header
}

// PrepareCapturedBody 对原始 body 做脱敏和截断后，返回适合写入审计日志的文本。
func PrepareCapturedBody(raw []byte, contentType string) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}

	if isJSONContent(contentType) || json.Valid(trimmed) {
		if value, ok := decodeJSON(trimmed); ok {
			return marshalStructuredWithinLimit(redactValue("", value), MaxBodyBytes)
		}
	}

	return truncatePlainText(string(trimmed), MaxBodyBytes)
}

// PrepareCapturedHeaders 对请求头或响应头做脱敏和截断，返回适合写入审计日志的 JSON 字符串。
func PrepareCapturedHeaders(headers map[string][]string) string {
	if len(headers) == 0 {
		return ""
	}

	value := make(map[string]any, len(headers))
	for key, items := range headers {
		if len(items) == 0 {
			value[key] = ""
			continue
		}
		if len(items) == 1 {
			value[key] = items[0]
			continue
		}
		cloned := make([]string, 0, len(items))
		cloned = append(cloned, items...)
		value[key] = cloned
	}

	return marshalStructuredWithinLimit(redactValue("", value), MaxBodyBytes)
}

// MarshalAuditValue 将业务代码传入的请求/响应摘要安全序列化为单行 JSON。
func MarshalAuditValue(value any) string {
	if value == nil {
		return ""
	}

	switch typed := value.(type) {
	case string:
		return PrepareCapturedBody([]byte(typed), "application/json")
	case []byte:
		return PrepareCapturedBody(typed, "application/json")
	}

	return marshalStructuredWithinLimit(redactValue("", logsafe.SanitizeValue(value)), MaxBodyBytes)
}

func decodeJSON(raw []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	return value, true
}

func isJSONContent(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func redactValue(key string, value any) any {
	if isSensitiveKey(key) {
		return "***"
	}

	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			result[childKey] = redactValue(childKey, childValue)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, redactValue(key, item))
		}
		return result
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	if key == "" {
		return false
	}
	_, ok := sensitiveKeys[strings.ToLower(key)]
	return ok
}

func marshalStructuredWithinLimit(value any, limit int) string {
	leafLimits := []int{2048, 1024, 512, 256, 128, 64, 32, 16}
	for _, leafLimit := range leafLimits {
		candidate := truncateLeafValues(value, leafLimit)
		if body, ok := tryMarshalWithinLimit(candidate, limit); ok {
			return body
		}
	}

	candidate := collapseNested(truncateLeafValues(value, 16), 2)
	if body, ok := tryMarshalWithinLimit(candidate, limit); ok {
		return body
	}

	candidate = collapseNested(truncateLeafValues(value, 8), 1)
	if body, ok := tryMarshalWithinLimit(candidate, limit); ok {
		return body
	}

	if body, err := json.Marshal(candidate); err == nil {
		return truncatePlainText(string(body), limit)
	}
	return ""
}

func tryMarshalWithinLimit(value any, limit int) (string, bool) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", false
	}
	if len(body) > limit {
		return "", false
	}
	return string(body), true
}

func truncateLeafValues(value any, leafLimit int) any {
	switch typed := value.(type) {
	case string:
		return truncateByRune(typed, leafLimit)
	case []any:
		maxItems := 32
		result := make([]any, 0, minInt(len(typed), maxItems)+1)
		for index, item := range typed {
			if index >= maxItems {
				result = append(result, fmt.Sprintf("...省略%d项", len(typed)-maxItems))
				break
			}
			result = append(result, truncateLeafValues(item, leafLimit))
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = truncateLeafValues(item, leafLimit)
		}
		return result
	default:
		return value
	}
}

func collapseNested(value any, depth int) any {
	if depth <= 0 {
		switch value.(type) {
		case map[string]any:
			return "{...已截断}"
		case []any:
			return "[...已截断]"
		default:
			return value
		}
	}

	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = collapseNested(item, depth-1)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, collapseNested(item, depth-1))
		}
		return result
	default:
		return value
	}
}

func truncatePlainText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	if limit <= len(truncationSuffix) {
		return truncationSuffix[:limit]
	}
	return truncateByByte(value, limit-len(truncationSuffix)) + truncationSuffix
}

func truncateByRune(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	if limit <= len([]rune(truncationSuffix)) {
		return string([]rune(truncationSuffix)[:limit])
	}

	runes := []rune(value)
	return string(runes[:limit-len([]rune(truncationSuffix))]) + truncationSuffix
}

func truncateByByte(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}

	var builder strings.Builder
	builder.Grow(limit)
	size := 0
	for _, r := range value {
		runeSize := utf8.RuneLen(r)
		if runeSize < 0 {
			runeSize = 1
		}
		if size+runeSize > limit {
			break
		}
		builder.WriteRune(r)
		size += runeSize
	}
	return builder.String()
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
