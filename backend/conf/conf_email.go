package conf

type Email struct {
	Domain       string `yaml:"domain" json:"domain"`
	Port         int    `yaml:"port" json:"port"`
	SendEmail    string `yaml:"send_email" json:"send_email"`
	AuthCode     string `yaml:"auth_code" json:"auth_code"`
	SendNickname string `yaml:"send_nickname" json:"send_nickname"`
	SSL          bool   `yaml:"ssl" json:"ssl"`
	TLS          bool   `yaml:"tls" json:"tls"`
}
