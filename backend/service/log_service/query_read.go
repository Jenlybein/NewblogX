package log_service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"myblogx/common"
	"myblogx/models/ctype"
)

// RuntimeLogRecord 对应运行日志列表与详情接口返回的单条记录。
type RuntimeLogRecord struct {
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
	File       string   `json:"file"`
	Func       string   `json:"func"`
	UserID     ctype.ID `json:"user_id"`
	IP         string   `json:"ip"`
	Method     string   `json:"method"`
	Path       string   `json:"path"`
	StatusCode uint16   `json:"status_code"`
	LatencyMS  uint32   `json:"latency_ms"`
	EventName  string   `json:"event_name"`
	ErrorCode  string   `json:"error_code"`
	ErrorMsg   string   `json:"error_message"`
	ErrorType  string   `json:"error_type"`
	ErrorStack string   `json:"error_stack"`
	ExtraJSON  string   `json:"extra_json"`
}

// LoginEventRecord 对应登录事件日志列表与详情接口返回的单条记录。
type LoginEventRecord struct {
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
	IP         string   `json:"ip"`
	EventName  string   `json:"event_name"`
	ErrorCode  string   `json:"error_code"`
	ErrorMsg   string   `json:"error_message"`
	Username   string   `json:"username"`
	LoginType  string   `json:"login_type"`
	Success    uint8    `json:"success"`
	Reason     string   `json:"reason"`
	Addr       string   `json:"addr"`
	UA         string   `json:"ua"`
	ExtraJSON  string   `json:"extra_json"`
}

// ActionAuditRecord 对应操作审计日志列表与详情接口返回的单条记录。
type ActionAuditRecord struct {
	EventID           ctype.ID `json:"event_id"`
	TS                string   `json:"ts"`
	Service           string   `json:"service"`
	Env               string   `json:"env"`
	Host              string   `json:"host"`
	InstanceID        string   `json:"instance_id"`
	Level             string   `json:"level"`
	Message           string   `json:"message"`
	RequestID         string   `json:"request_id"`
	TraceID           string   `json:"trace_id"`
	UserID            ctype.ID `json:"user_id"`
	IP                string   `json:"ip"`
	Method            string   `json:"method"`
	Path              string   `json:"path"`
	StatusCode        uint16   `json:"status_code"`
	ErrorCode         string   `json:"error_code"`
	ErrorMsg          string   `json:"error_message"`
	ActionName        string   `json:"action_name"`
	TargetType        string   `json:"target_type"`
	TargetID          string   `json:"target_id"`
	Success           uint8    `json:"success"`
	RequestBody       string   `json:"request_body"`
	ResponseBody      string   `json:"response_body"`
	RequestBodyRaw    string   `json:"request_body_raw,omitempty"`
	ResponseBodyRaw   string   `json:"response_body_raw,omitempty"`
	RequestHeaderRaw  string   `json:"request_header_raw,omitempty"`
	ResponseHeaderRaw string   `json:"response_header_raw,omitempty"`
	ExtraJSON         string   `json:"extra_json"`
}

// LogTimeRange 表示日志查询使用的开始和结束时间范围。
type LogTimeRange struct {
	StartAt string
	EndAt   string
}

// RuntimeLogQuery 定义运行日志查询条件。
type RuntimeLogQuery struct {
	common.PageInfo
	LogTimeRange
	Service         string
	Level           string
	Host            string
	Method          string
	Path            string
	UserID          ctype.ID
	Key             string
	RequestID       string
	RequestIDPrefix bool
	TraceID         string
	TraceIDPrefix   bool
	ErrorCode       string
	ErrorMessage    string
	EventName       string
	Module          string
}

