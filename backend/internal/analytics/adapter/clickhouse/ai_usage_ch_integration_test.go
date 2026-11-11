//go:build integration

package clickhouse

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"myblogx/internal/testkit/chcontainer"

	chdriver "github.com/ClickHouse/clickhouse-go/v2"
)

// 本文件是 #357 的真 ClickHouse 回归：AI 用量恒为 0 的根因在桶键对不齐，
// 只有真 CH 会按 formatDateTime 的语义渲染格式串 —— SQLite/mock 都会把 bug 放绿。
//
// 运行方式（有 Docker 时自起 ClickHouse；也可设 BLOGX_CH_ADDR=host:9000
// 指向已执行过 deploy/docker/clickhouse/init/01-logs.sql 的实例）：
//
//	go test -tags integration -run SumAIUsage ./internal/analytics/adapter/clickhouse/

// openAIUsageCH 连上真 ClickHouse；没有 Docker 时跳过而不是假绿。
func openAIUsageCH(t *testing.T) *sql.DB {
	t.Helper()

	addr := chcontainer.Addr(t)
	db := chdriver.OpenDB(&chdriver.Options{
		Addr: []string{addr},
		Auth: chdriver.Auth{
			Database: "blogx_logs",
			Username: "default",
			Password: "",
		},
		DialTimeout: 5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Skipf("ClickHouse 不可达（%s），跳过集成测试: %v", addr, err)
	}
	return db
}

// insertAICall 往 ai_call_logs 写一条调用日志。
//
// ts 写成东八区字符串（列被 DDL 钉在 Asia/Shanghai），与写入侧格式一致。
func insertAICall(t *testing.T, db *sql.DB, eventID uint64, at time.Time, success bool, prompt, completion, total uint32) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, `
INSERT INTO `+AICallLogTableName+`
	(event_id, ts, level, service, env, instance_id, scene, vendor, model, stream,
	 success, attempts, prompt_tokens, completion_tokens, total_tokens)
VALUES (?, ?, 'info', 'blogx', 'test', '1', 'chat', 'test', 'test-model', 0, ?, 1, ?, ?, ?)`,
		eventID, at.Format(clickhouseTimeLayout), boolToUInt8(success), prompt, completion, total)
	if err != nil {
		t.Fatalf("写入 ai_call_logs 失败: %v", err)
	}
}

// TestSumAIUsageCountsRealClickHouseRows 验证真 CH 里有两行调用时，今日聚合非零，
// 且命中的桶正是 Go 侧生成的那个键。
//
// 红：旧格式串 %H:%M:%S 里 %M 被 CH 当成月份英文名，桶键变成 "10:November:00"，
// 与 Go 的 "10:00:00" 全部错位 → summary 恒为 0。
func TestSumAIUsageCountsRealClickHouseRows(t *testing.T) {
	db := openAIUsageCH(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE "+AICallLogTableName); err != nil {
		t.Fatalf("清空 %s 失败: %v", AICallLogTableName, err)
	}

	// 用真实"现在"锚定：注入的 now 决定查询窗口，行也必须落在同一个窗口内，
	// 写死日期会在 TTL（400 天）与跨年之后变成假红。
	now := time.Now().In(cst)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cst)

	insertAICall(t, db, 1001, today.Add(10*time.Hour+15*time.Minute), true, 10, 20, 30)
	insertAICall(t, db, 1002, today.Add(11*time.Hour+5*time.Minute), false, 40, 60, 100)

	summary, err := SumAIUsage(Options{ClickHouseEnabled: true, ClickHouse: db}, "today", now)
	if err != nil {
		t.Fatalf("SumAIUsage 失败: %v", err)
	}
	if !summary.Available {
		t.Fatal("Available 应为 true（CH 已启用且查询成功）")
	}
	if len(summary.Points) != 24 {
		t.Fatalf("今日桶数 = %d，期望 24", len(summary.Points))
	}

	if summary.Calls != 2 || summary.Failures != 1 {
		t.Fatalf("calls/failures = %d/%d，期望 2/1（桶键对不齐时这里会是 0/0）",
			summary.Calls, summary.Failures)
	}
	if summary.PromptTokens != 50 || summary.CompletionTokens != 80 || summary.TotalTokens != 130 {
		t.Fatalf("token 汇总 = %d/%d/%d，期望 50/80/130",
			summary.PromptTokens, summary.CompletionTokens, summary.TotalTokens)
	}

	// 10 点桶：Go 侧键与 CH 侧键必须一致，这一格是整条链路的接缝。
	hourKey := today.Add(10 * time.Hour).Format(aiBucketGoFormat)
	if got := summary.Points[10].Bucket; got != hourKey {
		t.Fatalf("10 点桶键 = %q，期望 %q", got, hourKey)
	}
	if summary.Points[10].Calls != 1 || summary.Points[10].TotalTokens != 30 {
		t.Fatalf("10 点桶 = calls %d / total %d，期望 1 / 30",
			summary.Points[10].Calls, summary.Points[10].TotalTokens)
	}
	if summary.Points[11].Calls != 1 || summary.Points[11].Failures != 1 || summary.Points[11].TotalTokens != 100 {
		t.Fatalf("11 点桶 = calls %d / failures %d / total %d，期望 1 / 1 / 100",
			summary.Points[11].Calls, summary.Points[11].Failures, summary.Points[11].TotalTokens)
	}
	if summary.Points[12].Calls != 0 {
		t.Fatalf("12 点桶不该有调用，实际 %d", summary.Points[12].Calls)
	}
}
