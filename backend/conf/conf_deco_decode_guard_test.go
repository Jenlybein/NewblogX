package conf

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestCharmDecode_字段集合与Charm完全一致 是 `UnmarshalJSON` 镜像结构的守卫（票 #78）。
//
// 为什么必须有一条这样的测试：`charmDecode` 是 `Charm` 的 JSON 解码中间层
// （指针字段用来区分「键没给 / null」与「显式 0」，见票 #77）。它是 `Charm`
// 之外**第二份字段清单** —— 加参数时漏同步不会编译失败，而是那个键在请求体里
// 被静默忽略：encoding/json 不认识它、`UnmarshalJSON` 也不会落回默认值/显式值，
// 管理员改完保存后"没反应"，而现有测试全都看不到它。
//
// 判据是双向的：
//   - json tag 集合必须完全相等（少一个 / 多一个都失败）；
//   - 每个 tag 对应的字段类型必须正好是 `Charm` 字段的指针（`*bool` ↔ `bool`、
//     `*float64` ↔ `float64`）—— 解码层靠指针区分「没给」与「给了零值」，
//     写成值类型会让票 #77 的 null 回落整条失效。
func TestCharmDecode_字段集合与Charm完全一致(t *testing.T) {
	charmFields := charmJSONFields(t, reflect.TypeOf(Charm{}))
	decodeFields := charmJSONFields(t, reflect.TypeOf(charmDecode{}))

	if len(charmFields) != len(decodeFields) {
		t.Fatalf(
			"charmDecode 的 JSON 字段数（%d）与 Charm（%d）不一致：漏同步会让某个键在请求体里被静默忽略",
			len(decodeFields), len(charmFields),
		)
	}

	for key, charmType := range charmFields {
		decodeType, ok := decodeFields[key]
		if !ok {
			t.Errorf("charmDecode 缺少 json 键 %q（Charm 里的类型是 %s）：UnmarshalJSON 会静默忽略它", key, charmType)
			continue
		}
		if want := reflect.PointerTo(charmType); decodeType != want {
			t.Errorf(
				"charmDecode 里 %q 的类型是 %s，应为 %s（解码层用指针区分「键没给 / null」与「显式零值」）",
				key, decodeType, want,
			)
		}
	}

	for key := range decodeFields {
		if _, ok := charmFields[key]; !ok {
			t.Errorf("charmDecode 多出 json 键 %q（Charm 里没有）：它永远解不出值", key)
		}
	}
}

// TestCharmUnmarshalJSON_每个JSON键都会被落回Charm 补上反射守卫抓不到的另一半。
//
// 结构体字段对齐了，但 `UnmarshalJSON` 的赋值列表漏了一行，同样会静默丢键；
// 反射守卫只看类型定义，看不见赋值。这里按 `Charm` 的 json tag **动态**构造
// 一份"每个键都给了非零值"的请求体，解码后逐字段比对 —— 漏赋值的那一项会留在
// 零值上（0 / false），与本用例给的 1.5 / true 对不上。
//
// 用反射而不是手写一份键值对：加参数时这个用例自动覆盖新键，不需要人记得来补。
func TestCharmUnmarshalJSON_每个JSON键都会被落回Charm(t *testing.T) {
	charmType := reflect.TypeOf(Charm{})
	values := make(map[string]any, charmType.NumField())
	expected := reflect.New(charmType).Elem()

	for index := 0; index < charmType.NumField(); index++ {
		field := charmType.Field(index)
		key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		switch field.Type.Kind() {
		case reflect.Bool:
			// 与默认值 true 相反：漏赋值会留在 false 上，能被发现。
			values[key] = false
			expected.Field(index).SetBool(false)
		case reflect.Float64:
			// 1.5 与任何默认值都不同，且在 Normalize 之前不会被改动。
			values[key] = 1.5
			expected.Field(index).SetFloat(1.5)
		default:
			t.Fatalf("Charm.%s 是 %s：本守卫只认识 bool / float64，请扩展它", field.Name, field.Type)
		}
	}

	raw, err := json.Marshal(map[string]any{"charm": values})
	if err != nil {
		t.Fatalf("构造请求体失败: %v", err)
	}
	var deco Deco
	if err := json.Unmarshal(raw, &deco); err != nil {
		t.Fatalf("解码失败: %v", err)
	}

	if !reflect.DeepEqual(deco.Charm, expected.Interface()) {
		t.Fatalf("有 JSON 键没有落回 Charm（UnmarshalJSON 漏了一行赋值）：\n请求体 %s\n期望 %+v\n实际 %+v",
			raw, expected.Interface(), deco.Charm)
	}
}

// charmJSONFields 把结构体的 json tag 映射到字段类型；没有 tag / `-` 的字段直接失败。
func charmJSONFields(t *testing.T, structType reflect.Type) map[string]reflect.Type {
	t.Helper()
	fields := make(map[string]reflect.Type, structType.NumField())
	for index := 0; index < structType.NumField(); index++ {
		field := structType.Field(index)
		tag, ok := field.Tag.Lookup("json")
		if !ok {
			t.Fatalf("%s.%s 没有 json tag：请求体里的键名无从确定", structType.Name(), field.Name)
		}
		key, _, _ := strings.Cut(tag, ",")
		if key == "" || key == "-" {
			t.Fatalf("%s.%s 的 json tag %q 不可用作键", structType.Name(), field.Name, tag)
		}
		if _, exists := fields[key]; exists {
			t.Fatalf("%s 里 json 键 %q 重复", structType.Name(), key)
		}
		fields[key] = field.Type
	}
	return fields
}
