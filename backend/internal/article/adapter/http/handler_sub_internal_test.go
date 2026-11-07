package http

import (
	"testing"
	"time"

	"myblogx/internal/apperr"
)

// TestParseViewHistoryCursorUsesAppErrorCode 钉住游标校验的错误来源（评审 #248 / P1-1）。
//
// 适配层直接 `fmt.Errorf` + `res.FailWithMsg(err.Error())` 会绕过 apperr 收敛点
// （archcheck R8 只拦 `errors.New("中文")`，`fmt.Errorf` 恰好漏网）：文案散在
// 适配层、也没有稳定错误码。这里断言三类不合法输入都返回
// `apperr.CodeCommonInvalidArgument`，handler 再据此回 1001（参数错误）。
func TestParseViewHistoryCursorUsesAppErrorCode(t *testing.T) {
	cases := []struct {
		name     string
		rawTime  string
		rawID    string
		expected apperr.Code
	}{
		{name: "只有 before_id（半游标）", rawID: "1003", expected: apperr.CodeCommonInvalidArgument},
		{name: "只有 before_updated_at（半游标）", rawTime: "2026-01-01T12:00:00Z", expected: apperr.CodeCommonInvalidArgument},
		{name: "坏时间戳", rawTime: "not-a-time", rawID: "1003", expected: apperr.CodeCommonInvalidArgument},
		{name: "坏文章 ID", rawTime: "2026-01-01T12:00:00Z", rawID: "not-an-id", expected: apperr.CodeCommonInvalidArgument},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseViewHistoryCursor(tc.rawTime, tc.rawID)
			if err == nil {
				t.Fatal("不合法游标必须报错（静默退回第一页会让加载更多原地打转）")
			}
			code, ok := apperr.CodeOf(err)
			if !ok || code != tc.expected {
				t.Fatalf("错误应带错误码 %s，实际 code=%s ok=%v err=%v", tc.expected, code, ok, err)
			}
		})
	}
}

// TestParseViewHistoryCursorAcceptsCompletePairs 保证参数校验改走 apperr 后正常路径不变。
func TestParseViewHistoryCursorAcceptsCompletePairs(t *testing.T) {
	// 两个都缺省 = 第一页，没有错误、也没有游标。
	cursor, err := parseViewHistoryCursor("", "")
	if err != nil || cursor != nil {
		t.Fatalf("两个参数都缺省应表示第一页，实际 cursor=%v err=%v", cursor, err)
	}

	cursor, err = parseViewHistoryCursor("2026-01-01T12:00:00Z", "1003")
	if err != nil {
		t.Fatalf("合法游标不应报错: %v", err)
	}
	want := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if !cursor.ViewedAt.Equal(want) || cursor.ArticleID.Uint64() != 1003 {
		t.Fatalf("游标解析错误: %+v", cursor)
	}
}
