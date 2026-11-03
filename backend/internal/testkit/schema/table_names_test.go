package schema_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	mysqlddl "myblogx/internal/platform/mysql/ddl"
	"myblogx/internal/testkit/schema"
)

// TestTableNamesAgreeWithSchemaSQL 是"表名单一真相"的**默认可跑**守卫。
//
// # 为什么需要它
//
// 生产建表只认 internal/platform/mysql/ddl/schema.sql，而各模块查询时用的表名
// 写在自己 adapter 的 `TableName()` 里 —— 同一个名字有两个出处。漂移的表现是
// 生产执行到那条 SQL 才报 Error 1146（表不存在），测试全绿：
//   - chat 集成测试用 testkit 的 AutoMigrate 建表，表名天然正确；
//   - 原来只比对表清单的 schema_sql_integration_test.go 带 `//go:build integration`，
//     `go test ./...` 根本不会编译它。
//
// # 它查什么（全部静态，不连库、不需要 Docker）
//
//  1. 解析 schema.sql 的 CREATE TABLE 表名集合（真源头）；
//  2. 扫描 backend 下**所有非测试 Go 文件**的 `func (X) TableName() string` 声明，
//     含 adapter 私有行结构与 testkit 行结构；返回字符串常量的也能解析；
//  3. 声明了 schema.sql 里不存在的表名 → 失败（这正是 chat stateRow 的 bug）；
//  4. testkit 的 All 组必须与 schema.sql 表集合**逐一对齐**，防止新表只加进
//     schema.sql 却没有测试行结构（那样测试永远建不出它）。
//
// 故意把任意一处 TableName 改坏（例如 chat 的状态表改回 chat_msg_user_states），
// 这个测试就会红，而不是等到线上删消息时才发现。
func TestTableNamesAgreeWithSchemaSQL(t *testing.T) {
	sqlTables := schemaSQLTables(t)
	if len(sqlTables) < 30 {
		t.Fatalf("只从 schema.sql 解析出 %d 张表，文件可能被截断或解析规则失效", len(sqlTables))
	}

	sites, errs := declaredTableNames(t)
	if len(errs) > 0 {
		t.Fatalf("解析 TableName() 声明失败:\n%s", strings.Join(errs, "\n"))
	}
	if len(sites) == 0 {
		t.Fatal("没有扫描到任何 TableName() 声明，扫描规则可能失效")
	}
	for _, s := range sites {
		if !sqlTables[s.Name] {
			t.Errorf("%s:%d 声明了表名 %q，但 schema.sql 里没有这张表（生产查询/写入会报 Error 1146）",
				s.File, s.Line, s.Name)
		}
	}

	allRows := allRowTableNames(t)
	if diff := diffTableSets(allRows, sqlTables); diff != "" {
		t.Errorf("testkit schema.All 与 schema.sql 的表清单不一致%s", diff)
	}
}

// tableSite 是某一处 TableName 声明的位置。
type tableSite struct {
	Name string
	File string
	Line int
}

// createTableRE 只匹配真正的建表语句（行首、带反引号）。
//
// schema.sql 的注释里也出现过 "CREATE TABLE IF NOT EXISTS" 字样，
// 锚定行首与反引号避免把注释算进去。
var createTableRE = regexp.MustCompile("(?m)^CREATE TABLE IF NOT EXISTS `([a-z0-9_]+)`")

// schemaSQLTables 返回 schema.sql 声明的表名集合。
//
// 用 ddl.DDL（embed 后的正文）而不是自己找文件：测试与生产用的必须字节一致。
func schemaSQLTables(t *testing.T) map[string]bool {
	t.Helper()
	matches := createTableRE.FindAllStringSubmatch(mysqlddl.DDL, -1)
	out := make(map[string]bool, len(matches))
	for _, m := range matches {
		out[m[1]] = true
	}
	return out
}

// allRowTableNames 返回 testkit All 组每一行结构对应的表名。
//
// 与扫描出的声明比对的是"集合"而不是类型名，因为重点是 schema.sql 里每张表
// 都得有测试侧的行结构。
func allRowTableNames(t *testing.T) map[string]bool {
	t.Helper()
	out := make(map[string]bool, 40)
	for _, row := range schema.Models(schema.All) {
		namer, ok := row.(interface{ TableName() string })
		if !ok {
			t.Fatalf("schema.All 里的 %T 没有 TableName() 方法", row)
		}
		name := namer.TableName()
		if out[name] {
			t.Errorf("schema.All 里有两行结构指向同一张表 %q", name)
		}
		out[name] = true
	}
	return out
}

