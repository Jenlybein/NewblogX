// Elasticsearch配置

package conf

type ES struct {
	Addresses []string `yaml:"addresses"` // ES 地址列表
	Username  string   `yaml:"username"`  // ES 用户名
	Password  string   `yaml:"password"`  // ES 密码
	Index     string   `yaml:"index"`     // 默认索引名
	Addr      string   `yaml:"addr"`      // ES 地址
	IsHttps   bool     `yaml:"is_https"`  // 是否使用 HTTPS
}
