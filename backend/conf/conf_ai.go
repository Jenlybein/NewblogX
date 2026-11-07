package conf

// AI 是站点级 AI 设置。
//
// 模型列表已经独立成 ai_provider_models 表（见 internal/ai）：这里有供应商、
// 密钥、接口地址这些"多条并存"的东西都搬走了，只剩"站点怎么用 AI"——总闸、
// 展示信息、输入限制。
//
// 历史字段 secret / base_url / chat_model / reason_model 已删除：
// 前三个是单模型时代的产物，reason_model 从来没有被任何代码读过。
type AI struct {
	Enable        bool    `yaml:"enable" json:"enable"`
	MaxInputChars int     `yaml:"max_input_chars" json:"max_input_chars"`
	Temperature   float64 `yaml:"temperature" json:"temperature"`
	TimeoutSec    int     `yaml:"timeout_sec" json:"timeout_sec"`
	DailyQuota    int     `yaml:"daily_quota" json:"daily_quota"`
	Nickname      string  `yaml:"nickname" json:"nickname"`
	Avatar        string  `yaml:"avatar" json:"avatar"`
	Abstract      string  `yaml:"abstract" json:"abstract"`

	// SceneSkills 是「哪个 AI 场景用哪份运行时策略 skill」的产品接线（#310）。
	//
	// 它属于运行期配置而不是 skill 文件：skill 要保持可移植（任何人拿走都能直接用），
	// 场景绑定要能临时摘掉某份 skill 而不发版（grill §5.3 / ADR-0033）。
	// key 是 AI 调用场景（internal/ai 的 Scene* 常量），value 是 skills/<name> 的名称列表；
	// 一个场景可以拼多份 skill，正文按顺序拼接。
	SceneSkills map[string][]string `yaml:"scene_skills" json:"scene_skills"`

	// Active 是当前启用的模型配置。
	//
	// 由装配层每次现填（读 ProviderService 的内存快照），**不落库、不接受
	// 请求体**：yaml/json 都是 "-"，所以后台 PUT 上来的 body 里带不了它，
	// 整段写库时也不会把密钥写进 runtime_config。
	Active *AIProvider `yaml:"-" json:"-"`
}

// AIProvider 是"当前用哪条模型配置"的投影。
//
// 它不住在这一层：真实数据在 ai_provider_models 表，装配层读出来填进
// AI.Active。这样做的好处是 6 个 AI 子包仍然只收 conf.AI 一个入参，
// 不必为新表各加一个参数。
type AIProvider struct {
	ID     string
	Name   string
	Vendor string
	Model  string
	Fields map[string]string
}
