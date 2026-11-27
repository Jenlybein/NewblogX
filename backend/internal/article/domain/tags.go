package domain

// MaxArticleTags 是一篇文章最多可以带的标签数。
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
// # 与 apperr 文案的关系
//
// 中文文案里写死了这个数字（apperr 是最底层包，不能反向 import domain）。
// 两边靠 TestTagLimitMessageMatchesConstant 盯着，改这里忘了改文案会红。
const MaxArticleTags = 5

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

// CheckArticleTagLimit 校验（归一化之后的）标签数量是否在上限内。
//
// 只吃归一化后的集合：调用方要么先调 NormalizeTagIDs，要么用
// ValidateArticleTags 一次做完。分开是因为计数与归一化是两件不同的事，
// 而"先实现了一个却忘了另一个"正是这次要修的那类缺陷。
func CheckArticleTagLimit(tags []TagID) error {
	if len(tags) > MaxArticleTags {
		return ErrTooManyTags
	}
	return nil
}

// ValidateArticleTags 归一化并校验数量，返回可以继续往下走的标签集合。
//
// 创建与编辑走同一条路径（app 层的 loadUsableTags），这样"存草稿 / 直接发布 /
// 编辑重投"三个入口的上限不会各说各话。
func ValidateArticleTags(ids []TagID) ([]TagID, error) {
	normalized := NormalizeTagIDs(ids)
	if err := CheckArticleTagLimit(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}
