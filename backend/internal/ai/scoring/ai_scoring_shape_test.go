package scoring

import (
	"strings"
	"testing"
)

// 内容形态硬封顶（ADR-0023）。
//
// 这一层要挡的是一类具体的事故：两行祝福语在"清晰 / 没有语病"上确实挑不出毛病，
// 于是六维加权后被长板抬到及格线以上并被发布。提示词只能"请求"模型别这么干，
// 代码必须"保证"它不会发生——所以这里断言的是封顶后的数字，不是模型行为。

// longBody 是"形态正常"的正文：够长、够多段，不该被任何封顶命中。
func longBody() string {
	return strings.Join([]string{
		"最近在整理项目的接口层，发现几个反复出现的问题，记下来备查。",
		"第一个问题是返回值形状不一致：同一个模块里有的端点包了一层 envelope，有的直接返回数组。调用方每接一个端点都要先看清它长什么样，出错的地方往往就在这层包装上。",
		"第二个问题是错误码的粒度：所有失败都返回同一个 code，前端只能靠 message 里的中文分支，一旦文案改了就全崩。改成按场景分码之后，前端的判断才稳定下来。",
		"第三个问题是分页参数：有的用 page/limit，有的用 offset/count。统一成前者之后，前端的列表组件终于可以复用同一份翻页逻辑。",
		"最后留一条经验：接口的边界要在服务端收紧，前端的校验只是提示，不是保障。",
	}, "\n")
}

// dimensionScore 取某个维度的分。
func dimensionScore(resp *ArticleScoreResponse, name string) int {
	for _, item := range resp.Dimensions {
		if item.Name == name {
			return item.Score
		}
	}
	return -1
}

// response 造一份"模型给了高分"的结果，用来观察封顶是否生效。
//
// 总分按后端口径先算出来（模型给的 ai_total_score 会被丢弃重算），否则封顶逻辑
// 拿到的就是一个还没算分的 0。
func response(scores map[string]int) *ArticleScoreResponse {
	resp := &ArticleScoreResponse{
		AITotalScore: 88,
		ArticleType:  ArticleTypeExpository,
	}
	for _, name := range dimensionOrder {
		score, ok := scores[name]
		if !ok {
			score = 88
		}
		resp.Dimensions = append(resp.Dimensions, ArticleScoreDimension{Name: name, Score: score})
	}
	resp.TotalScore = calculateWeightedScore(resp.Dimensions, resp.ArticleType)
	return resp
}

func TestShapeNormalContentIsNotCapped(t *testing.T) {
	shape := analyzeContentShape(longBody())
	if shape.Greeting {
		t.Fatal("正常长文不该被判成礼节文本")
	}
	if shape.Runes < shapeShortRunes || shape.Paragraphs < shapeMinParagraphs {
		t.Fatalf("正常长文不该命中形态下限: runes=%d paragraphs=%d", shape.Runes, shape.Paragraphs)
	}

	resp := response(map[string]int{
		DimensionClarity: 82, DimensionStructure: 80, DimensionCompleteness: 78,
		DimensionReadability: 84, DimensionPersuasiveness: 76, DimensionLanguage: 86,
	})
	if capped := applyShapeCaps(resp, shape); capped {
		t.Fatal("形态正常的内容不该被形态封顶")
	}
	if resp.TotalScore == 0 || resp.TotalScore > 82 {
		t.Fatalf("总分应是加权结果，实际 %d", resp.TotalScore)
	}
}

// 这条对应线上真实事故：两行祝福语拿到 51 分。
func TestShapeShortTwoLineGreetingIsCappedHard(t *testing.T) {
	body := "受邀留言，祝愿网站越来越好！\n希望越办越好，加油。"
	shape := analyzeContentShape(body)
	if !shape.Greeting {
		t.Fatalf("两行祝福应被判成礼节文本，实际 runes=%d paragraphs=%d", shape.Runes, shape.Paragraphs)
	}

	resp := response(map[string]int{
		DimensionClarity: 78, DimensionStructure: 62, DimensionCompleteness: 55,
		DimensionReadability: 74, DimensionPersuasiveness: 58, DimensionLanguage: 82,
	})
	applyShapeCaps(resp, shape)

	if resp.TotalScore > capGreetingTotal {
		t.Fatalf("纯礼节文本总分应封顶 %d，实际 %d", capGreetingTotal, resp.TotalScore)
	}
	if got := dimensionScore(resp, DimensionCompleteness); got > capGreetingCompleteness {
		t.Fatalf("完整度应封顶 %d，实际 %d", capGreetingCompleteness, got)
	}
}

// 短但不寒暄（例如一条两行的技术笔记）：仍要压，但比纯礼节宽松。
func TestShapeShortNonGreetingIsCapped(t *testing.T) {
	body := "今天调了一晚上连接池，最后发现是超时时间配错了。\n记一下，免得下次再踩。"
	shape := analyzeContentShape(body)
	if shape.Greeting {
		t.Fatal("技术笔记不该被判成礼节文本")
	}

	resp := response(map[string]int{
		DimensionClarity: 76, DimensionStructure: 70, DimensionCompleteness: 68,
		DimensionReadability: 74, DimensionPersuasiveness: 60, DimensionLanguage: 78,
	})
	applyShapeCaps(resp, shape)

	if resp.TotalScore > capShortTotal {
		t.Fatalf("过短的正文总分应封顶 %d，实际 %d", capShortTotal, resp.TotalScore)
	}
	if got := dimensionScore(resp, DimensionCompleteness); got > capShortCompleteness {
		t.Fatalf("完整度应封顶 %d，实际 %d", capShortCompleteness, got)
	}
	if got := dimensionScore(resp, DimensionStructure); got > capShortStructure {
		t.Fatalf("结构性应封顶 %d，实际 %d", capShortStructure, got)
	}
}

