package config

import (
	"myblogx/internal/platform/mysql"
)

// runtimeSiteConfigRow 是 runtime_site_config_models 的行。
//
// 这张表只有一份"当前配置"，按 name 唯一；读出来反序列化成 RuntimeConfig。
type runtimeSiteConfigRow struct {
	mysql.BaseRow

	Name     string `gorm:"size:32;uniqueIndex;not null"`
	SiteJSON string `gorm:"type:longtext;not null"`
	AIJSON   string `gorm:"type:longtext;not null"`
}

// TableName 固定表名。
func (runtimeSiteConfigRow) TableName() string { return "runtime_site_config_models" }
