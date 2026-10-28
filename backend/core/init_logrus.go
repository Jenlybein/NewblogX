package core

import (
	"os"
	"strconv"
	"time"

	"myblogx/conf"
	"myblogx/internal/platform/idgen"
	"myblogx/internal/platform/log/writer"

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
		eventID, err := idgen.Next()
		if err == nil {
			entry.Data["event_id"] = eventID.Uint64()
		} else {
			entry.Data["event_id"] = uint64(time.Now().UnixNano())
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

// StructuredLogHook 把 logrus 条目转交结构化日志管道（runtime_logs 表）。
//
// 它取代了原来的 FileDateHook：编码、批量、落盘与 ClickHouse 投递都由
// log_service 的管道负责，这里只做转交——不持有文件句柄、不做任何 IO。
// 这样做的好处是请求路径上不再有"每条日志一次 write + 一把全局锁"。
type StructuredLogHook struct {
	Dir string
	App string
}

// Levels 实现 logrus.Hook 接口：对所有级别生效。
func (hook StructuredLogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire 实现 logrus.Hook 接口：把条目交给结构化管道。
func (hook StructuredLogHook) Fire(entry *logrus.Entry) error {
	writer.EmitRuntimeLog(hook.Dir, hook.App, entry)
	return nil
}

// InitLogrus 初始化日志系统（入口方法）
// 作用：配置日志级别、输出格式、公共字段钩子、文件日志、初始化审计日志文件
// ApplyLogLevel 按字符串设置日志级别，解析失败则退回 info。
//
// 单独抽出来是因为后台"日志设置"要能热改级别：改完立刻生效，
// 不必重启进程。写在 InitLogrus 里就只有启动时能调。
func ApplyLogLevel(logger *logrus.Logger, level string) {
	if logger == nil {
		return
	}
	parsed, err := logrus.ParseLevel(level)
	if err != nil {
		logger.SetLevel(logrus.InfoLevel)
		return
	}
	logger.SetLevel(parsed)
}

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
	ApplyLogLevel(logger, l.Level)

	// 注册公共字段钩子
	serviceName := writer.ResolveLogApp(l.App, l.App)
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
	// 注册结构化日志钩子：条目转交 log_service 的有界异步管道，
	// 由管道决定落本地文件还是直接写 ClickHouse（见 log_service/pipeline.go）。
	logger.AddHook(StructuredLogHook{
		Dir: writer.ResolveLogDir(l.Dir),
		App: serviceName,
	})

	return logger
}
