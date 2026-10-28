// 配置模块入口
package conf

// Config 是 settings.yaml 的结构，只放**进程级基础设施**配置。
// 业务配置（站点、AI、邮件、对象存储、第三方登录、上传策略、日志的可改部分）
// 已经迁到数据库的运行时配置里，默认值来自 site_default_settings.yaml，
// 后台可改。它们的类型仍然放在 conf 包里（conf.RuntimeDefaults 要用），
// 但不再有 yaml 段——写在这里就等于留了两份默认值，迟早对不上。
type Config struct {
	System        System        `yaml:"system"`
	Jwt           Jwt           `yaml:"jwt"`
	Log           Logrus        `yaml:"log"`
	DB            []DB          `yaml:"db"`
	GORM          GormConf      `yaml:"gorm"`
	Redis         Redis         `yaml:"redis"`
	ES            ES            `yaml:"es"`
	ClickHouse    ClickHouse    `yaml:"clickhouse"`
	River         River         `yaml:"river"`
	ImageRefRiver ImageRefRiver `yaml:"image_ref_river"`
	Outbox        Outbox        `yaml:"outbox"`
	Replay        Replay        `yaml:"replay"`
}
