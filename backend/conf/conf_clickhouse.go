package conf

type ClickHouse struct {
	Enabled      bool     `yaml:"enabled"`
	Addresses    []string `yaml:"addresses"`
	Database     string   `yaml:"database"`
	Username     string   `yaml:"username"`
	Password     string   `yaml:"password"`
	DialTimeout  int      `yaml:"dial_timeout"`   // 秒
	MaxOpenConns int      `yaml:"max_open_conns"` // 连接池上限
	MaxIdleConns int      `yaml:"max_idle_conns"` // 空闲连接数
}
