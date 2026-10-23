package log_service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"myblogx/common"
	"myblogx/models/ctype"
)

// CdcEventRecord 对应 CDC 执行日志列表与详情接口返回的单条记录。
type CdcEventRecord struct {
	EventID      ctype.ID `json:"event_id"`
	TS           string   `json:"ts"`
	Service      string   `json:"service"`
	Env          string   `json:"env"`
	Host         string   `json:"host"`
	InstanceID   string   `json:"instance_id"`
	Level        string   `json:"level"`
	Message      string   `json:"message"`
	RequestID    string   `json:"request_id"`
	TraceID      string   `json:"trace_id"`
	EventName    string   `json:"event_name"`
	ErrorCode    string   `json:"error_code"`
	ErrorMessage string   `json:"error_message"`
	CdcJobID     string   `json:"cdc_job_id"`
	Stream       string   `json:"stream"`
	SourceTable  string   `json:"source_table"`
	Action       string   `json:"action"`
	TargetKey    string   `json:"target_key"`
	RetryCount   uint8    `json:"retry_count"`
	Result       string   `json:"result"`
	ExtraJSON    string   `json:"extra_json"`
}

// ReplayEventRecord 对应回放日志列表与详情接口返回的单条记录。
type ReplayEventRecord struct {
	EventID      ctype.ID `json:"event_id"`
	TS           string   `json:"ts"`
	Service      string   `json:"service"`
	Env          string   `json:"env"`
	Host         string   `json:"host"`
	InstanceID   string   `json:"instance_id"`
	Level        string   `json:"level"`
	Message      string   `json:"message"`
	RequestID    string   `json:"request_id"`
	TraceID      string   `json:"trace_id"`
	EventName    string   `json:"event_name"`
	ErrorCode    string   `json:"error_code"`
	ErrorMessage string   `json:"error_message"`
	CdcJobID     string   `json:"cdc_job_id"`
	Stream       string   `json:"stream"`
	SourceTable  string   `json:"source_table"`
	Action       string   `json:"action"`
	TargetKey    string   `json:"target_key"`
	RetryCount   uint8    `json:"retry_count"`
	Result       string   `json:"result"`
	ExtraJSON    string   `json:"extra_json"`
}

// CdcEventQuery 定义 CDC 执行日志查询条件。
type CdcEventQuery struct {
	common.PageInfo
	LogTimeRange
	Service         string
	Level           string
	Stream          string
	SourceTable     string
	Action          string
	Result          string
	RetryCount      *int
	RequestID       string
	RequestIDPrefix bool
	TraceID         string
	TraceIDPrefix   bool
	CdcJobID        string
	CdcJobIDPrefix  bool
	ErrorCode       string
	ErrorMessage    string
	EventName       string
	Module          string
}

// ReplayEventQuery 定义回放日志查询条件。
type ReplayEventQuery struct {
	common.PageInfo
	LogTimeRange
	Service         string
	Level           string
	Stream          string
	SourceTable     string
	Action          string
	Result          string
	RetryCount      *int
	RequestID       string
	RequestIDPrefix bool
	TraceID         string
	TraceIDPrefix   bool
	CdcJobID        string
	CdcJobIDPrefix  bool
	ErrorCode       string
	ErrorMessage    string
	EventName       string
	Module          string
}

