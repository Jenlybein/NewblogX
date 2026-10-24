package persist

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

// fakeSink 是可控的内存 sink：fail 为 true 时模拟"主出口不可用"。
type fakeSink struct {
	mu      sync.Mutex
	fail    bool
	batches [][][]byte
	closed  bool
}

func (f *fakeSink) WriteBatch(_ context.Context, lines [][]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("clickhouse unavailable")
	}
	cp := make([][]byte, len(lines))
	for i, l := range lines {
		cp[i] = append([]byte(nil), l...)
	}
	f.batches = append(f.batches, cp)
	return nil
}

func (f *fakeSink) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeSink) setFail(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = v
}

func (f *fakeSink) batchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

func (f *fakeSink) allLines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, b := range f.batches {
		for _, l := range b {
			out = append(out, string(l))
		}
	}
	return out
}

// lines 生成 n 条测试记录，event_id 从 start 开始递增（保证跨批次唯一）。
func lines(start, n int) [][]byte {
	out := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, []byte(`{"event_id":`+itoa(start+i)+`,"message":"audit"}`))
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// ── ClickHouseSink ──────────────────────────────────────────────

func TestClickHouseSinkRejectsBadTableName(t *testing.T) {
	bad := []string{"", "a; DROP TABLE x", "tbl--", "1table", "a.b.c", "tbl `x`"}
	for _, name := range bad {
		if _, err := NewClickHouseSink(nil, name); err == nil {
			t.Fatalf("表名 %q 应被拒绝", name)
		}
	}
}

func TestClickHouseSinkAcceptsValidTableName(t *testing.T) {
	good := []string{"action_audit_logs", "blogx_logs.action_audit_logs"}
	for _, name := range good {
		if _, err := NewClickHouseSink(nil, name); err != nil {
			t.Fatalf("表名 %q 应被接受，实际 %v", name, err)
		}
	}
}

func TestClickHouseSinkNilDBReturnsError(t *testing.T) {
	s, err := NewClickHouseSink(nil, "action_audit_logs")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.WriteBatch(context.Background(), lines(0, 1)); err == nil {
		t.Fatal("db 为 nil 时应返回错误，交给 WAL 兜底")
	}
}

func TestClickHouseSinkEmptyBatchIsNoop(t *testing.T) {
	s, _ := NewClickHouseSink(nil, "action_audit_logs")
	if err := s.WriteBatch(context.Background(), nil); err != nil {
		t.Fatalf("空批次不应报错: %v", err)
	}
}

// ── WALSink ────────────────────────────────────────────────────

