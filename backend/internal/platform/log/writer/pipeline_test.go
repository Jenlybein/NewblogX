package writer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"myblogx/conf"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/sirupsen/logrus"
)

// testOptions 构造一份指向临时目录的日志依赖，避免污染仓库里的 logs/。
func testOptions(t *testing.T, rootDir string) Options {
	t.Helper()
	return Options{
		LogConfig:    conf.Logrus{App: "app", Dir: rootDir, WALDir: filepath.Join(rootDir, "wal")},
		SystemConfig: conf.System{ServerID: 1, Env: "test"},
	}
}

// mustInit 安装管道并在测试结束时收尾。
func mustInit(t *testing.T, cfg PipelineConfig) {
	t.Helper()
	if err := InitStructuredLog(cfg); err != nil {
		t.Fatalf("初始化日志管道失败: %v", err)
	}
	t.Cleanup(func() { _ = CloseStructuredLog(context.Background()) })
}

// 审计日志端到端：调用 EmitActionAudit 后，落盘内容应是 runtime_logs 之外
// 那张 action_audit_logs 表对应的行形态（字段名与 ClickHouse 列名一致）。
func TestPipelineWritesAuditToDailyFile(t *testing.T) {
	root := t.TempDir()
	deps := testOptions(t, root)
	mustInit(t, PipelineConfig{Options: deps})

	EmitActionAudit(deps, ActionAuditInput{
		ActionName: "article_create",
		TargetType: "article",
		TargetID:   "42",
		Method:     "POST",
		Path:       "/api/articles",
		StatusCode: 201,
		Success:    true,
		UserID:     7,
		IP:         "1.2.3.4",
		RequestID:  "req-1",
	})

	// 统计快照应能看到这一条（运维排查时靠它判断管道是否在工作）。
	stats := StructuredLogStats()
	if got := stats[ActionAuditLogDirName].Emitted; got != 1 {
		t.Fatalf("统计快照不符：期望 emitted=1，实际 %d（%+v）", got, stats)
	}

	// 关闭会把队列刷完；不关闭就断言文件内容会有竞态。
	if err := CloseStructuredLog(context.Background()); err != nil {
		t.Fatalf("关闭日志管道失败: %v", err)
	}

	path := filepath.Join(root, ActionAuditLogDirName, time.Now().Format("2006-01-02"), "app.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取审计日志失败: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("审计日志为空")
	}
	line := strings.TrimSpace(strings.Split(string(data), "\n")[0])

	var row map[string]any
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatalf("审计日志不是合法 JSON: %v\n%s", err, line)
	}

	checks := map[string]any{
		"log_kind":    "action_audit",
		"action_name": "article_create",
		"target_type": "article",
		"target_id":   "42",
		"method":      "POST",
		"path":        "/api/articles",
		"request_id":  "req-1",
		"ip":          "1.2.3.4",
		"service":     "app",
		"env":         "test",
	}
	for key, want := range checks {
		if row[key] != want {
			t.Errorf("字段 %s 不符：期望 %#v，实际 %#v（整行 %s）", key, want, row[key], line)
		}
	}
	if _, ok := row["event_id"]; !ok {
		t.Error("缺少 event_id")
	}
	if _, ok := row["ts"]; !ok {
		t.Error("缺少 ts")
	}
	if row["status_code"] != float64(201) {
		t.Errorf("status_code 不符: %#v", row["status_code"])
	}
	if row["success"] != float64(1) {
		t.Errorf("success 应编码为 1: %#v", row["success"])
	}
}

// 未启用 ClickHouse 时文件就是最终落点：不套 WAL，也不产生 wal 目录。
func TestPipelineFileModeDoesNotCreateWAL(t *testing.T) {
	root := t.TempDir()
	deps := testOptions(t, root)
	mustInit(t, PipelineConfig{Options: deps})

	EmitLoginEvent(deps, LoginEventInput{EventName: "login", Success: true, Username: "alice"})

	if err := CloseStructuredLog(context.Background()); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(root, "wal"))
	if err == nil && len(entries) > 0 {
		t.Fatalf("文件模式下不应产生 WAL 批次，实际 %d 项", len(entries))
	}
	if !os.IsNotExist(err) && err != nil {
		t.Fatalf("读取 wal 目录失败: %v", err)
	}
}

// 主出口不可用时，批次必须落进本地 WAL 而不是丢掉。
// 这里用一个指向 127.0.0.1:1（必然连不上）的 ClickHouse 连接模拟断库。
func TestPipelineFallsBackToWALWhenClickHouseDown(t *testing.T) {
	root := t.TempDir()
	deps := testOptions(t, root)

	db := clickhouse.OpenDB(&clickhouse.Options{
		Addr:        []string{"127.0.0.1:1"},
		DialTimeout: 300 * time.Millisecond,
	})
	defer func() { _ = db.Close() }()
	deps.ClickHouse = db
	deps.ClickHouseEnable = true

	mustInit(t, PipelineConfig{
		Options:    deps,
		WALDir:     filepath.Join(root, "wal"),
		ClickHouse: true,
	})

	for i := 0; i < 5; i++ {
		EmitActionAudit(deps, ActionAuditInput{
			ActionName: "article_update",
			TargetType: "article",
			TargetID:   "9",
			Success:    true,
		})
	}

	// 积压量必须可观测：写协程每秒刷一次，失败后才会落盘。
	deadline := time.Now().Add(5 * time.Second)
	for {
		if WALPending()[ActionAuditLogDirName] > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("主出口失败后 WAL 积压量始终为 0，说明没有落盘")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := CloseStructuredLog(context.Background()); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	walDir := filepath.Join(root, "wal", ActionAuditLogDirName)
	entries, err := os.ReadDir(walDir)
	if err != nil {
		t.Fatalf("WAL 目录不存在（日志丢了）: %v", err)
	}
	batch := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			batch++
		}
	}
	if batch == 0 {
		t.Fatal("主出口失败时应产生 WAL 批次，实际 0 个")
	}

	// 落盘内容必须是完整的 JSON 行，重放时才能直接用。
	var lines int
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(walDir, e.Name()))
		if err != nil {
			t.Fatalf("读取 WAL 失败: %v", err)
		}
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			if line == "" {
				continue
			}
			var row map[string]any
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				t.Fatalf("WAL 行不是合法 JSON: %v\n%s", err, line)
			}
			lines++
		}
	}
	if lines != 5 {
		t.Fatalf("WAL 里应保留 5 条审计日志，实际 %d 条", lines)
	}
}

