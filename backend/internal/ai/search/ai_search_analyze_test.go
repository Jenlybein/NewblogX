package aisearch

import (
	"strings"
	"testing"
)

func TestBuildArticleSearchAnalyzeMessagesUsesMarkdownLinkPrompt(t *testing.T) {
	msgList, err := buildArticleSearchAnalyzeMessages("帮我找 SSR 文章", nil, "")
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

// TestBuildArticleSearchAnalyzeMessagesInjectsScenePolicy 钉住 #310 的第一条：
// 场景策略文本（捏捏的口径）原样拼进 system prompt，而不是抄进 Go 代码里。
//
// 断言的是"这段文本确实出现在提示里"，不是"提示里有没有某个固定句子" ——
// 文本内容由运营改，测试不该替他们钉住某一版措辞。
func TestBuildArticleSearchAnalyzeMessagesInjectsScenePolicy(t *testing.T) {
	const policy = "自称「捏捏」，称呼读者用「你」。禁用「作为一个 AI」。"
	msgList, err := buildArticleSearchAnalyzeMessages("帮我找 SSR 文章", nil, policy)
	if err != nil {
		t.Fatalf("构造消息失败: %v", err)
	}
	systemPrompt := msgList[0].Content
	if !strings.Contains(systemPrompt, policy) {
		t.Fatalf("system prompt 应原样包含运行时策略文本: %q", systemPrompt)
	}
	// 口径文本只该出现一次：重复拼进去会让同一段约束在提示里出现两份，
	// 两处一旦漂移，模型收到的就是互相矛盾的指令。
	if strings.Count(systemPrompt, policy) != 1 {
		t.Fatalf("运行时策略文本应只出现一次，实际 %d 次", strings.Count(systemPrompt, policy))
	}
	// 策略为空时不能留下空的"口径"小节（空标题会让模型以为漏了内容）。
	withoutPolicy, err := buildArticleSearchAnalyzeMessages("帮我找 SSR 文章", nil, "  \n ")
	if err != nil {
		t.Fatalf("构造消息失败: %v", err)
	}
	if got := withoutPolicy[0].Content; strings.Contains(got, "助手口径") {
		t.Fatalf("没有配置策略时不应出现口径小节: %q", got)
	}
}
