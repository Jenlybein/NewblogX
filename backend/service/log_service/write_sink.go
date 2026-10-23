package log_service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// jsonLineSink JSON行日志写入器结构体
// 负责将日志序列化为单行JSON，按日期自动切割写入文件
type jsonLineSink struct {
	dirName     string     // 日志子目录名称（区分日志类型）
	appName     string     // 应用名称，用于日志文件名
	mu          sync.Mutex // 互斥锁，保证并发写入文件安全
	file        *os.File   // 当前打开的日志文件句柄
	currentDate string     // 当前日志文件对应的日期（2006-01-02格式）
	lastDir     string     // 上一次使用的日志根目录
	lastAppName string     // 上一次使用的应用名称
}

// write 将一条结构化日志序列化为单行 JSON 并追加到当日日志文件。
func (s *jsonLineSink) write(deps Deps, record any) error {
	// json 序列化
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 确保当前日期对应的日志文件已打开
	file, err := s.ensureFile(deps, time.Now())
	if err != nil {
		return err
	}

	if _, err = file.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

// ensureFile 确保当前日期对应的日志文件已打开，并在日期切换时自动轮转。
// 逻辑：
//  1. 检查日期、目录、应用名是否变更
//  2. 变更则关闭旧文件，创建新的日期目录
//  3. 打开/创建日志文件，设置文件权限
//
// 返回：打开的文件句柄、错误信息
func (s *jsonLineSink) ensureFile(deps Deps, now time.Time) (*os.File, error) {
	// 解析日志根目录和应用名称
	logDir := ResolveLogDir(runtimeLogDir(deps))
	appName := ResolveLogApp(s.appName, deps.LogConfig.App)

	// 格式化当前日期
	currentDate := now.Format("2006-01-02")
	// 校验：文件、日期、目录、应用名都未变化，直接复用现有文件句柄
	if s.file != nil && s.currentDate == currentDate && s.lastDir == logDir && s.lastAppName == appName {
		return s.file, nil
	}

	// 配置发生变化，关闭旧文件句柄
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}

	// 拼接完整日志目录：根目录/子目录/日期
	dir := filepath.Join(logDir, s.dirName, currentDate)
	// 递归创建目录，权限为系统最大权限
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, err
	}

	// 拼接日志文件路径
	filename := filepath.Join(dir, fmt.Sprintf("%s.log", appName))
	// 打开文件：只写、追加、不存在则创建，权限0644
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	// 显式设置文件权限，确保读写权限正常
	if chmodErr := os.Chmod(filename, 0644); chmodErr != nil {
		_ = file.Close()
		return nil, chmodErr
	}

	// 更新结构体状态，缓存当前文件句柄和配置
	s.file = file
	s.currentDate = currentDate
	s.lastDir = logDir
	s.lastAppName = appName
	return s.file, nil
}

// 单例对象：登录事件日志写入器（sync.Once保证全局只初始化一次）
var (
	loginEventSinkOnce  sync.Once
	loginEventSinkValue *jsonLineSink
	actionAuditSinkOnce sync.Once
	actionAuditSinkVal  *jsonLineSink
	cdcEventSinkOnce    sync.Once
	cdcEventSinkVal     *jsonLineSink
	replayEventSinkOnce sync.Once
	replayEventSinkVal  *jsonLineSink
)

// EnsureDailyLogFiles 提前创建当天日志文件，避免采集器在无文件时持续告警。
// 服务启动时调用，主动初始化今日所有类型的日志文件
func EnsureDailyLogFiles(deps Deps) error {
	now := time.Now()
	// 初始化登录事件日志文件
	if _, err := loginEventSink().ensureFile(deps, now); err != nil {
		return err
	}
	// 初始化操作审计日志文件
	if _, err := actionAuditSink().ensureFile(deps, now); err != nil {
		return err
	}
	if _, err := cdcEventSink().ensureFile(deps, now); err != nil {
		return err
	}
	if _, err := replayEventSink().ensureFile(deps, now); err != nil {
		return err
	}
	return nil
}

// loginEventSink 登录事件日志写入器单例
// 用于统一管理登录相关日志的写入，全局唯一实例
func loginEventSink() *jsonLineSink {
	loginEventSinkOnce.Do(func() {
		loginEventSinkValue = newJSONLineSink(LoginEventLogDirName, "")
	})
	return loginEventSinkValue
}

// actionAuditSink 返回操作审计日志专用的 JSON 行写入器单例。
// 用于统一管理操作审计日志，全局唯一实例
func actionAuditSink() *jsonLineSink {
	actionAuditSinkOnce.Do(func() {
		actionAuditSinkVal = newJSONLineSink(ActionAuditLogDirName, "")
	})
	return actionAuditSinkVal
}

func cdcEventSink() *jsonLineSink {
	cdcEventSinkOnce.Do(func() {
		cdcEventSinkVal = newJSONLineSink(CdcEventLogDirName, "")
	})
	return cdcEventSinkVal
}

func replayEventSink() *jsonLineSink {
	replayEventSinkOnce.Do(func() {
		replayEventSinkVal = newJSONLineSink(ReplayEventLogDirName, "")
	})
	return replayEventSinkVal
}

// newJSONLineSink 创建一个面向指定日志目录的 JSON 行写入器。
func newJSONLineSink(dirName, appName string) *jsonLineSink {
	return &jsonLineSink{
		dirName: dirName, // 日志子目录名
		appName: appName, // 应用名称
	}
}

func runtimeLogDir(deps Deps) string {
	return deps.LogConfig.Dir
}
