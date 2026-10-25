package log

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestEmitLineDropsOversized 超限的整行被丢弃并计入 Oversized。
//
// 关键不是"能不能丢"，而是**不能截断**：日志行是 JSON，
// 截断后既进不了 ClickHouse 的 JSONEachRow，也会让采集器整批报错。
func TestEmitLineDropsOversized(t *testing.T) {
	sink := &recordingSink{}
	e := New(Options{FlushInterval: 5 * time.Millisecond, BatchSize: 10}, sink)

	huge := []byte(`{"msg":"` + strings.Repeat("x", MaxEventBytes) + `"}`)
	e.EmitLine(context.Background(), LevelInfo, huge)

	// 再发一条正常大小的，确认丢弃没有影响后续写入。
	ok := []byte(`{"msg":"ok"}`)
	e.EmitLine(context.Background(), LevelInfo, ok)

	closeEmitter(t, e)

	if got := sink.lines(); len(got) != 1 {
		t.Fatalf("应当只写入 1 条（超大那条被丢），实际 %d 条", len(got))
	}
	if sink.got[0] != string(ok) {
		t.Fatalf("写入内容不对: %s", sink.got[0])
	}

	st := e.Stats()
	if st.Oversized != 1 {
		t.Fatalf("Oversized 计数应为 1，实际 %d", st.Oversized)
	}
	if st.Dropped != 1 {
		t.Fatalf("Dropped 也应 +1（对调用方而言就是丢了），实际 %d", st.Dropped)
	}
}

// TestEmitDropsOversizedEvent Event 路径同样受上限约束。
//
// 这条路径的行由 encodeEvent 生成，超限只能在编码后才发现 ——
// 它守的是"Fields 里塞了巨大字符串"这种情况。
func TestEmitDropsOversizedEvent(t *testing.T) {
	sink := &recordingSink{}
	e := New(Options{FlushInterval: 5 * time.Millisecond, BatchSize: 10}, sink)

	e.Emit(context.Background(), Event{
		Base:    Base{Level: LevelInfo},
		Message: "big",
		Fields:  []Field{Str("blob", strings.Repeat("y", MaxEventBytes))},
	})
	e.Emit(context.Background(), Event{
		Base:    Base{Level: LevelInfo},
		Message: "small",
	})

	closeEmitter(t, e)

	if got := sink.lines(); len(got) != 1 {
		t.Fatalf("应当只写入 1 条，实际 %d 条: %v", len(got), got)
	}
	if !strings.Contains(sink.got[0], "small") {
		t.Fatalf("留下的应当是那条小的: %s", sink.got[0])
	}
	if st := e.Stats(); st.Oversized != 1 {
		t.Fatalf("Oversized 计数应为 1，实际 %d", st.Oversized)
	}
}

// closeEmitter 收尾，失败直接终止测试。
func closeEmitter(t *testing.T, e *Emitter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := e.Close(ctx); err != nil {
		t.Fatalf("关闭 emitter 失败: %v", err)
	}
}

// recordingSink 记录写入的行。
type recordingSink struct {
	got []string
}

func (s *recordingSink) WriteBatch(_ context.Context, lines [][]byte) error {
	for _, line := range lines {
		s.got = append(s.got, string(line))
	}
	return nil
}

func (s *recordingSink) Close() error { return nil }

func (s *recordingSink) lines() []string { return s.got }
