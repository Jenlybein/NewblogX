package canalbase

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

func TestNewLogAdapter(t *testing.T) {
	logger, buf := newBufferLogger()
	adapter := NewLogAdapter(logger)

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

// 高频进度日志必须降级到 debug；正常消息不能被误降级。
func TestShouldDowngradeInfo(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"Create BinlogSyncer with config", true},
		{"rotate to next binlog", true},
		{"connected to server", true},
		{"同步文章失败", false},
		{"", false},
	}
	for _, c := range cases {
		if got := shouldDowngradeInfo(c.msg); got != c.want {
			t.Fatalf("shouldDowngradeInfo(%q) = %v, 期望 %v", c.msg, got, c.want)
		}
	}
}

// 文件名与函数名的截断规则不同：文件名含点不能被切掉扩展名。
func TestBaseNameHelpers(t *testing.T) {
	if got := baseFileName(`C:\a\b\river.go`); got != "river.go" {
		t.Fatalf("baseFileName 结果 %q，期望 river.go", got)
	}
	if got := baseFileName("a/b/imageref.go"); got != "imageref.go" {
		t.Fatalf("baseFileName 结果 %q，期望 imageref.go", got)
	}
	if got := baseFuncName("myblogx/cdc.(*River).Run"); got != "Run" {
		t.Fatalf("baseFuncName 结果 %q，期望 Run", got)
	}
}
