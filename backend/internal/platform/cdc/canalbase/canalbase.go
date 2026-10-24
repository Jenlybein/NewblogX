// Package canalbase 是 CDC 的单一 canal 引擎。
//
// 职责边界：只负责把业务配置翻译成一个可用的 canal 实例——连接参数、
// 日志适配、表过滤正则、全量转储范围。业务子包（river / imageref）只提供
// "schema.table → 处理逻辑"的映射，不再各自调用 canal.NewCanal。
//
// 刻意不接管 Run：river 需要 RunFrom(master.Position()) 做位点恢复，
// imageref 用 Run()，两者启动语义不同，由业务自己决定。
package canalbase

import (
	"sort"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/sirupsen/logrus"
)

// MySQL 是启动 canal 所需的连接信息，是各业务配置结构里的公共部分。
type MySQL struct {
	Addr     string
	User     string
	Password string
	Charset  string
	Flavor   string

	ServerID uint32

	DumpExec       string
	SkipMasterData bool
}

// Spec 描述一组 schema.table。用于两处：
//   - 构建 Build 时作为"监听范围"（生成 IncludeTableRegex）
//   - 调用 PrepareDump 时作为"全量转储范围"
//
// 两者在 river 里来源不同（Sources 管监听、Rules 管映射），所以分开传。
type Spec struct {
	Tables map[string][]string
}

// Build 构造一个已完成连接、日志适配、表过滤配置的 canal 实例。
// 注意：canal.NewCanal 不会立刻建立连接，真正连接发生在 Run 时。
func Build(m MySQL, listen Spec, logger *logrus.Logger) (*canal.Canal, error) {
	cfg := canal.NewDefaultConfig()

	cfg.Logger = NewLogAdapter(logger)

	cfg.Addr = m.Addr
	cfg.User = m.User
	cfg.Password = m.Password
	cfg.Charset = m.Charset
	cfg.Flavor = m.Flavor
	cfg.ServerID = m.ServerID

	cfg.Dump.ExecutionPath = m.DumpExec
	cfg.Dump.DiscardErr = false
	cfg.Dump.SkipMasterData = m.SkipMasterData

	cfg.IncludeTableRegex = buildIncludeRegex(listen)

	return canal.NewCanal(cfg)
}

// PrepareDump 设置全量转储范围：
// 只有一个 schema 时按表转储，多个 schema 时只能按库转储。
func PrepareDump(c *canal.Canal, dump Spec) {
	if c == nil || len(dump.Tables) == 0 {
		return
	}

	schemas := sortedSchemas(dump.Tables)

	if len(schemas) == 1 {
		tables := append([]string(nil), dump.Tables[schemas[0]]...)
		sort.Strings(tables)
		c.AddDumpTables(schemas[0], tables...)
		return
	}

	c.AddDumpDatabases(schemas...)
}

// buildIncludeRegex 把 Spec 展开成 canal 的表过滤正则列表。
//
// 排序是刻意的：Spec.Tables 是 map，直接遍历顺序随机，
// 会让生成的配置每次不同（不利于排查与对照）。
func buildIncludeRegex(spec Spec) []string {
	if len(spec.Tables) == 0 {
		return nil
	}

	schemas := sortedSchemas(spec.Tables)
	out := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		tables := append([]string(nil), spec.Tables[schema]...)
		sort.Strings(tables)
		for _, table := range tables {
			out = append(out, schema+`\.`+table)
		}
	}
	return out
}

func sortedSchemas(tables map[string][]string) []string {
	schemas := make([]string, 0, len(tables))
	for schema := range tables {
		schemas = append(schemas, schema)
	}
	sort.Strings(schemas)
	return schemas
}
