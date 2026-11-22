package http

import (
	"testing"

	"myblogx/conf"
)

// 后台「内容过滤」页的红条要能说清"哪几类是空的"。
//
// 判据只有一处（contentfilter.RequiredCategories），由后端算好下发 —— 后台不许自己
// 再抄一份类目清单，否则改一处忘另一处时，红条会在一台机器上安静、在另一台上尖叫。

// TestContentFilterSectionResponseReportsMissing 钉住缺类目时下发缺口清单。
func TestContentFilterSectionResponseReportsMissing(t *testing.T) {
	t.Parallel()

	// 生产当时的形状：五类有词、涉政与暴恐为空。
	response := contentFilterSectionResponse(conf.ContentFilter{
		Words: []conf.ContentFilterWord{
			{Word: "冰毒", Tier: "block", Category: "drugs"},
			{Word: "成人视频", Tier: "watch", Category: "porn"},
		},
	})

	if len(response.RequiredCategories) != 2 ||
		response.RequiredCategories[0] != "politics" ||
		response.RequiredCategories[1] != "violence" {
		t.Fatalf("必需类目应由后端下发 politics/violence，实际 %v", response.RequiredCategories)
	}
	if len(response.MissingRequiredCategories) != 2 {
		t.Fatalf("两类都为空时必须报两个缺口，实际 %v", response.MissingRequiredCategories)
	}
	// 词表本身要原样透出（页面靠它渲染列表，不能因为加了字段就丢）。
	if len(response.Words) != 2 {
		t.Fatalf("词表应原样返回，实际 %d 条", len(response.Words))
	}
}

// TestContentFilterSectionResponseNoMissingWhenCovered 钉住完整时不报红。
func TestContentFilterSectionResponseNoMissingWhenCovered(t *testing.T) {
	t.Parallel()

	response := contentFilterSectionResponse(conf.ContentFilter{
		Words: []conf.ContentFilterWord{
			{Word: "非法集会", Tier: "block", Category: "politics"},
			{Word: "炸药配方", Tier: "block", Category: "violence"},
		},
	})
	if len(response.MissingRequiredCategories) != 0 {
		t.Fatalf("两类都有词时不该有缺口，实际 %v", response.MissingRequiredCategories)
	}
}
