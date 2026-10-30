package middleware

import (
	"fmt"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/log/writer"
	"myblogx/utils/jwts"
	"myblogx/utils/requestmeta"

	"github.com/gin-gonic/gin"
)

// GinAuditInput 定义基于 Gin 上下文补充操作审计日志时的业务输入。
//
// 注意：这里不再接收请求/响应体。全量原始报文默认不入库（见 docs/architecture/governance.md §4.2 / L.10）；
// 需要排查时，请求/响应体由 CaptureLog 在 HTTP 边界采集「已脱敏 + ≤16KB」快照，
// 且只在**失败请求**时写入审计日志。
type GinAuditInput struct {
	Level      string
	Message    string
	ActionName string
	TargetType string
	TargetID   string
	Success    bool
	Extra      map[string]string
}

// EmitActionAuditFromGin 从 Gin 上下文提取请求元信息并写入操作审计日志。
func EmitActionAuditFromGin(c *gin.Context, input GinAuditInput) {
	deps := logOptionsFromContext(c)
	if c == nil {
		writer.EmitActionAudit(deps, writer.ActionAuditInput{
			Level:      input.Level,
			Message:    input.Message,
			ActionName: input.ActionName,
			TargetType: input.TargetType,
			TargetID:   input.TargetID,
			Success:    input.Success,
			Extra:      input.Extra,
		})
		return
	}

	userID := ctype.NewID(0)
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

	// 只记失败请求的 body：成功请求海量且价值低，失败请求才是排查所需。
	// 内容来自 CaptureLog 在边界做的「限量读 + 脱敏 + 截断」，不是全量原始报文。
	var reqBody, respBody []byte
	if !input.Success {
		if s := writer.GetCapturedRequestBody(c); s != "" {
			reqBody = []byte(s)
		}
		if s := writer.GetCapturedResponseBody(c); s != "" {
			respBody = []byte(s)
		}
	}

	writer.EmitActionAudit(deps, writer.ActionAuditInput{
		Level:        input.Level,
		Message:      input.Message,
		RequestID:    requestmeta.GetRequestID(c),
		TraceID:      requestmeta.GetTraceID(c),
		SpanID:       requestmeta.GetSpanID(c),
		ParentSpanID: requestmeta.GetParentSpanID(c),
		ErrorCode:    buildAuditErrorCode(statusCode, input.Success),
		ErrorMessage: buildAuditErrorMessage(c, input.Success),
		UserID:       userID,
		IP:           ip,
		Method:       method,
		Path:         path,
		StatusCode:   statusCode,
		ActionName:   input.ActionName,
		TargetType:   input.TargetType,
		TargetID:     input.TargetID,
		Success:      input.Success,
		RequestBody:  reqBody,
		ResponseBody: respBody,
		Extra:        input.Extra,
	})
}

// EmitLoginEventFromGin 基于 Gin 请求上下文补齐 IP、地域、UA 和 request_id 后记录登录事件。
func EmitLoginEventFromGin(c *gin.Context, eventName string, loginType enum.LoginType, success bool, username string, userID ctype.ID, reason string, extra map[string]string) {
	deps := logOptionsFromContext(c)
	meta := requestmeta.BuildSessionMeta(c)

	writer.EmitLoginEvent(deps, writer.LoginEventInput{
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

func logOptionsFromContext(c *gin.Context) writer.Options {
	if c == nil {
		return writer.Options{}
	}
	value, ok := c.Get("_log_deps")
	if !ok {
		return writer.Options{}
	}
	deps, _ := value.(writer.Options)
	return deps
}
