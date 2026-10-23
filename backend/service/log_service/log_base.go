package log_service

import (
	"myblogx/models/ctype"
	"myblogx/service/db_service"
	"os"
	"strconv"
	"time"
)

const (
	// 结构化日志目录名与 ClickHouse 表名常量保持一致，便于采集和查询对齐。
	RuntimeLogDirName     = "runtime_logs"
	LoginEventLogDirName  = "login_event_logs"
	ActionAuditLogDirName = "action_audit_logs"
	CdcEventLogDirName    = "cdc_event_logs"
	ReplayEventLogDirName = "replay_event_logs"

	RuntimeLogTableName     = "runtime_logs"
	LoginEventLogTableName  = "login_event_logs"
	ActionAuditLogTableName = "action_audit_logs"
	CdcEventLogTableName    = "cdc_event_logs"
	ReplayEventLogTableName = "replay_event_logs"

	// clickhouseTimeLayout 统一定义日志写入和查询使用的时间格式。
	clickhouseTimeLayout = "2006-01-02 15:04:05.000"
)

// baseEvent 定义三类结构化日志共用的基础字段。
type baseEvent struct {
	EventID      uint64 `json:"event_id"`
	TS           string `json:"ts"`
	LogKind      string `json:"log_kind"`
	Service      string `json:"service"`
	Env          string `json:"env"`
	Host         string `json:"host"`
	InstanceID   string `json:"instance_id"`
	Level        string `json:"level"`
	Message      string `json:"message"`
	RequestID    string `json:"request_id,omitempty"`
	TraceID      string `json:"trace_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
	EventName    string `json:"event_name,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	UserID       uint64 `json:"user_id,omitempty"`
	IP           string `json:"ip,omitempty"`
	ExtraJSON    string `json:"extra_json,omitempty"`
}

// newBaseEvent 构造一条基础日志事件，并补齐 event_id、时间、环境和实例信息。
func newBaseEvent(deps Deps, logKind, level, message string) baseEvent {
	eventID, err := db_service.NextSnowflakeID()
	if err != nil {
		eventID = ctype.ID(time.Now().UnixNano())
	}

	host, _ := os.Hostname()

	return baseEvent{
		EventID:    uint64(eventID),
		TS:         time.Now().Format(clickhouseTimeLayout),
		LogKind:    logKind,
		Service:    ResolveLogApp("", deps.LogConfig.App),
		Env:        runtimeEnv(deps),
		Host:       host,
		InstanceID: strconv.Itoa(int(runtimeServerID(deps))),
		Level:      level,
		Message:    message,
	}
}

func runtimeEnv(deps Deps) string {
	return deps.SystemConfig.Env
}

func runtimeServerID(deps Deps) uint32 {
	return deps.SystemConfig.ServerID
}
