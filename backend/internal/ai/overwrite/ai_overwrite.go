package overwrite

import (
	"context"
	"errors"
	"fmt"
	"myblogx/conf"
	"myblogx/internal/ai"
	"myblogx/internal/apperr"
	"strings"
)

const selectionTooShortMsg = "内容过短，建议选中完整句或完整段落"

// RewriteContentStream 对选中内容执行改写，并直接返回 AI 原始 token 流。
func RewriteContentStream(ctx context.Context, aiConf conf.AI, req RewriteRequest) (chan string, chan error, error) {
	normalized, err := normalizeRewriteRequest(req)
	if err != nil {
		return nil, nil, err
	}

	msgList := []ai.Message{
		{
			Role:    "system",
			Content: buildRewritePrompt(normalized.Mode, normalized.TargetStyle),
		},
		{
			Role:    "user",
			Content: buildRewriteUserContent(normalized),
		},
	}

	contentChan, errChan := ai.ChatStream(ctx, aiConf, msgList)
	return contentChan, errChan, nil
}

func normalizeRewriteRequest(req RewriteRequest) (RewriteRequest, error) {
	req.Mode = strings.TrimSpace(req.Mode)
	req.SelectionText = strings.TrimSpace(req.SelectionText)
	req.ArticleTitle = strings.TrimSpace(req.ArticleTitle)
	req.TargetStyle = strings.TrimSpace(req.TargetStyle)
	req.PrefixText = strings.TrimSpace(req.PrefixText)
	req.SuffixText = strings.TrimSpace(req.SuffixText)

	switch req.Mode {
	case ModePolish, ModeGrammarFix, ModeStyleTransform:
	default:
		return RewriteRequest{}, apperr.New(apperr.CodeAIModeUnsupported)
	}

	if req.ArticleTitle == "" {
		return RewriteRequest{}, apperr.New(apperr.CodeAITitleRequired)
	}
	if ai.RuneLen(req.ArticleTitle) > articleTitleMaxChars {
		return RewriteRequest{}, fmt.Errorf("文章标题不能超过 %d 字", articleTitleMaxChars)
	}

	if req.SelectionText == "" {
		return RewriteRequest{}, apperr.New(apperr.CodeAISelectionEmpty)
	}
	if ai.RuneLen(req.SelectionText) < selectionTextMinChars {
		return RewriteRequest{}, errors.New(selectionTooShortMsg)
	}
	if ai.RuneLen(req.SelectionText) > selectionTextMaxChars {
		return RewriteRequest{}, fmt.Errorf("选中内容不能超过 %d 字", selectionTextMaxChars)
	}

	if req.Mode == ModeStyleTransform {
		if req.TargetStyle == "" {
			return RewriteRequest{}, apperr.New(apperr.CodeAIStyleRequired)
		}
		if ai.RuneLen(req.TargetStyle) > targetStyleMaxChars {
			return RewriteRequest{}, fmt.Errorf("目标风格描述不能超过 %d 字", targetStyleMaxChars)
		}
	} else {
		req.TargetStyle = ""
	}

	req.PrefixText = ai.LastRunes(req.PrefixText, contextTextMaxChars)
	req.SuffixText = ai.FirstRunes(req.SuffixText, contextTextMaxChars)
	return req, nil
}
