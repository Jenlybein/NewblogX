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

	// —— 图片展示样式（ADR-0034）——
	//
	// 四档分别对应七牛控制台里已创建好的样式名。留空表示该档用原图。
	// 样式是**读取时派生**的：库里只存原图地址，所以改这里、清这里都立刻生效，
	// 不需要回填数据，也不会污染引用账本（它靠从 URL 反解 object key 工作）。
	ImageStyleCover   string `yaml:"image_style_cover" json:"image_style_cover"`
	ImageStyleAvatar  string `yaml:"image_style_avatar" json:"image_style_avatar"`
	ImageStyleContent string `yaml:"image_style_content" json:"image_style_content"`
	ImageStyleChat    string `yaml:"image_style_chat" json:"image_style_chat"`
	// StyleDelimiter 必须与七牛控制台的「样式分隔符」一致，默认 "-"。
	StyleDelimiter string `yaml:"style_delimiter" json:"style_delimiter"`
	// PrivateRead 为真时所有图片地址都要签名（私有空间）。
	PrivateRead bool `yaml:"private_read" json:"private_read"`
	// DownloadExpiry 是下载签名有效期（秒）。
	//
	// 与上面的 Expiry 分开：Expiry 是**上传凭证**的过期时间（默认 60 秒量级），
	// 拿它给图片链接签名会让页面上的图一分钟后全部 401。
	DownloadExpiry int `yaml:"download_expiry" json:"download_expiry"`

	// —— 图片内容审核（ADR-0034）——
	// AuditEnabled 是审核总开关。关掉等于"不审"，图恢复上传即通过的老行为。
	AuditEnabled bool `yaml:"audit_enabled" json:"audit_enabled"`
	// AuditTimeoutSeconds 是"等不到审核结论"的容忍时长，超过就转人工队列。
	// 注意：超时只把图转人工，**绝不**因为超时就放行。
	AuditTimeoutSeconds int `yaml:"audit_timeout_seconds" json:"audit_timeout_seconds"`
	// AuditAutoDeleteBlock 为真时，判定违规的图立刻删掉对象存储里的文件，
	// 且同哈希永久拒收（终局，不可人工放行）。
	AuditAutoDeleteBlock bool `yaml:"audit_auto_delete_block" json:"audit_auto_delete_block"`
	// AuditGateEnabled 是展示闸门的独立开关。
	// 关掉它等于"未过审也可展示"，留作审核链路故障时的紧急放行手段。
	AuditGateEnabled bool `yaml:"audit_gate_enabled" json:"audit_gate_enabled"`
}
