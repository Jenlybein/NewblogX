package conf

type Jwt struct {
	Expire        int64  `yaml:"expire"`
	RefreshExpire int64  `yaml:"refresh_expire"`
	Secret        string `yaml:"secret"`
	Issuer        string `yaml:"issuer"`
}
