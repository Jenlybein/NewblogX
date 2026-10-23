package ai_overwrite

import "testing"

func TestNormalizeRewriteRequest(t *testing.T) {
	req, err := normalizeRewriteRequest(RewriteRequest{
		Mode:          ModeStyleTransform,
		SelectionText: "这是一段足够长的选中内容，用来验证改写请求的规范化逻辑是否能够正常工作。",
		PrefixText:    "前文",
		SuffixText:    "后文",
		ArticleTitle:  "测试标题",
		TargetStyle:   "更正式一些",
	})
	if err != nil {
		t.Fatalf("规范化请求不应失败: %v", err)
	}
	if req.TargetStyle != "更正式一些" {
		t.Fatalf("目标风格保留错误: %+v", req)
	}
}

func TestNormalizeRewriteRequestTooShort(t *testing.T) {
	_, err := normalizeRewriteRequest(RewriteRequest{
		Mode:          ModePolish,
		SelectionText: "太短了",
		ArticleTitle:  "测试标题",
	})
	if err == nil || err.Error() != selectionTooShortMsg {
		t.Fatalf("短内容应返回固定提示: %v", err)
	}
}
