package canalbase

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
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
		entry = appendAttr(entry, "", a)
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

// appendAttr 把一个 slog 属性挂到 logrus entry 上，返回新的 entry。
//
// # 为什么不用反射
//
// 原来这里调 utils/logsafe.SlogAttrToField，它内部有一整套 reflect 实现
// （把任意值转成 JSON 友好结构）。两个问题：
//
//  1. **它只为这一个调用点存在** —— 那套 reflect 逻辑（map/slice/pointer 递归、
//     json 往返兜底）是为"任意输入"准备的，而 canal 的实际输入永远是基本类型或字符串。
//  2. **反射路径会把异常藏起来**：转不了的值被换成 `%T` 文本，
//     日志里出现 `*sync.Mutex` 这种字段，比直接打印值更难查。
//
// 现在默认分支直接交给 logrus：它的 formatter 对不可序列化的值走 fmt，
// 输出默认表示，既不会 panic 也不会假装成功。
func appendAttr(entry *logrus.Entry, prefix string, a slog.Attr) *logrus.Entry {
	// 空键 + 空值是 slog 标记"无效属性"的方式，直接丢。
	if a.Key == "" && a.Value.Kind() == slog.KindAny && a.Value.Any() == nil {
		return entry
	}

	key := a.Key
	if prefix != "" {
		key = prefix + "." + a.Key
	}

	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return entry.WithField(key, v.String())
	case slog.KindInt64:
		return entry.WithField(key, v.Int64())
	case slog.KindUint64:
		return entry.WithField(key, v.Uint64())
	case slog.KindFloat64:
		return entry.WithField(key, v.Float64())
	case slog.KindBool:
		return entry.WithField(key, v.Bool())
	case slog.KindDuration:
		return entry.WithField(key, v.Duration().String())
	case slog.KindTime:
		return entry.WithField(key, v.Time().Format(time.RFC3339Nano))
	case slog.KindGroup:
		// 展开成 "父.子" 扁平键，而不是塞一个嵌套 map：
		// logrus 单行输出里嵌套 map 会变成一坨 %v，没法 grep。
		for _, sub := range v.Group() {
			entry = appendAttr(entry, key, sub)
		}
		return entry
	default:
		return entry.WithField(key, v.Any())
	}
}

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
