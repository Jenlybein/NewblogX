package river_service

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func newBufferLogger() (*logrus.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	l := logrus.New()
	l.SetOutput(buf)
	l.SetLevel(logrus.DebugLevel)
	l.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
		DisableColors:    true,
	})
	return l, buf
}

func TestLogrusToSlogAdapter(t *testing.T) {
	logger, buf := newBufferLogger()
	adapter := logrusToSlogAdapter(logger)

	adapter.Info("hello", "k", "v")
	out := buf.String()
	if !strings.Contains(out, "hello") || !strings.Contains(out, "k=v") {
		t.Fatalf("适配器输出异常: %s", out)
	}
}

func TestSimpleLogrusHandlerHandleAndHelpers(t *testing.T) {
	logger, buf := newBufferLogger()
	h := &simpleLogrusHandler{logger: logger}

	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("Enabled 应返回 true")
	}
	if h.WithAttrs(nil) != h {
		t.Fatal("WithAttrs 应返回当前 handler")
	}
	if h.WithGroup("g") != h {
		t.Fatal("WithGroup 应返回当前 handler")
	}

	recInfo := slog.NewRecord(time.Now(), slog.LevelInfo, "info-msg", 0)
	recInfo.AddAttrs(slog.String("a", "1"))
	if err := h.Handle(context.Background(), recInfo); err != nil {
		t.Fatalf("处理 info 日志失败: %v", err)
	}

	recWarn := slog.NewRecord(time.Now(), slog.LevelWarn, "warn-msg", 0)
	if err := h.Handle(context.Background(), recWarn); err != nil {
		t.Fatalf("处理 warn 日志失败: %v", err)
	}

	recErr := slog.NewRecord(time.Now(), slog.LevelError, "err-msg", 0)
	if err := h.Handle(context.Background(), recErr); err != nil {
		t.Fatalf("处理 error 日志失败: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "info-msg") || !strings.Contains(out, "warn-msg") || !strings.Contains(out, "err-msg") {
		t.Fatalf("日志输出缺失: %s", out)
	}
	if !strings.Contains(out, "a=1") {
		t.Fatalf("日志属性输出缺失: %s", out)
	}
}
