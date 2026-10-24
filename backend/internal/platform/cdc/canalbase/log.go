package canalbase

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"

	"myblogx/utils/logsafe"
)

// NewLogAdapter 把 logrus 适配成 canal 需要的 *slog.Logger。
//
// canal 内部走 slog，项目其余部分走 logrus；这里做单向桥接，
// 保证 CDC 日志与其它模块进同一个出口、同一套格式。
func NewLogAdapter(logger *logrus.Logger) *slog.Logger {
	return slog.New(&simpleLogrusHandler{logger: logger})
}

// simpleLogrusHandler 是精简版 slog.Handler：不带 group/attr 继承，
// 只做"级别映射 + 字段透传 + 调用点前缀"。
type simpleLogrusHandler struct {
	logger *logrus.Logger
}

func (h *simpleLogrusHandler) Handle(_ context.Context, r slog.Record) error {
	entry := h.logger.WithFields(logrus.Fields{})

	r.Attrs(func(a slog.Attr) bool {
		key, value, ok := logsafe.SlogAttrToField(a)
		if ok {
			entry = entry.WithField(key, value)
		}
		return true
	})

	message := withCallSite(r)

	// 将高频同步进度日志降级到 debug，默认不打印，避免刷屏。
	if r.Level == slog.LevelInfo && shouldDowngradeInfo(r.Message) {
		entry.Debug(message)
		return nil
	}

	switch r.Level {
	case slog.LevelDebug:
		entry.Debug(message)
	case slog.LevelInfo:
		entry.Info(message)
	case slog.LevelWarn:
		entry.Warn(message)
	case slog.LevelError:
		entry.Error(message)
	default:
		entry.Info(message)
	}
	return nil
}

func (h *simpleLogrusHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *simpleLogrusHandler) WithGroup(string) slog.Handler            { return h }
func (h *simpleLogrusHandler) Enabled(context.Context, slog.Level) bool { return true }

// withCallSite 在消息前加上 [文件名:行号 函数名]，便于在 logrus 单行输出里定位来源。
func withCallSite(r slog.Record) string {
	if r.PC == 0 {
		return r.Message
	}

	frame, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
	if frame.File == "" {
		return r.Message
	}

	return fmt.Sprintf("[%s:%d %s] %s", baseFileName(frame.File), frame.Line, baseFuncName(frame.Function), r.Message)
}

// baseFileName 取路径最后一段（兼容 / 与 \ 两种分隔符）。
// 不能与函数名共用同一套分隔符：文件名本身含点（如 river.go）。
func baseFileName(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' || s[i] == '\\' {
			return s[i+1:]
		}
	}
	return s
}

// baseFuncName 取函数名最后一段（按点分隔）。
func baseFuncName(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return s[i+1:]
		}
	}
	return s
}

// shouldDowngradeInfo 判定 canal 的例行进度日志（每轮同步都会刷）。
//
// 这些消息本身没有诊断价值，但量很大；保留在 debug 级别即可。
func shouldDowngradeInfo(msg string) bool {
	m := strings.ToLower(msg)
	noisyMarkers := []string{
		"create binlogsyncer",
		"skip master data, get current binlog position",
		"try dump mysql and parse",
		"exec mysqldump with",
		"dump mysql and parse ok",
		"begin to sync binlog from position",
		"connected to server",
		"start sync binlog at binlog file",
		"rotate to next binlog",
		"received fake rotate event",
	}
	for _, marker := range noisyMarkers {
		if strings.Contains(m, marker) {
			return true
		}
	}
	return false
}
