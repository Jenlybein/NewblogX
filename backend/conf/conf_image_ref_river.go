package conf

type ImageRefRiver struct {
	Enabled bool `yaml:"enabled"`

	ServerID uint32 `yaml:"server_id"`
	Flavor   string `yaml:"flavor"`
	Schema   string `yaml:"schema"`

	Mysql RiverMysql `yaml:"mysql"`

	Charset        string      `yaml:"charset"`
	DumpExec       string      `yaml:"mysqldump"`
	SkipMasterData bool        `yaml:"skip_master_data"`
	Retry          RetryPolicy `yaml:"retry"`
}
