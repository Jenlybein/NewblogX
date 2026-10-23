package ai_scoring

import (
	"fmt"
	"myblogx/service/ai_service"
	"strings"
)

func buildScoringCorePrompt() string {
	return fmt.Sprintf(`
你是一名严格、克制、稳定的中文写作评审助手，只负责文章质量评分与写作指导。

请严格遵守以下规则：
1. 只输出一个 JSON 对象，不要输出任何解释、标题、前后缀、Markdown 代码块。
2. 分数要保守，不得虚高，不顾情面，对差文章敢给差分数，对好文章敢给好分数。
3. 允许输出 0 到 100 的任何整数分数，并严格使用以下分档：较差文章 0-30，不合格文章 31-59，常规文章 60-80，优质文章 81-89，精品文章 90-100。
4. 评分时必须使用以下 6 个维度，且维度名只能使用这些英文键：
   - %s：%s
   - %s：%s
   - %s：%s
   - %s：%s
   - %s：%s
   - %s：%s
5. 你必须先判断文章类型，article_type 只能从以下列表中选择一个：%s。
6. 不同文体的评审侧重点不同：说明文、教程、科普、报告、新闻、通知、邮件更重清晰度、结构性和完整度；议论文、评论、发言稿更重结构性和说服力；散文、小说、诗歌、剧本、童话、传说、记叙文、日记、网文更重可读性、语言表现与结构完成度；文案更重可读性、说服力和表达效率。
7. main_issues 请按重要性排序，并尽量覆盖不同层面的真实问题，不要只给两三条空泛结论。数量要求如下：当参考总分小于 30 分时，必须输出 5 到 9 条；当参考总分小于 60 分时，必须输出 4 到 7 条；当参考总分小于 70 分时，必须输出 2 到 5 条；当参考总分不低于 70 分时，输出 0 到 3 条即可。
8. 如果你看到内容在段首或段尾被工程切分、上下文被截断，不要把“截断本身”当成文章缺陷。
9. 原文位置中的 quote 请尽量截取 10 到 25 个字；如果定位不稳定，段落编号可写 0，但不要编造原文。
10. overall_comment 必须合并输出三层内容：文章整体评价、写作建议、优先修改措施；它是一个字符串，不要拆成列表。
11. total_score 是你基于整体判断给出的参考总分；最终展示总分会由系统按维度分和文章类型另行计算。
`, DimensionClarity, ExplanationClarity, DimensionStructure, ExplanationStructure, DimensionCompleteness, ExplanationCompleteness, DimensionReadability, ExplanationReadability, DimensionPersuasiveness, ExplanationPersuasiveness, DimensionLanguage, ExplanationLanguage, ai_service.MustJSONString(supportedArticleTypes))
}

func buildFirstChunkPrompt(title string, chunk scoringChunk, totalChunks int) string {
	return strings.TrimSpace(fmt.Sprintf(`
%s

当前任务：这是超长文章的第 %d/%d 段，请基于文章标题和当前片段，初始化一个“暂定评分状态”。

文章标题：
%s

当前片段正文：
%s

请输出以下 JSON 结构：
{
  "article_type": "从支持列表中选择一个中文文体名",
  "provisional_dimensions": [
    {"name":"clarity","score":0,"evidence":""},
    {"name":"structure","score":0,"evidence":""},
    {"name":"completeness","score":0,"evidence":""},
    {"name":"readability","score":0,"evidence":""},
    {"name":"persuasiveness","score":0,"evidence":""},
    {"name":"language","score":0,"evidence":""}
  ],
  "chunk_summary": "",
  "global_summary": "",
  "main_issues": [
    {
      "positions":[{"paragraph":0,"quote":""}],
      "reason":"",
      "suggestion":""
    }
  ],
  "overall_comment": "",
  "covered_chunk_index": %d,
  "covered_chunk_count": %d
}
`, buildScoringCorePrompt(), chunk.Index, totalChunks, title, chunk.Content, chunk.Index, totalChunks))
}

func buildMiddleChunkPrompt(title string, chunk scoringChunk, totalChunks int, state *scoringState) string {
	return strings.TrimSpace(fmt.Sprintf(`
%s

当前任务：这是超长文章的第 %d/%d 段。请基于已有暂定评分状态和当前新片段，更新评分状态。

文章标题：
%s

上一轮暂定评分状态 JSON：
%s

当前片段正文：
%s

请输出与上一轮完全相同结构的 JSON，并注意：
1. 维度分是暂定分，允许随着新内容出现而修正。
2. global_summary 需要更新为截至当前片段的累计概括。
3. main_issues 要继续按重要性排序，尽量避免重复空话。
4. overall_comment 要同步更新，继续保持“整体评价 + 写作建议 + 优先修改措施”的合并表达。
5. covered_chunk_index 更新为当前片段序号。
`, buildScoringCorePrompt(), chunk.Index, totalChunks, title, ai_service.MustJSONString(state), chunk.Content))
}

func buildFinalChunkPrompt(title string, chunk scoringChunk, totalChunks int, state *scoringState, headings []string) string {
	return strings.TrimSpace(fmt.Sprintf(`
%s

当前任务：这是超长文章的最后一段，请基于已有暂定评分状态、最后片段和文章小标题信息，输出最终评分结果。

文章标题：
%s

各级小标题：
%s

上一轮暂定评分状态 JSON：
%s

最后片段正文：
%s

请输出以下最终 JSON 结构：
{
  "ai_total_score": 0,
  "total_score": 0,
  "article_type": "从支持列表中选择一个中文文体名",
  "dimensions": [
    {"name":"clarity","score":0,"reason":""},
    {"name":"structure","score":0,"reason":""},
    {"name":"completeness","score":0,"reason":""},
    {"name":"readability","score":0,"reason":""},
    {"name":"persuasiveness","score":0,"reason":""},
    {"name":"language","score":0,"reason":""}
  ],
  "score_level": "较差文章|不合格文章|常规文章|优质文章|精品文章",
  "main_issues": [
    {
      "positions":[{"paragraph":0,"quote":""}],
      "reason":"",
      "suggestion":""
    }
  ],
  "overall_comment": ""
}
注意：article_type 只能从以下列表中选择一个：%s
`, buildScoringCorePrompt(), title, ai_service.MustJSONString(headings), ai_service.MustJSONString(state), chunk.Content, ai_service.MustJSONString(supportedArticleTypes)))
}

func buildFullArticlePrompt(title string, content string, headings []string) string {
	return strings.TrimSpace(fmt.Sprintf(`
%s

当前任务：请对整篇文章进行总体质量评分，并给出结构化写作建议。

文章标题：
%s

各级小标题：
%s

全文正文：
%s

请输出以下 JSON 结构：
{
  "ai_total_score": 0,
  "total_score": 0,
  "article_type": "从支持列表中选择一个中文文体名",
  "dimensions": [
    {"name":"clarity","score":0,"reason":""},
    {"name":"structure","score":0,"reason":""},
    {"name":"completeness","score":0,"reason":""},
    {"name":"readability","score":0,"reason":""},
    {"name":"persuasiveness","score":0,"reason":""},
    {"name":"language","score":0,"reason":""}
  ],
  "main_issues": [
    {
      "positions":[{"paragraph":0,"quote":""}],
      "reason":"",
      "suggestion":""
    }
  ],
  "score_level": "较差文章|不合格文章|常规文章|优质文章|精品文章",
  "overall_comment": ""
}
注意：article_type 只能从以下列表中选择一个：%s
`, buildScoringCorePrompt(), title, ai_service.MustJSONString(headings), content, ai_service.MustJSONString(supportedArticleTypes)))
}
