package rule_test

import (
	"myblogx/service/river_service/rule"
	"testing"
)

func TestRulePrepareAndFilter(t *testing.T) {
	r := rule.NewDefaultRule("db1", "ArticleTable")
	if r.Index != "articletable" || r.Type != "articletable" {
		t.Fatalf("默认规则大小写处理异常: index=%s type=%s", r.Index, r.Type)
	}

	r.Index = ""
	r.Type = ""
	r.Filter = []string{"id", "title"}
	if err := r.Prepare(); err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	if r.Index != "articletable" || r.Type != "articletable" {
		t.Fatalf("Prepare 默认值异常: index=%s type=%s", r.Index, r.Type)
	}
	if !r.CheckFilter("id") || r.CheckFilter("content") {
		t.Fatal("CheckFilter 结果异常")
	}
}
