package ai_metainfo

import (
	"fmt"
	"myblogx/conf"
	"myblogx/service/ai_service"
)

var articleMetainfoPrompt = `
你是一个帮助分析文章元信息的人工智能助手。你必须基于用户提供的文章内容，只输出合法 JSON，不要输出 Markdown 代码块、解释或补充说明。

请完成以下任务：
1. 生成一个精炼且贴合原文的标题，30 字以内。
2. 生成一个精炼且贴合原文的摘要，200 字以内。
3. 从已有分类中选择 0 或 1 个最合适的分类；如果没有合适分类或没有可用分类，返回 null。已有分类：%s
4. 从已有标签中选择 0~3 个最合适的标签；如果没有合适标签或没有可用标签，返回空数组。已有标签：%s
5. 分类和标签只能从给定候选里选择，不能编造 id 或 title。
6. 严格按照下面的 JSON 结构输出：
{
  "title": "",
  "abstract": "",
  "category": {"id": 1, "title": ""},
  "tags": [{"id": 1, "title": ""}]
}
`

func requestArticleMetainfoFromAI(aiConf conf.AI, article string, categoryOptions, tagOptions []Metainfos) (string, error) {
	prompt := fmt.Sprintf(
		articleMetainfoPrompt,
		ai_service.MustJSONString(categoryOptions),
		ai_service.MustJSONString(tagOptions),
	)

	msgList := []ai_service.Message{
		{
			Role:    "system",
			Content: prompt,
		},
		{
			Role:    "user",
			Content: article,
		},
	}

	reply, err := ai_service.Chat(aiConf, msgList)
	if err != nil {
		return "", fmt.Errorf("文章元信息请求失败: %w", err)
	}
	return reply, nil
}
