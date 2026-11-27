package http

import (
	"encoding/json"
	"testing"

	"myblogx/conf"
)

// 2026-11-28 / ADR-0037 §6.2：content_filter 端点从"整段覆盖"改成"字段级补丁"。
//
// 为什么必须打补丁而不是整段覆盖：这一段的字段现在分给两个页面写 —— 词表页管
// words / whitelist / whitelist_phrases / retain_days，文章审核设置的
// 「关键词命中处理」子页管 auto_dispose。整段覆盖会让"在词表页点一次保存"
// 把命中处置静默恢复默认值（反过来也一样）。

const currentFilterForPatch = `{
	"words":[{"word":"冰毒","tier":"block","category":"drugs"}],
	"whitelist":["独立"],
	"whitelist_phrases":["安全套接层"],
	"retain_days":180,
	"auto_dispose":{"block_action":"reject","watch_action":"draft"}
}`

func decodeCurrentFilter(t *testing.T) conf.ContentFilter {
	t.Helper()
	var current conf.ContentFilter
	if err := json.Unmarshal([]byte(currentFilterForPatch), &current); err != nil {
		t.Fatalf("预置现值失败: %v", err)
	}
	return current
}

// TestMergeContentFilterPatch只写传了的键 钉住"缺键保留现值"。
func TestMergeContentFilterPatch只写传了的键(t *testing.T) {
	t.Parallel()

	current := decodeCurrentFilter(t)
	// 词表页保存：只传词表与保留期，不传 auto_dispose。
	raw := map[string]json.RawMessage{
		"words":             json.RawMessage(`[]`),
		"retain_days":       json.RawMessage(`90`),
		"whitelist":         json.RawMessage(`["独立","进程"]`),
		"whitelist_phrases": json.RawMessage(`[]`),
	}
	merged, err := mergeContentFilterPatch(current, raw)
	if err != nil {
		t.Fatalf("合并失败: %v", err)
	}

	// 传了的键：被覆盖。
	if len(merged.Words) != 0 {
		t.Fatalf("words 应被覆盖成空表，实际 %d 条", len(merged.Words))
	}
	if merged.RetainDays != 90 {
		t.Fatalf("retain_days 应被覆盖成 90，实际 %d", merged.RetainDays)
	}
	if len(merged.Whitelist) != 2 {
		t.Fatalf("whitelist 应被覆盖成 2 条，实际 %v", merged.Whitelist)
	}
	// **没传的键必须原样保留** —— 这条是整条用例的重点：
	// 词表页保存不该把手动配好的命中处置恢复默认。
	if merged.AutoDispose.BlockAction != "reject" || merged.AutoDispose.WatchAction != "draft" {
		t.Fatalf("未传的 auto_dispose 不该被改动，实际 %+v", merged.AutoDispose)
	}
}

// TestMergeContentFilterPatch只传处置时词表不动 钉住反方向的隔离。
func TestMergeContentFilterPatch只传处置时词表不动(t *testing.T) {
	t.Parallel()

	current := decodeCurrentFilter(t)
	raw := map[string]json.RawMessage{
		"auto_dispose": json.RawMessage(`{"block_action":"release","watch_action":"release"}`),
	}
	merged, err := mergeContentFilterPatch(current, raw)
	if err != nil {
		t.Fatalf("合并失败: %v", err)
	}
	if merged.AutoDispose.BlockAction != "release" || merged.AutoDispose.WatchAction != "release" {
		t.Fatalf("传了的 auto_dispose 应被覆盖，实际 %+v", merged.AutoDispose)
	}
	if len(merged.Words) != 1 || merged.RetainDays != 180 || len(merged.WhitelistPhrases) != 1 {
		t.Fatalf("词表 / 保留期 / 短语白名单都应原样保留，实际 %+v", merged)
	}
}

// TestMergeContentFilterPatch非法JSON报段错误码 钉住错误码归属。
func TestMergeContentFilterPatch非法JSON报段错误码(t *testing.T) {
	t.Parallel()

	current := decodeCurrentFilter(t)
	raw := map[string]json.RawMessage{"words": json.RawMessage(`"不是数组"`)}
	if _, err := mergeContentFilterPatch(current, raw); err == nil {
		t.Fatal("非法 JSON 必须报错")
	}
}

// TestMergeContentFilterPatch未知键不改任何东西 钉住白名单。
func TestMergeContentFilterPatch未知键不改任何东西(t *testing.T) {
	t.Parallel()

	current := decodeCurrentFilter(t)
	raw := map[string]json.RawMessage{
		// 只读字段（后端算出来的类目覆盖结论）不该可写，写进来也一律忽略。
		"required_categories":         json.RawMessage(`["politics"]`),
		"missing_required_categories": json.RawMessage(`[]`),
	}
	merged, err := mergeContentFilterPatch(current, raw)
	if err != nil {
		t.Fatalf("未知键不该报错: %v", err)
	}
	if merged.AutoDispose.BlockAction != "reject" || merged.RetainDays != 180 {
		t.Fatalf("白名单外的键不得到场，实际 %+v", merged)
	}
}
