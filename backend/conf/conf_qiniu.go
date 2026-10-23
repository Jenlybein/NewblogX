// 对象存储配置

package conf

type QiNiu struct {
	Enable      bool   `yaml:"enable" json:"enable"`
	AccessKey   string `yaml:"access_key" json:"access_key"`
	SecretKey   string `yaml:"secret_key" json:"secret_key"`
	Bucket      string `yaml:"bucket" json:"bucket"`
	Uri         string `yaml:"uri" json:"uri"`
	Region      string `yaml:"region" json:"region"`
	Prefix      string `yaml:"prefix" json:"prefix"`
	Size        int    `yaml:"size" json:"size"`                 // 上传大小限制，单位 MB
	Expiry      int    `yaml:"expiry" json:"expiry"`             // 上传凭证过期时间，单位秒
	CallbackURL string `yaml:"callback_url" json:"callback_url"` // 七牛上传成功后的服务端回调地址
}
