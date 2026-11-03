package schema_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mysqlddl "myblogx/internal/platform/mysql/ddl"
)

// retiredFramingTables 是取景体系退场（#203 契约与索引、#205 文档与物理下线）之后
// 不允许再出现在后端任何位置（建表脚本、Go 源码、测试替身）的表名。
//
// 为什么需要这条守卫：表一旦物理下线，任何残留的「删主体时清取景孤儿行」调用都会去
// 操作一张不存在的表——删文章 / 删轮播会直接报 Error 1146。这类调用链不会让编译红
// （gorm 的模型名是运行期字符串），只能靠静态扫描拦住。
var retiredFramingTables = []string{
	"picture_crop_models",
	"banner_crop_models",
}

// retiredFramingSymbols 是随取景退场一起删除的端口、行结构、主体类型与调用链名字。
//
// 它们同样编译期可复活：只要有人重新声明一个同名端口或方法，就把已下线的链路接回来。
var retiredFramingSymbols = []string{
	"PictureCropRepository",
	"PictureCropRow",
	"CropTargetType",
	"CropTargetBanner",
	"CropTargetArticle",
	"CropTargetCollection",
	"deleteArticleCrop",
	"CropRepo",
	"cropRow",
}

// TestRetiredFramingTableHasNoCodePath 证明取景表在后端没有留下任何可执行或可建表的痕迹。
//
// 覆盖两件事：
//  1. schema.sql（编译进镜像的建表脚本）不得再出现退役表的 CREATE；
//  2. backend/ 下任何非隐藏目录的 .go 文件（含 _test.go）不得再引用退役表名或符号。
//
// 本测试文件自身必须豁免，否则断言列表里的名字会把测试自己判红。
func TestRetiredFramingTableHasNoCodePath(t *testing.T) {
	t.Helper()

	for _, table := range retiredFramingTables {
		if strings.Contains(mysqlddl.DDL, "`"+table+"`") {
			t.Errorf("schema.sql 仍在创建已下线的取景表 %q（物理下线由现场操作执行，仓库内不得再登记 CREATE）", table)
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
		for _, name := range append(retiredFramingTables, retiredFramingSymbols...) {
			if strings.Contains(text, name) {
				rel, _ := filepath.Rel(backendRoot, path)
				hits = append(hits, rel+" 命中 "+name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描后端源码失败: %v", err)
	}
	if len(hits) > 0 {
		t.Fatalf("取景体系已下线（表已物理下线），后端不得再引用这些表/符号：\n%s",
			strings.Join(hits, "\n"))
	}
}

// sameFile 报告两个路径是否指向同一个文件（大小写与分隔符在 Windows 上不敏感）。
func sameFile(a, b string) bool {
	absA, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(absA), filepath.Clean(absB))
}
