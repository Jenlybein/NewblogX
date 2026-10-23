package ai_diagnose

import (
	"errors"
	"fmt"
	"myblogx/conf"
	"myblogx/service/ai_service"
	"strings"
)

const selectionTooShortMsg = "内容过短，建议选中完整句或完整段落"

func DiagnoseSelectedText(aiConf conf.AI, req DiagnoseRequest) (*DiagnoseResponse, error) {
	normalized, err := normalizeDiagnoseRequest(req)
	if err != nil {
		return nil, err
	}

	reply, err := ai_service.Chat(aiConf, []ai_service.Message{
		{
			Role:    "system",
			Content: buildDiagnosePrompt(),
		},
		{
			Role:    "user",
			Content: buildDiagnoseUserContent(normalized),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("选中文本诊断失败: %w", err)
	}

	var response DiagnoseResponse
	if err = ai_service.UnmarshalJSONBlock(reply, &response); err != nil {
		return nil, fmt.Errorf("选中文本诊断结果不是有效 JSON: %w", err)
	}
	return normalizeDiagnoseResponse(&response), nil
}

func normalizeDiagnoseRequest(req DiagnoseRequest) (DiagnoseRequest, error) {
	req.SelectionText = strings.TrimSpace(req.SelectionText)
	req.ArticleTitle = strings.TrimSpace(req.ArticleTitle)
	req.PrefixText = strings.TrimSpace(req.PrefixText)
	req.SuffixText = strings.TrimSpace(req.SuffixText)

	if req.ArticleTitle == "" {
		return DiagnoseRequest{}, errors.New("文章标题不能为空")
	}
	if ai_service.RuneLen(req.ArticleTitle) > articleTitleMaxChars {
		return DiagnoseRequest{}, fmt.Errorf("文章标题不能超过 %d 字", articleTitleMaxChars)
	}
	if req.SelectionText == "" {
		return DiagnoseRequest{}, errors.New("选中内容不能为空")
	}
	if ai_service.RuneLen(req.SelectionText) < selectionTextMinChars {
		return DiagnoseRequest{}, errors.New(selectionTooShortMsg)
	}
	if ai_service.RuneLen(req.SelectionText) > selectionTextMaxChars {
		return DiagnoseRequest{}, fmt.Errorf("选中内容不能超过 %d 字", selectionTextMaxChars)
	}

	req.PrefixText = ai_service.LastRunes(req.PrefixText, contextTextMaxChars)
	req.SuffixText = ai_service.FirstRunes(req.SuffixText, contextTextMaxChars)
	return req, nil
}

func normalizeDiagnoseResponse(resp *DiagnoseResponse) *DiagnoseResponse {
	if resp == nil {
		resp = &DiagnoseResponse{}
	}

	resp.Summary = strings.TrimSpace(resp.Summary)
	if resp.Summary == "" {
		resp.Summary = "该片段存在值得改进的表达问题，建议优先处理影响理解的部分。"
	}

	result := make([]DiagnoseIssue, 0, len(resp.Issues))
	for _, item := range resp.Issues {
		item.Type = normalizeIssueType(item.Type)
		item.Severity = normalizeSeverity(item.Severity)
		item.Reason = strings.TrimSpace(item.Reason)
		item.Evidence = strings.TrimSpace(item.Evidence)
		item.Suggestion = strings.TrimSpace(item.Suggestion)
		if item.Reason == "" && item.Suggestion == "" {
			continue
		}
		result = append(result, item)
		if len(result) >= diagnoseMaxIssues {
			break
		}
	}
	resp.Issues = result
	return resp
}

func normalizeIssueType(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, "-", "")

	switch normalized {
	case "可读性", "清晰度", "表达清晰度", "易读性", "readability", "clarity":
		return "可读性"
	case "逻辑", "逻辑性", "逻辑问题", "logic", "logical":
		return "逻辑"
	case "完整度", "完整性", "信息完整度", "内容完整度", "completeness", "complete":
		return "完整度"
	case "结构", "结构性", "篇章结构", "structure", "structural":
		return "结构"
	case "重复", "冗余", "啰嗦", "赘述", "repetition", "redundancy", "duplicate":
		return "重复"
	case "语言", "语言规范", "语言表达", "语法", "language", "grammar":
		return "语言"
	case "语气", "口吻", "风格", "tone", "voice", "style":
		return "语气"
	default:
		return "可读性"
	}
}

func normalizeSeverity(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, "-", "")

	switch normalized {
	case "低", "low":
		return "低"
	case "中", "中等", "medium", "moderate", "mid":
		return "中"
	case "高", "严重", "high", "critical":
		return "高"
	default:
		return "中"
	}
}
