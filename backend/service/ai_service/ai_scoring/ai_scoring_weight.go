package ai_scoring

type scoringWeight struct {
	Clarity        int
	Structure      int
	Completeness   int
	Readability    int
	Persuasiveness int
	Language       int
}

func (w scoringWeight) toMap() map[string]int {
	return map[string]int{
		DimensionClarity:        w.Clarity,
		DimensionStructure:      w.Structure,
		DimensionCompleteness:   w.Completeness,
		DimensionReadability:    w.Readability,
		DimensionPersuasiveness: w.Persuasiveness,
		DimensionLanguage:       w.Language,
	}
}

var articleTypeWeights = map[string]scoringWeight{
	ArticleTypeEssay: {
		Clarity: 18, Structure: 16, Completeness: 10, Readability: 24, Persuasiveness: 12, Language: 20,
	},
	ArticleTypeNovel: {
		Clarity: 14, Structure: 18, Completeness: 10, Readability: 24, Persuasiveness: 14, Language: 20,
	},
	ArticleTypePoetry: {
		Clarity: 10, Structure: 12, Completeness: 8, Readability: 30, Persuasiveness: 16, Language: 24,
	},
	ArticleTypeDrama: {
		Clarity: 16, Structure: 22, Completeness: 12, Readability: 18, Persuasiveness: 14, Language: 18,
	},
	ArticleTypeFairyTale: {
		Clarity: 18, Structure: 18, Completeness: 12, Readability: 22, Persuasiveness: 12, Language: 18,
	},
	ArticleTypeLegend: {
		Clarity: 16, Structure: 18, Completeness: 12, Readability: 22, Persuasiveness: 14, Language: 18,
	},
	ArticleTypeNarrative: {
		Clarity: 20, Structure: 20, Completeness: 14, Readability: 20, Persuasiveness: 10, Language: 16,
	},
	ArticleTypeDiary: {
		Clarity: 18, Structure: 14, Completeness: 12, Readability: 22, Persuasiveness: 10, Language: 24,
	},
	ArticleTypeNews: {
		Clarity: 24, Structure: 24, Completeness: 20, Readability: 12, Persuasiveness: 6, Language: 14,
	},
	ArticleTypeExpository: {
		Clarity: 24, Structure: 22, Completeness: 22, Readability: 14, Persuasiveness: 6, Language: 12,
	},
	ArticleTypeTutorial: {
		Clarity: 20, Structure: 20, Completeness: 24, Readability: 16, Persuasiveness: 8, Language: 12,
	},
	ArticleTypeSciencePop: {
		Clarity: 20, Structure: 20, Completeness: 24, Readability: 16, Persuasiveness: 8, Language: 12,
	},
	ArticleTypeReport: {
		Clarity: 22, Structure: 24, Completeness: 24, Readability: 10, Persuasiveness: 8, Language: 12,
	},
	ArticleTypeEmail: {
		Clarity: 24, Structure: 18, Completeness: 18, Readability: 16, Persuasiveness: 8, Language: 16,
	},
	ArticleTypeNotice: {
		Clarity: 26, Structure: 22, Completeness: 18, Readability: 12, Persuasiveness: 6, Language: 16,
	},
	ArticleTypeSpeech: {
		Clarity: 20, Structure: 20, Completeness: 16, Readability: 18, Persuasiveness: 14, Language: 12,
	},
	ArticleTypeArgument: {
		Clarity: 18, Structure: 20, Completeness: 14, Readability: 14, Persuasiveness: 24, Language: 10,
	},
	ArticleTypeReview: {
		Clarity: 18, Structure: 20, Completeness: 14, Readability: 16, Persuasiveness: 22, Language: 10,
	},
	ArticleTypeCopywriting: {
		Clarity: 18, Structure: 14, Completeness: 8, Readability: 22, Persuasiveness: 26, Language: 12,
	},
	ArticleTypeWebNovel: {
		Clarity: 14, Structure: 18, Completeness: 10, Readability: 26, Persuasiveness: 14, Language: 18,
	},
}

func scoringWeights(articleType string) map[string]int {
	weight, ok := articleTypeWeights[normalizeArticleType(articleType)]
	if !ok {
		weight = articleTypeWeights[ArticleTypeExpository]
	}
	return weight.toMap()
}
