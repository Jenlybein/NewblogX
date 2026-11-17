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

	// Whitelist 是可上传的文件后缀（不带点，小写）。
	//
	// 从「上传策略」段并过来（票 410）：对象存储从此是"能不能传、传多大、什么格式"
	// 的唯一事实源。旧 upload 段在退场前仍保留，读取口径见 domain.EffectiveWhitelist：
	// 本字段非空则以本字段为准，为空才回落旧段，保证迁移期行为不变。
	Whitelist []string `yaml:"whitelist" json:"whitelist"`
}
