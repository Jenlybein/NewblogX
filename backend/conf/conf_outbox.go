package conf

// Outbox 是事务型发件箱（outbox_event_models）的保留期配置。
//
// 这里只暴露"清理"相关的旋钮；中继的并发、重试、扫描间隔仍用代码里的默认值，
// 因为它们属于实现细节，调错了只会让投递变慢，不会造成数据问题。
//
// 保留期只作用于已终结的行（done / dead）。pending / failed / processing
// 代表"尚未投递完"，无论多老都不清理，否则等于丢事件。
type Outbox struct {
	// RetentionHours 已完成（done）事件行保留小时数，超期物理删除。
	// 小于等于 0 时用默认 168（7 天）。
	RetentionHours int `yaml:"retention_hours"`
	// DeadRetentionHours 死信（dead）事件行保留小时数。
	// 小于等于 0 时用默认 720（30 天）；比 done 长，便于事后排查。
	DeadRetentionHours int `yaml:"dead_retention_hours"`
	// CleanupIntervalMinutes 保留期清理的检查间隔（分钟）。
	// 小于等于 0 时用默认 60。
	CleanupIntervalMinutes int `yaml:"cleanup_interval_minutes"`
	// CleanupBatchSize 单批删除行数上限。
	// 小于等于 0 时用默认 1000；分批删除避免大事务长时间持锁。
	CleanupBatchSize int `yaml:"cleanup_batch_size"`
}
