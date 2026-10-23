package log_service

import (
	"encoding/json"
)

// EmitLoginEvent 写入一条登录事件日志，并根据成功状态推导默认级别与消息。
func EmitLoginEvent(deps Deps, input LoginEventInput) {
	// 自动补齐基础字段
	base := newBaseEvent(deps, "login_event", loginLevel(input.Success), loginMessage(input))
	base.UserID = uint64(input.UserID)
	base.IP = input.IP
	base.RequestID = input.RequestID
	base.TraceID = defaultIfEmptyString(input.TraceID, input.RequestID)
	base.SpanID = input.SpanID
	base.ParentSpanID = input.ParentSpanID
	base.EventName = input.EventName
	base.ErrorCode = input.ErrorCode
	if !input.Success && base.ErrorCode == "" {
		base.ErrorCode = "AUTH_FAILED"
	}
	if !input.Success {
		base.ErrorMessage = input.Reason
	}
	if len(input.Extra) > 0 {
		if byteData, err := json.Marshal(input.Extra); err == nil {
			base.ExtraJSON = string(byteData)
		}
	}

	event := LoginEvent{
		baseEvent: base,
		Username:  input.Username,
		LoginType: input.LoginType.String(),
		Success:   boolToUInt8(input.Success),
		Reason:    input.Reason,
		Addr:      input.Addr,
		UA:        input.UA,
	}

	if err := loginEventSink().write(deps, event); err != nil {
		if deps.Logger != nil {
			deps.Logger.Errorf("写入登录事件日志失败: %v", err)
		}
	}
}

// loginLevel 根据登录结果映射日志级别。
func loginLevel(success bool) string {
	if success {
		return "info"
	}
	return "warn"
}

// loginMessage 根据事件类型和结果生成默认中文消息。
func loginMessage(input LoginEventInput) string {
	if input.Success {
		switch input.EventName {
		case "logout":
			return "用户退出登录"
		case "logout_all":
			return "用户退出全部设备"
		case "token_refresh":
			return "刷新令牌成功"
		default:
			return "用户登录成功"
		}
	}
	return "认证事件失败"
}

// boolToUInt8 将布尔值转换为 ClickHouse 友好的 0/1 数值。
func boolToUInt8(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}
