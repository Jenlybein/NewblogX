package app

import (
	"context"
	"strings"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
)

// 本文件是文章与合集两个文本面的内容过滤（票 397 / ADR-0032 决策 5/6）。
//
// 口径只有一条：**落库前**把命中词替换成等长 `*`，替换后的文本才是入库、进 ES、
// 进通知的唯一版本。所以顺序固定为「过滤替换 → 质量闸门 / 同步风险扫描（读替换后
// 文本）→ 落库」，闸门那一步不能挪到过滤之前 —— 否则评分读的是作者原文，被替换
// 掉的部分等于没被审过。
//
// 端口由本包定义、装配层实现（app/article_wiring.go 的两个 bridge），面枚举
// （article / collection）在 bridge 里钉死：文章模块不必认识另外四个面。

// FilterText 是一段要在落库前过滤的文本。
//
// 端口刻意不带"文本面"参数：article 模块只知道"这是我的文章标题 / 正文"，
// 把它归到哪个面属于内容过滤模块。
type FilterText struct {
	// UserID 是这段文本的作者，命中记录按它统计（风险页的"命中用户 Top"）。
	UserID domain.UserID
	// ContentID 是这段文本落库后的内容 ID：命中记录靠它指回真实内容
	// （后续票的"删除内容"处置要用）。新建时这个 ID 在过滤前先生成。
	// 类型是 ctype.ID 而不是 domain.ArticleID：合集面传的是合集 ID，
	// 两者本来就是同一个底层类型（domain 里的 ID 全是 ctype.ID 的别名）。
	ContentID ctype.ID
	Text      string
}

// FilteredText 是过滤结果。
type FilteredText struct {
	// Masked 是替换后的文本（与输入等长，按 rune 计）。
	Masked string
	// FilteredCount 是实际被替换的处数（只用于"已过滤 N 处"的提示）。
	FilteredCount int
	// BlockTierMasked 表示"这段文本里确实有 block 档命中被替换成了星号"。
	//
	// 实现方只报事实，是否因此拒发由调用方决定：本站默认两档都只打星，
	// "命中即拒"是按站点配置开启的 opt-in 行为。白名单覆盖的 block 命中
	// 不算（文本没变），所以这个字段为 true 依然意味着"真的改过这段文本"。
	BlockTierMasked bool
	// WatchTierMasked 表示"这段文本里确实有 watch 档命中被替换成了星号"。
	//
	// 两档各有一套处置档位（拦截词可到 reject，观察词只到 draft），所以调用方
	// 必须分得清命中的是哪一档 —— 只报"有命中"会让"观察词退草稿、拦截词放行"
	// 这种配置无法表达。
	WatchTierMasked bool
}

// ArticleTextFilterer 在全站唯一的内容过滤层里替换**文章**文本，并记一条关键词风险。
//
// 错误语义：实现方在"命中记录写失败"时应当自己吞掉并记日志（fail-open），
// 返回 error 只留给装配级异常；即便如此，调用方拿到 error 也按原文继续，
// 不阻断作者发布（ADR-0032 决策 11）。
//
// 实现方只报事实（命中记录写了没有、文本改没改），是否因此拒发由调用方决定：
// 内容过滤层不认识"草稿 / 投稿 / 编辑"这些动作，也不该替文章模块做发布决策。
type ArticleTextFilterer interface {
	FilterArticleText(ctx context.Context, in FilterText) (FilteredText, error)
}

// CollectionTextFilterer 在全站唯一的内容过滤层里替换**合集**文本。
//
// 与 ArticleTextFilterer 分成两个接口而不是一个带 surface 参数的接口：
// 两个方法名不同，"把文章 bridge 接给合集"就变成编译错误，而不是一条
// 悄悄写错面的命中记录。
type CollectionTextFilterer interface {
	FilterCollectionText(ctx context.Context, in FilterText) (FilteredText, error)
}

// articleTexts 是一篇文章里需要过滤的三个文本字段。
type articleTexts struct {
	Title    string
	Abstract string
	Content  string
}

// filterArticle 依次过滤标题、摘要、正文，返回替换后的三份文本与总处数。
//
// 三个字段各过滤一次、而不是拼成一段再切回来：引擎给出的上下文片段是按单段
// 文本算的，拼接会让风险页看到"跨越标题与正文"的假上下文；等长替换虽然能切回
// 原文偏移，但没必要为此把风险页的可读性搭进去。
//
// "有没有 block 档命中真被打星"按三个字段用 `||` 聚合：拒发是整篇文章的结论，
// 标题命中与正文命中没有区别。
func (s *Service) filterArticle(
	ctx context.Context, userID domain.UserID, articleID ctype.ID, in articleTexts,
) (articleTexts, int, hitFacts) {
	title, titleCount, titleHit := s.filterArticleText(ctx, userID, articleID, in.Title)
	abstract, abstractCount, abstractHit := s.filterArticleText(ctx, userID, articleID, in.Abstract)
	content, contentCount, contentHit := s.filterArticleText(ctx, userID, articleID, in.Content)
	return articleTexts{Title: title, Abstract: abstract, Content: content},
		titleCount + abstractCount + contentCount,
		hitFacts{
			Block: titleHit.Block || abstractHit.Block || contentHit.Block,
			Watch: titleHit.Watch || abstractHit.Watch || contentHit.Watch,
		}
}

// filterArticleText 过滤一段文章文本；未接入过滤器时原样返回。
//
// 更新路径只过滤"本次真的要改"的字段，所以它单独暴露出来，而不是非要凑成
// articleTexts 三件套（nil 指针变成空串会把"不改"写成"清空"）。
func (s *Service) filterArticleText(
	ctx context.Context, userID domain.UserID, articleID ctype.ID, text string,
) (string, int, hitFacts) {
	if s.filter == nil {
		return text, 0, hitFacts{}
	}
	return s.filterOne(ctx, userID, articleID, text, s.filter.FilterArticleText)
}

