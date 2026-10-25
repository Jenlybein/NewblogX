//go:build integration

// L.3 验收：生产入口（EmitActionAudit / EmitLoginEvent / EmitCdcEvent /
// EmitReplayEvent / logrus 钩子）接到管道后，日志真的进了 ClickHouse。
//
// 运行方式（默认用 testcontainers 自动起 ClickHouse；没有 Docker 时 skip）：
//
//	go test -tags integration -run Integration ./service/log_service/
//
// 复用已有实例时设 BLOGX_CH_ADDR=host:9000。
package log_service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myblogx/conf"
	"myblogx/internal/platform/idgen"
	"myblogx/internal/testkit/chcontainer"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/sirupsen/logrus"
)

func openIntegrationCH(t *testing.T) *sql.DB {
	t.Helper()

	addr := chcontainer.Addr(t)

	db := clickhouse.OpenDB(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: "blogx_logs",
			Username: "default",
			Password: "",
		},
		DialTimeout: 5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("ClickHouse 不可用（%s），跳过集成测试: %v", addr, err)
	}
	return db
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()

	var n int
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("查询失败: %v\nSQL: %s", err, query)
	}
	return n
}

// 端到端：1000 条审计日志走完「内存队列 → 批量 → ClickHouse」，
// 且四类事件都能被各自的表接住（列名对不上就会静默丢字段，这里靠
// 读回关键列来兜住）。
func TestIntegrationPipelineWritesAllKinds(t *testing.T) {
	db := openIntegrationCH(t)
	t.Cleanup(func() { _ = db.Close() })

	// 雪花 ID 保证 event_id 唯一；否则同毫秒内的审计日志会被
	// ReplacingMergeTree 当成重复行合并掉，count 断言会飘。
	if err := idgen.Init(1); err != nil {
		t.Fatalf("初始化雪花 ID 失败: %v", err)
	}

	root := t.TempDir()
	deps := Options{
		LogConfig:        conf.Logrus{App: "itest", Dir: root, WALDir: filepath.Join(root, "wal")},
		SystemConfig:     conf.System{ServerID: 1, Env: "integration"},
		ClickHouseEnable: true,
		ClickHouse:       db,
	}

	if err := InitStructuredLog(PipelineConfig{
		Options:    deps,
		WALDir:     filepath.Join(root, "wal"),
		ClickHouse: true,
	}); err != nil {
		t.Fatalf("初始化管道失败: %v", err)
	}
	t.Cleanup(func() { _ = CloseStructuredLog(context.Background()) })

	marker := fmt.Sprintf("m3l3-%d", time.Now().UnixNano())

	const total = 1000
	for i := 0; i < total; i++ {
		EmitActionAudit(deps, ActionAuditInput{
			ActionName: marker,
			TargetType: "article",
			TargetID:   fmt.Sprint(i),
			Method:     "POST",
			Path:       "/api/articles",
			StatusCode: 200,
			Success:    true,
			UserID:     7,
			IP:         "127.0.0.1",
			RequestID:  fmt.Sprintf("%s-%d", marker, i),
		})
	}

	EmitLoginEvent(deps, LoginEventInput{
		EventName: marker,
		Username:  "alice",
		Success:   true,
		UserID:    7,
		IP:        "127.0.0.1",
	})
	EmitCDCEvent(deps, CdcEventInput{
		CdcJobID:    marker,
		Stream:      "blogx",
		SourceTable: "articles",
		Action:      "insert",
		TargetKey:   "1",
		Result:      "success",
	})
	EmitReplayEvent(deps, ReplayEventInput{
		Message:     marker,
		CdcJobID:    marker,
		Stream:      "blogx",
		SourceTable: "articles",
		Action:      "replay",
		TargetKey:   "1",
		Result:      "success",
	})

	// runtime 走 logrus 桥：这条路径就是 HTTP 访问日志的生产入口
	// （core.StructuredLogHook 只是这里的三行转发，在 core 包里单独测）。
	EmitRuntimeLog(root, "itest", &logrus.Entry{
		Logger:  logrus.New(),
		Time:    time.Now(),
		Level:   logrus.InfoLevel,
		Message: marker,
		Data: logrus.Fields{
			"event_id":    uint64(999999),
			"request_id":  marker,
			"event_name":  "http_request",
			"method":      "GET",
			"path":        "/api/_m3l3",
			"status_code": 200,
			"latency_ms":  3,
			"biz_marker":  marker,
		},
	})

	// 关闭时把队列刷完。
	if err := CloseStructuredLog(context.Background()); err != nil {
		t.Fatalf("关闭管道失败: %v", err)
	}

	// 1) 审计日志：1000 条都在，且 event_id 不重复。
	gotAudit := countRows(t, db,
		"SELECT count(DISTINCT event_id) FROM blogx_logs.action_audit_logs WHERE action_name = ?", marker)
	if gotAudit != total {
		t.Fatalf("审计日志期望 %d 条，实际 %d 条", total, gotAudit)
	}

	// 2) 关键扩展列必须真的写进去了（列名漂移时这里会变 0）。
	gotWithColumns := countRows(t, db,
		`SELECT count() FROM blogx_logs.action_audit_logs
		 WHERE action_name = ? AND method = 'POST' AND path = '/api/articles'
		   AND status_code = 200 AND success = 1 AND user_id = 7`, marker)
	if gotWithColumns != total {
		t.Fatalf("审计日志扩展列缺失：期望 %d 条，实际 %d 条", total, gotWithColumns)
	}

	// 3) 其余三类事件各 1 条。
	if n := countRows(t, db, "SELECT count() FROM blogx_logs.login_event_logs WHERE event_name = ?", marker); n != 1 {
		t.Fatalf("登录事件期望 1 条，实际 %d 条", n)
	}
	if n := countRows(t, db, "SELECT count() FROM blogx_logs.cdc_event_logs WHERE cdc_job_id = ?", marker); n != 1 {
		t.Fatalf("CDC 事件期望 1 条，实际 %d 条", n)
	}
	if n := countRows(t, db, "SELECT count() FROM blogx_logs.replay_event_logs WHERE message = ?", marker); n != 1 {
		t.Fatalf("回放事件期望 1 条，实际 %d 条", n)
	}

	// 4) runtime 日志：HTTP 列与 extra_json 都要落到表里。
	if n := countRows(t, db,
		`SELECT count() FROM blogx_logs.runtime_logs
		 WHERE message = ? AND level = 'info' AND method = 'GET'
		   AND path = '/api/_m3l3' AND status_code = 200 AND latency_ms = 3
		   AND extra_json LIKE '%biz_marker%'`, marker); n != 1 {
		t.Fatalf("runtime 日志期望 1 条且扩展字段进 extra_json，实际 %d 条", n)
	}
}

