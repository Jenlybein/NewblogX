package ai_search

import (
	"strings"
	"testing"
)

func TestBuildArticleSearchAnalyzeMessagesUsesMarkdownLinkPrompt(t *testing.T) {
	msgList, err := buildArticleSearchAnalyzeMessages("帮我找 SSR 文章", nil)
	if err != nil {
		t.Fatalf("构造消息失败: %v", err)
	}
	if len(msgList) != 2 {
		t.Fatalf("消息数量错误: %d", len(msgList))
	}

	systemPrompt := msgList[0].Content
	if !strings.Contains(systemPrompt, `[python环境搭建](/article/9)`) {
		t.Fatalf("提示词应包含 Markdown 链接示例: %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, `<a href="/article/9">`) {
		t.Fatalf("提示词不应再包含 HTML 链接示例: %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "严禁输出 HTML 标签") {
		t.Fatalf("提示词应明确禁止输出 HTML 标签: %q", systemPrompt)
	}
}
