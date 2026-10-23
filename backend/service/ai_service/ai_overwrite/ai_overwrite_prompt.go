package ai_overwrite

import (
	"fmt"
	"strings"
)

func buildRewritePrompt(mode string, targetStyle string) string {
	action := "内容润色"
	modeRule := "在不改变原意的前提下，优化表达流畅度、句式自然度和局部结构。"
	switch mode {
	case ModeGrammarFix:
		action = "语法改正"
		modeRule = "只修正语法、病句、标点、错别字和明显表达错误，不要改变原有风格与观点。"
	case ModeStyleTransform:
		action = "风格转换"
		modeRule = fmt.Sprintf("在保留原始信息、观点和结论的前提下，将语气和句式改写为“%s”风格。", targetStyle)
	}

	return strings.TrimSpace(fmt.Sprintf(`
你是一名严格可靠的中文写作改写助手，当前任务是：%s。

请严格遵守以下规则：
1. 只输出最终改写后的正文，不要输出任何解释、标题、前缀、后缀、编号、引号或 Markdown 代码块。
2. 不新增原文没有的事实，不改变人名、时间、数字、地点、结论，不乱补专业知识。
3. 只允许在表达、语法、结构、语气上优化，不允许擅自扩写主题或改变作者立场。
4. 如果原文已经足够合适，也只能输出轻微调整后的正文，不能额外解释“无需修改”。
5. 你会看到标题与选区前后文，它们仅用于理解语境；你只能改写被明确标出的“选中内容”。
6. 输出内容必须可直接替换原文选区。

本次模式补充要求：
%s
`, action, modeRule))
}

func buildRewriteUserContent(req RewriteRequest) string {
	return strings.TrimSpace(fmt.Sprintf(`
文章标题：
%s

选中内容前文（仅供理解语境，不可改写）：
%s

选中内容（只能改写这一段）：
%s

选中内容后文（仅供理解语境，不可改写）：
%s
`, req.ArticleTitle, req.PrefixText, req.SelectionText, req.SuffixText))
}
