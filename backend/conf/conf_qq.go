package conf

import "fmt"

// QQ 是第三方登录（QQ 互联）段：既有"怎么跳过去"的凭据，也有"这条通道开不开"
// 的开关。
//
// QQLogin 从 site.login 迁入（票 票 481）：能不能用是这条通道自己的属性，
// 而 site 段有匿名可读的公开端点，通道开关跟着它公开虽然无害，但会让
// "改 QQ 登录要去站点配置里找"这件事永远成立。读者站要的登录方式一览由
// GET /api/site/login_capabilities 现读给出。
type QQ struct {
	AppID    string `yaml:"app_id" json:"app_id"`
	AppKey   string `yaml:"app_key" json:"app_key"`
	Redirect string `yaml:"redirect" json:"redirect"`
	// QQLogin 是否启用 QQ 登录；启用前还需要填好上面三项。
	QQLogin bool `yaml:"qq_login" json:"qq_login"`
}

func (q QQ) Url() string {
	return fmt.Sprintf("https://graph.qq.com/oauth2.0/show?which=Login&display=pc&response_type=code&client_id=%s&redirect_uri=%s", q.AppID, q.Redirect)
}
