// Package app 的 ES 响应取值层：把 _source 里的任意 JSON 值收敛成强类型。
//
// 从 search.go 拆出来（R6 单文件 800 行）：这一组函数只回答"ES 返回的
// 数字/字符串/时间怎么写进 domain.Item"，与 DSL 编译、请求体组装无关，
// 而后者才是 search.go 的主题。
package app

import (
	"encoding/json"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/search/domain"
	"myblogx/utils/markdown"
)

// ── ES _source 取值（容忍 ES 返回的数字/字符串类型差异） ──

func sourceID(source map[string]any, key string) ctype.ID {
	switch value := source[key].(type) {
	case ctype.ID:
		return value
	case string:
		var id ctype.ID
		if err := id.UnmarshalText([]byte(value)); err != nil {
			return ""
		}
		return id
	default:
		return ""
	}
}

func sourceInt(source map[string]any, key string) int {
	switch value := source[key].(type) {
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
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
		return 0
	default:
		return 0
	}
}

func sourceString(source map[string]any, key string) string {
	value, _ := source[key].(string)
	return value
}

func sourceBool(source map[string]any, key string) bool {
	switch value := source[key].(type) {
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
		if parsed, err := value.Int64(); err == nil {
			return parsed != 0
		}
		return false
	default:
		return false
	}
}

func sourceTime(source map[string]any, key string) time.Time {
	switch value := source[key].(type) {
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

func sourceFloat(source map[string]any, key string) float64 {
	switch value := source[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		if parsed, err := value.Float64(); err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func sourceStringSlice(source map[string]any, key string) []string {
	switch value := source[key].(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func sourceContentParts(source map[string]any, key string) []markdown.ContentPart {
	rawList, ok := source[key].([]any)
	if !ok {
		return nil
	}
	out := make([]markdown.ContentPart, 0, len(rawList))
	for _, raw := range rawList {
		itemMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		part := markdown.ContentPart{
			Level: sourceInt(itemMap, "level"),
			Title: sourceString(itemMap, "title"),
			Path:  sourceStringSlice(itemMap, "path"),
		}
		if part.Title == "" && part.Content == "" && len(part.Path) == 0 && part.Order == 0 && part.Level == 0 {
			continue
		}
		out = append(out, part)
	}
	return out
}

func sourceTags(source map[string]any, key string) []domain.Tag {
	rawList, ok := source[key].([]any)
	if !ok {
		return nil
	}
	out := make([]domain.Tag, 0, len(rawList))
	for _, raw := range rawList {
		itemMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		title := sourceString(itemMap, "title")
		id := sourceID(itemMap, "id")
		if title == "" && id.IsZero() {
			continue
		}
		out = append(out, domain.Tag{ID: id, Title: title})
	}
	return out
}
