package conf

type River struct {
	Enabled bool `yaml:"enabled"`

	ServerID uint32 `yaml:"server_id"` // canal 模拟 MySQL 从库时用的 server-id
	Flavor   string `yaml:"flavor"`
	DataDir  string `yaml:"data_dir"`

	Mysql RiverMysql `yaml:"mysql"`

	// Sources 是"监听哪些表"。这是本服务唯一的表清单来源：
	//
	// 以前还有一份 `rule`（表 → 索引 + 字段白名单），通用路径靠它把行搬进 ES。
	// 那条路径 2026-10-26 删除（见 ADR-008），表清单只剩这一份 ——
	// 少一份"看起来在生效"的配置，就少一个"改错了却没人报错"的机会。
	Sources []RiverSource `yaml:"source"`

	Charset string `yaml:"charset"`

	DumpExec       string `yaml:"mysqldump"`
	SkipMasterData bool   `yaml:"skip_master_data"`

	// BulkSize 仍是"一次批量写 ES 多少篇"，但使用者只剩全量重建命令
	// （`-es -s article-sync`）；增量路径由 articlesync 自己按批处理。
	BulkSize int `yaml:"bulk_size"`
}

type RiverSource struct {
	Schema string   `yaml:"schema"`
	Tables []string `yaml:"tables"`
}

type RiverMysql struct {
	Addr     string `yaml:"addr"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// RetryPolicy 是"写失败后重试几次、间隔多久"，图片引用链路的配置在用。
type RetryPolicy struct {
	MaxAttempts int `yaml:"max_attempts"`
	DelayMS     int `yaml:"delay_ms"`
}
