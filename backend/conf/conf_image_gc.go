package conf

// ImageGC 是孤儿图回收的运行配置（issue #139 起，issue #140 加真删开关）。
//
// 三项都是运行时现读：清单看哪些图、单次做多少、以及是否允许真删。
// 删除不可逆，所以 enabled 默认关闭，且关掉后下一次执行立即停手。
type ImageGC struct {
	// Enabled 是真删回收的总开关。
	//
	// 默认 false：不该让"配置写漏了"等同于"可以不可逆删除"。
	// 清单（只读）不受它影响，随时可以看；它只控制真删。
	Enabled bool `yaml:"enabled" json:"enabled"`
	// GraceDays 是冷静期天数：上传不足这么多天的图不进入候选。
	//
	// <=0 按默认 14 天处理：这个值是误删的最后一道防线，零值更可能是
	// 配置写漏了，不该被当成"关掉保护"。
	GraceDays int `yaml:"grace_days" json:"grace_days"`
	// BatchLimit 是单次最多检查（或真删）的图片条数。<=0 按默认 50 条，超过 500 按 500。
	BatchLimit int `yaml:"batch_limit" json:"batch_limit"`
}
