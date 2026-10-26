package dlq_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/cdc/dlq"
	"myblogx/internal/platform/log/writer"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"

	"gorm.io/gorm"
)

// fakeReplayer 是可控的执行器：按 stream 声明能力，按 err 决定这次的结果。
type fakeReplayer struct {
	streams []string
	err     error
	calls   []string
}

func (f *fakeReplayer) Streams() []string { return f.streams }

func (f *fakeReplayer) Replay(_ context.Context, rec deadletter.Record) error {
	f.calls = append(f.calls, rec.CdcJobID)
	return f.err
}

// captureEmitter 收集回放日志输入，便于断言"每次尝试都记了一条"。
func captureEmitter(sink *[]writer.ReplayEventInput) func(writer.ReplayEventInput) {
	return func(in writer.ReplayEventInput) { *sink = append(*sink, in) }
}

func seed(t *testing.T, db *gorm.DB, stream, jobID, status string) ctype.ID {
	t.Helper()
	row := &schema.DeadLetterRow{
		Stream:      stream,
		CdcJobID:    jobID,
		SourceTable: "article_models",
		Action:      "insert",
		TargetKey:   "doc_id=1",
		PayloadJSON: "{}",
		Status:      status,
		ErrorCode:   "ES_400",
		ErrorMsg:    "原始失败原因",
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("插入死信失败: %v", err)
	}
	return row.ID
}

func mustLoad(t *testing.T, db *gorm.DB, id ctype.ID) deadletter.Record {
	t.Helper()
	list, err := deadletter.ByIDs(db, []ctype.ID{id})
	if err != nil || len(list) != 1 {
		t.Fatalf("读取死信失败: err=%v len=%d", err, len(list))
	}
	return list[0]
}

// 重投成功：状态变 resolved，**原始错误原因保留**，写一条 success 回放日志。
func TestReplaySuccessMarksResolved(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.DeadLetterRow{})
	id := seed(t, db, "es_river", "es_river:db.article_models:bin.1:4:0", deadletter.StatusPending)

	var logs []writer.ReplayEventInput
	r := &fakeReplayer{streams: []string{"es_river"}}
	svc := dlq.New(dlq.Options{DB: db, Replayers: []dlq.Replayer{r}, Emit: captureEmitter(&logs)})

	results, err := svc.Replay(context.Background(), []ctype.ID{id})
	if err != nil {
		t.Fatalf("重投返回错误: %v", err)
	}
	if len(results) != 1 || results[0].Result != dlq.ResultSuccess {
		t.Fatalf("期望 success，实际 %+v", results)
	}

	got := mustLoad(t, db, id)
	if got.Status != deadletter.StatusResolved {
		t.Fatalf("状态应为 resolved，实际 %s", got.Status)
	}
	if got.RetryCount != 1 {
		t.Fatalf("retry_count 应为 1，实际 %d", got.RetryCount)
	}
	if got.ErrorCode != "ES_400" {
		t.Fatalf("补上之后仍要保留原始失败原因，实际 error_code=%q", got.ErrorCode)
	}
	if len(r.calls) != 1 {
		t.Fatalf("执行器应被调用 1 次，实际 %d", len(r.calls))
	}
	if len(logs) != 1 || logs[0].Result != dlq.ResultSuccess || logs[0].CdcJobID != got.CdcJobID {
		t.Fatalf("回放日志不正确: %+v", logs)
	}
}

// 重投失败：状态保持 pending、retry_count+1、错误信息覆盖为本次失败原因。
func TestReplayFailureKeepsPending(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.DeadLetterRow{})
	id := seed(t, db, "es_river", "job-boom", deadletter.StatusPending)

	var logs []writer.ReplayEventInput
	r := &fakeReplayer{streams: []string{"es_river"}, err: errors.New("es 仍然 400")}
	svc := dlq.New(dlq.Options{DB: db, Replayers: []dlq.Replayer{r}, Emit: captureEmitter(&logs)})

	results, _ := svc.Replay(context.Background(), []ctype.ID{id})
	if results[0].Result != dlq.ResultFailed {
		t.Fatalf("期望 failed，实际 %+v", results)
	}

	got := mustLoad(t, db, id)
	if got.Status != deadletter.StatusPending {
		t.Fatalf("失败后状态应保持 pending，实际 %s", got.Status)
	}
	if got.ErrorMsg != "es 仍然 400" {
		t.Fatalf("错误信息应被本次失败覆盖，实际 %q", got.ErrorMsg)
	}
	if len(logs) != 1 || logs[0].Result != dlq.ResultFailed || logs[0].ErrorMessage != "es 仍然 400" {
		t.Fatalf("失败也要记回放日志: %+v", logs)
	}
}

