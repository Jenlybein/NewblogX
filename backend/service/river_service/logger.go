// 适配 logrus 到 slog，用于 canal 日志
package river_service

import (
	// 其他导入...
	"context"
	"fmt"
	"log/slog"
	"myblogx/utils/logsafe"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

// 简化的 logrus 到 slog 适配器
func logrusToSlogAdapter(logger *logrus.Logger) *slog.Logger {
	return slog.New(&simpleLogrusHandler{logger: logger})
}

// simpleLogrusHandler 是一个简化的 slog.Handler 实现
type simpleLogrusHandler struct {
	logger *logrus.Logger
}

// Handle 实现 slog.Handler.Handle 方法
func (h *simpleLogrusHandler) Handle(ctx context.Context, r slog.Record) error {
	// 构建 logrus 条目
	entry := h.logger.WithFields(logrus.Fields{})

	// 添加 slog 的属性
	r.Attrs(func(a slog.Attr) bool {
		key, value, ok := logsafe.SlogAttrToField(a)
		if ok {
			entry = entry.WithField(key, value)
		}
		return true
	})

	// 准备消息，包含原始位置信息
	message := r.Message
	if r.PC != 0 {
		// 获取原始调用者信息
		fs := runtime.CallersFrames([]uintptr{r.PC})
		if frame, _ := fs.Next(); frame.File != "" {
			// 提取文件名（只保留最后一部分，类似于其他日志的格式）
			fileName := frame.File
			if lastSlash := len(fileName) - 1; lastSlash >= 0 {
				for i := lastSlash; i >= 0; i-- {
					if fileName[i] == '/' || fileName[i] == '\\' {
						fileName = fileName[i+1:]
						break
					}
				}
			}
			// 提取函数名（只保留最后一部分）
			funcName := frame.Function
			if lastDot := len(funcName) - 1; lastDot >= 0 {
				for i := lastDot; i >= 0; i-- {
					if funcName[i] == '.' {
						funcName = funcName[i+1:]
						break
					}
				}
			}
			// 构建包含位置信息的消息，格式：[fileName:line funcName] message
			message = fmt.Sprintf("[%s:%d %s] %s", fileName, frame.Line, funcName, message)
		}
	}

	// 将高频同步进度日志降级到 debug，默认不打印，避免刷屏。
	if r.Level == slog.LevelInfo && shouldDowngradeRiverInfo(r.Message) {
		entry.Debug(message)
		return nil
	}

	// 映射日志级别并记录
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

// 实现其他必要的 slog.Handler 方法（简化版）
func (h *simpleLogrusHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *simpleLogrusHandler) WithGroup(string) slog.Handler            { return h }
func (h *simpleLogrusHandler) Enabled(context.Context, slog.Level) bool { return true }

func shouldDowngradeRiverInfo(msg string) bool {
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
