package logsafe

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"time"
)

// SlogAttrToField 将 slog 属性转换为适合结构化 JSON 日志的字段。
func SlogAttrToField(attr slog.Attr) (string, any, bool) {
	if attr.Key == "" {
		return "", nil, false
	}
	return attr.Key, sanitizeSlogValue(attr.Value.Resolve()), true
}

func sanitizeSlogValue(value slog.Value) any {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindBool:
		return value.Bool()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindGroup:
		group := make(map[string]any)
		for _, attr := range value.Group() {
			key, fieldValue, ok := SlogAttrToField(attr)
			if ok {
				group[key] = fieldValue
			}
		}
		return group
	case slog.KindAny:
		return SanitizeValue(value.Any())
	default:
		return value.String()
	}
}

// SanitizeValue 将任意值转换成 JSON 友好的字段，避免函数等不可序列化对象导致写日志失败。
func SanitizeValue(value any) any {
	if value == nil {
		return nil
	}

	switch typed := value.(type) {
	case error:
		return typed.Error()
	case fmt.Stringer:
		return typed.String()
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	case time.Duration:
		return typed.String()
	case []byte:
		return string(typed)
	case json.RawMessage:
		return string(typed)
	}

	rv := reflect.ValueOf(value)
	return sanitizeReflectValue(rv)
}

func sanitizeReflectValue(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}

	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return sanitizeReflectValue(value.Elem())
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		return value.Float()
	case reflect.String:
		return value.String()
	case reflect.Slice, reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return string(value.Bytes())
		}
		items := make([]any, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			items = append(items, sanitizeReflectValue(value.Index(i)))
		}
		return items
	case reflect.Map:
		items := make(map[string]any, value.Len())
		for _, key := range value.MapKeys() {
			items[fmt.Sprint(sanitizeReflectValue(key))] = sanitizeReflectValue(value.MapIndex(key))
		}
		return items
	case reflect.Func, reflect.Chan, reflect.UnsafePointer, reflect.Complex64, reflect.Complex128:
		return value.Type().String()
	default:
		if value.CanInterface() {
			typed := value.Interface()
			if raw, err := json.Marshal(typed); err == nil {
				var decoded any
				if err = json.Unmarshal(raw, &decoded); err == nil {
					return decoded
				}
				return string(raw)
			}
			return fmt.Sprintf("%T", typed)
		}
		return value.Type().String()
	}
}