// 文件出口按日期切换目录；同一天复用同一个句柄。
func TestDailyFileSinkRotatesByDate(t *testing.T) {
	root := t.TempDir()
	sink := newDailyFileSink(root, RuntimeLogDirName, "app")
	t.Cleanup(func() { _ = sink.Close() })

	day1 := time.Date(2026, 10, 23, 23, 59, 0, 0, time.Local)
	file1, err := sink.ensureFile(day1)
	if err != nil {
		t.Fatalf("打开第 1 天文件失败: %v", err)
	}

	again, err := sink.ensureFile(time.Date(2026, 10, 23, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("同日再次取文件失败: %v", err)
	}
	if again != file1 {
		t.Fatal("同一天应复用同一个文件句柄")
	}

	day2 := time.Date(2026, 10, 24, 0, 1, 0, 0, time.Local)
	file2, err := sink.ensureFile(day2)
	if err != nil {
		t.Fatalf("打开第 2 天文件失败: %v", err)
	}
	if file2 == file1 {
		t.Fatal("跨天应切换到新文件")
	}

	for _, day := range []string{"2026-10-23", "2026-10-24"} {
		path := filepath.Join(root, RuntimeLogDirName, day, "app.log")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("缺少 %s 的日志文件: %v", day, err)
		}
	}
}

// runtime 行映射：表里有的列按名取，表里没有的字段进 extra_json。
func TestBuildRuntimeRowMapsColumnsAndKeepsExtra(t *testing.T) {
	logger := logrus.New()
	logger.SetReportCaller(true)

	entry := &logrus.Entry{
		Logger:  logger,
		Time:    time.Date(2026, 10, 24, 9, 0, 0, 0, time.Local),
		Level:   logrus.WarnLevel,
		Message: "请求执行完成",
		Caller:  &runtime.Frame{File: "/app/api/x.go", Function: "api.Handler"},
		Data: logrus.Fields{
			"event_id":              uint64(99),
			"log_kind":              "runtime",
			"service":               "blogx",
			"env":                   "test",
			"instance_id":           "1",
			"host":                  "host-1",
			"request_id":            "req-9",
			"trace_id":              "trace-9",
			"span_id":               "span-9",
			"parent_span_id":        "parent-9",
			"event_name":            "http_request",
			"error_code":            "HTTP_500",
			"error_message":         "boom",
			"user_id":               uint64(7),
			"ip":                    "1.1.1.1",
			"method":                "GET",
			"path":                  "/api/x",
			"status_code":           500,
			"latency_ms":            int64(12),
			"error_type":            "db",
			"error_stack":           "stack...",
			"biz_field":             "keep-me",
			"error_stack_truncated": true,
		},
	}

	row := buildRuntimeRow(entry)

	if row.EventID != 99 || row.UserID != 7 {
		t.Fatalf("整数列映射失败: %+v", row)
	}
	if row.StatusCode != 500 || row.LatencyMS != 12 {
		t.Fatalf("HTTP 列映射失败: %+v", row)
	}
	if row.TS != "2026-10-24 09:00:00.000" {
		t.Fatalf("ts 格式不符: %q", row.TS)
	}
	if row.Level != "warning" {
		t.Fatalf("level 不符: %q", row.Level)
	}
	if row.Message != "请求执行完成" {
		t.Fatalf("message 不符: %q", row.Message)
	}
	if row.File != "/app/api/x.go" || row.Func != "api.Handler" {
		t.Fatalf("caller 未落到 file/func 列: file=%q func=%q", row.File, row.Func)
	}
	if row.TraceID != "trace-9" || row.ParentSpanID != "parent-9" {
		t.Fatalf("链路字段映射失败: %+v", row)
	}

	var extra map[string]any
	if err := json.Unmarshal([]byte(row.ExtraJSON), &extra); err != nil {
		t.Fatalf("extra_json 不是合法 JSON: %v（%s）", err, row.ExtraJSON)
	}
	if extra["biz_field"] != "keep-me" {
		t.Fatalf("未知字段应进 extra_json: %#v", extra)
	}
	if extra["error_stack_truncated"] != true {
		t.Fatalf("布尔未知字段应保留在 extra_json: %#v", extra)
	}
}

// 级别字符串到平台级别的映射：未知值不能把日志吞掉。
func TestParseLevel(t *testing.T) {
	cases := map[string]string{
		"debug":   "debug",
		"INFO":    "info",
		"warning": "warn",
		"error":   "error",
		"fatal":   "error",
		"":        "info",
		"weird":   "info",
	}
	for in, want := range cases {
		if got := parseLevel(in).String(); got != want {
			t.Errorf("parseLevel(%q) = %s，期望 %s", in, got, want)
		}
	}
}
