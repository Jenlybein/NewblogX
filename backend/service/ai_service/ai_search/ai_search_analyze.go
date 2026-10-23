package ai_search

import (
	"errors"
	"fmt"
	"myblogx/conf"
	"myblogx/service/ai_service"
	"myblogx/service/search_service"
	"strings"
)

var articleSearchAnswerPrompt = `
你是一个站内文章搜索助手。请根据用户问题和搜索结果，生成一段简洁自然的中文回复。

要求：
1. 只输出回复正文，不要输出 Markdown 代码块。
2. 如果有结果，先用一句话概括，再列出最多 6 篇最相关文章。
3. 文章使用这种格式：
   1. [python环境搭建](/article/9)
4. 如果没有结果，明确告诉用户暂时没找到相关文章。
5. 不要编造文章，不要输出不在结果里的链接。
6. 严禁输出 HTML 标签（尤其是 <a> 标签），链接必须使用 Markdown 语法。

用户问题：%s

搜索结果(JSON)：%s
`

func AnalyzeArticleSearch(aiConf conf.AI, question string, list []search_service.SearchListResponse) (string, error) {
	msgList, err := buildArticleSearchAnalyzeMessages(question, list)
	if err != nil {
		return "", err
	}

	reply, err := ai_service.Chat(aiConf, msgList)
	if err != nil {
		return "", fmt.Errorf("文章搜索结果分析失败: %w", err)
	}

	reply = strings.TrimSpace(reply)
	if reply == "" {
		return "", errors.New("文章搜索结果分析为空")
	}
	return reply, nil
}

func AnalyzeArticleSearchStream(aiConf conf.AI, question string, list []search_service.SearchListResponse) (chan string, chan error, error) {
	msgList, err := buildArticleSearchAnalyzeMessages(question, list)
	if err != nil {
		return nil, nil, err
	}
	contentChan, errChan := ai_service.ChatStream(aiConf, msgList)
	return contentChan, errChan, nil
}

func buildArticleSearchAnalyzeMessages(question string, list []search_service.SearchListResponse) ([]ai_service.Message, error) {
	if strings.TrimSpace(question) == "" {
		return nil, errors.New("用户问题不能为空")
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

	return []ai_service.Message{
		{
			Role:    "system",
			Content: fmt.Sprintf(articleSearchAnswerPrompt, strings.TrimSpace(question), ai_service.MustJSONString(searchList)),
		},
		{
			Role:    "user",
			Content: strings.TrimSpace(question),
		},
	}, nil
}
