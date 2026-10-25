package writer

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	platformlog "myblogx/internal/platform/log"

	"github.com/sirupsen/logrus"
)

// RuntimeLogEvent 是 runtime_logs 表的一行。
//
// 字段集合必须与 deploy/docker/clickhouse/init/01-logs.sql 的 runtime_logs
// 列一致。列名改动时这里不会编译失败，所以由测试兜住：
// clickhouse_integration_test.go 用 DESCRIBE TABLE 与这里的 JSON 字段逐列对照。
type RuntimeLogEvent struct {
	EventID      uint64 `json:"event_id"`
	TS           string `json:"ts"`
	Level        string `json:"level"`
	TraceID      string `json:"trace_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
	Service      string `json:"service,omitempty"`
	Env          string `json:"env,omitempty"`
	InstanceID   string `json:"instance_id,omitempty"`
	EventName    string `json:"event_name,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`

	Message    string `json:"message,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	LogKind    string `json:"log_kind,omitempty"`
	Host       string `json:"host,omitempty"`
	File       string `json:"file,omitempty"`
	Func       string `json:"func,omitempty"`
	UserID     uint64 `json:"user_id,omitempty"`
	IP         string `json:"ip,omitempty"`
	Method     string `json:"method,omitempty"`
	Path       string `json:"path,omitempty"`
	StatusCode uint32 `json:"status_code,omitempty"`
	LatencyMS  uint32 `json:"latency_ms,omitempty"`
	ErrorType  string `json:"error_type,omitempty"`
	ErrorStack string `json:"error_stack,omitempty"`
	ExtraJSON  string `json:"extra_json,omitempty"`
}

// EmitRuntimeLog 把一条 logrus 条目写进 runtime_logs。
//
// 这是 logrus 与结构化管道之间唯一的桥。hook 只负责转交，字段映射集中在
// buildRuntimeRow 一处，便于测试与后续演进。
//
// rootDir / appName 只在管道尚未初始化时用于确定本地兜底路径——
// InitLogrus 早于 InitStructuredLog，启动早期那几行日志不能因此丢掉。
func EmitRuntimeLog(rootDir, appName string, entry *logrus.Entry) {
	if entry == nil {
		return
	}

	line, err := json.Marshal(buildRuntimeRow(entry))
	if err != nil {
		// 此时 logger 正在初始化，没有可用的上报通道，只能放弃这一条。
		return
	}

	pipelineMu.RLock()
	p := activePipe
	pipelineMu.RUnlock()

	if p != nil {
		p.emit(platformlog.KindRuntime, parseLevel(entry.Level.String()), line)
		return
	}

	// 管道未就绪：直写每日文件，保持与初始化前一致的行为。
	_ = fallbackRuntimeSink(rootDir, appName).WriteBatch(context.Background(), [][]byte{line})
}

// buildRuntimeRow 把 logrus 条目映射成 runtime_logs 的行。
//
// 映射策略：
//  1. 表里有的列按名字取（值可能来自 entry.Data，也可能是 logrus 的
//     标准字段 level/message/time/caller）；
//  2. 表里没有的字段一律进 extra_json——宁可多带一列 JSON，也不要静默丢字段。
//
// 这正是 JSONEachRow 会「跳过未知字段」时最需要的一层：去重靠这里，
// 而不是指望 ClickHouse 报错。
func buildRuntimeRow(entry *logrus.Entry) RuntimeLogEvent {
	row := RuntimeLogEvent{
		Level:   entry.Level.String(),
		Message: entry.Message,
		LogKind: "runtime",
		TS:      entry.Time.Format(clickhouseTimeLayout),
	}
	if entry.Time.IsZero() {
		row.TS = time.Now().Format(clickhouseTimeLayout)
	}
	if entry.HasCaller() {
		row.File = entry.Caller.File
		row.Func = entry.Caller.Function
	}

	leftover := make(map[string]any, len(entry.Data))
	for key, value := range entry.Data {
		switch key {
		case "event_id":
			row.EventID = asUint64(value)
		case "ts":
			row.TS = asString(value)
		case "level":
			row.Level = asString(value)
		case "trace_id":
			row.TraceID = asString(value)
		case "span_id":
			row.SpanID = asString(value)
		case "parent_span_id":
			row.ParentSpanID = asString(value)
		case "service":
			row.Service = asString(value)
		case "env":
			row.Env = asString(value)
		case "instance_id":
			row.InstanceID = asString(value)
		case "event_name":
			row.EventName = asString(value)
		case "error_code":
			row.ErrorCode = asString(value)
		case "error_message":
			row.ErrorMessage = asString(value)
		case "message":
			row.Message = asString(value)
		case "request_id":
			row.RequestID = asString(value)
		case "log_kind":
			row.LogKind = asString(value)
		case "host":
			row.Host = asString(value)
		case "file":
			row.File = asString(value)
		case "func":
			row.Func = asString(value)
		case "user_id":
			row.UserID = asUint64(value)
		case "ip":
			row.IP = asString(value)
		case "method":
			row.Method = asString(value)
		case "path":
			row.Path = asString(value)
		case "status_code":
			row.StatusCode = uint32(asUint64(value))
		case "latency_ms":
			row.LatencyMS = uint32(asUint64(value))
		case "error_type":
			row.ErrorType = asString(value)
		case "error_stack":
			row.ErrorStack = asString(value)
		case "extra_json":
			row.ExtraJSON = asString(value)
		default:
			leftover[key] = value
		}
	}

	if len(leftover) > 0 {
		out := leftover
		if row.ExtraJSON != "" {
			// 已有 extra_json：合并成一个对象，且不覆盖业务显式写入的同名键。
			var existing map[string]any
			if err := json.Unmarshal([]byte(row.ExtraJSON), &existing); err == nil {
				for k, v := range existing {
					if _, dup := out[k]; !dup {
						out[k] = v
					}
				}
			}
		}
		if encoded, err := json.Marshal(out); err == nil {
			row.ExtraJSON = string(encoded)
		}
	}

	return row
}

// asString 只接受字符串；其它类型一律视为空，避免把数字/结构体写进文本列。
func asString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

// asUint64 兼容 logrus.Fields 里可能出现的各种整数表示。
func asUint64(value any) uint64 {
	switch v := value.(type) {
	case uint64:
		return v
	case uint32:
		return uint64(v)
	case uint16:
		return uint64(v)
	case uint8:
		return uint64(v)
	case uint:
		return uint64(v)
	case int:
		if v < 0 {
			return 0
		}
		return uint64(v)
	case int64:
		if v < 0 {
			return 0
		}
		return uint64(v)
	case int32:
		if v < 0 {
			return 0
		}
		return uint64(v)
	case float64:
		if v < 0 {
			return 0
		}
		return uint64(v)
	case json.Number:
		if n, err := v.Int64(); err == nil && n >= 0 {
			return uint64(n)
		}
	}
	return 0
}

// 兜底出口按「根目录 + 应用名」缓存，避免初始化早期的日志反复开关文件。
var (
	fallbackMu    sync.Mutex
	fallbackSinks = map[string]*dailyFileSink{}
)

func fallbackRuntimeSink(rootDir, appName string) *dailyFileSink {
	key := rootDir + "\x00" + appName

	fallbackMu.Lock()
	defer fallbackMu.Unlock()

	if sink, ok := fallbackSinks[key]; ok {
		return sink
	}
	sink := newDailyFileSink(rootDir, RuntimeLogDirName, appName)
	fallbackSinks[key] = sink
	return sink
}

// closeFallbackSinks 关闭兜底出口（停机时调用）。
func closeFallbackSinks() {
	fallbackMu.Lock()
	defer fallbackMu.Unlock()

	for key, sink := range fallbackSinks {
		_ = sink.Close()
		delete(fallbackSinks, key)
	}
}
