package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var globalTokenPattern = regexp.MustCompile(`\bglobal\.`)

func TestNoGlobalReferencesInNonTestCode(t *testing.T) {
	root := projectRoot(t)

	var violations []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".turbo" || name == "bin" || name == "logs" || name == "var" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(content)

		if strings.Contains(text, `"myblogx/global"`) || globalTokenPattern.MatchString(text) {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			violations = append(violations, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 global 引用失败: %v", err)
	}

	if len(violations) > 0 {
		t.Fatalf("检测到被禁止的 global 引用（非测试代码）: %v", violations)
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("获取当前文件路径失败")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}