// LoginEventQuery 定义登录事件日志查询条件。
type LoginEventQuery struct {
	common.PageInfo
	LogTimeRange
	UserID          ctype.ID
	IP              string
	Username        string
	LoginType       string
	EventName       string
	EventNameLike   string
	Success         *bool
	RequestID       string
	RequestIDPrefix bool
	TraceID         string
	TraceIDPrefix   bool
	ErrorCode       string
	ErrorMessage    string
	Module          string
}

// ActionAuditQuery 定义操作审计日志查询条件。
type ActionAuditQuery struct {
	common.PageInfo
	LogTimeRange
	UserID          ctype.ID
	IP              string
	ActionName      string
	TargetType      string
	TargetID        string
	Success         *bool
	RequestID       string
	RequestIDPrefix bool
	TraceID         string
	TraceIDPrefix   bool
	ErrorCode       string
	ErrorMessage    string
	EventNameLike   string
	Path            string
	Module          string
}

// ListRuntimeLogs 按条件分页查询运行日志列表。
func ListRuntimeLogs(deps Deps, query RuntimeLogQuery) ([]RuntimeLogRecord, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	whereSQL, args, err := buildRuntimeWhere(query)
	if err != nil {
		return nil, 0, err
	}
	limit, offset := normalizeLogPage(deps, query.PageInfo)
	countQuery := fmt.Sprintf("SELECT count() FROM %s %s", RuntimeLogTableName, whereSQL)
	count, err := queryCount(deps, ctx, countQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	sqlText := fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, file, func, user_id, ip, method, path, status_code, latency_ms, event_name, error_code, error_message, error_type, error_stack, extra_json
FROM %s %s
ORDER BY ts DESC, event_id DESC
LIMIT ? OFFSET ?`, RuntimeLogTableName, whereSQL)
	args = append(args, limit, offset)

	rows, err := deps.ClickHouse.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]RuntimeLogRecord, 0)
	for rows.Next() {
		var item RuntimeLogRecord
		if err = rows.Scan(
			&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
			&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.File, &item.Func,
			&item.UserID, &item.IP, &item.Method, &item.Path, &item.StatusCode, &item.LatencyMS,
			&item.EventName, &item.ErrorCode, &item.ErrorMsg, &item.ErrorType, &item.ErrorStack, &item.ExtraJSON,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, count, rows.Err()
}

// GetRuntimeLog 按 event_id 查询单条运行日志详情。
func GetRuntimeLog(deps Deps, eventID uint64) (*RuntimeLogRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := queryRowExists(deps, ctx, fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, file, func, user_id, ip, method, path, status_code, latency_ms, event_name, error_code, error_message, error_type, error_stack, extra_json
FROM %s WHERE event_id = ? LIMIT 1`, RuntimeLogTableName), eventID)
	var item RuntimeLogRecord
	if err := row.Scan(
		&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
		&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.File, &item.Func,
		&item.UserID, &item.IP, &item.Method, &item.Path, &item.StatusCode, &item.LatencyMS,
		&item.EventName, &item.ErrorCode, &item.ErrorMsg, &item.ErrorType, &item.ErrorStack, &item.ExtraJSON,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

// ListLoginEvents 按条件分页查询登录事件日志列表。
func ListLoginEvents(deps Deps, query LoginEventQuery) ([]LoginEventRecord, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	whereSQL, args, err := buildLoginWhere(query)
	if err != nil {
		return nil, 0, err
	}
	limit, offset := normalizeLogPage(deps, query.PageInfo)
	count, err := queryCount(deps, ctx, fmt.Sprintf("SELECT count() FROM %s %s", LoginEventLogTableName, whereSQL), args...)
	if err != nil {
		return nil, 0, err
	}
	sqlText := fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, user_id, ip, event_name, error_code, error_message, username, login_type, success, reason, addr, ua, extra_json
FROM %s %s
ORDER BY ts DESC, event_id DESC
LIMIT ? OFFSET ?`, LoginEventLogTableName, whereSQL)
	args = append(args, limit, offset)
	rows, err := deps.ClickHouse.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]LoginEventRecord, 0)
	for rows.Next() {
		var item LoginEventRecord
		if err = rows.Scan(
			&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
			&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.UserID, &item.IP,
			&item.EventName, &item.ErrorCode, &item.ErrorMsg, &item.Username, &item.LoginType, &item.Success, &item.Reason,
			&item.Addr, &item.UA, &item.ExtraJSON,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, count, rows.Err()
}

// GetLoginEvent 按 event_id 查询单条登录事件日志详情。
func GetLoginEvent(deps Deps, eventID uint64) (*LoginEventRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := queryRowExists(deps, ctx, fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, user_id, ip, event_name, error_code, error_message, username, login_type, success, reason, addr, ua, extra_json
FROM %s WHERE event_id = ? LIMIT 1`, LoginEventLogTableName), eventID)
	var item LoginEventRecord
	if err := row.Scan(
		&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
		&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.UserID, &item.IP,
		&item.EventName, &item.ErrorCode, &item.ErrorMsg, &item.Username, &item.LoginType, &item.Success, &item.Reason,
		&item.Addr, &item.UA, &item.ExtraJSON,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

// ListActionAudits 按条件分页查询操作审计日志列表。
func ListActionAudits(deps Deps, query ActionAuditQuery) ([]ActionAuditRecord, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	whereSQL, args, err := buildActionWhere(query)
	if err != nil {
		return nil, 0, err
	}
	limit, offset := normalizeLogPage(deps, query.PageInfo)
	count, err := queryCount(deps, ctx, fmt.Sprintf("SELECT count() FROM %s %s", ActionAuditLogTableName, whereSQL), args...)
	if err != nil {
		return nil, 0, err
	}
	sqlText := fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, user_id, ip, method, path, status_code, error_code, error_message, action_name, target_type, target_id, success, request_body, response_body, extra_json
FROM %s %s
ORDER BY ts DESC, event_id DESC
LIMIT ? OFFSET ?`, ActionAuditLogTableName, whereSQL)
	args = append(args, limit, offset)
	rows, err := deps.ClickHouse.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]ActionAuditRecord, 0)
	for rows.Next() {
		var item ActionAuditRecord
		if err = rows.Scan(
			&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
			&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.UserID, &item.IP,
			&item.Method, &item.Path, &item.StatusCode, &item.ErrorCode, &item.ErrorMsg, &item.ActionName, &item.TargetType,
			&item.TargetID, &item.Success, &item.RequestBody, &item.ResponseBody, &item.ExtraJSON,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, count, rows.Err()
}

// GetActionAudit 按 event_id 查询单条操作审计日志详情。
func GetActionAudit(deps Deps, eventID uint64) (*ActionAuditRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := queryRowExists(deps, ctx, fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, user_id, ip, method, path, status_code, error_code, error_message, action_name, target_type, target_id, success, request_body, response_body, request_body_raw, response_body_raw, request_header_raw, response_header_raw, extra_json
FROM %s WHERE event_id = ? LIMIT 1`, ActionAuditLogTableName), eventID)
	var item ActionAuditRecord
	if err := row.Scan(
		&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
		&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.UserID, &item.IP,
		&item.Method, &item.Path, &item.StatusCode, &item.ErrorCode, &item.ErrorMsg, &item.ActionName, &item.TargetType,
		&item.TargetID, &item.Success, &item.RequestBody, &item.ResponseBody, &item.RequestBodyRaw, &item.ResponseBodyRaw, &item.RequestHeaderRaw, &item.ResponseHeaderRaw, &item.ExtraJSON,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

// CountDistinctLoginUsersSince 统计指定时间之后成功登录的去重用户数。
func CountDistinctLoginUsersSince(deps Deps, since time.Time) (int64, error) {
	if !clickhouseEnabled(deps) {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return queryCount(deps, ctx, fmt.Sprintf("SELECT count(DISTINCT user_id) FROM %s WHERE event_name = ? AND success = 1 AND ts >= ?", LoginEventLogTableName), "login_success", since.Format(clickhouseTimeLayout))
}

// LoadLatestLoginMap 批量加载用户最近一次成功登录事件，供后台用户列表补充登录信息。
func LoadLatestLoginMap(deps Deps, userIDs []ctype.ID) (map[ctype.ID]LoginEventRecord, error) {
	result := make(map[ctype.ID]LoginEventRecord)
	if !clickhouseEnabled(deps) {
		return result, nil
	}
	if len(userIDs) == 0 {
		return result, nil
	}

	values := make([]uint64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID == 0 {
			continue
		}
		values = append(values, uint64(userID))
	}
	if len(values) == 0 {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	placeholders := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for _, value := range values {
		placeholders = append(placeholders, "?")
		args = append(args, value)
	}
	rows, err := deps.ClickHouse.QueryContext(ctx, fmt.Sprintf(`
SELECT event_id, toString(ts), service, env, host, instance_id, level, message, request_id, trace_id, user_id, ip, event_name, username, login_type, success, reason, addr, ua, extra_json
FROM %s
WHERE user_id IN (%s) AND event_name = 'login_success' AND success = 1
ORDER BY user_id, ts DESC, event_id DESC
LIMIT 1 BY user_id`, LoginEventLogTableName, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item LoginEventRecord
		if err = rows.Scan(
			&item.EventID, &item.TS, &item.Service, &item.Env, &item.Host, &item.InstanceID,
			&item.Level, &item.Message, &item.RequestID, &item.TraceID, &item.UserID, &item.IP,
			&item.EventName, &item.Username, &item.LoginType, &item.Success, &item.Reason,
			&item.Addr, &item.UA, &item.ExtraJSON,
		); err != nil {
			return nil, err
		}
		result[item.UserID] = item
	}
	return result, rows.Err()
}

// normalizeLogPage 统一处理后台日志分页参数和默认限制。
func normalizeLogPage(deps Deps, pageInfo common.PageInfo) (limit int, offset int) {
	defaultLimit := 20
	if deps.LogConfig.QueryDefaultLimit > 0 {
		defaultLimit = deps.LogConfig.QueryDefaultLimit
	}
	maxLimit := 200
	if deps.LogConfig.QueryMaxLimit > 0 {
		maxLimit = deps.LogConfig.QueryMaxLimit
	}

	limit = pageInfo.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	page := pageInfo.Page
	if page <= 0 {
		page = 1
	}
	offset = (page - 1) * limit
	return
}

// buildRuntimeWhere 根据运行日志筛选条件拼接 WHERE 子句。
func buildRuntimeWhere(query RuntimeLogQuery) (string, []any, error) {
	where := []string{"WHERE 1 = 1"}
	args := make([]any, 0)
	if err := ensureFuzzyTimeRange(query.LogTimeRange, query.Key, query.Path, query.Module, query.ErrorMessage, query.EventName); err != nil {
		return "", nil, err
	}
	if err := appendTimeRange(&where, &args, query.LogTimeRange); err != nil {
		return "", nil, err
	}
	appendEqual(&where, &args, "service", query.Service)
	appendEqual(&where, &args, "level", query.Level)
	appendEqual(&where, &args, "host", query.Host)
	appendEqual(&where, &args, "method", query.Method)
	appendExactOrPrefix(&where, &args, "request_id", query.RequestID, query.RequestIDPrefix)
	appendExactOrPrefix(&where, &args, "trace_id", query.TraceID, query.TraceIDPrefix)
	appendEqual(&where, &args, "error_code", query.ErrorCode)
	appendLike(&where, &args, "error_message", query.ErrorMessage)
	appendLike(&where, &args, "event_name", query.EventName)
	appendLike(&where, &args, "path", query.Path)
	appendModuleLike(&where, &args, query.Module)
	if query.UserID > 0 {
		where = append(where, "AND user_id = ?")
		args = append(args, uint64(query.UserID))
	}
	appendKeyword(&where, &args, query.Key, "message", "path", "request_id", "trace_id", "error_message", "event_name")
	return strings.Join(where, " "), args, nil
}

// buildLoginWhere 根据登录事件筛选条件拼接 WHERE 子句。
func buildLoginWhere(query LoginEventQuery) (string, []any, error) {
	where := []string{"WHERE 1 = 1"}
	args := make([]any, 0)
	if err := ensureFuzzyTimeRange(query.LogTimeRange, query.Username, query.EventNameLike, query.ErrorMessage, query.Module); err != nil {
		return "", nil, err
	}
	if err := appendTimeRange(&where, &args, query.LogTimeRange); err != nil {
		return "", nil, err
	}
	if query.UserID > 0 {
		where = append(where, "AND user_id = ?")
		args = append(args, uint64(query.UserID))
	}
	appendEqual(&where, &args, "ip", query.IP)
	appendLike(&where, &args, "username", query.Username)
	appendEqual(&where, &args, "login_type", query.LoginType)
	appendEqual(&where, &args, "event_name", query.EventName)
	appendLike(&where, &args, "event_name", query.EventNameLike)
	appendExactOrPrefix(&where, &args, "request_id", query.RequestID, query.RequestIDPrefix)
	appendExactOrPrefix(&where, &args, "trace_id", query.TraceID, query.TraceIDPrefix)
	appendEqual(&where, &args, "error_code", query.ErrorCode)
	appendLike(&where, &args, "error_message", query.ErrorMessage)
	appendModuleLike(&where, &args, query.Module)
	if query.Success != nil {
		where = append(where, "AND success = ?")
		args = append(args, boolToUInt8(*query.Success))
	}
	return strings.Join(where, " "), args, nil
}

// buildActionWhere 根据操作审计筛选条件拼接 WHERE 子句。
func buildActionWhere(query ActionAuditQuery) (string, []any, error) {
	where := []string{"WHERE 1 = 1"}
	args := make([]any, 0)
	if err := ensureFuzzyTimeRange(query.LogTimeRange, query.TargetID, query.Path, query.EventNameLike, query.ErrorMessage, query.Module); err != nil {
		return "", nil, err
	}
	if err := appendTimeRange(&where, &args, query.LogTimeRange); err != nil {
		return "", nil, err
	}
	if query.UserID > 0 {
		where = append(where, "AND user_id = ?")
		args = append(args, uint64(query.UserID))
	}
	appendEqual(&where, &args, "ip", query.IP)
	appendEqual(&where, &args, "action_name", query.ActionName)
	appendEqual(&where, &args, "target_type", query.TargetType)
	appendLike(&where, &args, "path", query.Path)
	appendLike(&where, &args, "event_name", query.EventNameLike)
	appendExactOrPrefix(&where, &args, "request_id", query.RequestID, query.RequestIDPrefix)
	appendExactOrPrefix(&where, &args, "trace_id", query.TraceID, query.TraceIDPrefix)
	appendEqual(&where, &args, "error_code", query.ErrorCode)
	appendLike(&where, &args, "error_message", query.ErrorMessage)
	appendLike(&where, &args, "target_id", query.TargetID)
	appendModuleLike(&where, &args, query.Module)
	if query.Success != nil {
		where = append(where, "AND success = ?")
		args = append(args, boolToUInt8(*query.Success))
	}
	return strings.Join(where, " "), args, nil
}

// appendTimeRange 将时间范围条件追加到 WHERE 子句，未传值时默认查询最近 7 天。
func appendTimeRange(where *[]string, args *[]any, rng LogTimeRange) error {
	if rng.StartAt == "" && rng.EndAt == "" {
		now := time.Now()
		*where = append(*where, "AND ts >= ?")
		*args = append(*args, now.Add(-7*24*time.Hour).Format(clickhouseTimeLayout))
		return nil
	}
	if rng.StartAt != "" {
		startAt, err := parseLogTime(rng.StartAt)
		if err != nil {
			return err
		}
		*where = append(*where, "AND ts >= ?")
		*args = append(*args, startAt.Format(clickhouseTimeLayout))
	}
	if rng.EndAt != "" {
		endAt, err := parseLogTime(rng.EndAt)
		if err != nil {
			return err
		}
		*where = append(*where, "AND ts <= ?")
		*args = append(*args, endAt.Format(clickhouseTimeLayout))
	}
	return nil
}

// appendEqual 追加精确匹配条件。
func appendEqual(where *[]string, args *[]any, column string, value string) {
	if value == "" {
		return
	}
	*where = append(*where, fmt.Sprintf("AND %s = ?", column))
	*args = append(*args, value)
}

// appendLike 追加不区分大小写的模糊匹配条件。
func appendLike(where *[]string, args *[]any, column string, value string) {
	if value == "" {
		return
	}
	*where = append(*where, fmt.Sprintf("AND positionCaseInsensitive(%s, ?) > 0", column))
	*args = append(*args, value)
}

// appendExactOrPrefix 追加“精确匹配或前缀匹配”条件。
func appendExactOrPrefix(where *[]string, args *[]any, column string, value string, prefix bool) {
	if value == "" {
		return
	}
	if prefix {
		*where = append(*where, fmt.Sprintf("AND startsWith(%s, ?)", column))
		*args = append(*args, value)
		return
	}
	*where = append(*where, fmt.Sprintf("AND %s = ?", column))
	*args = append(*args, value)
}

// appendModuleLike 兼容 module 未单列的场景，先匹配 service，再回退 extra_json。
func appendModuleLike(where *[]string, args *[]any, value string) {
	if value == "" {
		return
	}
	*where = append(*where, "AND (positionCaseInsensitive(service, ?) > 0 OR positionCaseInsensitive(extra_json, ?) > 0)")
	*args = append(*args, value, value)
}

// appendKeyword 将同一个关键词同时作用于多个列的模糊搜索。
func appendKeyword(where *[]string, args *[]any, key string, columns ...string) {
	if key == "" || len(columns) == 0 {
		return
	}
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, fmt.Sprintf("positionCaseInsensitive(%s, ?) > 0", column))
		*args = append(*args, key)
	}
	*where = append(*where, "AND ("+strings.Join(parts, " OR ")+")")
}

// ensureFuzzyTimeRange 对模糊搜索做重查询保护：必须显式时间范围，且范围不超过 7 天。
func ensureFuzzyTimeRange(rng LogTimeRange, fuzzyValues ...string) error {
	hasFuzzy := false
	for _, value := range fuzzyValues {
		if strings.TrimSpace(value) != "" {
			hasFuzzy = true
			break
		}
	}
	if !hasFuzzy {
		return nil
	}
	if strings.TrimSpace(rng.StartAt) == "" || strings.TrimSpace(rng.EndAt) == "" {
		return fmt.Errorf("模糊搜索必须同时传 start_at 和 end_at")
	}
	startAt, err := parseLogTime(rng.StartAt)
	if err != nil {
		return err
	}
	endAt, err := parseLogTime(rng.EndAt)
	if err != nil {
		return err
	}
	if endAt.Before(startAt) {
		return fmt.Errorf("end_at 不能早于 start_at")
	}
	if endAt.Sub(startAt) > 7*24*time.Hour {
		return fmt.Errorf("模糊搜索时间范围不能超过 7 天")
	}
	return nil
}

// parseLogTime 解析后台日志查询使用的时间字符串。
func parseLogTime(value string) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("时间格式错误，应为 2006-01-02 15:04:05")
	}
	return parsed, nil
}
