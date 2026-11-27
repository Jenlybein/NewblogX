package domain

// MaxArticleTags 是一篇文章**默认**最多可以带的标签数。
//
// # 为什么要有上限
//
// 标签在读者站是浏览入口（标签云、按标签搜索），标签越多越没有指向性 ——
// 一篇文章挂 20 个标签，等于告诉读者"它什么都算"，反而搜不出来。上限也让
// "标签"保持语义：作者每选一个都要判断它是不是这篇文章真正的主题。
//
// 5 这个数字是产品拍的：覆盖"语言 + 框架 + 领域 + 类型 + 场景"这种典型组合
// 仍有余量，但多到能塞下所有热门词就会失去区分度。
//
// # 它只是默认值，不是唯一上限
//
// 真正生效的上限来自站点运行时配置（conf.Site.Article.MaxTagsPerArticle，
// 后台可改，见 NormalizeTagLimit）。本常量只剩三个用途：配置缺省 / 越界时回落、
// site_default_settings.yaml 的默认值、以及"哪些值是合法上限"的边界定义。
//
// 名字保留 MaxArticleTags 是为了避免全仓 churn：消费点仍然需要"默认上限"这个
// 语义，只是不该再把 5 当成不可变的业务事实抄进文案 —— apperr 的文案因此改成
// 中性说法（数字由前端按站点配置显示），见 CodeArticleTagLimitExceeded。
const MaxArticleTags = 5

// MaxArticleTagLimit 是上限可配之后允许的最大值（20）。
//
// 为什么还要给一个上界：上限是作者侧的产品约束，不是"想填多少都行"的自由参数。
// 填 1000 与不设上限等价（等于关掉了这条规则），而越界的值如果按字面执行，
// 管理员一次误填就会让前面所有的区分度设计失效。越界一律回落默认值。
const MaxArticleTagLimit = 20

// MinArticleTagLimit 是上限可配之后允许的最小值（1）。
//
// 下限是 1 而不是 0：0 表示"一个标签都不许选"，那不是"更严格的上限"，而是把
// 标签功能整个关掉；这种意图应该用站点开关表达，不该借道一个数字字段。
// 0 / 负数因此按"没配置好"处理，回落到默认上限。
const MinArticleTagLimit = 1

// NormalizeTagLimit 把站点配置里的标签上限收敛到合法区间，返回本次实际生效的上限。
//
// 归一化只做这一处：创建与编辑两条入口都经 ValidateArticleTags 拿到同一份
// 收敛后的值，HTTP 层与用例层不再各判一次"配置是否合法"。这样"填成 0 会怎样"
// 只有一个答案（回落到默认 5），而不是每个消费点一个答案。
func NormalizeTagLimit(limit int) int {
	if limit < MinArticleTagLimit || limit > MaxArticleTagLimit {
		return MaxArticleTags
	}
	return limit
}

// NormalizeTagIDs 去重并丢掉空 ID，返回可直接用于校验与写库的集合。
//
// 为什么要去重：同一标签传两次在语义上就是"一个标签"。不去重会有两处连锁反应 ——
// 计数把一个标签算成两个（5 个标签里有一个重复就被判超限），而
// repository.FindMany 对重复 ID 返回去重后的结果，"请求 6 个只回来 5 个"
// 又会掉进"标签不存在或已停用"那条检查，报出一个与真实原因无关的错误。
//
// 顺序保持不变（首次出现的位置），因为标签回显的顺序由调用方决定。
func NormalizeTagIDs(ids []TagID) []TagID {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[TagID]struct{}, len(ids))
	out := make([]TagID, 0, len(ids))
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// CheckArticleTagLimit 校验（归一化之后的）标签数量是否在生效上限内。
//
// 只吃归一化后的集合：调用方要么先调 NormalizeTagIDs，要么用
// ValidateArticleTags 一次做完。分开是因为计数与归一化是两件不同的事，
// 而"先实现了一个却忘了另一个"正是这次要修的那类缺陷。
//
// limit 是站点配置传来的原始值（0 表示调用方没填），这里先归一化再比较：
// 上界合法性只由 NormalizeTagLimit 一处定义。
func CheckArticleTagLimit(tags []TagID, limit int) error {
	if len(tags) > NormalizeTagLimit(limit) {
		return ErrTooManyTags
	}
	return nil
}

// ValidateArticleTags 归一化并校验数量，返回可以继续往下走的标签集合。
//
// 创建与编辑走同一条路径（app 层的 loadUsableTags），这样"存草稿 / 直接发布 /
// 编辑重投"三个入口的上限不会各说各话 —— 每个入口都由 adapter 现读站点配置、
// 把同一个上限经 cmd 传进来，而不是各自写死一个数字。
func ValidateArticleTags(ids []TagID, limit int) ([]TagID, error) {
	normalized := NormalizeTagIDs(ids)
	if err := CheckArticleTagLimit(normalized, limit); err != nil {
		return nil, err
	}
	return normalized, nil
}
