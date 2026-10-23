package middleware

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"myblogx/conf"
	"myblogx/service/db_service"
	"myblogx/service/log_service"
	"myblogx/utils/jwts"
	"myblogx/utils/requestmeta"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// LogMiddleware 运行日志中间件
func LogMiddleware(c *gin.Context) {
	runtimeFromContext(c).LogMiddleware(c)
}

// LogMiddleware 运行日志中间件
func (h Runtime) LogMiddleware(c *gin.Context) {
	traceID, spanID, parentSpanID := resolveTraceContext(c, h.LogConfig)
	requestID := resolveRequestID(h.LogConfig, traceID)

	c.Set("_log_deps", h.Log)
	c.Set("_authenticator", h.Authenticator)
	c.Set("_jwt_config", h.JWT)
	c.Set("_redis_deps", h.Redis)

	requestmeta.SetRequestID(c, requestID)
	requestmeta.SetTraceContext(c, traceID, spanID, parentSpanID)

	c.Writer.Header().Set("X-Request-Id", requestID)
	if traceID != "" {
		c.Writer.Header().Set("X-Trace-Id", traceID)
	}

	start := time.Now()
	c.Next()

	if !h.LogConfig.RequestLogEnabled {
		return
	}

	method := ""
	rawPath := ""
	if c.Request != nil {
		method = c.Request.Method
		if c.Request.URL != nil {
			rawPath = c.Request.URL.Path
		}
	}

	fields := logrus.Fields{
		"request_id":  requestID,
		"event_name":  "http_request",
		"method":      method,
		"path":        c.FullPath(),
		"status_code": c.Writer.Status(),
		"latency_ms":  time.Since(start).Milliseconds(),
		"ip":          c.ClientIP(),
	}
	if rawPath != "" && fields["path"] == "" {
		fields["path"] = rawPath
	}
	if traceID != "" {
		fields["trace_id"] = traceID
		fields["span_id"] = spanID
		fields["parent_span_id"] = parentSpanID
	}

	if claims, err := jwts.ParseToken(h.JWT, jwts.GetTokenByGin(c)); err == nil {
		fields["user_id"] = uint64(claims.UserID)
	}

	entry := log_service.RuntimeEntry(h.Log, fields)
	switch {
	case c.Writer.Status() >= 500:
		fields["error_code"] = fmt.Sprintf("HTTP_%d", c.Writer.Status())
		errMsg := strings.TrimSpace(c.Errors.String())
		if errMsg == "" {
			errMsg = fmt.Sprintf("http_status_%d", c.Writer.Status())
		}
		fields["error_message"] = errMsg
		if log_service.ShouldCaptureStack(h.LogConfig, "error") {
			stack, truncated := log_service.TruncateByBytes(string(debug.Stack()), log_service.StackMaxBytes(h.LogConfig))
			fields["error_stack"] = stack
			if truncated {
				fields["error_stack_truncated"] = true
			}
		}
		entry = log_service.RuntimeEntry(h.Log, fields)
		entry.Error("请求执行失败")
	case c.Writer.Status() >= 400:
		entry.Warn("请求执行完成")
	default:
		entry.Info("请求执行完成")
	}
}

// newRequestID 生成唯一请求ID
func newRequestID() string {
	if id, err := db_service.NextSnowflakeID(); err == nil {
		return strconv.FormatUint(uint64(id), 10)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func resolveRequestID(cfg conf.Logrus, traceID string) string {
	if traceID != "" && log_service.RequestIDEqualsTraceID(cfg) {
		return traceID
	}
	return newRequestID()
}

func resolveTraceContext(c *gin.Context, cfg conf.Logrus) (traceID, spanID, parentSpanID string) {
	if !log_service.TraceEnabled(cfg) {
		return "", "", ""
	}

	if log_service.InheritTraceFromGateway(cfg) && c != nil && c.Request != nil {
		for _, header := range log_service.GatewayHeaderPriority(cfg) {
			value := strings.TrimSpace(c.GetHeader(header))
			if value == "" {
				continue
			}
			if strings.EqualFold(header, "traceparent") {
				if tid, parent, ok := parseTraceparent(value); ok {
					traceID = tid
					parentSpanID = parent
					break
				}
				continue
			}
			if tid := normalizeExternalTraceID(value); tid != "" {
				traceID = tid
				break
			}
		}
	}

	if traceID == "" {
		traceID = newRandomHex(16)
	}
	spanID = newRandomHex(8)
	return traceID, spanID, parentSpanID
}

func parseTraceparent(value string) (traceID, parentSpanID string, ok bool) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) != 4 {
		return "", "", false
	}
	if !isHex(parts[1], 32) || !isHex(parts[2], 16) {
		return "", "", false
	}
	return strings.ToLower(parts[1]), strings.ToLower(parts[2]), true
}

func normalizeExternalTraceID(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "-", "")
	if isHex(value, 32) {
		return value
	}
	if isHex(value, 16) {
		return strings.Repeat("0", 16) + value
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:16])
}

func newRandomHex(size int) string {
	if size <= 0 {
		return ""
	}
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buffer)
}

func isHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
