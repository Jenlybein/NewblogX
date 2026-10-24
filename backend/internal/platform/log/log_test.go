package log_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"myblogx/internal/platform/log"
)

// memSink 在内存里收集编码后的行，供断言使用。
type memSink struct {
	mu     sync.Mutex
	lines  [][]byte
	closed bool
}

func (m *memSink) WriteBatch(_ context.Context, lines [][]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range lines {
		cp := make([]byte, len(l))
		copy(cp, l)
		m.lines = append(m.lines, cp)
	}
	return nil
}

func (m *memSink) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *memSink) Lines() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]byte, len(m.lines))
	copy(out, m.lines)
	return out
}

// blockingSink 阻塞在 WriteBatch，用于制造“队列写不出去”的场景。
type blockingSink struct {
	release chan struct{}
	started chan struct{}
	once    sync.Once
}

func newBlockingSink() *blockingSink {
	return &blockingSink{release: make(chan struct{}), started: make(chan struct{})}
}

func (b *blockingSink) WriteBatch(ctx context.Context, _ [][]byte) error {
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *blockingSink) Close() error { return nil }

// 全部字段类型都要按预期编码。
func TestEncodeAllFieldTypes(t *testing.T) {
	t.Parallel()

	sink := &memSink{}
	em := log.New(log.Options{
		Service:       "svc",
		Env:           "test",
		Level:         log.LevelDebug,
		BufferEntries: 64,
		BatchSize:     64,
		FlushInterval: time.Hour,
	}, sink)

	ts := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	em.Emit(context.Background(), log.Event{
		Base: log.Base{
			TS:        ts,
			Level:     log.LevelError,
			Kind:      log.KindAudit,
			EventName: "article.delete",
			ErrorCode: "E_CONFLICT",
			ErrorMsg:  "conflict",
			UserID:    42,
			IP:        "10.0.0.1",
			TraceID:   "trace-1",
			RequestID: "req-1",
		},
		Message: "deleted",
		Fields: []log.Field{
			log.Str("s", "v"),
			log.Int("i", -7),
			log.Uint("u", 8),
			log.Float("f", 1.5),
			log.Bool("b", true),
			log.Dur("d", 1500*time.Millisecond),
			log.Time("t", ts),
			log.Strs("ss", []string{"a", "b"}),
		},
	})

	if err := em.Close(context.Background()); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	lines := sink.Lines()
	if len(lines) != 1 {
		t.Fatalf("期望 1 行，实际 %d", len(lines))
	}

	var got map[string]any
	if err := json.Unmarshal(lines[0], &got); err != nil {
		t.Fatalf("输出不是合法 JSON: %v\n%s", err, lines[0])
	}

	checks := map[string]any{
		"service":    "svc",
		"env":        "test",
		"level":      "error",
		"kind":       "audit",
		"event":      "article.delete",
		"error_code": "E_CONFLICT",
		"message":    "deleted",
		"s":          "v",
		"i":          float64(-7),
		"u":          float64(8),
		"f":          1.5,
		"b":          true,
		"d":          float64(1500),
		"user_id":    float64(42),
		"ip":         "10.0.0.1",
	}
	for k, want := range checks {
		if got[k] != want {
			t.Errorf("字段 %q 期望 %v，实际 %v", k, want, got[k])
		}
	}

	if got["ts"] != ts.Format(log.DefaultTimeFormat) {
		t.Errorf("ts 不符: %v", got["ts"])
	}
	if st, ok := got["ss"].([]any); !ok || len(st) != 2 || st[0] != "a" {
		t.Errorf("ss 不符: %v", got["ss"])
	}
	if _, ok := got["event_id"]; !ok {
		t.Error("缺少 event_id")
	}
}

