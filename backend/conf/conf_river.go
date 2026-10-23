package conf

import "myblogx/service/river_service/rule"

type River struct {
	Enabled bool `yaml:"enabled"`

	ServerID uint32 `yaml:"server_id"` // canal 模拟 MySQL 从库时用的 server-id
	Flavor   string `yaml:"flavor"`
	DataDir  string `yaml:"data_dir"`

	Mysql RiverMysql `yaml:"mysql"`

	Sources []RiverSource `yaml:"source"`

	Rules []*rule.Rule `yaml:"rule"`

	Charset string `yaml:"charset"`

	DumpExec       string `yaml:"mysqldump"`
	SkipMasterData bool   `yaml:"skip_master_data"`

	BulkSize      int         `yaml:"bulk_size"`
	FlushBulkTime int         `yaml:"flush_bulk_time"`
	Retry         RetryPolicy `yaml:"retry"`
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

type RetryPolicy struct {
	MaxAttempts int `yaml:"max_attempts"`
	DelayMS     int `yaml:"delay_ms"`
}
