package schema_test

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	mysqlddl "myblogx/internal/platform/mysql/ddl"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// TestColumnNamesAgreeWithSchemaSQL 是"表名单一真相"的**默认可跑**列比对。
//
// # 为什么需要它
//
// table_names_test.go 只比表名：`project_models` 少一个字段、改一个字段名，
// 表名仍然一致，生产写入会静默忽略那一列。列名漂移正是"测试全绿、线上丢数据"
// 的形态。既有 columns_integration_test.go 带 `//go:build integration`，
// `go test ./...` 根本不编译它。
//
// # 它查什么（全部静态，不连库、不需要 Docker）
//
//  1. 解析 schema.sql 每张表反引号声明的列名集合（真源头）；
//  2. 用反射把 testkit 每一行结构推导成列名（含嵌入的 BaseRow；
//     跳过 gorm:"-" 字段与 struct/slice 关系字段）；
//  3. 两边逐表比集合：多一列、少一列都报出来。
//
// 类型与可空性继续留在 integration 用例（columns_integration_test.go /
// schema_sql_integration_test.go）：那是真库行为，静态解析不冒充。
func TestColumnNamesAgreeWithSchemaSQL(t *testing.T) {
	sqlColumns := schemaSQLColumnSets(t)
	if len(sqlColumns) < 30 {
		t.Fatalf("只从 schema.sql 解析出 %d 张表，文件可能被截断或解析规则失效", len(sqlColumns))
	}

	for _, row := range schema.Models(schema.All) {
		namer, ok := row.(interface{ TableName() string })
		if !ok {
			t.Fatalf("schema.All 里的 %T 没有 TableName() 方法", row)
		}
		table := namer.TableName()
		want, ok := sqlColumns[table]
		if !ok {
			// 表名缺失由 table_names_test.go 直接负责；这里跳过避免重复报。
			continue
		}
		got := rowColumnNames(t, row)
		if diff := diffColumnSets(got, want); diff != "" {
			t.Errorf("表 %s 的列清单与 schema.sql 不一致%s", table, diff)
		}
	}
}

// rowColumnNames 从行结构的字段声明推导列名集合。
//
// 嵌入结构（BaseRow）递归展开；关系字段（别的 Row 类型、切片、map）不落列；
// 显式非持久化字段（gorm:"-"）跳过。
func rowColumnNames(t *testing.T, row any) map[string]bool {
	t.Helper()

	rowType := reflect.TypeOf(row)
	if rowType.Kind() == reflect.Ptr {
		rowType = rowType.Elem()
	}

	timeType := reflect.TypeOf(time.Time{})
	deletedAtType := reflect.TypeOf(gorm.DeletedAt{})
	out := make(map[string]bool, 32)

	var walk func(structType reflect.Type)
	walk = func(structType reflect.Type) {
		for i := 0; i < structType.NumField(); i++ {
			field := structType.Field(i)
			gormTag := field.Tag.Get("gorm")

			if field.Anonymous {
				embedded := field.Type
				if embedded.Kind() == reflect.Ptr {
					embedded = embedded.Elem()
				}
				if embedded.Kind() == reflect.Struct {
					walk(embedded)
					continue
				}
			}

			if strings.TrimSpace(gormTag) == "-" {
				continue
			}

			fieldType := field.Type
			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}
			switch fieldType.Kind() {
			case reflect.Slice, reflect.Array, reflect.Map:
				// 关系切片（hasMany / many2many）不落列；但序列化列
				// （gorm:"type:...;serializer:json"，如 like_tags）是一个真实列。
				if strings.Contains(gormTag, "foreignKey:") || strings.Contains(gormTag, "many2many:") {
					continue
				}
				if !strings.Contains(gormTag, "type:") && !strings.Contains(gormTag, "serializer:") {
					continue
				}
			case reflect.Chan, reflect.Func:
				continue
			case reflect.Struct:
				// 只放行真正的列类型（时间与 gorm 软删标记）；其余 struct 是关系。
				if fieldType != timeType && fieldType != deletedAtType {
					continue
				}
			}

			column := gormColumnName(gormTag)
			if column == "" {
				column = toSnakeCase(field.Name)
			}
			if !out[column] {
				out[column] = true
			}
		}
	}
	walk(rowType)
	return out
}

