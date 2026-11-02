package conf

// ImageGC 是孤儿图回收的运行配置（issue #139 起）。
//
// 两项都只影响"清单看哪些图"，不涉及删除：本段在只读清单票里先落地，
// 删除能力（issue #140）会复用它，并另外需要一个能关的总开关。
type ImageGC struct {
	// GraceDays 是冷静期天数：上传不足这么多天的图不进入候选。
	//
	// <=0 按默认 14 天处理：这个值是误删的最后一道防线，零值更可能是
	// 配置写漏了，不该被当成"关掉保护"。
	GraceDays int `yaml:"grace_days" json:"grace_days"`
	// BatchLimit 是单次最多检查的图片条数。<=0 按默认 50 条，超过 500 按 500。
	BatchLimit int `yaml:"batch_limit" json:"batch_limit"`
}