// 队列写不出去时，Emit 必须立刻返回并计数丢弃，不能阻塞调用方。
func TestEmitNeverBlocksWhenQueueFull(t *testing.T) {
	t.Parallel()

	sink := newBlockingSink()
	em := log.New(log.Options{
		BufferEntries: 2,
		MaxBytes:      1 << 20,
		BatchSize:     2,
		FlushInterval: time.Hour,
	}, sink)

	// 先把写协程卡在第一批上
	em.Emit(context.Background(), log.Event{Base: log.Base{Level: log.LevelInfo}, Message: "a"})
	em.Emit(context.Background(), log.Event{Base: log.Base{Level: log.LevelInfo}, Message: "b"})
	select {
	case <-sink.started:
	case <-time.After(2 * time.Second):
		t.Fatal("写协程未开始处理")
	}

	start := time.Now()
	for i := 0; i < 5000; i++ {
		em.Emit(context.Background(), log.Event{Base: log.Base{Level: log.LevelInfo}, Message: "flood"})
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("队列满时 Emit 阻塞了 %v", elapsed)
	}

	st := em.Stats()
	if st.Dropped == 0 {
		t.Fatal("队列满时应当有丢弃计数")
	}

	close(sink.release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = em.Close(ctx)
}

// 字节预算超限时直接丢弃，不占内存。
func TestEmitRespectsByteBudget(t *testing.T) {
	t.Parallel()

	sink := &memSink{}
	em := log.New(log.Options{
		BufferEntries: 128,
		MaxBytes:      1, // 任何事件都超预算
		BatchSize:     128,
		FlushInterval: time.Hour,
	}, sink)

	for i := 0; i < 10; i++ {
		em.Emit(context.Background(), log.Event{Base: log.Base{Level: log.LevelInfo}, Message: "x"})
	}

	st := em.Stats()
	if st.Emitted != 0 || st.Dropped != 10 {
		t.Fatalf("字节预算未生效: %+v", st)
	}
	if st.BufferedBytes != 0 {
		t.Fatalf("预算泄漏: %d", st.BufferedBytes)
	}
}

// Close 必须把队列里剩余条目全部刷出去。
func TestCloseDrains(t *testing.T) {
	t.Parallel()

	sink := &memSink{}
	em := log.New(log.Options{
		BufferEntries: 1024,
		BatchSize:     100,
		FlushInterval: time.Hour,
	}, sink)

	const n = 350
	for i := 0; i < n; i++ {
		em.Emit(context.Background(), log.Event{Base: log.Base{Level: log.LevelInfo}, Message: "m"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := em.Close(ctx); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	if got := len(sink.Lines()); got < n {
		t.Fatalf("drain 不完整：期望至少 %d 行，实际 %d", n, got)
	}
	if st := em.Stats(); st.Written < n {
		t.Fatalf("written 统计不符: %+v", st)
	}
	if !sink.closed {
		t.Fatal("Close 应关闭 sink")
	}
}

// 低于阈值的级别不输出。
func TestLevelFilter(t *testing.T) {
	t.Parallel()

	sink := &memSink{}
	em := log.New(log.Options{Level: log.LevelWarn, BufferEntries: 16, BatchSize: 16, FlushInterval: time.Hour}, sink)

	em.Emit(context.Background(), log.Event{Base: log.Base{Level: log.LevelInfo}, Message: "info"})
	em.Emit(context.Background(), log.Event{Base: log.Base{Level: log.LevelError}, Message: "err"})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = em.Close(ctx)

	lines := sink.Lines()
	if len(lines) != 1 {
		t.Fatalf("期望只输出 1 条（error），实际 %d 条", len(lines))
	}
	if st := em.Stats(); st.Emitted != 1 {
		t.Fatalf("emitted 统计不符: %+v", st)
	}
}

// 并发投递：计数自洽（emitted + dropped = 总尝试数），且 -race 下无竞争。
func TestConcurrentEmitAccounting(t *testing.T) {
	t.Parallel()

	sink := &memSink{}
	em := log.New(log.Options{
		BufferEntries: 4096,
		MaxBytes:      32 << 20,
		BatchSize:     200,
		FlushInterval: 10 * time.Millisecond,
	}, sink)

	const workers, perWorker = 8, 500
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				em.Emit(context.Background(), log.Event{
					Base:    log.Base{Level: log.LevelInfo},
					Message: "m",
					Fields:  []log.Field{log.Int("worker", int64(id))},
				})
			}
		}(w)
	}
	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := em.Close(ctx); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	st := em.Stats()
	total := uint64(workers * perWorker)
	if st.Emitted+st.Dropped != total {
		t.Fatalf("计数不自洽: emitted=%d dropped=%d total=%d", st.Emitted, st.Dropped, total)
	}
	if st.Written != st.Emitted {
		t.Fatalf("正常 sink 下 written 应等于 emitted: %+v", st)
	}
	if got := len(sink.Lines()); uint64(got) != st.Written {
		t.Fatalf("落盘行数 %d 与 written %d 不符", got, st.Written)
	}
}

// 文件 sink：一行一条 JSON，采集器可直接按行读取（L.2 的落盘出口）。
func TestFileSinkWritesJSONLines(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/app.log"
	sink, err := log.NewFileSink(path, 0o644)
	if err != nil {
		t.Fatalf("创建文件 sink 失败: %v", err)
	}

	em := log.New(log.Options{BufferEntries: 64, BatchSize: 10, FlushInterval: time.Hour}, sink)
	const n = 25
	for i := 0; i < n; i++ {
		em.Emit(context.Background(), log.Event{Base: log.Base{Level: log.LevelInfo}, Message: "line"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := em.Close(ctx); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("期望 %d 行，实际 %d", n, len(lines))
	}
	for i, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("第 %d 行不是合法 JSON: %v\n%s", i+1, err, line)
		}
		if m["message"] != "line" {
			t.Fatalf("第 %d 行 message 不符: %v", i+1, m["message"])
		}
	}
}

// EmitLine 投递的是调用方编码好的整行，管道不得改写、不得重新编码。
func TestEmitLinePassesThroughUntouched(t *testing.T) {
	t.Parallel()

	sink := &memSink{}
	em := log.New(log.Options{BufferEntries: 64, BatchSize: 2, FlushInterval: time.Hour}, sink)

	raw := []byte(`{"event_id":7,"ts":"2026-10-24 10:00:00.000","action_name":"create"}`)
	em.EmitLine(context.Background(), log.LevelInfo, raw)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := em.Close(ctx); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	lines := sink.Lines()
	if len(lines) != 1 {
		t.Fatalf("期望 1 行，实际 %d 行", len(lines))
	}
	if string(lines[0]) != string(raw) {
		t.Fatalf("行内容被改写：\n期望 %s\n实际 %s", raw, lines[0])
	}
	if st := em.Stats(); st.Emitted != 1 || st.Written != 1 {
		t.Fatalf("统计不符: %+v", st)
	}
}

// EmitLine 同样受级别阈值约束，且空行直接丢弃。
func TestEmitLineLevelFilterAndEmpty(t *testing.T) {
	t.Parallel()

	sink := &memSink{}
	em := log.New(log.Options{Level: log.LevelWarn, BufferEntries: 16, BatchSize: 16, FlushInterval: time.Hour}, sink)

	em.EmitLine(context.Background(), log.LevelInfo, []byte(`{"a":1}`))
	em.EmitLine(context.Background(), log.LevelError, []byte(`{"a":2}`))
	em.EmitLine(context.Background(), log.LevelError, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = em.Close(ctx)

	lines := sink.Lines()
	if len(lines) != 1 {
		t.Fatalf("期望只输出 1 行，实际 %d 行", len(lines))
	}
	if string(lines[0]) != `{"a":2}` {
		t.Fatalf("输出行不符: %s", lines[0])
	}
}

// EmitLine 与 Emit 混用时应共享同一个有界队列与统计。
func TestEmitLineMixedWithEmit(t *testing.T) {
	t.Parallel()

	sink := &memSink{}
	em := log.New(log.Options{BufferEntries: 128, BatchSize: 50, FlushInterval: time.Hour}, sink)

	const n = 40
	for i := 0; i < n; i++ {
		em.Emit(context.Background(), log.Event{Base: log.Base{Level: log.LevelInfo}, Message: "typed"})
		em.EmitLine(context.Background(), log.LevelInfo, []byte(`{"kind":"line"}`))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := em.Close(ctx); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	if got := len(sink.Lines()); got != 2*n {
		t.Fatalf("期望 %d 行，实际 %d 行", 2*n, got)
	}
}