// ListCdcEvents 按条件分页查询 CDC 执行日志列表。
func ListCdcEvents(deps Deps, query CdcEventQuery) ([]CdcEventRecord, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	whereSQL, args, err := buildCdcWhere(query)
	if err != nil {
		return nil, 0, err
	}
	limit, offset := normalizeLogPage(deps, query.PageInfo)
	count, err := queryCount(deps, ctx, fmt.Sprintf("SELECT count() FROM %s %s", CdcEventLogTableName, whereSQL), args...)
	if err != nil {
		return nil, 0, err
	}

	sqlText := fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, event_name, error_code, error_message, cdc_job_id, stream, source_table, action, target_key, retry_count, result, extra_json
FROM %s %s
ORDER BY ts DESC, event_id DESC
LIMIT ? OFFSET ?`, CdcEventLogTableName, whereSQL)
	args = append(args, limit, offset)
	rows, err := deps.ClickHouse.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]CdcEventRecord, 0)
	for rows.Next() {
		var item CdcEventRecord
		if err = rows.Scan(
			&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
			&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.EventName,
			&item.ErrorCode, &item.ErrorMessage, &item.CdcJobID, &item.Stream, &item.SourceTable,
			&item.Action, &item.TargetKey, &item.RetryCount, &item.Result, &item.ExtraJSON,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, count, rows.Err()
}

// GetCdcEvent 按 event_id 查询单条 CDC 执行日志详情。
func GetCdcEvent(deps Deps, eventID uint64) (*CdcEventRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := queryRowExists(deps, ctx, fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, event_name, error_code, error_message, cdc_job_id, stream, source_table, action, target_key, retry_count, result, extra_json
FROM %s WHERE event_id = ? LIMIT 1`, CdcEventLogTableName), eventID)
	var item CdcEventRecord
	if err := row.Scan(
		&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
		&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.EventName,
		&item.ErrorCode, &item.ErrorMessage, &item.CdcJobID, &item.Stream, &item.SourceTable,
		&item.Action, &item.TargetKey, &item.RetryCount, &item.Result, &item.ExtraJSON,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

// ListReplayEvents 按条件分页查询回放日志列表。
func ListReplayEvents(deps Deps, query ReplayEventQuery) ([]ReplayEventRecord, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	whereSQL, args, err := buildReplayWhere(query)
	if err != nil {
		return nil, 0, err
	}
	limit, offset := normalizeLogPage(deps, query.PageInfo)
	count, err := queryCount(deps, ctx, fmt.Sprintf("SELECT count() FROM %s %s", ReplayEventLogTableName, whereSQL), args...)
	if err != nil {
		return nil, 0, err
	}

	sqlText := fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, event_name, error_code, error_message, cdc_job_id, stream, source_table, action, target_key, retry_count, result, extra_json
FROM %s %s
ORDER BY ts DESC, event_id DESC
LIMIT ? OFFSET ?`, ReplayEventLogTableName, whereSQL)
	args = append(args, limit, offset)
	rows, err := deps.ClickHouse.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]ReplayEventRecord, 0)
	for rows.Next() {
		var item ReplayEventRecord
		if err = rows.Scan(
			&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
			&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.EventName,
			&item.ErrorCode, &item.ErrorMessage, &item.CdcJobID, &item.Stream, &item.SourceTable,
			&item.Action, &item.TargetKey, &item.RetryCount, &item.Result, &item.ExtraJSON,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, count, rows.Err()
}

// GetReplayEvent 按 event_id 查询单条回放日志详情。
func GetReplayEvent(deps Deps, eventID uint64) (*ReplayEventRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := queryRowExists(deps, ctx, fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, event_name, error_code, error_message, cdc_job_id, stream, source_table, action, target_key, retry_count, result, extra_json
FROM %s WHERE event_id = ? LIMIT 1`, ReplayEventLogTableName), eventID)
	var item ReplayEventRecord
	if err := row.Scan(
		&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
		&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.EventName,
		&item.ErrorCode, &item.ErrorMessage, &item.CdcJobID, &item.Stream, &item.SourceTable,
		&item.Action, &item.TargetKey, &item.RetryCount, &item.Result, &item.ExtraJSON,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

// buildCdcWhere 根据 CDC 执行日志筛选条件拼接 WHERE 子句。
func buildCdcWhere(query CdcEventQuery) (string, []any, error) {
	where := []string{"WHERE 1 = 1"}
	args := make([]any, 0)
	if err := ensureFuzzyTimeRange(query.LogTimeRange, query.ErrorMessage, query.EventName, query.Module); err != nil {
		return "", nil, err
	}
	if err := appendTimeRange(&where, &args, query.LogTimeRange); err != nil {
		return "", nil, err
	}
	appendEqual(&where, &args, "service", query.Service)
	appendEqual(&where, &args, "level", query.Level)
	appendEqual(&where, &args, "stream", query.Stream)
	appendEqual(&where, &args, "source_table", query.SourceTable)
	appendEqual(&where, &args, "action", query.Action)
	appendEqual(&where, &args, "result", query.Result)
	appendExactOrPrefix(&where, &args, "request_id", query.RequestID, query.RequestIDPrefix)
	appendExactOrPrefix(&where, &args, "trace_id", query.TraceID, query.TraceIDPrefix)
	appendExactOrPrefix(&where, &args, "cdc_job_id", query.CdcJobID, query.CdcJobIDPrefix)
	appendEqual(&where, &args, "error_code", query.ErrorCode)
	appendLike(&where, &args, "error_message", query.ErrorMessage)
	appendLike(&where, &args, "event_name", query.EventName)
	appendModuleLike(&where, &args, query.Module)
	if query.RetryCount != nil {
		where = append(where, "AND retry_count = ?")
		args = append(args, *query.RetryCount)
	}
	return strings.Join(where, " "), args, nil
}

// buildReplayWhere 根据回放日志筛选条件拼接 WHERE 子句。
func buildReplayWhere(query ReplayEventQuery) (string, []any, error) {
	where := []string{"WHERE 1 = 1"}
	args := make([]any, 0)
	if err := ensureFuzzyTimeRange(query.LogTimeRange, query.ErrorMessage, query.EventName, query.Module); err != nil {
		return "", nil, err
	}
	if err := appendTimeRange(&where, &args, query.LogTimeRange); err != nil {
		return "", nil, err
	}
	appendEqual(&where, &args, "service", query.Service)
	appendEqual(&where, &args, "level", query.Level)
	appendEqual(&where, &args, "stream", query.Stream)
	appendEqual(&where, &args, "source_table", query.SourceTable)
	appendEqual(&where, &args, "action", query.Action)
	appendEqual(&where, &args, "result", query.Result)
	appendExactOrPrefix(&where, &args, "request_id", query.RequestID, query.RequestIDPrefix)
	appendExactOrPrefix(&where, &args, "trace_id", query.TraceID, query.TraceIDPrefix)
	appendExactOrPrefix(&where, &args, "cdc_job_id", query.CdcJobID, query.CdcJobIDPrefix)
	appendEqual(&where, &args, "error_code", query.ErrorCode)
	appendLike(&where, &args, "error_message", query.ErrorMessage)
	appendLike(&where, &args, "event_name", query.EventName)
	appendModuleLike(&where, &args, query.Module)
	if query.RetryCount != nil {
		where = append(where, "AND retry_count = ?")
		args = append(args, *query.RetryCount)
	}
	return strings.Join(where, " "), args, nil
}
