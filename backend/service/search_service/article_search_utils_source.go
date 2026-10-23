package search_service

import (
	"encoding/json"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/utils/markdown"
	"time"
)

// sourceIDValue 从 ES _source 中提取雪花 ID 字段。
func sourceIDValue(sourceMap map[string]any, key string) ctype.ID {
	switch value := sourceMap[key].(type) {
	case ctype.ID:
		return value
	case string:
		var id ctype.ID
		if err := id.UnmarshalText([]byte(value)); err != nil {
			return 0
		}
		return id
	default:
		return 0
	}
}

// sourceIntValue 从 ES _source 中提取 int 字段。
func sourceIntValue(sourceMap map[string]any, key string) int {
	switch value := sourceMap[key].(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		intValue, err := value.Int64()
		if err != nil {
			return 0
		}
		return int(intValue)
	default:
		return 0
	}
}

// sourceStringValue 从 ES _source 中提取 string 字段。
func sourceStringValue(sourceMap map[string]any, key string) string {
	value, _ := sourceMap[key].(string)
	return value
}

// sourceBoolValue 从 ES _source 中提取 bool 字段。
func sourceBoolValue(sourceMap map[string]any, key string) bool {
	switch value := sourceMap[key].(type) {
	case bool:
		return value
	case int:
		return value != 0
	case int8:
		return value != 0
	case int16:
		return value != 0
	case int32:
		return value != 0
	case int64:
		return value != 0
	case uint:
		return value != 0
	case uint8:
		return value != 0
	case uint16:
		return value != 0
	case uint32:
		return value != 0
	case uint64:
		return value != 0
	case float32:
		return value != 0
	case float64:
		return value != 0
	case json.Number:
		intValue, err := value.Int64()
		if err != nil {
			return false
		}
		return intValue != 0
	default:
		return false
	}
}

// sourceTimeValue 从 ES _source 中提取时间字段。
func sourceTimeValue(sourceMap map[string]any, key string) time.Time {
	switch value := sourceMap[key].(type) {
	case time.Time:
		return value
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// sourceStringSliceValue 从 ES _source 中提取字符串切片字段。
func sourceStringSliceValue(sourceMap map[string]any, key string) []string {
	switch value := sourceMap[key].(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				continue
			}
			result = append(result, text)
		}
		return result
	default:
		return nil
	}
}

func sourceFloatValue(sourceMap map[string]any, key string) float64 {
	switch value := sourceMap[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		floatValue, err := value.Float64()
		if err != nil {
			return 0
		}
		return floatValue
	default:
		return 0
	}
}

// sourceContentPartsValue 从 ES _source 中提取正文分段字段。
func sourceContentPartsValue(sourceMap map[string]any, key string) []markdown.ContentPart {
	rawList, ok := sourceMap[key].([]any)
	if !ok {
		return nil
	}

	result := make([]markdown.ContentPart, 0, len(rawList))
	for _, rawItem := range rawList {
		itemMap, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}

		part := markdown.ContentPart{
			Level: sourceIntValue(itemMap, "level"),
			Title: sourceStringValue(itemMap, "title"),
			Path:  sourceStringSliceValue(itemMap, "path"),
		}
		if part.Title == "" && part.Content == "" && len(part.Path) == 0 && part.Order == 0 && part.Level == 0 {
			continue
		}
		result = append(result, part)
	}

	return result
}

// sourceTagTitlesValue 从 ES _source 中提取 tags 里的标签标题列表。
func sourceTagTitlesValue(sourceMap map[string]any, key string) []string {
	rawList, ok := sourceMap[key].([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(rawList))
	for _, rawItem := range rawList {
		itemMap, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		title, _ := itemMap["title"].(string)
		if title == "" {
			continue
		}
		result = append(result, title)
	}
	return result
}

func sourceTagItemsValue(sourceMap map[string]any, key string) []SearchTag {
	rawList, ok := sourceMap[key].([]any)
	if !ok {
		return nil
	}

	result := make([]SearchTag, 0, len(rawList))
	for _, rawItem := range rawList {
		itemMap, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		title := sourceStringValue(itemMap, "title")
		id := sourceIDValue(itemMap, "id")
		if title == "" && id == 0 {
			continue
		}
		result = append(result, SearchTag{
			ID:    id,
			Title: title,
		})
	}
	return result
}

// sourceArticleStatusValue 从 ES _source 中提取文章状态字段。
func sourceArticleStatusValue(sourceMap map[string]any, key string) enum.ArticleStatus {
	return enum.ArticleStatus(sourceIntValue(sourceMap, key))
}

func sourceArticleVisibilityStatusValue(sourceMap map[string]any, key string) enum.ArticleVisibilityStatus {
	return enum.ArticleVisibilityStatus(sourceStringValue(sourceMap, key))
}
