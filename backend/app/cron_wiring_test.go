package app_test

import (
	"testing"

	"myblogx/app"
)

// TestArticleCounterMetricsAreWellFormed 校验"Redis key → 表列"这份映射本身。
//
// 这类错误不会编译失败、不会 panic：把两个指标写成同一个 key 或同一列，
// 表现只是"某个计数偶尔不涨"或"两个计数互相覆盖"，只能对着线上数据才发现。
// 所以把映射导出并在这里逐条校验。
func TestArticleCounterMetricsAreWellFormed(t *testing.T) {
	t.Parallel()

	metrics := app.ArticleCounterMetrics
	if len(metrics) != 4 {
		t.Fatalf("文章计数应有 4 个指标（收藏/点赞/浏览/评论），实际 %d", len(metrics))
	}

	seenKey := make(map[string]string, len(metrics))
	seenCol := make(map[string]string, len(metrics))
	for _, m := range metrics {
		if m.ActiveKey == "" {
			t.Fatalf("指标 %q 的 ActiveKey 为空", m.Metric)
		}
		if m.ArticleCol == "" {
			t.Fatalf("指标 %q 的目标列为空", m.Metric)
		}
		if m.Task == "" {
			t.Fatalf("指标 %q 的 Task 为空（日志会看不出是哪个任务）", m.Metric)
		}

		if prev, ok := seenKey[m.ActiveKey]; ok {
			t.Fatalf("ActiveKey 重复: %q（%s 与 %s）", m.ActiveKey, prev, m.Metric)
		}
		seenKey[m.ActiveKey] = m.Metric

		if prev, ok := seenCol[string(m.ArticleCol)]; ok {
			t.Fatalf("目标列重复: %q（%s 与 %s）", m.ArticleCol, prev, m.Metric)
		}
		seenCol[string(m.ArticleCol)] = m.Metric

		// sync key 必须与 active key 不同：相同会导致 RENAME 把自己的数据覆盖掉。
		if m.SyncKey() == m.ActiveKey {
			t.Fatalf("指标 %q 的 SyncKey 与 ActiveKey 相同: %q", m.Metric, m.ActiveKey)
		}
	}
}

// TestCounterMetricSyncKeyIsDerived 锁定 sync key 的派生规则。
//
// 规则本身不重要，重要的是"从 ActiveKey 派生"这件事不能被改回手写 ——
// 手写时四个指标复制粘贴很容易漏改一个，两个指标共用 sync key 会互相覆盖。
func TestCounterMetricSyncKeyIsDerived(t *testing.T) {
	t.Parallel()

	m := app.CounterMetric{ActiveKey: "article_view"}
	if got := m.SyncKey(); got != "article_view:syncing" {
		t.Fatalf("SyncKey 派生规则变了: got=%q want=%q", got, "article_view:syncing")
	}
}
