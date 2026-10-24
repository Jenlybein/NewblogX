package testkit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// UpdateGoldenEnv 置为 "1" 时刷新 golden 文件。
const UpdateGoldenEnv = "UPDATE_GOLDEN"

// AssertGolden 比较 got 与 testdata/<name> 的内容。
//
// 设置环境变量 UPDATE_GOLDEN=1 可刷新基准文件。
// 注意：刷新会写文件，此时不要带 -parallel 之外的花样（不同用例写不同文件是安全的）。
func AssertGolden(t testing.TB, name string, got []byte) {
	t.Helper()
	CompareGolden(t, "testdata", name, got, os.Getenv(UpdateGoldenEnv) == "1")
}

// CompareGolden 是 AssertGolden 的显式版本，dir/update 由调用方指定，
// 便于在临时目录中自测这个助手本身。
func CompareGolden(t testing.TB, dir, name string, got []byte, update bool) {
	t.Helper()

	path := filepath.Join(dir, name)

	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("testkit: 创建 golden 目录失败: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("testkit: 写入 golden 失败: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("testkit: 读取 golden %q 失败（首次生成请设 %s=1）: %v", path, UpdateGoldenEnv, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("testkit: 与 golden %q 不一致\n--- want ---\n%s\n--- got ---\n%s\n（确认新行为正确后设 %s=1 刷新）",
			path, want, got, UpdateGoldenEnv)
	}
}
