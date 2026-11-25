package qiniu

import (
	"reflect"
	"testing"
)

// 审核场景必须是七牛文档里的 code（2026-11-25 线上回 400 invalid scenes: ad）。
//
// 场景名写错的后果不是"少审一类"，而是**整次审核 400 作废**：图停在待审核、
// 超时后进人工队列，运营看到的是"图片审核坏了"。而七牛只报第一个非法值，
// 所以一次只能纠一个 —— 这条用例把六个 code 一起钉死，别再逐个试。
// 参考：https://developer.qiniu.com/censor/7299/censor-params
func TestCensorScenesUseQiniuCodes(t *testing.T) {
	t.Parallel()

	want := []string{"pulp", "terror", "politician", "ads", "behavior", "abnormal"}
	if !reflect.DeepEqual(censorScenes, want) {
		t.Fatalf("审核场景 code 不对：\n want=%v\n  got=%v", want, censorScenes)
	}
}
