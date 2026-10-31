package conf

// RuntimeDefaults 是运行时配置各段的默认值。
//
// 由 site_default_settings.yaml 反序列化而来；数据库里某段缺失、或某段
// 的 JSON 里少了某个字段时，就用这里的值补齐（见 platform/config 的 merge）。
//
// Log 是特例：日志属于进程级配置，默认值来自 settings.yaml 而不是这个文件，
// 由装配层填进来。
type RuntimeDefaults struct {
	Site    Site   `yaml:"site"`
	AI      AI     `yaml:"ai"`
	Mail    Mail   `yaml:"mail"`
	Storage QiNiu  `yaml:"storage"`
	OAuth   QQ     `yaml:"oauth"`
	Log     Logrus `yaml:"log"`
	Upload  Upload `yaml:"upload"`
	Deco    Deco   `yaml:"deco"`
}
