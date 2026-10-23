// 日志配置

package conf

type Logrus struct {
	App               string         `yaml:"app"`
	Dir               string         `yaml:"dir"`
	Level             string         `yaml:"level"`
	StdoutFormat      string         `yaml:"stdout_format"`       // text/json，默认 json
	RequestLogEnabled bool           `yaml:"request_log_enabled"` // 是否记录请求访问日志
	QueryDefaultLimit int            `yaml:"query_default_limit"` // 后台日志查询默认条数
	QueryMaxLimit     int            `yaml:"query_max_limit"`     // 后台日志查询最大条数
	Error             LogErrorConfig `yaml:"error"`
	Trace             LogTraceConfig `yaml:"trace"`
	Cleanup           LogCleanup     `yaml:"cleanup"`
}

type LogErrorConfig struct {
	CaptureStack    bool   `yaml:"capture_stack"`
	CaptureMinLevel string `yaml:"capture_min_level"`
	StackMaxBytes   int    `yaml:"stack_max_bytes"`
	CauseChainDepth int    `yaml:"cause_chain_depth"`
}

type LogTraceConfig struct {
	Enabled                bool   `yaml:"enabled"`
	RequestIDEqualsTraceID bool   `yaml:"request_id_equals_trace_id"`
	InheritFromGateway     bool   `yaml:"inherit_from_gateway"`
	GatewayHeaderPriority  string `yaml:"gateway_header_priority"`
}

type LogCleanup struct {
	Enabled       bool   `yaml:"enabled"`
	RetentionDays int    `yaml:"retention_days"`
	RunAt         string `yaml:"run_at"`
}
