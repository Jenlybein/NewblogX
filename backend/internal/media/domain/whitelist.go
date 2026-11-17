package domain

import "strings"

// EffectiveWhitelist 合并上传白名单。
//
// 为什么需要"合并"而不是直接读一处（票 410 → 票 418）：
// 白名单原来住在「上传策略」（upload）段，票 410 把它并进「对象存储」（storage）
// 段，但为了让迁移期的行为与迁移前**逐字节一致**，旧段在退场前仍然可读。
// 于是读取口径定成一条有明确优先级的规则：
//
//  1. storage 段的白名单非空 → 以它为准（新事实源生效）；
//  2. storage 段为空 → 回落 upload 段（老部署没补新字段时行为不变）；
//  3. 两者都空 → 返回空列表，由调用方按"白名单为空即全拒"处理。
//
// 返回的列表会做归一化（去空白、转小写、去空项、保持首次出现顺序去重），
// 这样上层比较后缀时不需要再各自处理大小写与空格 —— 归一化只在这里做一次。
func EffectiveWhitelist(storage, upload []string) []string {
	if normalized := normalizeWhitelist(storage); len(normalized) > 0 {
		return normalized
	}
	return normalizeWhitelist(upload)
}

// normalizeWhitelist 归一化一份白名单。
//
// 去重键用小写而不是原写法：后缀在 HTTP 请求里的大小写不受控（.JPG 很常见），
// 而"看起来一样的两条"留在列表里只会让后台难以排查。
func normalizeWhitelist(list []string) []string {
	out := make([]string, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	for _, item := range list {
		item = strings.ToLower(strings.TrimSpace(item))
		item = strings.TrimPrefix(item, ".")
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
