// 配置模块入口
package conf

type Config struct {
	System        System        `yaml:"system"`
	Jwt           Jwt           `yaml:"jwt"`
	Log           Logrus        `yaml:"log"`
	DB            []DB          `yaml:"db"`
	GORM          GormConf      `yaml:"gorm"`
	Redis         Redis         `yaml:"redis"`
	Kafka         Kafka         `yaml:"kafka"`
	ES            ES            `yaml:"es"`
	ClickHouse    ClickHouse    `yaml:"clickhouse"`
	River         River         `yaml:"river"`
	ImageRefRiver ImageRefRiver `yaml:"image_ref_river"`
	Replay        Replay        `yaml:"replay"`
	Upload        Upload        `yaml:"upload"`
	Site          Site          `yaml:"site"`
	Email         Email         `yaml:"email"`
	QQ            QQ            `yaml:"qq"`
	QiNiu         QiNiu         `yaml:"qiniu"`
	AI            AI            `yaml:"ai"`
}
