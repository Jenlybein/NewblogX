package river_service_test

import (
	river_service "myblogx/service/river_service"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	file := filepath.ToSlash(filepath.Join(dir, "atomic.txt"))
	data := []byte("hello")

	if err := river_service.WriteFileAtomic(file, data, 0644); err != nil {
		t.Fatalf("WriteFileAtomic 失败: %v", err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("文件内容错误: %s", string(got))
	}
}
