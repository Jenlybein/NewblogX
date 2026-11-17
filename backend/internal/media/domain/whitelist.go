package domain

import "strings"

// EffectiveWhitelist 归一化上传白名单。
//
// 「对象存储」段是"能不能传、传多大、什么格式"的唯一事实源（ADR-0034）：
// 白名单曾经属于独立的「上传策略」段，那个段已经退场（票 418），
// 所以这里不再有"跨段回落"的分支 —— 只有一个来源，也就没有优先级可讲。
//
// 归一化三件事：小写、去空白、去重，并保持首次出现的顺序（后台展示与它一致）。
// 空结果表示"没配"，由调用方决定是拒绝上传还是回落到默认值：
// 本函数不替调用方猜测默认值，否则"没配"会被伪装成"配了默认值"。
func EffectiveWhitelist(storage []string) []string {
	seen := make(map[string]struct{}, len(storage))
	out := make([]string, 0, len(storage))
	for _, raw := range storage {
		item := strings.ToLower(strings.TrimSpace(raw))
		// 后缀统一不带点：库里存 ".png" 与 "png" 两种写法时后端只认后一种，
		// 这里一并纠正，避免"后台填对了却传不上去"。
		item = strings.TrimPrefix(item, ".")
		if item == "" {
			continue
		}
		if _, dup := seen[item]; dup {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