// 已解决的记录再点一次：跳过，不调执行器、不改库。
func TestReplaySkipsResolved(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.DeadLetterRow{})
	id := seed(t, db, "es_river", "job-done", deadletter.StatusResolved)

	var logs []writer.ReplayEventInput
	r := &fakeReplayer{streams: []string{"es_river"}}
	svc := dlq.New(dlq.Options{DB: db, Replayers: []dlq.Replayer{r}, Emit: captureEmitter(&logs)})

	results, _ := svc.Replay(context.Background(), []ctype.ID{id})
	if results[0].Result != dlq.ResultSkipped {
		t.Fatalf("期望 skipped，实际 %+v", results)
	}
	if len(r.calls) != 0 {
		t.Fatal("已解决的记录不应再调用执行器")
	}
	got := mustLoad(t, db, id)
	if got.Status != deadletter.StatusResolved || got.RetryCount != 0 {
		t.Fatalf("跳过不应改动记录: %+v", got)
	}
}

// 没有执行器的 stream：明确报"不支持"，并说明该改用别的什么办法。
func TestReplayUnsupportedStream(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.DeadLetterRow{})
	id := seed(t, db, "es_projection", "proj-1", deadletter.StatusPending)

	var logs []writer.ReplayEventInput
	svc := dlq.New(dlq.Options{
		DB:        db,
		Replayers: []dlq.Replayer{&fakeReplayer{streams: []string{"es_river"}}},
		Emit:      captureEmitter(&logs),
	})

	results, _ := svc.Replay(context.Background(), []ctype.ID{id})
	if results[0].Result != dlq.ResultUnsupported {
		t.Fatalf("期望 unsupported，实际 %+v", results)
	}
	if results[0].Message == "" || !strings.Contains(results[0].Message, "重建搜索投影") {
		t.Fatalf("应给出替代动作，实际 %q", results[0].Message)
	}
	// 不改库：不支持不是"处理失败"，记录要原样留给人工。
	got := mustLoad(t, db, id)
	if got.Status != deadletter.StatusPending || got.RetryCount != 0 {
		t.Fatalf("不支持时不应改动记录: %+v", got)
	}
}

// 执行器说"无需重投"：算 skipped，不改库。
func TestReplayHonoursSkipped(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.DeadLetterRow{})
	id := seed(t, db, "outbox_relay", "outbox:1", deadletter.StatusPending)

	r := &fakeReplayer{streams: []string{"outbox_relay"}, err: dlq.ErrSkipped}
	svc := dlq.New(dlq.Options{DB: db, Replayers: []dlq.Replayer{r}})

	results, _ := svc.Replay(context.Background(), []ctype.ID{id})
	if results[0].Result != dlq.ResultSkipped {
		t.Fatalf("期望 skipped，实际 %+v", results)
	}
	if got := mustLoad(t, db, id); got.Status != deadletter.StatusPending {
		t.Fatalf("skipped 不应改动状态，实际 %s", got.Status)
	}
}

// 批量里一条失败不影响另一条 —— 运维要的是"这批哪几条补上了"。
func TestReplayBatchIsolatesFailures(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.DeadLetterRow{})
	okID := seed(t, db, "ok_stream", "ok-1", deadletter.StatusPending)
	badID := seed(t, db, "bad_stream", "bad-1", deadletter.StatusPending)

	svc := dlq.New(dlq.Options{DB: db, Replayers: []dlq.Replayer{
		&fakeReplayer{streams: []string{"ok_stream"}},
		&fakeReplayer{streams: []string{"bad_stream"}, err: errors.New("boom")},
	}})

	results, err := svc.Replay(context.Background(), []ctype.ID{okID, badID})
	if err != nil {
		t.Fatalf("批量重投不应整体失败: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("应返回 2 条结果，实际 %d", len(results))
	}
	if mustLoad(t, db, okID).Status != deadletter.StatusResolved {
		t.Fatal("成功的那条应变为 resolved")
	}
	if mustLoad(t, db, badID).Status != deadletter.StatusPending {
		t.Fatal("失败的那条应保持 pending")
	}
}

// SupportedStreams 用于给前端/运维看"哪些流能重投"。
func TestSupportedStreams(t *testing.T) {
	svc := dlq.New(dlq.Options{Replayers: []dlq.Replayer{
		&fakeReplayer{streams: []string{"outbox_relay"}},
		&fakeReplayer{streams: []string{"es_river"}},
	}})
	got := svc.SupportedStreams()
	if len(got) != 2 || got[0] != "es_river" || got[1] != "outbox_relay" {
		t.Fatalf("应去重并排序，实际 %v", got)
	}
}
