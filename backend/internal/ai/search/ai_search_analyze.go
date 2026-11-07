package aisearch

import (
	"context"
	"fmt"
	"myblogx/conf"
	"myblogx/internal/ai"
	"myblogx/internal/apperr"
	searchdomain "myblogx/internal/search/domain"
	"strings"
)

// articleSearchAnswerIntro 是搜索回答的功能性角色说明。
//
// 口吻、称呼与禁用语**不**写在这里：那些来自运行期配置绑定的运行时策略文本
// （当前是 skills/jelly-voice/SKILL.md，见 #310），运营改文即改行为。
const articleSearchAnswerIntro = `你负责回答读者关于站内文章的问题：讲清站内有什么，并把站内的内容讲清楚。请根据用户问题和搜索结果，生成一段简洁自然的中文回复。`

// articleSearchAnswerRules 是不随运营改动的东西：输出格式、链接形态、不许编造。
//
// 它们与"怎么说"分开维护：口径文本可以天天改，链接语法写错会让前端渲染出
// 一段不可点的文本。
const articleSearchAnswerRules = `
要求：
1. 只输出回复正文，不要输出 Markdown 代码块。
2. 如果有结果，先用一句话概括，再列出最多 6 篇最相关文章。
3. 文章使用这种格式：
   1. [python环境搭建](/article/9)
4. 如果没有结果，明确告诉用户暂时没找到相关文章。
5. 不要编造文章，不要输出不在结果里的链接。
6. 严禁输出 HTML 标签（尤其是 <a> 标签），链接必须使用 Markdown 语法。
`

func AnalyzeArticleSearchStream(ctx context.Context, aiConf conf.AI, question string, list []searchdomain.Item, policy string) (chan string, chan error, error) {
	msgList, err := buildArticleSearchAnalyzeMessages(question, list, policy)
	if err != nil {
		return nil, nil, err
	}
	contentChan, errChan := ai.ChatStream(ctx, aiConf, msgList)
	return contentChan, errChan, nil
}

func buildArticleSearchAnalyzeMessages(question string, list []searchdomain.Item, policy string) ([]ai.Message, error) {
	if strings.TrimSpace(question) == "" {
		return nil, apperr.New(apperr.CodeAIQuestionRequired)
	}

	searchList := make([]AISearchList, 0, len(list))
	for _, item := range list {
		tagTitles := make([]string, 0, len(item.Tags))
		for _, tag := range item.Tags {
			if tag.Title == "" {
				continue
			}
			tagTitles = append(tagTitles, tag.Title)
		}
		searchList = append(searchList, AISearchList{
			ID:           item.ID,
			CreatedAt:    item.CreatedAt,
			Title:        item.Title,
			Abstract:     item.Abstract,
			Content:      item.Content,
			Part:         item.Part,
			ViewCount:    item.ViewCount,
			DiggCount:    item.DiggCount,
			CommentCount: item.CommentCount,
			FavorCount:   item.FavorCount,
			Tags:         tagTitles,
		})
	}

	return []ai.Message{
		{
			Role:    "system",
			Content: articleSearchAnswerSystemPrompt(strings.TrimSpace(question), searchList, policy),
		},
		{
			Role:    "user",
			Content: strings.TrimSpace(question),
		},
	}, nil
}

// articleSearchAnswerSystemPrompt 拼最终 system prompt。
//
// policy 是装配层按「场景 → skill」配置读出来的正文：这里只负责把它放在
// 角色说明之后、功能规则之前，不改写、不摘要、不重复。
func articleSearchAnswerSystemPrompt(question string, list []AISearchList, policy string) string {
	var b strings.Builder
	b.WriteString(articleSearchAnswerIntro)
	if policy = strings.TrimSpace(policy); policy != "" {
		b.WriteString("\n\n助手口径（与上面的角色说明同等优先，必须遵守）：\n\n")
		b.WriteString(policy)
	}
	b.WriteString("\n")
	b.WriteString(articleSearchAnswerRules)
	fmt.Fprintf(&b, "\n用户问题：%s\n\n搜索结果(JSON)：%s\n", question, ai.MustJSONString(list))
	return b.String()
}
