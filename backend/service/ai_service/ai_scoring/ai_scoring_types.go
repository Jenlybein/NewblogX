package ai_scoring

import "strings"

const (
	articleScoringDirectMaxChars = 100000
	articleScoringChunkMaxChars  = 80000
	articleScoringMaxIssues      = 9
	articleScoringMinChars       = 20
)

const (
	ArticleTypeEssay       = "散文"
	ArticleTypeNovel       = "小说"
	ArticleTypePoetry      = "诗歌"
	ArticleTypeDrama       = "剧本"
	ArticleTypeFairyTale   = "童话"
	ArticleTypeLegend      = "传说"
	ArticleTypeNarrative   = "记叙文"
	ArticleTypeDiary       = "日记"
	ArticleTypeNews        = "新闻"
	ArticleTypeExpository  = "说明文"
	ArticleTypeTutorial    = "教程"
	ArticleTypeSciencePop  = "科普"
	ArticleTypeReport      = "报告"
	ArticleTypeEmail       = "邮件"
	ArticleTypeNotice      = "通知"
	ArticleTypeSpeech      = "发言稿"
	ArticleTypeArgument    = "议论文"
	ArticleTypeReview      = "评论"
	ArticleTypeCopywriting = "文案"
	ArticleTypeWebNovel    = "网文"
)

const (
	DimensionClarity        = "clarity"        // 清晰度
	DimensionStructure      = "structure"      // 结构性
	DimensionCompleteness   = "completeness"   // 信息完整度
	DimensionReadability    = "readability"    // 可读性
	DimensionPersuasiveness = "persuasiveness" // 说服力
	DimensionLanguage       = "language"       // 语言规范度
)

const (
	ExplanationClarity        = "表达是否清晰易懂、核心观点是否突出，无表达混乱、重复啰嗦及语句不当问题"
	ExplanationStructure      = "章节安排合理，逻辑推进顺畅，有没有无关内容穿插堆砌"
	ExplanationCompleteness   = "论点明确，解释及例子准确，能有效支撑核心观点，无关键信息缺失"
	ExplanationReadability    = "句子长度与术语密度适中，节奏流畅、句式多样，无语病及没有无关内容，无反复内容"
	ExplanationPersuasiveness = "论证扎实严谨，引用准确，逻辑闭环，具备较强说服力"
	ExplanationLanguage       = "语法正确、无语病，表达精准规范，无用词不当问题"
)

var dimensionOrder = []string{
	DimensionClarity,
	DimensionStructure,
	DimensionCompleteness,
	DimensionReadability,
	DimensionPersuasiveness,
	DimensionLanguage,
}

type ArticleScoreRequest struct {
	Title   string
	Content string
}

type ArticleScoreResponse struct {
	AITotalScore   int                     `json:"ai_total_score"`
	TotalScore     int                     `json:"total_score"`
	ScoreLevel     string                  `json:"score_level"`
	ArticleType    string                  `json:"article_type"`
	Dimensions     []ArticleScoreDimension `json:"dimensions"`
	MainIssues     []ArticleScoreIssue     `json:"main_issues"`
	OverallComment string                  `json:"overall_comment"`
}

type ArticleScoreDimension struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Reason string `json:"reason"`
}

type ArticleScoreIssue struct {
	Positions  []ArticleScorePosition `json:"positions"`
	Reason     string                 `json:"reason"`
	Suggestion string                 `json:"suggestion"`
}

type ArticleScorePosition struct {
	Paragraph int    `json:"paragraph"`
	Quote     string `json:"quote"`
}

type scoringState struct {
	ArticleType           string                  `json:"article_type"`
	ProvisionalDimensions []scoringStateDimension `json:"provisional_dimensions"`
	ChunkSummary          string                  `json:"chunk_summary"`
	GlobalSummary         string                  `json:"global_summary"`
	MainIssues            []ArticleScoreIssue     `json:"main_issues"`
	OverallComment        string                  `json:"overall_comment"`
	CoveredChunkIndex     int                     `json:"covered_chunk_index"`
	CoveredChunkCount     int                     `json:"covered_chunk_count"`
}

type scoringStateDimension struct {
	Name     string `json:"name"`
	Score    int    `json:"score"`
	Evidence string `json:"evidence"`
}

type scoringChunk struct {
	Index   int
	Content string
}

type articleParagraph struct {
	Number int
	Text   string
}

var supportedArticleTypes = []string{
	ArticleTypeEssay,
	ArticleTypeNovel,
	ArticleTypePoetry,
	ArticleTypeDrama,
	ArticleTypeFairyTale,
	ArticleTypeLegend,
	ArticleTypeNarrative,
	ArticleTypeDiary,
	ArticleTypeNews,
	ArticleTypeExpository,
	ArticleTypeTutorial,
	ArticleTypeSciencePop,
	ArticleTypeReport,
	ArticleTypeEmail,
	ArticleTypeNotice,
	ArticleTypeSpeech,
	ArticleTypeArgument,
	ArticleTypeReview,
	ArticleTypeCopywriting,
	ArticleTypeWebNovel,
}

func normalizeArticleType(articleType string) string {
	articleType = strings.TrimSpace(articleType)
	for _, item := range supportedArticleTypes {
		if articleType == item {
			return item
		}
	}
	return ArticleTypeExpository
}

func normalizeArticleTypeWithFallback(articleType string, fallback string) string {
	if normalized := normalizeArticleType(articleType); normalized != ArticleTypeExpository || strings.TrimSpace(articleType) == ArticleTypeExpository {
		return normalized
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		return normalizeArticleType(fallback)
	}
	return ArticleTypeExpository
}
