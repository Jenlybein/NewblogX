package ddl

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// 本文件提供「在既有库上补列」的幂等执行通道。
//
// # 为什么需要它
//
// schema.sql 里的每条语句都是 CREATE TABLE IF NOT EXISTS：对**已经存在**的表
// 它是空操作。所以只把新列写进 CREATE TABLE，全新库会有这一列，而既有的开发库
// 与生产库永远不会有——代码查这一列就报 Unknown column。
//
// 在此之前项目没有 ALTER 通道（`-db` 只建表），于是「加列」在实践中等于
// 「必须人工在各环境敲 ALTER」。columnAdditions 把这个动作收进仓库：
// 登记一条、`-db` 幂等执行、可 review、可重复跑。
//
// # 边界（别把它用成 AutoMigrate）
//
// 这里**只做加法**：补列、补唯一索引。不做改类型、不改列名、不删列、不补外键。
// 那几类动作会让 CDC（按 binlog 列名解析的两条链路）静默错位，仍然禁止。
// 每条登记都必须能在“列已存在 / 索引已存在”时安全跳过。
//
// # 为什么不用 MySQL 的 IF NOT EXISTS
//
// MySQL 8 的 ADD COLUMN / CREATE INDEX 都不支持 IF NOT EXISTS（那是 MariaDB 的
// 扩展）。所以幂等只能靠先查 information_schema 再决定是否执行。

// ColumnAddition 是一条「补列」登记。
//
// 字段刻意用显式 SQL 片段而不是 Go 结构体：这是 DDL，要能被 review，
// 也要能脱离 Go 类型独立存在（schema.sql 才是表结构的权威来源）。
type ColumnAddition struct {
	// Table 表名（不带反引号）。
	Table string
	// Column 列名，用于判断是否已存在。
	Column string
	// Definition 完整列定义，含列名与类型，例如 "`phone` varchar(20) DEFAULT NULL"。
	Definition string
	// UniqueIndex 唯一索引名；空表示不补索引。
	UniqueIndex string
	// IndexColumns 唯一索引的列表达式，例如 "`phone`"；UniqueIndex 非空时必填。
	IndexColumns string
}

// columnAdditions 是补列的唯一权威清单。
//
// 新增一条之前先问：能不能不补列？补列是给「既有表」动结构，代价是这条登记
// 必须长期留在仓库里、并在每个环境跑一遍。能开新表就开新表。
var columnAdditions = []ColumnAddition{
	{
		Table:  "user_models",
		Column: "phone",
		// 与 schema.sql 里那一行的类型/可空性必须逐字一致：
		// 「全新库走 CREATE、既有库走 ALTER」两条路径得到同一张表，
		// 由 migrate_roundtrip_test.go 往返比对钉住。
		Definition: "`phone` varchar(20) DEFAULT NULL",
		// 手机号是登录标识，必须唯一；NULL 不受唯一约束限制，所以未绑定手机号的
		// 老账号（含邮箱注册与 QQ 建档）可以共存。
		UniqueIndex:  "uk_user_phone",
		IndexColumns: "`phone`",
	},
}

// Additions 返回补列清单的副本（供测试与文档断言，不允许调用方改内部状态）。
func Additions() []ColumnAddition {
	out := make([]ColumnAddition, len(columnAdditions))
	copy(out, columnAdditions)
	return out
}

// ApplyColumnAdditions 逐条补齐缺失的列与唯一索引，已存在则跳过。
//
// 幂等：可以重复执行；中途失败留下已完成的部分，下次重跑会从缺失处继续。
func ApplyColumnAdditions(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("补列失败: 数据库未初始化")
	}
	for _, add := range columnAdditions {
		if err := applyOneAddition(db, add); err != nil {
			return err
		}
	}
	return nil
}

// applyOneAddition 处理单条登记。
func applyOneAddition(db *gorm.DB, add ColumnAddition) error {
	exists, err := columnExists(db, add.Table, add.Column)
	if err != nil {
		return fmt.Errorf("检查列 %s.%s 是否存在失败: %w", add.Table, add.Column, err)
	}
	if !exists {
		stmt := fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN %s", add.Table, add.Definition)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("补列 %s.%s 失败: %w\n语句: %s", add.Table, add.Column, err, stmt)
		}
	}

	if add.UniqueIndex == "" {
		return nil
	}
	if strings.TrimSpace(add.IndexColumns) == "" {
		// 登记本身写错了：宁可报错，也不要静默建出一个空索引。
		return fmt.Errorf("补列 %s.%s 的登记缺 IndexColumns", add.Table, add.Column)
	}
	hasIndex, err := indexExists(db, add.Table, add.UniqueIndex)
	if err != nil {
		return fmt.Errorf("检查索引 %s.%s 是否存在失败: %w", add.Table, add.UniqueIndex, err)
	}
	if hasIndex {
		return nil
	}
	stmt := fmt.Sprintf("ALTER TABLE `%s` ADD UNIQUE KEY `%s` (%s)", add.Table, add.UniqueIndex, add.IndexColumns)
	if err := db.Exec(stmt).Error; err != nil {
		return fmt.Errorf("补唯一索引 %s.%s 失败: %w\n语句: %s", add.Table, add.UniqueIndex, err, stmt)
	}
	return nil
}

// columnExists 判断列是否存在。
func columnExists(db *gorm.DB, table, column string) (bool, error) {
	var n int64
	err := db.Raw(
		"SELECT COUNT(*) FROM information_schema.columns "+
			"WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?",
		table, column,
	).Scan(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// indexExists 判断索引是否存在。
func indexExists(db *gorm.DB, table, index string) (bool, error) {
	var n int64
	err := db.Raw(
		"SELECT COUNT(*) FROM information_schema.statistics "+
			"WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?",
		table, index,
	).Scan(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
