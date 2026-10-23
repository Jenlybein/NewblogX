package ai_search

import (
	"encoding/json"
	"errors"
	"fmt"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/service/ai_service"
	"strings"

	"gorm.io/gorm"
)

var articleSearchPrompt = `
你是一个博客文章搜索助手。你的任务是判断用户当前输入是不是“搜索文章”的请求，并想象如何转化词汇能搜索到用户真正需要的文章，把搜索条件提炼成结构化 JSON。

规则：
1. 只能输出合法 JSON，不要输出解释、Markdown、代码块。
2. intent 只能是 "search" 或 "other"。
3. 如果用户是在找站内文章、教程、内容、主题资料，intent="search"。
4. 如果用户明显是在闲聊、问候、写作、聊天、与文章搜索无关，intent="other"。
5. 若 intent=="other"，则除 intent 和 content 外的所有字段都返回对应格式的空；若 intent=="search"，则 content 留空，其他字段按要求填写。
6. content 为正常回复用户的文本，不包含任何搜索词。
6. query 是搜索词数组，保留 1~5 个最相关关键词或同义词，不能是空白字符串。
7. tag_list 只能从给定标签候选中选择，最多 3 个；没有合适标签就返回空数组。
8. sort 只能为 1~6：
   1 默认相关度
   2 最新发布
   3 最多回复
   4 最多点赞
   5 最多收藏
   6 最多浏览
9. 如果用户没有明确排序偏好，sort 返回 1。
10. 严格输出：
{
  "intent": "search",
  "content": "你好，我是 AI 助手。",
  "query": ["Go", "Gin", "中间件"],
  "tag_list": ["Go", "Gin"],
  "sort": 1
}

标签候选：%s
`

func RewriteArticleSearch(db *gorm.DB, aiConf conf.AI, content string) (*ArticleSearchRewrite, error) {
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("搜索内容不能为空")
	}
	if db == nil {
		return nil, errors.New("数据库未初始化")
	}

	var tagList []string
	if err := db.Model(&models.TagModel{}).
		Where("is_enabled = ?", true).
		Order("sort desc, id asc").
		Pluck("title", &tagList).Error; err != nil {
		return nil, fmt.Errorf("查询标签候选失败: %w", err)
	}

	msgList := []ai_service.Message{
		{
			Role:    "system",
			Content: fmt.Sprintf(articleSearchPrompt, ai_service.MustJSONString(tagList)),
		},
		{
			Role:    "user",
			Content: strings.TrimSpace(content),
		},
	}

	reply, err := ai_service.Chat(aiConf, msgList)
	if err != nil {
		return nil, fmt.Errorf("文章搜索意图分析失败: %w", err)
	}

	return normalizeArticleSearchRewrite(reply, content, tagList)
}

func AnalyzeArticleSearchIntent(aiConf conf.AI, content string) (*ArticleSearchRewrite, error) {
	msgList := []ai_service.Message{
		{
			Role:    "system",
			Content: articleSearchPrompt,
		},
		{
			Role:    "user",
			Content: strings.TrimSpace(content),
		},
	}

	reply, err := ai_service.Chat(aiConf, msgList)
	if err != nil {
		return nil, fmt.Errorf("文章搜索意图分析失败: %w", err)
	}

	return normalizeArticleSearchRewrite(reply, content, nil)
}

func normalizeArticleSearchRewrite(raw, fallbackContent string, validTags []string) (*ArticleSearchRewrite, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}

	var payload ArticleSearchRewrite
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("文章搜索改写结果不是有效 JSON: %w", err)
	}

	result := &ArticleSearchRewrite{
		Intent:  normalizeArticleSearchIntent(payload.Intent),
		Content: normalizeArticleSearchReplyContent(payload.Content),
		Query:   normalizeArticleSearchQueries(payload.Query, fallbackContent),
		Sort:    normalizeArticleSearchSort(payload.Sort),
	}

	validTagMap := make(map[string]string, len(validTags))
	for _, tag := range validTags {
		key := normalizeArticleSearchText(tag)
		if key == "" {
			continue
		}
		validTagMap[key] = tag
	}

	seen := make(map[string]struct{}, len(payload.TagList))
	for _, tag := range payload.TagList {
		key := normalizeArticleSearchText(tag)
		if key == "" {
			continue
		}
		actualTag, ok := validTagMap[key]
		if !ok {
			continue
		}
		if _, exists := seen[actualTag]; exists {
			continue
		}
		seen[actualTag] = struct{}{}
		result.TagList = append(result.TagList, actualTag)
		if len(result.TagList) >= 3 {
			break
		}
	}

	if result.Intent == "search" && len(result.Query) == 0 {
		return nil, errors.New("搜索关键词不能为空")
	}

	return result, nil
}

func normalizeArticleSearchIntent(intent string) string {
	switch strings.ToLower(strings.TrimSpace(intent)) {
	case "search", "article_search":
		return "search"
	default:
		return "other"
	}
}

func normalizeArticleSearchQueries(queryList []string, fallbackContent string) []string {
	seen := make(map[string]struct{}, len(queryList))
	result := make([]string, 0, len(queryList))
	for _, item := range queryList {
		item = normalizeArticleSearchText(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
		if len(result) >= 5 {
			return result
		}
	}

	fallbackContent = normalizeArticleSearchText(fallbackContent)
	if fallbackContent != "" {
		return []string{fallbackContent}
	}
	return result
}

func normalizeArticleSearchSort(sort int8) int8 {
	if sort < 1 || sort > 6 {
		return 1
	}
	return sort
}

func normalizeArticleSearchText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func normalizeArticleSearchReplyContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "我目前主要帮你搜索站内文章，你也可以直接告诉我想找什么主题的文章。"
	}
	return content
}