// filterCollection 过滤合集的标题与描述，返回替换后的两份文本与总处数。
//
// 合集面本期不参与"命中即拒"（只有文章面按配置拒发），所以这里只丢掉
// filterOne 报回的 block 档事实。
func (s *Service) filterCollection(
	ctx context.Context, userID domain.UserID, collectionID ctype.ID, title, description string,
) (string, string, int) {
	if s.collectionFilter == nil {
		return title, description, 0
	}
	maskedTitle, titleCount, _ := s.filterOne(ctx, userID, collectionID, title,
		s.collectionFilter.FilterCollectionText)
	maskedDescription, descriptionCount, _ := s.filterOne(ctx, userID, collectionID, description,
		s.collectionFilter.FilterCollectionText)
	return maskedTitle, maskedDescription, titleCount + descriptionCount
}

// filterOne 过滤一段文本，失败时退回原文。
//
// 四条失败方向，全部按"照常发布"处理（ADR-0032 决策 11）：
//   - 过滤器没装配 → 原文；
//   - 过滤器返回错误（装配级异常，命中记录写失败由过滤模块自己吞掉）→ 原文；
//   - 过滤器返回空文本（不该发生，但不能把作者的话弄丢）→ 原文；
//   - 空串不送进过滤器：没有文本就没有命中，省一次 AC 自动机扫描。
//
// 后三条一律把 block 档事实置为 false：调用方拿到的是"没被过滤过的原文"，
// 此时按"有命中、该拒发"处理，等于用一次装配故障把作者的文章拒掉。
func (s *Service) filterOne(
	ctx context.Context, userID domain.UserID, contentID ctype.ID, text string,
	apply func(context.Context, FilterText) (FilteredText, error),
) (string, int, hitFacts) {
	// apply 恒非 nil：调用方（filterArticleText / filterCollectionText）已经各判过一次
	// "端口没装配"，方法值也永远不是 nil。这里只判"这段文本是不是空的"。
	if strings.TrimSpace(text) == "" {
		return text, 0, hitFacts{}
	}
	got, err := apply(ctx, FilterText{UserID: userID, ContentID: contentID, Text: text})
	if err != nil {
		return text, 0, hitFacts{}
	}
	facts := hitFacts{Block: got.BlockTierMasked, Watch: got.WatchTierMasked}
	if got.Masked == "" {
		return text, got.FilteredCount, facts
	}
	return got.Masked, got.FilteredCount, facts
}

// hitFacts 是"这次过滤里哪几档词真的命中了"（只含真被打星的那些）。
//
// 两档分开报的原因：它们的处置档位是**各自独立配置**的（拦截词可到 reject，
// 观察词只到 draft）。把两档合成一个 bool 之后，「观察词退草稿、拦截词放行」
// 这种合法配置就再也表达不出来。
type hitFacts struct {
	Block bool
	Watch bool
}

// FilterDisposition 是站点当前的「关键词命中处理」口径（串行链路的第①环配置）。
//
// 由装配层注入读取函数而不是在用例层读全局配置：article 模块不认识内容过滤段，
// 它只需要知道"命中之后我该做什么"。取值语义见 conf.ContentFilterAutoDispose。
type FilterDisposition struct {
	// Block 是拦截词命中的处置：release / draft / reject。
	Block string
	// Watch 是观察词命中的处置：release / draft。
	Watch string
}

// 命中处置的三个档位（与 conf 段的三个值同名，用例层不外传字符串字面量之外的判断）。
const (
	// HitRelease 命中只打星，照常继续后面的链路。
	HitRelease = "release"
	// HitDraft 命中落草稿，作者改完可重投；配额不归还。
	HitDraft = "draft"
	// HitReject 命中落已拒绝且不可改。
	HitReject = "reject"
)

// dispositionAfterFilter 把"命中了哪几档"与"站点配置的处置档位"合成本次的处置结论。
//
// 两档都命中时取**更重**的那一档（reject > draft > release）：一条内容不该因为
// 观察词配置更宽松就绕过拦截词的处置。
//
// 配置读不到（没注入 / 空串 / 不认识的取值）一律按 release：默认行为必须与改造前
// 逐字节一致，"配置读不到"不能变成把所有人的文章拒掉（ADR-0032 决策 11 的同一条口径）。
func (s *Service) dispositionAfterFilter(facts hitFacts) string {
	if s.disposition == nil {
		return HitRelease
	}
	conf := s.disposition()
	best := HitRelease
	if facts.Block {
		best = maxDisposition(best, conf.Block)
	}
	if facts.Watch {
		best = maxDisposition(best, conf.Watch)
	}
	return best
}

// maxDisposition 按"重度"取较大者；不认识的取值一律当 release。
func maxDisposition(a, b string) string {
	return severityDisposition(a, b)
}

// severityDisposition 返回两者中更重的一档。
func severityDisposition(a, b string) string {
	if dispositionRank(b) > dispositionRank(a) {
		return b
	}
	return a
}

// dispositionRank 是档位的重度排序：reject 3 > draft 2 > release 1 > 其他 0。
func dispositionRank(d string) int {
	switch d {
	case HitReject:
		return 3
	case HitDraft:
		return 2
	case HitRelease:
		return 1
	default:
		return 0
	}
}

// WithFilterDisposition 注入命中处置档位的现读函数（2026-11-28）。
//
// 不注入时按 release（改造前的行为）：装配缺失只可能发生在测试里，生产装配
// 在 backend/app/article_wiring.go 必然给出。
func (s *Service) WithFilterDisposition(read func() FilterDisposition) *Service {
	s.disposition = read
	return s
}
