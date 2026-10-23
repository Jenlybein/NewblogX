package service_test

import (
	"myblogx/models/ctype"
	"myblogx/service/es_service"
	river_service "myblogx/service/river_service"
	"myblogx/service/river_service/rule"
	"os"
	"path/filepath"
	"testing"
)

func TestRulePrepareAndFilter(t *testing.T) {
	r := rule.NewDefaultRule("db1", "ArticleTable")
	if r.Index != "articletable" || r.Type != "articletable" {
		t.Fatalf("默认规则大小写处理异常: index=%s type=%s", r.Index, r.Type)
	}

	r.Index = ""
	r.Type = ""
	r.Filter = []string{"id", "title"}
	if err := r.Prepare(); err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	if r.Index != "articletable" || r.Type != "articletable" {
		t.Fatalf("Prepare 默认值异常: index=%s type=%s", r.Index, r.Type)
	}
	if !r.CheckFilter("id") || r.CheckFilter("content") {
		t.Fatal("CheckFilter 结果异常")
	}
}

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
	if articles[0].ID != ctype.ID(1) || articles[0].Title != "title-1" {
		t.Fatalf("解析结果异常: %+v", articles[0])
	}
}
