package ai_metainfo

import (
	"encoding/json"
	"fmt"
	"myblogx/models/ctype"
	"myblogx/utils/markdown"
	"strings"
)

// cleanArticleMetainfoContent 清洗文章正文，去除格式并裁剪输入长度。
func cleanArticleMetainfoContent(content string, maxChars int) string {
	text := markdown.MdToTextParagraph(content)

	if maxChars <= 0 {
		maxChars = 4096
	}

	return markdown.ExtractText(text, maxChars)
}

func normalizeArticleMetainfoReply(raw string, categoryOptions, tagOptions []Metainfos) (*MetainfoResponse, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}

	var payload MetainfoResponse
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("文章元信息结果不是有效 JSON: %w", err)
	}

	result := &MetainfoResponse{
		Title:    payload.Title,
		Abstract: payload.Abstract,
		Category: nil,
	}

	tagMap := make(map[ctype.ID]struct{}, len(tagOptions))
	for _, tag := range tagOptions {
		tagMap[tag.ID] = struct{}{}
	}

	validTags := make([]Metainfos, 0, len(payload.Tags))
	for _, tag := range payload.Tags {
		if _, exists := tagMap[tag.ID]; exists {
			validTags = append(validTags, tag)
		}
	}
	result.Tags = validTags

	if payload.Category != nil {
		for _, category := range categoryOptions {
			if category.ID == payload.Category.ID {
				result.Category = &category
				break
			}
		}
	}

	return result, nil
}
