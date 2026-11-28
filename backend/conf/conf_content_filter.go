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
	// AutoDispose 是两档命中的处置口径（见 ContentFilterAutoDispose）。
	//
	// 它与上面那份"整段替换"的清单放在一起，是因为它也是词表策略的一部分：
	// 换一份词表的人要能顺手说清"命中了怎么处理"，而不是去别处找开关。
	// 老库（以及还没跟上的后台请求体）没有这个键时回落 mask —— 空串即 mask，
	// 默认行为因此与改造前逐字节一致。
	AutoDispose ContentFilterAutoDispose `yaml:"auto_dispose" json:"auto_dispose"`
}

// ContentFilterAutoDispose 是「关键词命中处理」的三档口径。
//
// 取值 release / draft / reject，**空串与旧值 mask 都按 release 处理**：老库那段
// JSON 里没有这个键、旧版后台请求体里只有 mask/reject，都必须能读能存 —— 把它们
// 当成非法值拒绝保存，会让"打开设置页什么都没改就点保存"直接失败。
//
// 三档语义（**串行链路的第①环**，命中且非 release 时后续质量评分与 AI 合规判定
// 全部短路，不再花钱）：
//   - release：命中→替换成等长的 *→照常走后面的链路（改造前的 mask 就是它）；
//   - draft  ：命中→落草稿 / 评论拒收，作者改完可重投；提交配额**不归还**
//     （作者自己能改的失败不退还额度，与旧 reject 同一条理由）；
//   - reject ：命中→文章落「已拒绝」且不可改 / 评论拒收；读者看不到。
//
// 档位差异见 app 层的落点实现；**观察词没有 reject**（watch 是误报代价低的词，
// 拒发它们等于把"加微信"三个字写成违规内容）。
//
// 生效范围：
//   - block_action 只作用于**文章面**（标题 / 摘要 / 正文）；评论、昵称 / 简介、
//     私信、合集仍是"只打星"；
//   - watch_action 本期只有 mask 一种行为，reject 是预留取值：watch 档是推广话术
//     这类误报代价低的词，拒发它们等于把"加微信"三个字写成违规内容。
//
// reject 是 opt-in：不配就是 mask，与改造前完全一致（命中的地方照样打星，
// 稿件照常保存 —— 只是不会再"命中即拒"）。
// 命中处置的三个取值（`ContentFilterAutoDispose` 的取值域）。
//
// 定义在 conf 包：这是**配置**的取值域，写入侧（校验）、读取侧（链路分流）、
// 数据迁移（旧值 mask → release）与文档都要引用同一份常量。散在用例包里会让
// "改一个值忘一处"变成静默的字符串比较失败。
const (
	// ContentFilterActionRelease 只把命中替换成等长星号，文本照常入库并继续后续链路。
	ContentFilterActionRelease = "release"
	// ContentFilterActionDraft 命中落草稿（文章），提交配额不归还。
	ContentFilterActionDraft = "draft"
	// ContentFilterActionReject 文章落「已拒绝」且不可改 / 评论拒收（见 CommentAction）。
	ContentFilterActionReject = "reject"
)

// ContentFilterActionLegacyMask 是改造前"只打星"的取值（与 release 同义）。
//
// 只在两处出现：读面兼容映射与数据迁移。新写入一律是 release。
const ContentFilterActionLegacyMask = "mask"

type ContentFilterAutoDispose struct {
	// BlockAction 是 block（拦截词）命中且**真被打星**时的处置：release（默认）/ draft / reject。
	//
	// 判据是"真被打星"：白名单豁免的命中文本原样返回，不该触发处置。
	BlockAction string `yaml:"block_action" json:"block_action" enums:"release,draft,reject"`
	// CommentAction 是**评论**命中拦截词时的处置：release（打星照发，默认）/ reject（拒收）。
	//
	// 为什么评论单独一档而不是跟着 BlockAction：文章能"退回草稿、作者改完重投"，
	// 评论没有草稿态、也没有申诉通道（票 400 之后更没有待审队列）。两者共用一个
	// 取值会把运营逼到"要么文章只能放行、要么评论全拒"的二选一 —— 评论命中
	// 一条推广话术就整条不发，代价远大于打星。
	//
	// 观察词命中在评论里**永远只打星**：没有 reject 这一档（理由同上，误报代价低）。
	CommentAction string `yaml:"comment_action" json:"comment_action" enums:"release,reject"`

	// WatchAction 是 watch（观察词）命中的处置：release（默认）/ draft。
	//
	// **没有 reject**：观察档是误报代价低的词（推广话术、口头语），拒发它们等于把
	// "加微信"三个字写成违规内容。保存侧把 reject 当非法值拒绝（见 site/app 的归一化）。
	WatchAction string `yaml:"watch_action" json:"watch_action" enums:"release,draft"`
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