// gormColumnName 取出 `gorm:"column:xxx"` 里显式声明的列名。
func gormColumnName(tag string) string {
	for _, part := range strings.Split(tag, ";") {
		part = strings.TrimSpace(part)
		if name, ok := strings.CutPrefix(part, "column:"); ok {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// toSnakeCase 复刻 GORM 默认命名的大写缩写规则：
// ID -> id、URL -> url、AITotalScore -> ai_total_score、DimensionsJSON -> dimensions_json。
//
// 静态守卫必须与 GORM 建表时的默认命名一致，否则会报一堆假差异。
func toSnakeCase(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			prevLowerOrDigit := i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]))
			endOfAcronym := i > 0 && unicode.IsUpper(runes[i-1]) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevLowerOrDigit || endOfAcronym {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// schemaSQLColumnSets 解析 schema.sql：每张表 → 列名集合。
func schemaSQLColumnSets(t *testing.T) map[string]map[string]bool {
	t.Helper()

	out := make(map[string]map[string]bool)
	var current string
	for _, line := range strings.Split(mysqlddl.DDL, "\n") {
		if m := createTableColumnsRE.FindStringSubmatch(line); m != nil {
			current = m[1]
			out[current] = map[string]bool{}
			continue
		}
		if current == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ") ENGINE") {
			current = ""
			continue
		}
		// 列声明行以反引号开头；PRIMARY KEY / UNIQUE KEY / KEY / CONSTRAINT 都不是列。
		if !strings.HasPrefix(trimmed, "`") {
			continue
		}
		end := strings.Index(trimmed[1:], "`")
		if end <= 0 {
			t.Fatalf("schema.sql 列声明无法解析: %q", line)
		}
		out[current][trimmed[1:1+end]] = true
	}
	return out
}

var createTableColumnsRE = regexp.MustCompile("(?m)^CREATE TABLE IF NOT EXISTS `([a-z0-9_]+)` \\(")

// TestColumnNamesGuardCatchesDrift 证明这个默认门禁真的会红。
//
// 与 columns_integration_test.go 的 TestColumnsDiffCatchesDrift 同一思路：
// 一个永远通过的断言等于没有。这里用纯静态的人造行结构，分别制造
// "多一列"与"少一大片字段"两种漂移。
func TestColumnNamesGuardCatchesDrift(t *testing.T) {
	t.Parallel()

	want := schemaSQLColumnSets(t)["article_digg_models"]
	if len(want) == 0 {
		t.Fatal("schema.sql 里没有 article_digg_models，无法验证判据")
	}

	extra := rowColumnNames(t, staticGhostRow{})
	if diff := diffColumnSets(extra, want); !strings.Contains(diff, ghostColumn) {
		t.Fatalf("行结构多出来的列应当被报成差异，实际 diff=%q", diff)
	}

	narrow := rowColumnNames(t, staticNarrowRow{})
	if diff := diffColumnSets(narrow, want); !strings.Contains(diff, "created_at") {
		t.Fatalf("行结构漏掉的列应当被报成差异，实际 diff=%q", diff)
	}
}

const ghostColumn = "ghost_column"

// staticGhostRow 比真实表多一列（模拟改错列名 / 凭空捏造列）。
type staticGhostRow struct {
	schema.BaseRow
	GhostColumn string `json:"ghost_column"`
}

// TableName 指向真实表。
func (staticGhostRow) TableName() string { return "article_digg_models" }

// staticNarrowRow 只有一列（模拟搬迁时漏了一大片字段）。
type staticNarrowRow struct {
	ID uint64
}

// TableName 指向真实表。
func (staticNarrowRow) TableName() string { return "article_digg_models" }

// diffColumnSets 返回 a（行结构）与 b（schema.sql）差异的可读文本。
func diffColumnSets(a, b map[string]bool) string {
	var onlyA, onlyB []string
	for name := range a {
		if !b[name] {
			onlyA = append(onlyA, name)
		}
	}
	for name := range b {
		if !a[name] {
			onlyB = append(onlyB, name)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	var msg string
	if len(onlyA) > 0 {
		msg += "\n  行结构有、schema.sql 没有: " + strings.Join(onlyA, " | ")
	}
	if len(onlyB) > 0 {
		msg += fmt.Sprintf("\n  schema.sql 有、行结构没映射: %s", strings.Join(onlyB, " | "))
	}
	return msg
}
