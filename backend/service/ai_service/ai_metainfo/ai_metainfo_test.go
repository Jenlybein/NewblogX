package ai_metainfo

import "testing"

func TestNormalizeArticleMetainfoReply(t *testing.T) {
	raw := "```json\n{\n  \"title\": \"Go 中间件实践总结\",\n  \"abstract\": \"文章围绕 Gin 中间件设计、日志链路与鉴权流程展开。\",\n  \"category\": {\"id\": 8, \"title\": \"Go 后端\"},\n  \"tags\": [\n    {\"id\": 1, \"title\": \"Go\"},\n    {\"id\": 2, \"title\": \"Gin\"},\n    {\"id\": 4, \"title\": \"无效标签\"}\n  ]\n}\n```"

	categoryOptions := []Metainfos{
		{ID: 8, Title: "Go 后端"},
		{ID: 9, Title: "Redis"},
	}
	tagOptions := []Metainfos{
		{ID: 1, Title: "Go"},
		{ID: 2, Title: "Gin"},
		{ID: 3, Title: "中间件"},
	}

	result, err := normalizeArticleMetainfoReply(raw, categoryOptions, tagOptions)
	if err != nil {
		t.Fatalf("解析文章元信息失败: %v", err)
	}

	if result.Title != "Go 中间件实践总结" {
		t.Fatalf("标题解析错误: %q", result.Title)
	}
	if result.Abstract != "文章围绕 Gin 中间件设计、日志链路与鉴权流程展开。" {
		t.Fatalf("摘要解析错误: %q", result.Abstract)
	}
	if result.Category == nil || result.Category.ID != 8 {
		t.Fatalf("分类匹配错误: %+v", result.Category)
	}
	if len(result.Tags) != 2 {
		t.Fatalf("标签数量错误: %+v", result.Tags)
	}
	if result.Tags[0].ID != 1 || result.Tags[1].ID != 2 {
		t.Fatalf("标签匹配错误: %+v", result.Tags)
	}
}

func TestNormalizeArticleMetainfoReplyInvalidCategoryAndTags(t *testing.T) {
	raw := `{"title":"","abstract":"","category":{"id":100,"title":"不存在"},"tags":[{"id":100,"title":"不存在"}]}`

	result, err := normalizeArticleMetainfoReply(raw, nil, nil)
	if err != nil {
		t.Fatalf("解析文章元信息失败: %v", err)
	}

	if result.Title != "" {
		t.Fatalf("标题应保持原值: %q", result.Title)
	}
	if result.Abstract != "" {
		t.Fatalf("摘要应保持原值: %q", result.Abstract)
	}
	if result.Category != nil {
		t.Fatalf("无效分类不应保留: %+v", result.Category)
	}
	if len(result.Tags) != 0 {
		t.Fatalf("无效标签不应保留: %+v", result.Tags)
	}
}
