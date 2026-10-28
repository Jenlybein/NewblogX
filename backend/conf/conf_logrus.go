// 日志配置

package conf

// Logrus 是日志配置。
//
// json tag 与 yaml tag 保持一致：这份结构现在也是后台"日志配置"段的
// 请求/响应体，缺了 json tag 会用 Go 字段名（App / StdoutFormat / ...）出边界，
// 与配置文件里的名字对不上。
type Logrus struct {
	App               string         `yaml:"app" json:"app"`
	Dir               string         `yaml:"dir" json:"dir"`
	WALDir            string         `yaml:"wal_dir" json:"wal_dir"`                         // 日志本地兜底目录，空则用 <dir>/wal
	Level             string         `yaml:"level" json:"level"`                             // trace/debug/info/warn/error；改完立即生效
	StdoutFormat      string         `yaml:"stdout_format" json:"stdout_format"`             // text/json，默认 json；改完需重启
	RequestLogEnabled bool           `yaml:"request_log_enabled" json:"request_log_enabled"` // 是否记录请求访问日志
	QueryDefaultLimit int            `yaml:"query_default_limit" json:"query_default_limit"` // 后台日志查询默认条数
	QueryMaxLimit     int            `yaml:"query_max_limit" json:"query_max_limit"`         // 后台日志查询最大条数
	Error             LogErrorConfig `yaml:"error" json:"error"`
	Trace             LogTraceConfig `yaml:"trace" json:"trace"`
	Cleanup           LogCleanup     `yaml:"cleanup" json:"cleanup"`
}

// LogErrorConfig 控制错误日志里 stack / cause chain 的采集策略。
type LogErrorConfig struct {
	CaptureStack    bool   `yaml:"capture_stack" json:"capture_stack"`
	CaptureMinLevel string `yaml:"capture_min_level" json:"capture_min_level"`
	StackMaxBytes   int    `yaml:"stack_max_bytes" json:"stack_max_bytes"`
	CauseChainDepth int    `yaml:"cause_chain_depth" json:"cause_chain_depth"`
}

// LogTraceConfig 控制 trace_id / span_id 的透传与继承。
type LogTraceConfig struct {
	Enabled                bool   `yaml:"enabled" json:"enabled"`
	RequestIDEqualsTraceID bool   `yaml:"request_id_equals_trace_id" json:"request_id_equals_trace_id"`
	InheritFromGateway     bool   `yaml:"inherit_from_gateway" json:"inherit_from_gateway"`
	GatewayHeaderPriority  string `yaml:"gateway_header_priority" json:"gateway_header_priority"`
}

// LogCleanup 是本地日志文件的清理策略。
type LogCleanup struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
	RunAt         string `yaml:"run_at" json:"run_at"`
}
