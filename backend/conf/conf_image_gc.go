package conf

// ImageGC 是孤儿图回收的运行参数（spec #138）。
//
// 放在运行时配置而不是命令行常量：冷静期与单次上限是要按运营节奏调的量
// （刚上线要保守、稳定后可以放宽），不该为了改一个天数重新发版。
//
// 取值来源与其它段一致：site_default_settings.yaml 给默认值，数据库里的
// 运行时配置可覆盖；执行时现读，不缓存。
type ImageGC struct {
	// CooldownDays 是冷静期（天）。上传不满这么多天的图不进候选，
	// 覆盖"本周传图、下周才发文"的周更节奏。
	CooldownDays int `yaml:"cooldown_days" json:"cooldown_days"`
	// BatchLimit 是单次扫描的条数上限，避免一次扫爆。
	BatchLimit int `yaml:"batch_limit" json:"batch_limit"`
}
