package log_service

import (
	"time"

	"github.com/sirupsen/logrus"
)

// RuntimeEntry 基于公共字段创建一条运行日志 entry，供业务模块继续补充字段后输出。
func RuntimeEntry(deps Deps, fields logrus.Fields) *logrus.Entry {
	// 空值处理：传入字段为nil时，初始化空字段map
	if fields == nil {
		fields = logrus.Fields{}
	}

	// 获取基础公共日志事件（包含服务、环境、实例ID等通用信息）
	base := newBaseEvent(deps, "runtime", "info", "")

	// 填充字段，部分字段填入时为空则使用基础公共配置的默认值
	// 分别是：日志类型、服务名称、运行环境、实例ID、时间戳
	fields["log_kind"] = "runtime"
	fields["service"] = defaultIfEmptyString(anyToString(fields["service"]), base.Service)
	fields["env"] = defaultIfEmptyString(anyToString(fields["env"]), base.Env)
	fields["instance_id"] = defaultIfEmptyString(anyToString(fields["instance_id"]), base.InstanceID)
	if _, ok := fields["ts"]; !ok {
		fields["ts"] = time.Now().Format(clickhouseTimeLayout)
	}

	// 绑定所有字段并返回日志实例
	if deps.Logger == nil {
		return logrus.New().WithFields(fields)
	}
	return deps.Logger.WithFields(fields)
}

// defaultIfEmptyString 在字段为空时回退到默认值。
func defaultIfEmptyString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// anyToString 尝试将任意字段安全转换为字符串。
func anyToString(value any) string {
	if value == nil {
		return ""
	}
	if v, ok := value.(string); ok {
		return v
	}
	return ""
}
