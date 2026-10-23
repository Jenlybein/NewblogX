package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"myblogx/conf"
	"myblogx/models/ctype"
	"myblogx/service/db_service"
	"myblogx/service/log_service"

	"github.com/sirupsen/logrus"
)

// CommonFieldsHook 公共字段钩子
// 作用：为所有日志自动注入通用公共字段（事件ID、时间、服务、环境、实例、主机名等）
type CommonFieldsHook struct {
	Service    string
	Env        string
	InstanceID string
}

// Levels 实现 logrus.Hook 接口
// 作用：指定该钩子对所有日志级别生效
func (hook CommonFieldsHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire 实现 logrus.Hook 接口，日志输出前自动执行
// 作用：自动补充公共日志字段，确保所有日志格式统一、字段完整
func (hook CommonFieldsHook) Fire(entry *logrus.Entry) error {
	// 初始化日志字段map，防止空指针
	if entry.Data == nil {
		entry.Data = logrus.Fields{}
	}

	// 自动生成事件ID：优先雪花算法，失败则降级使用纳秒时间戳
	if _, ok := entry.Data["event_id"]; !ok {
		eventID, err := db_service.NextSnowflakeID()
		if err == nil {
			entry.Data["event_id"] = uint64(eventID)
		} else {
			entry.Data["event_id"] = uint64(ctype.ID(time.Now().UnixNano()))
		}
	}

	// 自动补充日志时间戳，格式：2006-01-02 15:04:05.000
	if _, ok := entry.Data["ts"]; !ok {
		entry.Data["ts"] = entry.Time.Format("2006-01-02 15:04:05.000")
	}

	// 自动补充日志类型，默认运行时日志
	if _, ok := entry.Data["log_kind"]; !ok {
		entry.Data["log_kind"] = "runtime"
	}

	// 自动补充服务名称
	if _, ok := entry.Data["service"]; !ok {
		entry.Data["service"] = hook.Service
	}

	// 自动补充运行环境（dev/prod/test）
	if _, ok := entry.Data["env"]; !ok {
		entry.Data["env"] = hook.Env
	}

	// 自动补充服务实例ID
	if _, ok := entry.Data["instance_id"]; !ok {
		entry.Data["instance_id"] = hook.InstanceID
	}

	// 自动补充主机名
	if _, ok := entry.Data["host"]; !ok {
		host, _ := os.Hostname()
		entry.Data["host"] = host
	}

	return nil
}

// FileDateHook 按日期切割日志文件钩子
// 作用：按天自动生成日志文件，每天一个独立目录，实现日志自动轮转
type FileDateHook struct {
	file      *os.File              // 当前打开的日志文件句柄
	logPath   string                // 日志根目录
	fileDate  string                // 当前日志文件对应的日期
	appName   string                // 应用名称
	formatter *logrus.JSONFormatter // JSON格式化器
}

// createLogFile 创建日志文件（含目录）
// 作用：按日期创建日志目录，打开/创建日志文件并设置权限
func createLogFile(logPath, timer, appName string) (*os.File, error) {
	// 拼接日志目录：根目录/日期
	dir := filepath.Join(logPath, timer)
	// 递归创建日志目录
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 拼接日志文件路径
	filename := filepath.Join(dir, fmt.Sprintf("%s.log", appName))
	// 打开文件：只写、追加、不存在创建，权限0644
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %w", err)
	}
	// 显式设置文件权限，确保跨平台可用
	if chmodErr := os.Chmod(filename, 0644); chmodErr != nil {
		_ = file.Close()
		return nil, fmt.Errorf("修正日志文件权限失败: %w", chmodErr)
	}
	return file, nil
}

// Levels 实现 logrus.Hook 接口
// 作用：该钩子对所有日志级别生效
func (hook FileDateHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire 实现 logrus.Hook 接口，日志输出前执行
// 作用：自动按日期切换日志文件，写入JSON格式日志
func (hook *FileDateHook) Fire(entry *logrus.Entry) error {
	// 获取当前日志的日期
	timer := entry.Time.Format("2006-01-02")
	// 将日志序列化为JSON格式
	line, err := hook.formatter.Format(entry)
	if err != nil {
		return err
	}

	// 日期变更或文件未初始化，切换到新的日志文件
	if hook.fileDate != timer || hook.file == nil {
		if hook.file != nil {
			_ = hook.file.Close() // 关闭旧文件
		}
		// 创建新日期的日志文件
		newFile, createErr := createLogFile(hook.logPath, timer, hook.appName)
		if createErr != nil {
			return fmt.Errorf("切换日志文件失败: %w", createErr)
		}
		hook.file = newFile
		hook.fileDate = timer
	}

	// 写入日志内容到文件
	if _, err = hook.file.Write([]byte(line)); err != nil {
		return fmt.Errorf("写入日志文件失败: %w", err)
	}
	return nil
}

// InitFile 初始化文件日志钩子
// 作用：给日志器添加按日期切割的文件钩子
func InitFile(logger *logrus.Logger, logPath, appName string) {
	// 获取当前日期
	fileDate := time.Now().Format("2006-01-02")
	// 创建当天日志文件
	file, err := createLogFile(logPath, fileDate, appName)
	if err != nil {
		logger.Error(err)
		return
	}
	// 初始化文件钩子
	fileHook := FileDateHook{
		file:     file,
		logPath:  logPath,
		fileDate: fileDate,
		appName:  appName,
		formatter: &logrus.JSONFormatter{
			TimestampFormat:   "2006-01-02 15:04:05.000", // 时间戳格式
			DisableHTMLEscape: true,                      // 关闭HTML转义
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime: "ts",      // 时间字段重命名为ts
				logrus.FieldKeyMsg:  "message", // 消息字段重命名为message
			},
		},
	}
	// 将文件钩子注册到日志器
	logger.AddHook(&fileHook)
}

// InitLogrus 初始化日志系统（入口方法）
// 作用：配置日志级别、输出格式、公共字段钩子、文件日志、初始化审计日志文件
func InitLogrus(l *conf.Logrus, system *conf.System) *logrus.Logger {
	// 创建新的logrus日志实例
	logger := logrus.New()
	// 默认输出到控制台
	logger.SetOutput(os.Stdout)
	// 开启调用行号记录（方便定位代码位置）
	logger.SetReportCaller(true)

	// 配置控制台输出格式：text/json，默认json
	stdoutFormat := l.StdoutFormat
	if stdoutFormat == "" {
		stdoutFormat = "json"
	}
	if stdoutFormat == "text" {
		// 文本格式
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05.000",
		})
	} else {
		// JSON格式（统一日志格式，便于收集）
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat:   "2006-01-02 15:04:05.000",
			DisableHTMLEscape: true,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime: "ts",
				logrus.FieldKeyMsg:  "message",
			},
		})
	}

	// 配置日志级别，解析失败则默认info
	level, err := logrus.ParseLevel(l.Level)
	if err != nil {
		logger.SetLevel(logrus.InfoLevel)
	} else {
		logger.SetLevel(level)
	}

	// 注册公共字段钩子
	serviceName := log_service.ResolveLogApp(l.App, l.App)
	env := ""
	instanceID := ""
	if system != nil {
		env = system.Env
		instanceID = strconv.Itoa(int(system.ServerID))
	}
	logger.AddHook(CommonFieldsHook{
		Service:    serviceName,
		Env:        env,
		InstanceID: instanceID,
	})
	// 初始化运行时日志文件输出
	InitFile(
		logger,
		filepath.Join(log_service.ResolveLogDir(l.Dir), log_service.RuntimeLogDirName),
		serviceName,
	)

	return logger
}
