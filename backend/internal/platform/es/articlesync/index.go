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

// articleIndexName 是**写入**时使用的索引名。
//
// # 为什么需要它
//
// 读侧（搜索、AI 检索）和 CLI（建索引/全量重建）一直取配置里的 `es.index`，
// 而增量写入曾经硬编码 `DefaultArticleESIndex`。两边默认值恰好都等于 "article_index"，
// 所以从来没暴露过：一旦运维把 `BLOGX_ES_INDEX` 改成别的名字，就会变成
// "建在新索引、搜在新索引、增量写进旧索引" —— 而且不报错，
// 要么写进一个没人读的自动建出来的索引，要么每次写都失败刷死信。
//
// 现在写入也跟随配置：装配层（app.Bootstrap）在启动时注入一次。
//
// # 为什么用包级变量而不是加参数
//
// 本包对外有 12 个入口，真正用到索引名的只有 2 个内部函数。
// 把 index 一路穿过 12 个签名和十几个调用点，是给代码加噪音换不来任何安全。
//
// # 约定（必须遵守）
//
// **只允许在装配阶段写一次，之后只读。** 写入发生在 canal 回调与其他 worker 里，
// 启动完成后再改它等于制造数据竞争。默认值保证单测不需要注入。
var articleIndexName = DefaultArticleESIndex

// SetIndexName 注入索引名（空值忽略，保持默认）。只应在装配阶段调用一次。
func SetIndexName(name string) {
	if clean := strings.TrimSpace(name); clean != "" {
		articleIndexName = clean
	}
}

// IndexName 返回当前写入使用的索引名。
func IndexName() string {
	return articleIndexName
}