func TestWALSinkPassThroughWhenInnerOK(t *testing.T) {
	dir := t.TempDir()
	inner := &fakeSink{}
	s, err := NewWALSink(inner, filepath.Join(dir, "wal"), nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.WriteBatch(context.Background(), lines(0, 3)); err != nil {
		t.Fatalf("主出口正常时不应报错: %v", err)
	}

	if inner.batchCount() != 1 {
		t.Fatalf("主出口应收到 1 批，实际 %d", inner.batchCount())
	}
	pending, _ := PendingBatches(filepath.Join(dir, "wal"))
	if pending != 0 {
		t.Fatalf("主出口成功时不应产生 WAL，实际 %d 个", pending)
	}
}

func TestWALSinkFallsBackToDisk(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	inner := &fakeSink{fail: true}
	s, err := NewWALSink(inner, walDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.WriteBatch(context.Background(), lines(0, 5)); err != nil {
		t.Fatalf("落 WAL 成功应返回 nil（数据没丢）: %v", err)
	}

	pending, err := PendingBatches(walDir)
	if err != nil || pending != 1 {
		t.Fatalf("应产生 1 个 WAL 批次，实际 %d（err=%v）", pending, err)
	}

	// 不应残留 .tmp
	entries, _ := os.ReadDir(walDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("不应残留临时文件: %s", e.Name())
		}
	}

	// 重放后内容应完整
	restored := &fakeSink{}
	n, err := ReplayWAL(context.Background(), walDir, restored)
	if err != nil {
		t.Fatalf("重放失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("应重放 1 批，实际 %d", n)
	}
	if got := restored.allLines(); len(got) != 5 {
		t.Fatalf("重放应得 5 行，实际 %d", len(got))
	}
}

func TestWALSinkCloseRejectsWrites(t *testing.T) {
	dir := t.TempDir()
	s, err := NewWALSink(&fakeSink{}, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteBatch(context.Background(), lines(0, 1)); err == nil {
		t.Fatal("关闭后写入应报错")
	}
}

func TestNewWALSinkRequiresDir(t *testing.T) {
	if _, err := NewWALSink(&fakeSink{}, "  ", nil); err == nil {
		t.Fatal("空目录应被拒绝")
	}
}

// ── ReplayWAL ──────────────────────────────────────────────────

func TestReplayWALInOrderAndRemoves(t *testing.T) {
	walDir := t.TempDir()

	// 手工造三个批次，文件名带 unixnano 前缀
	write := func(name string, body string) {
		if err := os.WriteFile(filepath.Join(walDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("00000000000000000002-000002.jsonl", `{"seq":"b"}`+"\n")
	write("00000000000000000001-000001.jsonl", `{"seq":"a"}`+"\n")
	write("00000000000000000003-000003.jsonl", `{"seq":"c"}`+"\n")

	sink := &fakeSink{}
	n, err := ReplayWAL(context.Background(), walDir, sink)
	if err != nil {
		t.Fatalf("重放失败: %v", err)
	}
	if n != 3 {
		t.Fatalf("应重放 3 批，实际 %d", n)
	}

	got := sink.allLines()
	want := []string{`{"seq":"a"}`, `{"seq":"b"}`, `{"seq":"c"}`}
	if len(got) != len(want) {
		t.Fatalf("重放行数 %d，期望 %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 行顺序错误: %s，期望 %s", i, got[i], want[i])
		}
	}

	pending, _ := PendingBatches(walDir)
	if pending != 0 {
		t.Fatalf("重放成功后目录应为空，实际剩 %d", pending)
	}
}

func TestReplayWALStopsAndKeepsOnFailure(t *testing.T) {
	walDir := t.TempDir()
	for _, n := range []string{"00000000000000000001-000001.jsonl", "00000000000000000002-000002.jsonl"} {
		if err := os.WriteFile(filepath.Join(walDir, n), []byte(`{"x":1}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := ReplayWAL(context.Background(), walDir, &fakeSink{fail: true})
	if err == nil {
		t.Fatal("目标不可用时应返回错误")
	}

	// 失败不能删除文件，否则数据永久丢失
	pending, _ := PendingBatches(walDir)
	if pending != 2 {
		t.Fatalf("失败时应保留全部批次，实际剩 %d", pending)
	}
}

func TestReplayWALMissingDirIsNoop(t *testing.T) {
	n, err := ReplayWAL(context.Background(), filepath.Join(t.TempDir(), "nope"), &fakeSink{})
	if err != nil || n != 0 {
		t.Fatalf("目录不存在应静默返回，实际 n=%d err=%v", n, err)
	}
}

func TestReplayWALNilSink(t *testing.T) {
	if _, err := ReplayWAL(context.Background(), t.TempDir(), nil); err == nil {
		t.Fatal("sink 为 nil 应报错")
	}
}

// 端到端：断库 1000 条全部落盘，恢复后一条不少地重放回来。
func TestWALEndToEnd1000(t *testing.T) {
	walDir := t.TempDir()
	inner := &fakeSink{fail: true}
	s, err := NewWALSink(inner, walDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	const total = 1000
	const batch = 100
	for i := 0; i < total/batch; i++ {
		if err := s.WriteBatch(context.Background(), lines(i*batch, batch)); err != nil {
			t.Fatalf("第 %d 批落盘失败: %v", i, err)
		}
	}

	pending, _ := PendingBatches(walDir)
	if pending != total/batch {
		t.Fatalf("应有 %d 个批次，实际 %d", total/batch, pending)
	}

	// "恢复"：换一个可用的 sink 重放
	restored := &fakeSink{}
	n, err := ReplayWAL(context.Background(), walDir, restored)
	if err != nil {
		t.Fatalf("重放失败: %v", err)
	}
	if n != total/batch {
		t.Fatalf("应重放 %d 批，实际 %d", total/batch, n)
	}
	if got := len(restored.allLines()); got != total {
		t.Fatalf("重放后应有 %d 条，实际 %d", total, got)
	}

	// event_id 不应重复（顺序由 WAL 文件名保证，内部按批写入）
	ids := make([]string, 0, total)
	for _, l := range restored.allLines() {
		ids = append(ids, l)
	}
	sort.Strings(ids)
	dup := 0
	for i := 1; i < len(ids); i++ {
		if ids[i] == ids[i-1] {
			dup++
		}
	}
	if dup != 0 {
		t.Fatalf("重放出现 %d 条重复", dup)
	}
}