// declaredTableNames 扫描 backend 下所有非测试 Go 文件里 `TableName()` 的字面量返回值。
//
// 包级字符串常量也能解析：outboxrelay 的 outboxRow 返回 OutboxTableName，
// 不能因此被静默忽略（静默跳过等于守卫有个洞）。
func declaredTableNames(t *testing.T) ([]tableSite, []string) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件路径")
	}
	backendRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")

	type parsedFile struct {
		dir  string
		path string
		file *ast.File
	}
	fset := token.NewFileSet()
	var files []parsedFile
	err := filepath.WalkDir(backendRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return fmt.Errorf("解析 %s 失败: %w", path, perr)
		}
		files = append(files, parsedFile{dir: filepath.Dir(path), path: path, file: file})
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 backend 源码失败: %v", err)
	}

	// 第一遍：每个包的字符串常量表，供 TableName() 返回常量名时解析。
	consts := make(map[string]map[string]string, len(files))
	for _, f := range files {
		byName := consts[f.dir]
		if byName == nil {
			byName = map[string]string{}
			consts[f.dir] = byName
		}
		for _, decl := range f.file.Decls {
			gd, isGen := decl.(*ast.GenDecl)
			if !isGen || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, isValue := spec.(*ast.ValueSpec)
				if !isValue || len(vs.Names) != 1 || len(vs.Values) != 1 {
					continue
				}
				lit, isLit := vs.Values[0].(*ast.BasicLit)
				if !isLit || lit.Kind != token.STRING {
					continue
				}
				if value, uerr := strconv.Unquote(lit.Value); uerr == nil {
					byName[vs.Names[0].Name] = value
				}
			}
		}
	}

	// 第二遍：收集 TableName() 声明。
	var sites []tableSite
	var errs []string
	for _, f := range files {
		for _, decl := range f.file.Decls {
			// 只看方法（有 receiver），避免把同名普通函数算进来。
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv == nil || fn.Name.Name != "TableName" {
				continue
			}
			pos := fset.Position(fn.Pos())
			name, rerr := tableNameFromFunc(fn, consts[f.dir])
			if rerr != "" {
				errs = append(errs, fmt.Sprintf("%s: %s", pos, rerr))
				continue
			}
			sites = append(sites, tableSite{Name: name, File: pos.Filename, Line: pos.Line})
		}
	}
	return sites, errs
}

// tableNameFromFunc 从 `func (...) TableName() string { return "x" }` 里取出 x。
//
// 形状不认识就返回错误而不是空值：守卫宁可红，也不能因为写法变了而悄悄不检查。
func tableNameFromFunc(fn *ast.FuncDecl, consts map[string]string) (string, string) {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return "", fmt.Sprintf("%s.TableName 的 body 不是单条 return，无法静态解析", fn.Name.Name)
	}
	stmt, isReturn := fn.Body.List[0].(*ast.ReturnStmt)
	if !isReturn || len(stmt.Results) != 1 {
		return "", fmt.Sprintf("%s.TableName 的 body 不是单条 return，无法静态解析", fn.Name.Name)
	}
	switch expr := stmt.Results[0].(type) {
	case *ast.BasicLit:
		if expr.Kind != token.STRING {
			break
		}
		value, err := strconv.Unquote(expr.Value)
		if err != nil {
			return "", fmt.Sprintf("%s.TableName 返回的字符串无法解析: %v", fn.Name.Name, err)
		}
		return value, ""
	case *ast.Ident:
		if value, ok := consts[expr.Name]; ok {
			return value, ""
		}
		return "", fmt.Sprintf("%s.TableName 返回常量 %s，但没在同包里解析到它的字符串值", fn.Name.Name, expr.Name)
	}
	return "", fmt.Sprintf("%s.TableName 的返回值不是字符串字面量/字符串常量，无法静态解析", fn.Name.Name)
}

// diffTableSets 返回 a（声明清单）与 b（schema.sql）差异的可读文本。
func diffTableSets(a, b map[string]bool) string {
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
		msg += "\n  testkit 有、schema.sql 没有: " + strings.Join(onlyA, " | ")
	}
	if len(onlyB) > 0 {
		msg += "\n  schema.sql 有、testkit 没有: " + strings.Join(onlyB, " | ")
	}
	return msg
}
