package ai_diagnose

const (
	selectionTextMinChars = 25
	selectionTextMaxChars = 3500
	contextTextMaxChars   = 800
	articleTitleMaxChars  = 50
	diagnoseMaxIssues     = 5
)

type DiagnoseRequest struct {
	SelectionText string
	PrefixText    string
	SuffixText    string
	ArticleTitle  string
}

type DiagnoseResponse struct {
	Summary string          `json:"summary"`
	Issues  []DiagnoseIssue `json:"issues"`
}

type DiagnoseIssue struct {
	Type       string `json:"type"`
	Severity   string `json:"severity"`
	Reason     string `json:"reason"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion"`
}
