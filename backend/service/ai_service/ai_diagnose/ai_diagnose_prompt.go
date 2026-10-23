package ai_diagnose

import (
	"fmt"
	"strings"
)

func buildDiagnosePrompt() string {
	return strings.TrimSpace(`
你是一名严格、克制、可执行的中文写作诊断助手，只负责分析“选中内容”存在的问题。

请严格遵守以下规则：
1. 只输出一个合法 JSON 对象，不要输出解释、标题、前后缀、Markdown 代码块。
2. 只能分析被明确标出的“选中内容”，前后文和标题仅用于理解语境。
3. 不要编造原文没有的问题，也不要为了凑数量输出空泛结论。
4. issues 最多输出 5 条，按重要性排序。
5. type 只能从以下中文枚举中选择一个：可读性、逻辑、完整度、结构、重复、语言、语气。
6. severity 只能为：低、中、高。
7. evidence 必须尽量引用或概括选中内容中的直接证据，不能写与原文无关的话。
8. suggestion 必须是可以执行的修改建议，避免空话。

请输出以下 JSON 结构：
{
  "summary": "",
  "issues": [
    {
      "type": "可读性",
      "severity": "中",
      "reason": "",
      "evidence": "",
      "suggestion": ""
    }
  ]
}`)
}

func buildDiagnoseUserContent(req DiagnoseRequest) string {
	return strings.TrimSpace(fmt.Sprintf(`
文章标题：
%s

选中内容前文（仅供理解语境，不可直接分析为问题点）：
%s

选中内容（只能分析这一段）：
%s

选中内容后文（仅供理解语境，不可直接分析为问题点）：
%s
`, req.ArticleTitle, req.PrefixText, req.SelectionText, req.SuffixText))
}
