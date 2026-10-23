package ai_diagnose

import "testing"

func TestNormalizeDiagnoseResponse(t *testing.T) {
	resp := normalizeDiagnoseResponse(&DiagnoseResponse{
		Summary: "",
		Issues: []DiagnoseIssue{
			{Type: "未知类型", Severity: "未知", Reason: "句子太长", Evidence: "一句话塞了太多内容", Suggestion: "拆成两句"},
		},
	})

	if resp.Summary == "" {
		t.Fatalf("summary 应有默认值: %+v", resp)
	}
	if len(resp.Issues) != 1 {
		t.Fatalf("issues 数量错误: %+v", resp)
	}
	if resp.Issues[0].Type != "可读性" || resp.Issues[0].Severity != "中" {
		t.Fatalf("类型或严重度规范化失败: %+v", resp.Issues[0])
	}
}

func TestNormalizeDiagnoseRequestTooShort(t *testing.T) {
	_, err := normalizeDiagnoseRequest(DiagnoseRequest{
		SelectionText: "太短了",
		ArticleTitle:  "测试标题",
	})
	if err == nil || err.Error() != selectionTooShortMsg {
		t.Fatalf("短内容应返回固定提示: %v", err)
	}
}

func TestNormalizeIssueTypeAliases(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "readability", want: "可读性"},
		{input: "logic", want: "逻辑"},
		{input: "内容完整度", want: "完整度"},
		{input: "structure", want: "结构"},
		{input: "redundancy", want: "重复"},
		{input: "grammar", want: "语言"},
		{input: "style", want: "语气"},
	}

	for _, item := range cases {
		if got := normalizeIssueType(item.input); got != item.want {
			t.Fatalf("normalizeIssueType(%q) = %q, want %q", item.input, got, item.want)
		}
	}
}

func TestNormalizeSeverityAliases(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "low", want: "低"},
		{input: "medium", want: "中"},
		{input: "critical", want: "高"},
	}

	for _, item := range cases {
		if got := normalizeSeverity(item.input); got != item.want {
			t.Fatalf("normalizeSeverity(%q) = %q, want %q", item.input, got, item.want)
		}
	}
}
