package models

// RuntimeSiteConfigModel 存储可在线修改的运行时站点配置。
// 部署级配置仍保留在 settings.yaml / 环境变量中，不走这张表。
type RuntimeSiteConfigModel struct {
	Model
	Name     string `gorm:"size:32;uniqueIndex;not null" json:"name"`
	SiteJSON string `gorm:"type:longtext;not null" json:"site_json"`
	AIJSON   string `gorm:"type:longtext;not null" json:"ai_json"`
}
