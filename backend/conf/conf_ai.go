package conf

type AI struct {
	Enable        bool    `yaml:"enable" json:"enable"`
	SecretKey     string  `yaml:"secret" json:"secret"`
	BaseURL       string  `yaml:"base_url" json:"base_url"`
	ChatModel     string  `yaml:"chat_model" json:"chat_model"`
	ReasonModel   string  `yaml:"reason_model" json:"reason_model"`
	TimeoutSec    int     `yaml:"timeout_sec" json:"timeout_sec"`
	MaxInputChars int     `yaml:"max_input_chars" json:"max_input_chars"`
	Temperature   float64 `yaml:"temperature" json:"temperature"`
	DailyQuota    int     `yaml:"daily_quota" json:"daily_quota"`
	Abstract      string  `yaml:"abstract" json:"abstract"`
	Nickname      string  `yaml:"nickname" json:"nickname"`
	Avatar        string  `yaml:"avatar" json:"avatar"`
}
