package scoring

import "strings"

// 内容形态的硬封顶（ADR-0040）。
//
// 为什么要有这一层：提示词只是"请求"，模型会给出与常识冲突的分——线上真实出现过
// 两行祝福语拿到 51 分并被发布。单句文本在 clarity / language 上确实挑不出毛病，
// 加权之后就被长板抬上及格线。所以**可数的硬事实**（字数、段落数、是否纯礼节）
// 必须在代码里封顶，不能交给模型自觉。
//
// 这一层只认"能机械判定"的事实，不判语义好坏：语义判断留在策略文本
// （skills/scoring/SKILL.md）里，运营可以改；这里的数字要可复现、可测试。

const (
	// shapeShortRunes 是"太短"的字数线：不足这么多字不可能承载一件完整的事。
	shapeShortRunes = 100
	// shapeMinParagraphs 是"不成篇"的段落线：少于三段谈不上结构。
	shapeMinParagraphs = 3
	// shapeGreetingMaxRunes 是"纯礼节文本"的字数上界：礼节话本身就不会长，
	// 留出富余避免把一封真正的说明性邮件误判进来。
	shapeGreetingMaxRunes = 200
	// shapeGreetingHitRatio 是礼节句占比线：整篇大部分句子都在寒暄才算纯礼节，
	// 正文里偶尔说一句"感谢"不该触发。
	shapeGreetingHitRatio = 0.6
)

// 三个封顶值。取这些数字的理由：
//   - 总分 30 = 提示词分档里的"较差文章"上沿，语义上就是"这篇不该发布"；
//   - completeness 30 / 20：信息量维度是这类文本最该被扣的地方，事实上接近 0；
//   - structure 45：两段话谈不上结构，但比"完全没有内容"略高一点；
//   - 总分 40：在"较差 / 不合格"之间，明显低于任何及格线配置，不会被误放行。
const (
	capShortTotal        = 40
	capShortCompleteness = 30
	capShortStructure    = 45

	capGreetingTotal        = 30
	capGreetingCompleteness = 20

	// capBelowLowestDimension 是"总分与维度自洽"的余量：总分不得高出最低维度分
	// 这么多，否则一篇 45/90/90/90/90/90 的文章会拿到 82 分，读者一眼看出矛盾。
	capBelowLowestDimension = 15

	// missingDimensionScore 是模型漏给某个维度时的兜底分。
	//
	// 原来是 50（"保守中间分"）—— 在只有一个总分的年代它无害，但木桶判定下
	// 50 恰好等于默认及格线：**漏给一项就等于白送过关**。取 40 是为了让它
	// 稳定低于任何合理的及格线配置，漏项必须表现为"这一项没达标"，
	// 而不是悄悄算过。
	missingDimensionScore = 40
)

// shapeGreetingWords 是礼节性文本的高频词。
//
// 判据是"整篇大部分句子都命中"，不是"出现任意一个"：真正的文章里写到
// "感谢同事的 review" 不会因此被当成贺词。
var shapeGreetingWords = []string{
	"祝愿", "祝福", "祝", "恭贺", "恭喜", "贺", "生日快乐", "节日快乐",
	"谢谢", "感谢", "多谢", "支持", "加油", "越来越好", "越办越好",
	"留言", "寄语", "问候", "欢迎", "期待", "恭喜发财", "新年", "春节",
}

// contentShape 是一篇文章可机械判定的形态事实。
type contentShape struct {
	// Runes 是正文（去首尾空白）的字符数。
	Runes int
	// Paragraphs 是非空行（自然段）数。
	Paragraphs int
	// Greeting 表示整篇基本是寒暄 / 祝福 / 致谢 / 链接这类礼节文本。
	Greeting bool
}