// 结构契约：每个事件结构体编码出的字段名，都必须是表中的真实列。
//
// ClickHouse 的 JSONEachRow 默认跳过未知字段（input_format_skip_unknown_fields=1），
// 所以"列名改了、代码没改"这类漂移不会在运行时报错——只会静默丢数据。
// 这条测试就是补这个缺口的：JSON 键 ⊆ 表列。
func TestIntegrationEventStructsMatchTableColumns(t *testing.T) {
	db := openIntegrationCH(t)
	t.Cleanup(func() { _ = db.Close() })

	type testCase struct {
		table string
		rows  []any
	}

	runtimeEntry := &logrus.Entry{
		Time:    time.Now(),
		Level:   logrus.InfoLevel,
		Message: "sample",
		Data: logrus.Fields{
			"event_id":       uint64(1),
			"ts":             "2026-10-24 00:00:00.000",
			"level":          "info",
			"trace_id":       "t",
			"span_id":        "s",
			"parent_span_id": "p",
			"service":        "svc",
			"env":            "test",
			"instance_id":    "1",
			"event_name":     "e",
			"error_code":     "c",
			"error_message":  "m",
			"message":        "sample",
			"request_id":     "r",
			"log_kind":       "runtime",
			"host":           "h",
			"file":           "f",
			"func":           "fun",
			"user_id":        uint64(1),
			"ip":             "1.1.1.1",
			"method":         "GET",
			"path":           "/p",
			"status_code":    200,
			"latency_ms":     1,
			"error_type":     "t",
			"error_stack":    "s",
			"extra_json":     "{}",
		},
	}

	cases := []testCase{
		{"runtime_logs", []any{buildRuntimeRow(runtimeEntry)}},
		{"action_audit_logs", []any{ActionAuditEvent{
			baseEvent:    fullBaseEventForTest(),
			Method:       "POST",
			Path:         "/p",
			StatusCode:   200,
			ActionName:   "a",
			TargetType:   "t",
			TargetID:     "1",
			Success:      1,
			RequestBody:  "{}",
			ResponseBody: "{}",
		}}},
		{"login_event_logs", []any{LoginEvent{
			baseEvent: fullBaseEventForTest(),
			Username:  "alice",
			LoginType: "password",
			Success:   1,
			Reason:    "ok",
			Addr:      "addr",
			UA:        "ua",
		}}},
		{"cdc_event_logs", []any{CdcEvent{
			baseEvent:   fullBaseEventForTest(),
			CdcJobID:    "job",
			Stream:      "blogx",
			SourceTable: "articles",
			Action:      "insert",
			TargetKey:   "1",
			RetryCount:  1,
			Result:      "success",
		}}},
		{"replay_event_logs", []any{ReplayEvent{
			baseEvent:   fullBaseEventForTest(),
			CdcJobID:    "job",
			Stream:      "blogx",
			SourceTable: "articles",
			Action:      "replay",
			TargetKey:   "1",
			RetryCount:  1,
			Result:      "success",
		}}},
	}

	for _, c := range cases {
		columns := tableColumns(t, db, c.table)
		for _, row := range c.rows {
			for _, key := range jsonKeys(t, row) {
				if !columns[key] {
					t.Errorf("表 %s 没有列 %q——该字段会被 ClickHouse 静默丢弃", c.table, key)
				}
			}
		}
	}
}

