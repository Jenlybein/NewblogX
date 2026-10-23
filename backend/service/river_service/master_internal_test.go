package river_service

import (
	"myblogx/test/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-mysql-org/go-mysql/mysql"
)

func TestLoadMasterInfoAndSaveReload(t *testing.T) {
	testutil.InitGlobals()
	dir := t.TempDir()

	m, err := loadMasterInfo(dir, nil)
	if err != nil {
		t.Fatalf("loadMasterInfo 初次加载失败: %v", err)
	}
	if m == nil {
		t.Fatal("loadMasterInfo 不应返回 nil")
	}

	m.lastSaveTime = time.Now().Add(-2 * time.Second)
	if err = m.Save(mysql.Position{Name: "mysql-bin.000001", Pos: 123}); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	masterFile := filepath.Join(dir, "master.yaml")
	b, err := os.ReadFile(masterFile)
	if err != nil {
		t.Fatalf("读取 master.yaml 失败: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "mysql-bin.000001") || !strings.Contains(content, "123") {
		t.Fatalf("master.yaml 内容异常: %s", content)
	}

	m2, err := loadMasterInfo(dir, nil)
	if err != nil {
		t.Fatalf("loadMasterInfo 二次加载失败: %v", err)
	}
	pos := m2.Position()
	if pos.Name != "mysql-bin.000001" || pos.Pos != 123 {
		t.Fatalf("读取位置异常: %+v", pos)
	}
}

func TestMasterSaveThrottleAndClose(t *testing.T) {
	testutil.InitGlobals()
	dir := t.TempDir()
	m, err := loadMasterInfo(dir, nil)
	if err != nil {
		t.Fatalf("loadMasterInfo 失败: %v", err)
	}

	m.lastSaveTime = time.Now()
	if err = m.Save(mysql.Position{Name: "mysql-bin.000002", Pos: 456}); err != nil {
		t.Fatalf("Save 节流分支不应报错: %v", err)
	}
	if pos := m.Position(); pos.Name != "mysql-bin.000002" || pos.Pos != 456 {
		t.Fatalf("节流分支后内存位置应更新: %+v", pos)
	}

	if err = m.Close(); err != nil {
		t.Fatalf("Close 不应报错: %v", err)
	}
}

func TestLoadMasterInfoInvalidYAML(t *testing.T) {
	testutil.InitGlobals()
	dir := t.TempDir()
	fp := filepath.Join(dir, "master.yaml")
	if err := os.WriteFile(fp, []byte(":\n:\n"), 0644); err != nil {
		t.Fatalf("写入非法 yaml 失败: %v", err)
	}

	_, err := loadMasterInfo(dir, nil)
	if err == nil {
		t.Fatal("非法 master.yaml 应返回错误")
	}
}

func TestLoadMasterInfoEmptyDir(t *testing.T) {
	testutil.InitGlobals()
	m, err := loadMasterInfo("", nil)
	if err != nil {
		t.Fatalf("空 dataDir 不应报错: %v", err)
	}
	if m == nil {
		t.Fatal("空 dataDir 应返回非 nil masterInfo")
	}
}
