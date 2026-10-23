package middleware

import (
	"fmt"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/log_service"
	"myblogx/utils/jwts"
	"myblogx/utils/requestmeta"

	"github.com/gin-gonic/gin"
)

// GinAuditInput 定义基于 Gin 上下文补充操作审计日志时的业务输入。
type GinAuditInput struct {
	Level              string
	Message            string
	ActionName         string
	TargetType         string
	TargetID           string
	Success            bool
	RequestBody        any
	ResponseBody       any
	UseRawRequestBody  bool
	UseRawResponseBody bool
	UseRawRequestHead  bool
	UseRawResponseHead bool
	Extra              map[string]any
}

// EmitActionAuditFromGin 从 Gin 上下文提取请求元信息并写入操作审计日志。
func EmitActionAuditFromGin(c *gin.Context, input GinAuditInput) {
	deps := logDepsFromContext(c)
	if c == nil {
		log_service.EmitActionAudit(deps, log_service.ActionAuditInput{
			Level:        input.Level,
			Message:      input.Message,
			ActionName:   input.ActionName,
			TargetType:   input.TargetType,
			TargetID:     input.TargetID,
			Success:      input.Success,
			RequestBody:  input.RequestBody,
			ResponseBody: input.ResponseBody,
			Extra:        input.Extra,
		})
		return
	}

	userID := ctype.ID(0)
	if c.Request != nil {
		if claims := jwts.GetClaimsByGin(c); claims != nil {
			userID = claims.UserID
		}
	}

	path := c.FullPath()
	method := ""
	ip := ""
	if c.Request != nil {
		method = c.Request.Method
		if path == "" && c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		ip = c.ClientIP()
	}

	statusCode := 0
	if c.Writer != nil {
		statusCode = c.Writer.Status()
	}

	rawRequestBody := ""
	if input.UseRawRequestBody {
		rawRequestBody = log_service.GetRawRequestBody(c)
	}
	rawResponseBody := ""
	if input.UseRawResponseBody {
		rawResponseBody = log_service.GetRawResponseBody(c)
	}
	rawRequestHeader := ""
	if input.UseRawRequestHead {
		rawRequestHeader = log_service.GetRawRequestHeader(c)
	}
	rawResponseHeader := ""
	if input.UseRawResponseHead {
		rawResponseHeader = log_service.GetRawResponseHeader(c)
	}

	log_service.EmitActionAudit(deps, log_service.ActionAuditInput{
		Level:             input.Level,
		Message:           input.Message,
		RequestID:         requestmeta.GetRequestID(c),
		TraceID:           requestmeta.GetTraceID(c),
		SpanID:            requestmeta.GetSpanID(c),
		ParentSpanID:      requestmeta.GetParentSpanID(c),
		ErrorCode:         buildAuditErrorCode(statusCode, input.Success),
		ErrorMessage:      buildAuditErrorMessage(c, input.Success),
		UserID:            userID,
		IP:                ip,
		Method:            method,
		Path:              path,
		StatusCode:        statusCode,
		ActionName:        input.ActionName,
		TargetType:        input.TargetType,
		TargetID:          input.TargetID,
		Success:           input.Success,
		RequestBody:       input.RequestBody,
		ResponseBody:      input.ResponseBody,
		RequestBodyRaw:    rawRequestBody,
		ResponseBodyRaw:   rawResponseBody,
		RequestHeaderRaw:  rawRequestHeader,
		ResponseHeaderRaw: rawResponseHeader,
		Extra:             input.Extra,
	})
}

// EmitLoginEventFromGin 基于 Gin 请求上下文补齐 IP、地域、UA 和 request_id 后记录登录事件。
func EmitLoginEventFromGin(c *gin.Context, eventName string, loginType enum.LoginType, success bool, username string, userID ctype.ID, reason string, extra map[string]any) {
	deps := logDepsFromContext(c)
	meta := requestmeta.BuildSessionMeta(c)

	log_service.EmitLoginEvent(deps, log_service.LoginEventInput{
		EventName:    eventName,
		Username:     username,
		LoginType:    loginType,
		Success:      success,
		Reason:       reason,
		UserID:       userID,
		IP:           meta.IP,
		Addr:         meta.Addr,
		UA:           meta.UA,
		RequestID:    requestmeta.GetRequestID(c),
		TraceID:      requestmeta.GetTraceID(c),
		SpanID:       requestmeta.GetSpanID(c),
		ParentSpanID: requestmeta.GetParentSpanID(c),
		ErrorCode:    buildLoginErrorCode(success),
		Extra:        extra,
	})
}

func buildAuditErrorCode(statusCode int, success bool) string {
	if success {
		return ""
	}
	return fmt.Sprintf("HTTP_%d", statusCode)
}

func buildAuditErrorMessage(c *gin.Context, success bool) string {
	if success || c == nil {
		return ""
	}
	return c.Errors.String()
}

func buildLoginErrorCode(success bool) string {
	if success {
		return ""
	}
	return "AUTH_FAILED"
}

func logDepsFromContext(c *gin.Context) log_service.Deps {
	if c == nil {
		return log_service.Deps{}
	}
	value, ok := c.Get("_log_deps")
	if !ok {
		return log_service.Deps{}
	}
	deps, _ := value.(log_service.Deps)
	return deps
}