// 长但只有一段（一段到底）：段落数不足同样算"不成篇"。
func TestShapeSingleParagraphIsCapped(t *testing.T) {
	body := strings.Repeat("这段文字很长但完全没有分段，读起来是一堵墙。", 8)
	shape := analyzeContentShape(body)
	if shape.Paragraphs != 1 {
		t.Fatalf("应该数出 1 个自然段，实际 %d", shape.Paragraphs)
	}
	if shape.Runes < shapeShortRunes {
		t.Fatal("这条用例要的是「够长但没分段」，字数不该低于下限")
	}

	resp := response(map[string]int{
		DimensionClarity: 74, DimensionStructure: 72, DimensionCompleteness: 70,
		DimensionReadability: 68, DimensionPersuasiveness: 62, DimensionLanguage: 76,
	})
	applyShapeCaps(resp, shape)
	if resp.TotalScore > capShortTotal {
		t.Fatalf("单段正文总分应封顶 %d，实际 %d", capShortTotal, resp.TotalScore)
	}
}

// 长文里出现"感谢"之类礼节词不算礼节文本：判据是整篇占比，不是关键词。
func TestShapeGreetingNeedsMostSentencesToBeGreetings(t *testing.T) {
	body := longBody() + "\n最后感谢同事帮忙 review。"
	if analyzeContentShape(body).Greeting {
		t.Fatal("长文里的致谢不该把它判成礼节文本")
	}
}

// 总分与维度自洽：不允许用长板把总分抬出"最低维度 + 15"之外。
func TestShapeTotalCannotExceedLowestDimensionCeiling(t *testing.T) {
	shape := analyzeContentShape(longBody())
	resp := response(map[string]int{
		DimensionClarity: 95, DimensionStructure: 92, DimensionCompleteness: 40, // 只有一个短板
		DimensionReadability: 90, DimensionPersuasiveness: 88, DimensionLanguage: 94,
	})
	applyShapeCaps(resp, shape)

	lowest := dimensionScore(resp, DimensionCompleteness)
	if resp.TotalScore > lowest+capBelowLowestDimension {
		t.Fatalf("总分 %d 不该超过最低维度 %d + %d", resp.TotalScore, lowest, capBelowLowestDimension)
	}
}

// 策略文本被注入到提示词里；没绑定时不能凭空多出一段"本站在意的口径"。
func TestPromptInjectsRuntimePolicy(t *testing.T) {
	withPolicy := buildFullArticlePrompt("木桶：六项都要过线。", "标题", longBody(), nil)
	if !strings.Contains(withPolicy, "木桶：六项都要过线。") {
		t.Fatal("策略正文应被注入提示词")
	}
	if !strings.Contains(withPolicy, "以**更严**的那条为准") {
		t.Fatal("策略与内置规则冲突时要说清以哪条为准")
	}

	withoutPolicy := buildFullArticlePrompt("", "标题", longBody(), nil)
	if strings.Contains(withoutPolicy, "本站在意的评分口径") {
		t.Fatal("没有绑定策略文本时不该出现该段落")
	}
	// 但内置的形态下限必须始终在场：摘掉 skill 不能等于关掉底线。
	if !strings.Contains(withoutPolicy, "completeness 不得高于 30") {
		t.Fatal("内置的形态下限不能依赖策略文本")
	}
}

func TestAnalyzeContentShapeCountsParagraphs(t *testing.T) {
	shape := analyzeContentShape("第一段\n\n第二段\n   \n第三段\n")
	if shape.Paragraphs != 3 {
		t.Fatalf("空白行不算段落，期望 3 实际 %d", shape.Paragraphs)
	}
}

// 模型漏给某个维度时的兜底分必须**低于**默认及格线，否则"少给一项"等于
// 白送过关——木桶判定下 50 分正好压在及格线上，这是旧的 50 分兜底的漏洞。
func TestMissingDimensionFallsBelowPassLine(t *testing.T) {
	if missingDimensionScore >= 50 {
		t.Fatalf("缺失维度兜底分应低于默认及格线 50，实际 %d", missingDimensionScore)
	}

	resp := normalizeFinalResponse(&ArticleScoreResponse{
		ArticleType: ArticleTypeExpository,
		Dimensions: []ArticleScoreDimension{
			{Name: DimensionClarity, Score: 90},
			{Name: DimensionStructure, Score: 90},
			// completeness / readability / persuasiveness / language 全部缺失
		},
	}, "")

	// 补齐六个维度，缺的那几个必须是兜底分而不是 50。
	if len(resp.Dimensions) != len(dimensionOrder) {
		t.Fatalf("应补齐到六个维度，实际 %d", len(resp.Dimensions))
	}
	if got := dimensionScore(resp, DimensionCompleteness); got != missingDimensionScore {
		t.Fatalf("缺失维度应取兜底分 %d，实际 %d", missingDimensionScore, got)
	}
}
