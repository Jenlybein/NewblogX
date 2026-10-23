// GORM配置

package conf

type GormConf struct {
	Debug           bool `yaml:"debug"` // 是否开启调试模式：开启后会打印 SQL 语句
	MaxIdleConns    int  `yaml:"max_idle_conns"`
	MaxOpenConns    int  `yaml:"max_open_conns"`
	ConnMaxLifetime int  `yaml:"conn_max_lifetime"`
}
