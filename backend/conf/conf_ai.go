package conf

// AI 是站点级 AI 设置。
//
// 模型列表已经独立成 ai_provider_models 表（见 internal/ai）：这里有供应商、
// 密钥、接口地址这些"多条并存"的东西都搬走了，只剩"站点怎么用 AI"——总闸、
// 展示信息、输入限制与每日额度。
//
// 历史字段 secret / base_url / chat_model / reason_model 已删除：
// 前三个是单模型时代的产物，reason_model 从来没有被任何代码读过。
// 单值的 daily_quota 也已删除：它从上线起就没有被任何调用点读取过，
// 现在换成按场景分设的 DailyLimits（见下方类型注释）。
type AI struct {
	Enable        bool    `yaml:"enable" json:"enable"`
	MaxInputChars int     `yaml:"max_input_chars" json:"max_input_chars"`
	Temperature   float64 `yaml:"temperature" json:"temperature"`
	TimeoutSec    int     `yaml:"timeout_sec" json:"timeout_sec"`
	Nickname      string  `yaml:"nickname" json:"nickname"`
	Avatar        string  `yaml:"avatar" json:"avatar"`
	Abstract      string  `yaml:"abstract" json:"abstract"`

	// DailyLimits 是七个 AI 场景各自的「每人每自然日」调用上限。
	//
	// 为什么不再是一个站点级数字：一次 30 秒的投稿评分和一次两行的选区改写
	// 成本差两个数量级，共用一个预算只会让运营在对成本无感的地方先撞墙。
	// 0 表示不限；负数与超过 9999 的值在保存时被拒绝，读取侧另有一道归一化防线。
	DailyLimits DailyLimits `yaml:"daily_limits" json:"daily_limits"`

	// SceneSkills 是「哪个 AI 场景用哪份运行时策略 skill」的产品接线（#310）。
	//
	// 它属于运行期配置而不是 skill 文件：skill 要保持可移植（任何人拿走都能直接用），
	// 场景绑定要能临时摘掉某份 skill 而不发版（grill §5.3 / ADR-0021）。
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

// DailyLimits 是七个 AI 场景的每日调用上限（每人、每自然日）。
//
// 字段名与 internal/ai 的场景常量一一对应，但**场景名不在这里重复声明**：
// conf 是最底层包，import 不了 internal/ai；映射写在 internal/ai 的配额出口，
// 那里是场景常量的唯一定义处。
type DailyLimits struct {
	Scoring       int `yaml:"scoring" json:"scoring"`
	Metainfo      int `yaml:"metainfo" json:"metainfo"`
	Overwrite     int `yaml:"overwrite" json:"overwrite"`
	Diagnose      int `yaml:"diagnose" json:"diagnose"`
	SearchRewrite int `yaml:"search_rewrite" json:"search_rewrite"`
	SearchAnswer  int `yaml:"search_answer" json:"search_answer"`
	Moderation    int `yaml:"moderation" json:"moderation"`
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
