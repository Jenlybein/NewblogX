package core

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestFileDateHookFire(t *testing.T) {
	dir := t.TempDir()
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: true})

	hook := &FileDateHook{
		logPath:  dir,
		fileDate: "2000-01-01",
		appName:  "app",
		formatter: &logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime: "ts",
				logrus.FieldKeyMsg:  "message",
			},
		},
	}

	entry := &logrus.Entry{
		Logger:  logger,
		Time:    time.Date(2026, 3, 2, 9, 0, 0, 0, time.Local),
		Level:   logrus.InfoLevel,
		Message: "line-1",
		Buffer:  &bytes.Buffer{},
	}

	if err := hook.Fire(entry); err != nil {
		t.Fatalf("首次 Fire 失败: %v", err)
	}
	if hook.file == nil {
		t.Fatal("首次 Fire 后应创建日志文件")
	}
	t.Cleanup(func() {
		_ = hook.file.Close()
	})

	entry2 := &logrus.Entry{
		Logger:  logger,
		Time:    time.Date(2026, 3, 2, 10, 0, 0, 0, time.Local),
		Level:   logrus.WarnLevel,
		Message: "line-2",
		Buffer:  &bytes.Buffer{},
	}
	if err := hook.Fire(entry2); err != nil {
		t.Fatalf("同日再次 Fire 失败: %v", err)
	}

	logFile := filepath.Join(dir, "2026-03-02", "app.log")
	b, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "line-1") || !strings.Contains(s, "line-2") {
		t.Fatalf("日志内容不完整: %s", s)
	}
}
