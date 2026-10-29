package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/page"
)

// 本文件是 AI 调用日志（ai_call_logs）的查询侧。
//
// 单独成文件而不是塞进 read.go：那边已经 650 多行，R6 的单文件 800 行上限
// 只剩不到 150 行的余量，而本文件自己的 WHERE 拼装 + 聚合也要 200 行。

// AICallRecord 对应 AI 调用日志列表与详情接口返回的单条记录。
type AICallRecord struct {
	EventID    ctype.ID `json:"event_id"`
	TS         string   `json:"ts"`
	Service    string   `json:"service"`
	Env        string   `json:"env"`
	Host       string   `json:"host"`
	InstanceID string   `json:"instance_id"`
	Level      string   `json:"level"`
	Message    string   `json:"message"`
	RequestID  string   `json:"request_id"`
	TraceID    string   `json:"trace_id"`
	UserID     ctype.ID `json:"user_id"`
	EventName  string   `json:"event_name"`

	Scene            string `json:"scene"`
	Vendor           string `json:"vendor"`
	Model            string `json:"model"`
	Stream           uint8  `json:"stream"`
	Success          uint8  `json:"success"`
	Attempts         uint8  `json:"attempts"`
	PromptTokens     uint32 `json:"prompt_tokens"`
	CompletionTokens uint32 `json:"completion_tokens"`
	TotalTokens      uint32 `json:"total_tokens"`
	CachedTokens     uint32 `json:"cached_tokens"`
	ReasoningTokens  uint32 `json:"reasoning_tokens"`
	UsageAvailable   uint8  `json:"usage_available"`
	LatencyMS        uint32 `json:"latency_ms"`

	ErrorCode string `json:"error_code"`
	ErrorMsg  string `json:"error_message"`
	ExtraJSON string `json:"extra_json"`
}

// AICallQuery 定义 AI 调用日志查询条件。
type AICallQuery struct {
	page.PageInfo
	LogTimeRange
	UserID    ctype.ID
	Scene     string
	Vendor    string
	Model     string
	Success   *bool
	ErrorCode string
}

// aiCallColumns 是列表与详情共用的列顺序，改这里必须同步改两处 Scan。
const aiCallColumns = `event_id, toString(ts), service, env, host, instance_id, level, message,
request_id, trace_id, user_id, event_name, scene, vendor, model, stream, success, attempts,
prompt_tokens, completion_tokens, total_tokens, cached_tokens, reasoning_tokens,
usage_available, latency_ms, error_code, error_message, extra_json`

// ListAICalls 按条件分页查询 AI 调用日志列表。
//
// 刻意不加关键词模糊搜索：与其它日志页一致，关键字由前端在当前页内过滤
// （后端 key 搜索会触发"必须同时给 7 天内时间范围"的保护，用户一点搜索就报错）。
func ListAICalls(opts Options, query AICallQuery) ([]AICallRecord, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	whereSQL, args, err := buildAIWhere(query)
	if err != nil {
		return nil, 0, err
	}
	limit, offset := normalizeLogPage(opts, query.PageInfo)
	count, err := queryCount(opts, ctx, fmt.Sprintf("SELECT count() FROM %s %s", AICallLogTableName, whereSQL), args...)
	if err != nil {
		return nil, 0, err
	}
	sqlText := fmt.Sprintf(`
SELECT %s
FROM %s %s
ORDER BY ts DESC, event_id DESC
LIMIT ? OFFSET ?`, aiCallColumns, AICallLogTableName, whereSQL)
	args = append(args, limit, offset)
	rows, err := opts.ClickHouse.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]AICallRecord, 0)
	for rows.Next() {
		var item AICallRecord
		if err = rows.Scan(
			&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
			&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.UserID, &item.EventName,
			&item.Scene, &item.Vendor, &item.Model, &item.Stream, &item.Success, &item.Attempts,
			&item.PromptTokens, &item.CompletionTokens, &item.TotalTokens, &item.CachedTokens,
			&item.ReasoningTokens, &item.UsageAvailable, &item.LatencyMS,
			&item.ErrorCode, &item.ErrorMsg, &item.ExtraJSON,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, count, rows.Err()
}

// GetAICall 按 event_id 查询单条 AI 调用日志详情。
func GetAICall(opts Options, eventID uint64) (*AICallRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := queryRowExists(opts, ctx, fmt.Sprintf(`
SELECT %s FROM %s WHERE event_id = ? LIMIT 1`, aiCallColumns, AICallLogTableName), eventID)
	var item AICallRecord
	if err := row.Scan(
		&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
		&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.UserID, &item.EventName,
		&item.Scene, &item.Vendor, &item.Model, &item.Stream, &item.Success, &item.Attempts,
		&item.PromptTokens, &item.CompletionTokens, &item.TotalTokens, &item.CachedTokens,
		&item.ReasoningTokens, &item.UsageAvailable, &item.LatencyMS,
		&item.ErrorCode, &item.ErrorMsg, &item.ExtraJSON,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

// buildAIWhere 根据 AI 调用筛选条件拼接 WHERE 子句。
func buildAIWhere(query AICallQuery) (string, []any, error) {
	where := []string{"WHERE 1 = 1"}
	args := make([]any, 0)
	if err := appendTimeRange(&where, &args, query.LogTimeRange); err != nil {
		return "", nil, err
	}
	if !query.UserID.IsZero() {
		where = append(where, "AND user_id = ?")
		args = append(args, query.UserID.Uint64())
	}
	appendEqual(&where, &args, "scene", query.Scene)
	appendEqual(&where, &args, "vendor", query.Vendor)
	appendEqual(&where, &args, "model", query.Model)
	appendEqual(&where, &args, "error_code", query.ErrorCode)
	if query.Success != nil {
		where = append(where, "AND success = ?")
		args = append(args, boolToUInt8(*query.Success))
	}
	return strings.Join(where, " "), args, nil
}
