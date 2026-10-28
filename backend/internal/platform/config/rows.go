package config

import (
	"myblogx/internal/platform/mysql"
)

// runtimeConfigRow 是 runtime_config_models 的行。
//
// 一段一行：name 是段名（site / ai / mail / storage / oauth / log / upload），
// payload 是这一段的完整 JSON。
//
// 为什么不是"一段一列"：项目的 -db 只执行 CREATE TABLE IF NOT EXISTS，
// 不做 ALTER。列化意味着每加一个配置字段都要改 schema.sql + 行结构 + 列契约
// 三处并跑 DDL；而一段一行的形状让"加字段"退化成"改一个 Go 结构体"。
type runtimeConfigRow struct {
	mysql.BaseRow

	Name    string `gorm:"size:32;uniqueIndex;not null"`
	Payload string `gorm:"type:longtext;not null"`
}

// TableName 固定表名。
func (runtimeConfigRow) TableName() string { return "runtime_config_models" }
