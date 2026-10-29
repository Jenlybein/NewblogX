package clickhouse

import (
	"context"
	"fmt"
	"time"
)

// AIUsagePoint 是用量趋势上的一个桶。
type AIUsagePoint struct {
	// Bucket 与 ClickHouse 的分桶键同构（2026-10-29 13:00:00），便于直接对账。
	Bucket string `json:"bucket"`
	// Label 是给人看的短标签：13:00 / 10-29 / 2026-10。
	Label            string `json:"label"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	Calls            int64  `json:"calls"`
	Failures         int64  `json:"failures"`
}

// AIUsageSummary 是一个周期的聚合结果。
type AIUsageSummary struct {
	// Available=false 表示日志库没启用，前端应显示空态而不是一条零线。
	Available        bool           `json:"available"`
	Range            string         `json:"range"`
	Granularity      string         `json:"granularity"`
	PromptTokens     int64          `json:"prompt_tokens"`
	CompletionTokens int64          `json:"completion_tokens"`
	TotalTokens      int64          `json:"total_tokens"`
	Calls            int64          `json:"calls"`
	Failures         int64          `json:"failures"`
	Points           []AIUsagePoint `json:"points"`
}

// SumAIUsage 按周期（today/week/month/year）聚合 AI 用量。
//
// now 由调用方注入，便于单测钉边界；生产侧传 time.Now()。
func SumAIUsage(opts Options, rng string, now time.Time) (AIUsageSummary, error) {
	buckets, err := buildAIBuckets(rng, now)
	if err != nil {
		return AIUsageSummary{}, err
	}
	summary := AIUsageSummary{
		Range:       rng,
		Granularity: buckets.Granularity,
		Points:      make([]AIUsagePoint, 0, len(buckets.Keys)),
	}
	if !clickhouseEnabled(opts) {
		// 返回"不可用"而不是一条全零曲线：全零会被读成"今天完全没用 AI"，
		// 与"日志库没开、我们什么都不知道"是两件完全不同的事。
		return summary, nil
	}
	summary.Available = true

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// FINAL 是必须的：ReplacingMergeTree 是"至少一次"投递语义，WAL 重放会把
	// 同一批再投一遍。列表页容忍重复（与其它日志一致），但这里是加总，
	// 不去重会把用量算成两倍。
	sqlText := fmt.Sprintf(`
SELECT %s AS bucket,
       count() AS calls,
       countIf(success = 0) AS failures,
       sum(prompt_tokens) AS prompt_tokens,
       sum(completion_tokens) AS completion_tokens,
       sum(total_tokens) AS total_tokens
FROM %s FINAL
WHERE ts >= ? AND ts < ?
GROUP BY bucket
ORDER BY bucket`, buckets.Expr, AICallLogTableName)

	// 参数用东八区字符串：列被 DDL 钉在 Asia/Shanghai，传 UTC 时间会让
	// 窗口整体偏 8 小时（驱动默认按 UTC 序列化 time.Time）。
	rows, err := opts.ClickHouse.QueryContext(ctx, sqlText,
		buckets.Start.Format(clickhouseTimeLayout), buckets.End.Format(clickhouseTimeLayout))
	if err != nil {
		return summary, err
	}
	defer rows.Close()

	type bucketRow struct {
		prompt     uint64
		completion uint64
		total      uint64
		calls      uint64
		failures   uint64
	}
	found := make(map[string]bucketRow, len(buckets.Keys))
	for rows.Next() {
		var key string
		var row bucketRow
		if err = rows.Scan(&key, &row.calls, &row.failures, &row.prompt, &row.completion, &row.total); err != nil {
			return summary, err
		}
		found[key] = row
	}
	if err = rows.Err(); err != nil {
		return summary, err
	}

	// 按时间轴逐桶取，取不到就补 0；总计由桶相加得出，不再单独查一次 sum()，
	// 免得两条 SQL 因为边界写法不同而对不上。
	for index, key := range buckets.Keys {
		point := AIUsagePoint{Bucket: key, Label: buckets.Labels[index]}
		if row, ok := found[key]; ok {
			point.PromptTokens = int64(row.prompt)
			point.CompletionTokens = int64(row.completion)
			point.TotalTokens = int64(row.total)
			point.Calls = int64(row.calls)
			point.Failures = int64(row.failures)
		}
		summary.PromptTokens += point.PromptTokens
		summary.CompletionTokens += point.CompletionTokens
		summary.TotalTokens += point.TotalTokens
		summary.Calls += point.Calls
		summary.Failures += point.Failures
		summary.Points = append(summary.Points, point)
	}
	return summary, nil
}
