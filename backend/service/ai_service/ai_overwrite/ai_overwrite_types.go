package ai_overwrite

const (
	ModePolish         = "polish"
	ModeGrammarFix     = "grammar_fix"
	ModeStyleTransform = "style_transform"
)

const (
	selectionTextMinChars = 25
	selectionTextMaxChars = 3500
	contextTextMaxChars   = 800
	articleTitleMaxChars  = 50
	targetStyleMaxChars   = 30
)

type RewriteRequest struct {
	// Mode 枚举值:polish, grammar_fix, style_transform
	Mode          string
	TargetStyle   string
	SelectionText string
	PrefixText    string
	SuffixText    string
	ArticleTitle  string
}