// fullBaseEventForTest 构造一份所有字段都非空的 baseEvent，
// 这样 omitempty 不会把列藏起来，契约检查才是完整的。
func fullBaseEventForTest() baseEvent {
	return baseEvent{
		EventID:      1,
		TS:           "2026-10-24 00:00:00.000",
		LogKind:      "test",
		Service:      "svc",
		Env:          "test",
		Host:         "h",
		InstanceID:   "1",
		Level:        "info",
		Message:      "m",
		RequestID:    "r",
		TraceID:      "t",
		SpanID:       "s",
		ParentSpanID: "p",
		EventName:    "e",
		ErrorCode:    "c",
		ErrorMessage: "m",
		UserID:       1,
		IP:           "1.1.1.1",
		ExtraJSON:    "{}",
	}
}

func tableColumns(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, "DESCRIBE TABLE blogx_logs."+table)
	if err != nil {
		t.Fatalf("DESCRIBE %s 失败: %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	columns := map[string]bool{}
	for rows.Next() {
		var name, typ string
		var defType, defaultExpr, comment, codecExpr, ttlExpr sql.NullString
		if err := rows.Scan(&name, &typ, &defType, &defaultExpr, &comment, &codecExpr, &ttlExpr); err != nil {
			t.Fatalf("读取 %s 列定义失败: %v", table, err)
		}
		columns[strings.TrimSpace(name)] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历 %s 列定义失败: %v", table, err)
	}
	if len(columns) == 0 {
		t.Fatalf("表 %s 没有任何列（建表脚本没跑？）", table)
	}
	return columns
}

// jsonKeys 返回结构体编码后的顶层键。
func jsonKeys(t *testing.T, row any) []string {
	t.Helper()

	line, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("编码样本失败: %v", err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(line, &body); err != nil {
		t.Fatalf("解析样本 JSON 失败: %v", err)
	}
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	return keys
}
