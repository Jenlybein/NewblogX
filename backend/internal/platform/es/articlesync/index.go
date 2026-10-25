package articlesync

import "strings"

// ResolveIndex 把配置里的索引名收敛成实际使用的名字。
//
// 空值落到 DefaultArticleESIndex。放在本包而不是调用方（cmd/cli）：
// 索引名是"投影长什么样"的一部分，和 mapping / pipeline 属于同一类知识，
// 三处各写一份默认值迟早会不一致。
//
// 注意它不校验名字合法性 —— ES 索引名规则（小写、不含空格等）由 ES 自己拒绝，
// 在这里预校验只会让错误信息更难查。
func ResolveIndex(name string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return DefaultArticleESIndex
	}
	return clean
}
