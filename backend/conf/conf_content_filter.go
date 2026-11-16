package conf

// ContentFilter 是第 11 个运行时配置段：全站唯一的词表与白名单（ADR-0032）。
//
// 它**刻意不放进 site 段**：site 段有匿名可读的公开端点（GET /api/site/site），
// 词表一旦混进去就等于让任何人一次请求把整套词表拿走 —— 绕过过滤的成本降到零。
// 本段只有后台具名端点：GET /site/admin/content_filter 与 PUT /site/content_filter。
//
// 两个清单都是整段替换语义（与 site.article.moderation 一致）：删掉一个词必须真的
// 删掉，不能因为默认值里还留着而"删了没反应"（合并语义见 platform/config/merge.go）。
type ContentFilter struct {
	// Words 是词表。匹配前会做归一化（全角折叠、拉丁小写、零宽字符当分隔符），
	// 单字词不入表（误报压过收益），档位与类目决定风险页的标识与筛选。
	Words []ContentFilterWord `yaml:"words" json:"words"`
	// Whitelist 是白名单：词或短语。命中白名单仍会记录一条命中（用于统计误伤），
	// 但不替换文本。
	Whitelist []string `yaml:"whitelist" json:"whitelist"`
}

// ContentFilterWord 是词表里的一条。
type ContentFilterWord struct {
	// Word 是词条原文。不做归一化存储：后台要按运营填的写法展示与编辑。
	Word string `yaml:"word" json:"word"`
	// Tier 是档位：block（拦截词）/ watch（观察词），两档都替换，颜色不同。
	Tier string `yaml:"tier" json:"tier" enums:"block,watch"`
	// Category 是类目，固定七类，用于词云筛选与风险页归类。
	Category string `yaml:"category" json:"category" enums:"drugs,gambling,fraud,porn,fake_docs,politics,violence"`
}
