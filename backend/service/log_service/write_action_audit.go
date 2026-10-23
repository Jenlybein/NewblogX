package log_service

import (
	"encoding/json"
	"fmt"

	"myblogx/models/ctype"
)

// ActionAuditEvent 表示落盘到操作审计日志中的最终事件结构。
// 最终写入日志文件的结构化数据，包含请求、操作、目标、结果等完整审计信息
type ActionAuditEvent struct {
	baseEvent                // 嵌入基础日志事件（公共字段：时间、服务、环境、请求ID等）
	Method            string `json:"method,omitempty"`              // HTTP请求方法
	Path              string `json:"path,omitempty"`                // 请求路由路径
	StatusCode        int    `json:"status_code,omitempty"`         // HTTP响应状态码
	ActionName        string `json:"action_name"`                   // 操作名称
	TargetType        string `json:"target_type,omitempty"`         // 操作目标类型
	TargetID          string `json:"target_id,omitempty"`           // 操作目标ID
	Success           uint8  `json:"success"`                       // 操作是否成功
	RequestBody       string `json:"request_body,omitempty"`        // 请求体JSON字符串
	ResponseBody      string `json:"response_body,omitempty"`       // 响应体JSON字符串
	RequestBodyRaw    string `json:"request_body_raw,omitempty"`    // 脱敏截断后的原始请求体
	ResponseBodyRaw   string `json:"response_body_raw,omitempty"`   // 脱敏截断后的原始响应体
	RequestHeaderRaw  string `json:"request_header_raw,omitempty"`  // 脱敏截断后的原始请求头
	ResponseHeaderRaw string `json:"response_header_raw,omitempty"` // 脱敏截断后的原始响应头
}

// ActionAuditInput 描述业务侧写入操作审计日志时可传入的上下文。
// 业务层传入的审计日志参数，用于组装最终审计事件
type ActionAuditInput struct {
	Level             string // 日志级别 info/warn/error
	Message           string // 日志描述信息
	RequestID         string // 全链路追踪ID
	TraceID           string
	SpanID            string
	ParentSpanID      string
	ErrorCode         string
	ErrorMessage      string
	UserID            ctype.ID       // 操作用户ID
	IP                string         // 操作IP地址
	Method            string         // HTTP请求方法
	Path              string         // HTTP请求路径
	StatusCode        int            // HTTP响应状态码
	ActionName        string         // 操作名称
	TargetType        string         // 操作目标类型
	TargetID          string         // 操作目标ID
	Success           bool           // 操作是否成功
	RequestBody       any            // 请求体（任意结构）
	ResponseBody      any            // 响应体（任意结构）
	RequestBodyRaw    string         // 原始请求体快照
	ResponseBodyRaw   string         // 原始响应体快照
	RequestHeaderRaw  string         // 原始请求头快照
	ResponseHeaderRaw string         // 原始响应头快照
	Extra             map[string]any // 扩展自定义字段
}

// EmitActionAudit 写入一条操作审计日志，并自动补齐基础字段与默认级别。
// 核心函数：统一处理操作审计日志的组装、序列化、文件写入
// 参数：input - 业务层传入的审计日志输入参数
func EmitActionAudit(deps Deps, input ActionAuditInput) {
	// 自动设置日志级别：未指定时，成功=info，失败=warn
	level := input.Level
	if level == "" {
		if input.Success {
			level = "info"
		} else {
			level = "warn"
		}
	}

	// 自动设置日志消息：未指定时，使用操作名称作为默认消息
	message := input.Message
	if message == "" {
		message = input.ActionName
	}

	// 创建基础日志事件（公共字段：时间、服务、环境、实例等）
	base := newBaseEvent(deps, "action_audit", level, message)
	// 填充公共审计字段
	base.RequestID = input.RequestID
	base.TraceID = defaultIfEmptyString(input.TraceID, input.RequestID)
	base.SpanID = input.SpanID
	base.ParentSpanID = input.ParentSpanID
	base.EventName = "audit_action_success"
	if !input.Success {
		base.EventName = "audit_action_failed"
	}
	base.ErrorCode = input.ErrorCode
	base.ErrorMessage = input.ErrorMessage
	base.UserID = uint64(input.UserID)
	base.IP = input.IP
	if !input.Success && base.ErrorCode == "" {
		base.ErrorCode = fmt.Sprintf("HTTP_%d", input.StatusCode)
	}
	if !input.Success && base.ErrorMessage == "" {
		base.ErrorMessage = message
	}
	// 扩展字段：序列化为JSON字符串存入base.ExtraJSON
	if len(input.Extra) > 0 {
		if byteData, err := json.Marshal(input.Extra); err == nil {
			base.ExtraJSON = string(byteData)
		}
	}

	// 组装最终的审计日志事件
	event := ActionAuditEvent{
		baseEvent:         base,
		Method:            input.Method,
		Path:              input.Path,
		StatusCode:        input.StatusCode,
		ActionName:        input.ActionName,
		TargetType:        input.TargetType,
		TargetID:          input.TargetID,
		Success:           boolToUInt8(input.Success),                      // bool转uint8 便于日志存储
		RequestBody:       mustMarshalCompactJSON(input.RequestBody),       // 压缩序列化请求体
		ResponseBody:      mustMarshalCompactJSON(input.ResponseBody),      // 压缩序列化响应体
		RequestBodyRaw:    mustMarshalCompactJSON(input.RequestBodyRaw),    // 脱敏截断后的原始请求体
		ResponseBodyRaw:   mustMarshalCompactJSON(input.ResponseBodyRaw),   // 脱敏截断后的原始响应体
		RequestHeaderRaw:  mustMarshalCompactJSON(input.RequestHeaderRaw),  // 脱敏截断后的原始请求头
		ResponseHeaderRaw: mustMarshalCompactJSON(input.ResponseHeaderRaw), // 脱敏截断后的原始响应头
	}

	// 写入审计日志文件，写入失败则打印错误日志
	if err := actionAuditSink().write(deps, event); err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("写入操作审计日志失败: %v", err)
		}
	}
}

// mustMarshalCompactJSON 将任意请求/响应摘要压缩成单行 JSON，失败时返回空串。
// 工具方法：安全序列化对象为单行JSON字符串，不抛出panic
// 参数：value - 任意需要序列化的对象
// 返回：单行JSON字符串 / 空串
func mustMarshalCompactJSON(value any) string {
	if value == nil {
		return ""
	}
	return MarshalAuditValue(value)
}
