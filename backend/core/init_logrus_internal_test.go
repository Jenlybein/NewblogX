package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myblogx/internal/platform/log/writer"

	"github.com/sirupsen/logrus"
)

func TestCommonFieldsHookFire(t *testing.T) {
	logger := logrus.New()
	entry := &logrus.Entry{
		Logger:  logger,
		Time:    time.Date(2026, 3, 2, 12, 0, 0, 0, time.Local),
		Level:   logrus.InfoLevel,
		Message: "hello",
	}

	hook := CommonFieldsHook{}
	if err := hook.Fire(entry); err != nil {
		t.Fatalf("CommonFieldsHook Fire 失败: %v", err)
	}
	if entry.Data["log_kind"] != "runtime" {
		t.Fatalf("log_kind 未写入: %#v", entry.Data)
	}
	if entry.Data["message"] != nil {
		t.Fatalf("CommonFieldsHook 不应覆盖 message 字段")
	}

	formatter := &logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05.000",
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime: "ts",
			logrus.FieldKeyMsg:  "message",
		},
	}
	b, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("JSON Format 失败: %v", err)
	}
	var body map[string]any
	if err = json.Unmarshal(b, &body); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if body["message"] != "hello" {
		t.Fatalf("message 字段异常: %#v", body["message"])
	}
}

// StructuredLogHook 只做转交：管道未初始化时应同步落到「每日文件」，
// 保证 InitLogrus 到 InitStructuredLog 之间那几行日志不丢。
func TestStructuredLogHookFireFallsBackToFile(t *testing.T) {
	dir := t.TempDir()
	logger := logrus.New()

	// 兜底出口会一直持有文件句柄，测试结束前必须收掉，否则临时目录删不掉。
	t.Cleanup(func() { _ = writer.CloseStructuredLog(context.Background()) })

	hook := StructuredLogHook{Dir: dir, App: "app"}

	now := time.Now()
	entry := &logrus.Entry{
		Logger:  logger,
		Time:    now,
		Level:   logrus.InfoLevel,
		Message: "line-1",
		Data:    logrus.Fields{"request_id": "req-1"},
	}
	if err := hook.Fire(entry); err != nil {
		t.Fatalf("Fire 失败: %v", err)
	}

	entry2 := &logrus.Entry{
		Logger:  logger,
		Time:    now,
		Level:   logrus.WarnLevel,
		Message: "line-2",
	}
	if err := hook.Fire(entry2); err != nil {
		t.Fatalf("再次 Fire 失败: %v", err)
	}

	logFile := filepath.Join(dir, "runtime_logs", now.Format("2006-01-02"), "app.log")
	b, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("期望 2 行，实际 %d 行：%s", len(lines), b)
	}

	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("第 1 行不是合法 JSON: %v", err)
	}
	if first["message"] != "line-1" {
		t.Fatalf("message 不符: %#v", first["message"])
	}
	if first["level"] != "info" {
		t.Fatalf("level 不符: %#v", first["level"])
	}
	if first["request_id"] != "req-1" {
		t.Fatalf("request_id 不符: %#v", first["request_id"])
	}
	if first["log_kind"] != "runtime" {
		t.Fatalf("log_kind 不符: %#v", first["log_kind"])
	}

	var second map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("第 2 行不是合法 JSON: %v", err)
	}
	if second["level"] != "warning" {
		t.Fatalf("warn 级别应写成 warning: %#v", second["level"])
	}
}