// analyzeContentShape 计算形态事实。config 与标题不参与判定字数：评的是正文。
func analyzeContentShape(content string) contentShape {
	trimmed := strings.TrimSpace(content)
	shape := contentShape{Runes: len([]rune(trimmed))}

	paragraphs := 0
	for _, line := range strings.Split(trimmed, "\n") {
		if strings.TrimSpace(line) != "" {
			paragraphs++
		}
	}
	shape.Paragraphs = paragraphs

	shape.Greeting = looksLikeGreeting(trimmed, shape.Runes, paragraphs)
	return shape
}

// looksLikeGreeting 判断整篇是否基本是礼节文本。
//
// 三个条件同时成立才算：短、段落少、**大部分句子**都是礼节句。缺一条就按正常
// 文章处理——误判成本不对称：把一篇真文章当贺词封顶，作者会觉得系统坏了。
func looksLikeGreeting(content string, runes int, paragraphs int) bool {
	if runes == 0 || runes > shapeGreetingMaxRunes || paragraphs > shapeMinParagraphs-1 {
		return false
	}

	sentences := splitSentences(content)
	if len(sentences) == 0 {
		return false
	}

	hits := 0
	for _, sentence := range sentences {
		for _, word := range shapeGreetingWords {
			if strings.Contains(sentence, word) {
				hits++
				break
			}
		}
	}
	return float64(hits)/float64(len(sentences)) >= shapeGreetingHitRatio
}

// splitSentences 按中文/英文句末标点与换行切句，过滤空句。
func splitSentences(content string) []string {
	fields := strings.FieldsFunc(content, func(r rune) bool {
		switch r {
		case '。', '！', '？', '!', '?', '；', ';', '\n', '\r':
			return true
		}
		return false
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// applyShapeCaps 把形态封顶落到一次评分结果上，返回是否发生了封顶。
//
// 顺序有意义：先压维度分、再按最低维度压总分，这样"总分 ≤ 最低维度 + 15"这条
// 自洽规则不会把刚被压低的维度又反向抬高。最后再套总分上限。
func applyShapeCaps(resp *ArticleScoreResponse, shape contentShape) bool {
	if resp == nil {
		return false
	}

	capped := false
	completenessCap := 0
	structureCap := 0
	totalCap := 0

	if shape.Greeting {
		completenessCap = capGreetingCompleteness
		totalCap = capGreetingTotal
		capped = true
	} else if shape.Runes < shapeShortRunes || shape.Paragraphs < shapeMinParagraphs {
		completenessCap = capShortCompleteness
		structureCap = capShortStructure
		totalCap = capShortTotal
		capped = true
	}
	if !capped {
		// 形态正常也要守住"总分不高于最低维度 + 15"：这条与桶判定同源，
		// 防止一篇某项很弱的文章靠其余维度加权后拿到高分。
		applyLowestDimensionCeiling(resp)
		return false
	}

	for index := range resp.Dimensions {
		switch strings.ToLower(resp.Dimensions[index].Name) {
		case DimensionCompleteness:
			if completenessCap > 0 {
				resp.Dimensions[index].Score = minInt(resp.Dimensions[index].Score, completenessCap)
			}
		case DimensionStructure:
			if structureCap > 0 {
				resp.Dimensions[index].Score = minInt(resp.Dimensions[index].Score, structureCap)
			}
		}
	}

	// 维度被压低之后总分必须跟着下来，否则会出现"完整度 30、总分 62"的矛盾。
	resp.TotalScore = calculateWeightedScore(resp.Dimensions, resp.ArticleType)
	applyLowestDimensionCeiling(resp)
	if totalCap > 0 {
		resp.TotalScore = minInt(resp.TotalScore, totalCap)
	}
	resp.ScoreLevel = scoreLevel(resp.TotalScore)
	return true
}

// applyLowestDimensionCeiling 让总分不超过「最低维度分 + 余量」。
func applyLowestDimensionCeiling(resp *ArticleScoreResponse) {
	if len(resp.Dimensions) == 0 {
		return
	}
	lowest := 100
	for _, item := range resp.Dimensions {
		lowest = minInt(lowest, clampScore(item.Score))
	}
	resp.TotalScore = minInt(resp.TotalScore, clampScore(lowest+capBelowLowestDimension))
}
