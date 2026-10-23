package requestmeta

import (
	"myblogx/utils/ipmeta"

	"github.com/gin-gonic/gin"
)

const requestIDContextKey = "request_id"
const (
	traceIDContextKey      = "trace_id"
	spanIDContextKey       = "span_id"
	parentSpanIDContextKey = "parent_span_id"
)

type Meta struct {
	IP   string
	Addr string
	UA   string
}

func SetRequestID(c *gin.Context, requestID string) {
	if c == nil {
		return
	}
	c.Set(requestIDContextKey, requestID)
}

func GetRequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if raw, ok := c.Get(requestIDContextKey); ok {
		if requestID, ok := raw.(string); ok {
			return requestID
		}
	}
	return ""
}

func SetTraceContext(c *gin.Context, traceID, spanID, parentSpanID string) {
	if c == nil {
		return
	}
	c.Set(traceIDContextKey, traceID)
	c.Set(spanIDContextKey, spanID)
	c.Set(parentSpanIDContextKey, parentSpanID)
}

func GetTraceID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if raw, ok := c.Get(traceIDContextKey); ok {
		if value, ok := raw.(string); ok {
			return value
		}
	}
	return ""
}

func GetSpanID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if raw, ok := c.Get(spanIDContextKey); ok {
		if value, ok := raw.(string); ok {
			return value
		}
	}
	return ""
}

func GetParentSpanID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if raw, ok := c.Get(parentSpanIDContextKey); ok {
		if value, ok := raw.(string); ok {
			return value
		}
	}
	return ""
}

func BuildSessionMeta(c *gin.Context) Meta {
	if c == nil || c.Request == nil {
		return Meta{}
	}

	ip := c.ClientIP()
	return Meta{
		IP:   ip,
		Addr: ipmeta.GetAddr(ip),
		UA:   c.Request.UserAgent(),
	}
}
