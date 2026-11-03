package schema_test

// 旧扁平收录中间表的退役守卫（#225）。
//
// # 为什么需要它
//
// 两张表（`collection_article_models` / `project_article_models`）自 #218 起不再
// 被读写，用户要求"旧表不留"：代码里不再登记建表，生产库由部署时手动 DROP。
//
// 而"代码里不再登记"这件事没有任何编译器保障：
//   - schema.sql 是编译进镜像的建表脚本（`-db` 只做 CREATE TABLE IF NOT EXISTS），
//     留着 CREATE 就等于下次初始化/升级又把表建回来；
//   - testkit 的 rows/groups 是测试侧的第二份真相，AutoMigrate 会把它们建出来，
//     于是测试里"旧表还能写"看起来永远成立，而生产已经没这张表了；
//   - 仓储代码里的 TableName() 是运行期字符串，留着一行 SQL 编译照过。
//
// 所以这里按"取景体系退场"（retired_framing_test.go，#203/#205）的同一套做法
// 扫描：schema.sql 不得再出现这张表，backend 下任何 .go 文件（含 _test.go）
// 也不得再提到这两个表名。

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mysqlddl "myblogx/internal/platform/mysql/ddl"
	"myblogx/internal/testkit/schema"
)

// retiredFlatIncludeTables 是 #225 之后不允许再出现在后端的旧中间表。
var retiredFlatIncludeTables = []string{
	"collection_article_models",
	"project_article_models",
}

// TestRetiredFlatIncludeTablesHaveNoCodePath 覆盖三件事：建表脚本、测试行登记、
// 全后端源码（含测试）。
//
// 本测试文件自身必须豁免：断言列表里的表名会把测试自己判红。
func TestRetiredFlatIncludeTablesHaveNoCodePath(t *testing.T) {
	t.Helper()

	for _, table := range retiredFlatIncludeTables {
		if strings.Contains(mysqlddl.DDL, "`"+table+"`") {
			t.Errorf("schema.sql 仍在创建已退役的旧中间表 %q：`-db` 会把表建回来，"+
				"与「旧表不留」的要求相反", table)
		}
	}

	retired := make(map[string]bool, len(retiredFlatIncludeTables))
	for _, table := range retiredFlatIncludeTables {
		retired[table] = true
	}
	for _, row := range schema.Models(schema.All) {
		namer, ok := row.(interface{ TableName() string })
		if !ok {
			t.Fatalf("schema.All 里的 %T 没有 TableName() 方法", row)
		}
		if retired[namer.TableName()] {
			t.Errorf("testkit schema.All 仍登记旧中间表 %q：测试会把它建出来，"+
				"于是「旧表已删」在测试里永远看不出问题", namer.TableName())
		}
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件路径")
	}
	backendRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")

	var hits []string
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
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if sameFile(path, thisFile) {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		text := string(data)
		for _, table := range retiredFlatIncludeTables {
			if strings.Contains(text, table) {
				rel, _ := filepath.Rel(backendRoot, path)
				hits = append(hits, rel+" 命中 "+table)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描后端源码失败: %v", err)
	}
	if len(hits) > 0 {
		t.Fatalf("旧扁平收录中间表已退役（生产库由部署时手动 DROP），后端不得再引用：\n%s",
			strings.Join(hits, "\n"))
	}
}
