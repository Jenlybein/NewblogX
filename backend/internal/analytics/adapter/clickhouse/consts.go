package clickhouse

// 日志表名：与写入侧（platform/log）以及 ClickHouse 建表脚本保持一致。
const (
	RuntimeLogTableName     = "runtime_logs"
	LoginEventLogTableName  = "login_event_logs"
	ActionAuditLogTableName = "action_audit_logs"
	CdcEventLogTableName    = "cdc_event_logs"
	ReplayEventLogTableName = "replay_event_logs"
	AICallLogTableName      = "ai_call_logs"

	// clickhouseTimeLayout 统一定义日志查询使用的时间格式（与写入侧一致）。
	clickhouseTimeLayout = "2006-01-02 15:04:05.000"
)

// boolToUInt8 把布尔转成 ClickHouse 里的 UInt8 字面量。
func boolToUInt8(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}
