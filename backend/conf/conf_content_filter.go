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
	// Whitelist 是**词级**白名单：命中这个词就永远不替换文本。
	//
	// 适用场景是"正常用法远多于违规用法"的词（`独立`、`证书`、`验证码`）：
	// 词留在词表里继续参与匹配、命中继续记一条风险，只是不再打星。
	// 代价是这个词的拦截力整体交出去了 —— 真想保留拦截力，用下面的短语白名单。
	Whitelist []string `yaml:"whitelist" json:"whitelist"`
	// WhitelistPhrases 是**短语级**白名单：只豁免这一个具体说法，词本身的其它用法照旧打星。
	//
	// 例：把 `安全套接层` 放进这里，`安全套接层` 不打星，而真正写 `安全套` 仍然打星。
	// 所以要对付"某个正常词恰好包含了敏感词"（`安全套接层`、`现代开发`、`fork 炸弹`、
	// `瑞士军刀`）时，应该用这一份而不是 Whitelist。
	//
	// 引擎侧两者合并成同一张白名单表（都是"区间覆盖"语义），分开纯粹是为了让人在
	// 配置里分得清意图：整词豁免是"放弃这个词"，短语豁免是"只放过这种说法"。
	WhitelistPhrases []string `yaml:"whitelist_phrases" json:"whitelist_phrases"`
	// RetainDays 是命中记录的保留天数（默认 90，范围 1–3650）。
	//
	// 它是**运行时配置**而不是常量：命中记录里存着 ±20 字上下文，保留多久是运营
	// 对"审计价值 vs 数据量"的取舍，不该由一次发版决定。清理任务每天读一次现值，
	// 所以改它不需要重启。
	RetainDays int `yaml:"retain_days" json:"retain_days" min:"1" max:"3650"`
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
