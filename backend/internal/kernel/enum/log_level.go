// 日志级别枚举

package enum

type LogLevelType int8

const (
	LogInfoLevel  LogLevelType = 1
	LogWarnLevel  LogLevelType = 2
	LogErrorLevel LogLevelType = 3
)

func (l LogLevelType) String() string {
	switch l {
	case LogInfoLevel:
		return "info"
	case LogWarnLevel:
		return "warn"
	case LogErrorLevel:
		return "error"
	}
	return ""
}
