package log_service

import (
	"myblogx/models/ctype"
	"myblogx/models/enum"
)

// LoginEvent 表示落盘后的登录事件日志结构。
type LoginEvent struct {
	baseEvent
	Username  string `json:"username,omitempty"`
	LoginType string `json:"login_type,omitempty"`
	Success   uint8  `json:"success"`
	Reason    string `json:"reason,omitempty"`
	Addr      string `json:"addr,omitempty"`
	UA        string `json:"ua,omitempty"`
}

// LoginEventInput 描述业务层写入登录事件日志时可提供的字段。
type LoginEventInput struct {
	EventName    string
	Username     string
	LoginType    enum.LoginType
	Success      bool
	Reason       string
	UserID       ctype.ID
	IP           string
	Addr         string
	UA           string
	RequestID    string
	TraceID      string
	SpanID       string
	ParentSpanID string
	ErrorCode    string
	Extra        map[string]any
}
