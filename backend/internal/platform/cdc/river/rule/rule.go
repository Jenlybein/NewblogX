package rule

import (
	"strings"

	"github.com/go-mysql-org/go-mysql/schema"
)

// Rule 是从MySQL到ES同步数据的规则。
// 如果你想将MySQL数据同步到elasticsearch，你必须设置一个规则让程序知道如何做。
// 映射规则可能是：schema + table <-> index + document type。
// schema和table是用于MySQL的，index和document type是用于Elasticsearch的。
type Rule struct {
	Schema string   `yaml:"schema"` // MySQL数据库名
	Table  string   `yaml:"table"`  // MySQL表名
	Index  string   `yaml:"index"`  // ES索引名
	Type   string   `yaml:"type"`   // ES文档类型
	Parent string   `yaml:"parent"` // 父文档ID
	ID     []string `yaml:"id"`     // 用作文档ID的字段列表

	// 默认情况下，MySQL表字段名映射到Elasticsearch字段名。
	// 有时，你想使用不同的名称，例如，MySQL字段名为title，
	// 但在Elasticsearch中，你想将其命名为my_title。
	FieldMapping map[string]string `yaml:"field"` // 字段映射

	// MySQL表信息
	TableInfo *schema.Table // 表结构信息

	// 只有在过滤器中的MySQL字段才会被同步，默认同步所有字段
	Filter []string `yaml:"filter"` // 字段过滤器

	// Elasticsearch处理管道
	// 在索引前预处理文档
	Pipeline string `yaml:"pipeline"` // 处理管道名称
}

// NewDefaultRule 创建默认规则
func NewDefaultRule(schema string, table string) *Rule {
	r := new(Rule)

	r.Schema = schema
	r.Table = table

	lowerTable := strings.ToLower(table)
	r.Index = lowerTable
	r.Type = lowerTable

	r.FieldMapping = make(map[string]string)

	return r
}

// Prepare 准备规则，设置默认值
func (r *Rule) Prepare() error {
	if r.FieldMapping == nil {
		r.FieldMapping = make(map[string]string)
	}

	if len(r.Index) == 0 {
		r.Index = r.Table
	}

	if len(r.Type) == 0 {
		r.Type = r.Index
	}

	// ES必须使用小写的Type
	// 这里我们也对Index使用小写
	r.Index = strings.ToLower(r.Index)
	r.Type = strings.ToLower(r.Type)

	return nil
}

// CheckFilter 检查字段是否需要被过滤。
func (r *Rule) CheckFilter(field string) bool {
	if r.Filter == nil {
		return true // 如果没有设置过滤器，则不过滤任何字段
	}

	for _, f := range r.Filter {
		if f == field {
			return true // 如果字段在过滤器中，则保留该字段
		}
	}
	return false // 如果字段不在过滤器中，则过滤掉该字段
}
