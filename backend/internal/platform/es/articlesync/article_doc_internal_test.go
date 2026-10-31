package articlesync

// ES 文档形状的回归用例。
//
// 这些断言原本在 cmd/cli 里（当时的入口是导出的 BuildArticleESDocument，
// 只被 CLI 的一个包装函数用、而那个包装只被测试用）。删掉那串死代码之后，
// 断言搬到这里，直接针对生产真正走的那条路：buildArticleESDocumentFromSnapshot。
//
// 盯的是"文档长什么样"——最容易被重构悄悄改掉、改错了还不报错的东西：
// content 不该同步、content_parts 必须是纯文本、tags/category/author 的嵌套形状，
// 以及 author_is_admin 必须来自作者角色（ADR-0017 决策 6 要求删掉三个置顶派生字段）。

import (
	"strings"
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/utils/markdown"
)

func TestArticleDocumentShape(t *testing.T) {
	categoryID := ctype.NewID(3)
	authorID := ctype.NewID(9)
	articleID := ctype.NewID(7)

	snapshot := ArticleRowSnapshot{
		ID:             articleID,
		Title:          "文章标题",
		Abstract:       "文章摘要",
		Content:        "# 标题\n正文",
		CategoryID:     &categoryID,
		Cover:          "/cover.png",
		AuthorID:       authorID,
		ViewCount:      11,
		CommentsToggle: true,
		PublishStatus:  3,
	}
	deps := articleProjectionOptions{
		CategoryTitleMap: map[ctype.ID]string{categoryID: "后端"},
		AuthorMap: map[ctype.ID]authorSnapshot{
			authorID: {Nickname: "作者A", Avatar: "/avatar.png", IsAdmin: true},
		},
		ArticleTagsMap: map[ctype.ID][]ESTag{
			articleID: {
				{ID: ctype.NewID(1), Title: "Go"},
				{ID: ctype.NewID(2), Title: "Redis"},
			},
		},
	}

	doc := buildArticleESDocumentFromSnapshot(snapshot, deps)

	if _, ok := doc["content"]; ok {
		t.Fatal("content 不应被同步到 ES 文档（正文只以 content_parts / content_head 形式存在）")
	}
	if got, ok := doc["content_head"].(string); !ok || got == "" {
		t.Fatalf("content_head 应被生成, got=%#v", doc["content_head"])
	}
	parts, ok := doc["content_parts"].([]markdown.ContentPart)
	if !ok || len(parts) == 0 {
		t.Fatalf("content_parts 应被生成, got=%#v", doc["content_parts"])
	}
	if strings.Contains(parts[0].Content, "# ") {
		t.Fatalf("content_parts 应存纯文本内容, got=%q", parts[0].Content)
	}
	if got, ok := doc["comments_toggle"].(bool); !ok || !got {
		t.Fatalf("comments_toggle 应按 boolean 落库, got=%#v", doc["comments_toggle"])
	}

	tags, ok := doc["tags"].([]ESTag)
	if !ok || len(tags) != 2 || tags[0].Title != "Go" || tags[1].Title != "Redis" {
		t.Fatalf("tags 同步结果不正确: %#v", doc["tags"])
	}
	category, ok := doc["category"].(map[string]any)
	if !ok || category["id"] != categoryID.String() || category["title"] != "后端" {
		t.Fatalf("分类字段同步结果不正确: %#v", doc["category"])
	}
	author, ok := doc["author"].(map[string]any)
	if !ok || author["id"] != authorID.String() || author["nickname"] != "作者A" || author["avatar"] != "/avatar.png" {
		t.Fatalf("作者字段同步结果不正确: %#v", doc["author"])
	}
	if got, ok := doc["author_is_admin"].(bool); !ok || !got {
		t.Fatalf("author_is_admin 应由作者角色投影为 true, got=%#v", doc["author_is_admin"])
	}
	// ADR-0017 决策 6：置顶派生字段全部删除（作者置顶功能保留，只是不再进 ES）。
	for _, key := range []string{"top", "admin_top", "author_top"} {
		if _, ok := doc[key]; ok {
			t.Fatalf("ES 文档不应再出现置顶字段 %q, got=%#v", key, doc[key])
		}
	}
}
