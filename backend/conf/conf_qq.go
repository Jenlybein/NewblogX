package conf

import "fmt"

type QQ struct {
	AppID    string `yaml:"app_id" json:"app_id"`
	AppKey   string `yaml:"app_key" json:"app_key"`
	Redirect string `yaml:"redirect" json:"redirect"`
}

func (q QQ) Url() string {
	return fmt.Sprintf("https://graph.qq.com/oauth2.0/show?which=Login&display=pc&response_type=code&client_id=%s&redirect_uri=%s", q.AppID, q.Redirect)
}
