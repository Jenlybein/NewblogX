package service_test

import (
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/river"
	"myblogx/internal/platform/es"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	file := filepath.ToSlash(filepath.Join(dir, "atomic.txt"))
	data := []byte("hello")

	if err := river.WriteFileAtomic(file, data, 0644); err != nil {
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

func TestExtractArticles(t *testing.T) {
	input := map[string]any{
		"hits": []any{
			map[string]any{
				"_source": map[string]any{
					"id":    1,
					"title": "title-1",
				},
			},
		},
	}

	articles := es_service.ExtractArticles(input)
	if len(articles) != 1 {
		t.Fatalf("数量错误: %d", len(articles))
	}
	if articles[0].ID != ctype.NewID(1) || articles[0].Title != "title-1" {
		t.Fatalf("解析结果异常: %+v", articles[0])
	}
}
