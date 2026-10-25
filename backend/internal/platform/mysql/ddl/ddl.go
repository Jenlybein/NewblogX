// Package ddl 是 MySQL 表结构的**唯一权威来源**。
//
// # 为什么表结构是 SQL 文件，而不是运行时 AutoMigrate
//
// 「零列变更」口径下表结构是冻结的契约（ES 投影靠 CDC 按 binlog 列名解析）。
// 冻结的东西该存成文本、进版本库、可 review；每次运行时从 Go 结构推导，
// 等于让"当前代码长什么样"决定"库长什么样"——那正是零 DDL 要禁止的事。
//
// 另外 M6 删掉 models 之后，Go 侧不再有任何地方持有"全表"这个概念，
// AutoMigrate 无处可依。
//
// # 文件怎么来的
//
// 由 M6 的一次性生成器从当时的 models 全部行结构导出
// （AutoMigrate 后逐表 SHOW CREATE TABLE）。此后**手工维护**：
// 加表就加一段 CREATE TABLE，改列就改这一段 —— 但改列违反零列变更口径，
// 真要改必须先想清楚 ES 投影怎么办。
//
// # 一致性怎么保证
//
// 光有文件不够：文件与各模块自持的行结构可能各写各的。所以有成对的契约测试
// （见 internal/testkit/schema）：
//
//   - 把本文件应用到一个空库；
//   - 对每张投影表用 information_schema 比对"行结构推导的列"与"真实列"。
//
// 任一侧漂移都会红。
package ddl

import (
	_ "embed"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// DDL 是全部表的建表语句。
//
//go:embed schema.sql
var DDL string

// Apply 在目标库上建出全部表。
//
// 幂等：语句都是 CREATE TABLE IF NOT EXISTS，可重复执行。
//
// 关外键检查是必须的：DDL 按表名字母序排列，
// 而 article_message_models 引用 user_models —— 引用方先建，
// 不关检查就必然报 Error 1215/1824。整段执行完再恢复，
// 因为恢复是 defer，中途 panic 也不会把连接留在"检查关闭"的状态。
func Apply(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("建表失败: 数据库未初始化")
	}

	if err := db.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
		return fmt.Errorf("关闭外键检查失败: %w", err)
	}
	defer func() {
		// 尽力恢复；失败也无法从这里上报（defer 里没有返回值出口），
		// 但连接归还池前 MySQL 会把会话变量留在当前值 —— 所以这行不能省。
		_ = db.Exec("SET FOREIGN_KEY_CHECKS = 1").Error
	}()

	for i, stmt := range Statements() {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("执行第 %d 条建表语句失败: %w\n语句: %s", i+1, err, stmt)
		}
	}
	return nil
}

// Statements 把 DDL 拆成可逐条执行的语句。
//
// 拆分依据是"空行"而不是分号：生成的 DDL 里每条语句内部都有换行
// （列定义一行一列），而 COMMENT 的文本理论上可能含分号 ——
// 按空行拆不受这个影响。生成器保证语句之间恰好隔一个空行。
func Statements() []string {
	var out []string
	for _, block := range strings.Split(DDL, "\n\n") {
		stmt := stripComments(block)
		if stmt == "" {
			continue
		}
		out = append(out, stmt)
	}
	return out
}

// stripComments 去掉块内的 `--` 注释行，并抹掉结尾的分号。
func stripComments(block string) string {
	var b strings.Builder
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(b.String()), ";"))
}
