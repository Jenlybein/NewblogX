// 运行时配置的默认值合并。
//
// 为什么需要它：数据库里存的是"整段 JSON"，而默认值在 site_default_settings.yaml。
// 两者必须能合并，且必须按**字段**粒度合并 —— 否则一旦某段 JSON 是旧版本
// （少几个后来新增的字段），整段就会退化成零值：日志级别变空、请求日志开关变
// false、上传白名单变空数组，都是"改了没反应"甚至"功能静默消失"的形状。
//
// 合并语义刻意做成最朴素的两种：
//   - 对象（JSON object）递归合并；
//   - 标量与数组整体替换（数组不做逐元素合并，否则删元素永远删不掉）。
package config

import (
	"encoding/json"
	"strings"
)

// mergeJSON 把 payload 深度覆盖到 base 上，返回合并后的对象。
//
// base 可以是任意结构体，payload 是数据库里存的 JSON 字符串。
// payload 为空或为 "null" 时直接返回 base。
func mergeJSON(base any, payload string) (map[string]any, error) {
	baseMap, err := toJSONMap(base)
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(payload)
	if trimmed == "" || trimmed == "null" {
		return baseMap, nil
	}

	var override map[string]any
	if err := json.Unmarshal([]byte(trimmed), &override); err != nil {
		return nil, err
	}
	return mergeMaps(baseMap, override), nil
}

// toJSONMap 借助 JSON 往返把任意结构体转成 map[string]any。
//
// 不手写反射：结构体的 json tag 就是唯一口径，绕过去反而会出现
// "yaml 名与 json 名不一致时合并结果对不上"的问题。
func toJSONMap(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{}, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// mergeMaps 递归合并两个 JSON 对象。
//
// 三条规则：
//  1. override 里值为 null 的键**不覆盖** —— null 在 JSON 里常用来表示
//     "没填"，让它把一个有默认值的字段清空没有意义。
//  2. 两边都是对象时递归。
//  3. 其余情况（标量、数组、类型不匹配）一律以 override 为准。
func mergeMaps(base, override map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		if v == nil {
			continue
		}
		baseValue, exists := out[k]
		baseMap, baseIsMap := baseValue.(map[string]any)
		overrideMap, overrideIsMap := v.(map[string]any)
		if exists && baseIsMap && overrideIsMap {
			out[k] = mergeMaps(baseMap, overrideMap)
			continue
		}
		out[k] = v
	}
	return out
}
