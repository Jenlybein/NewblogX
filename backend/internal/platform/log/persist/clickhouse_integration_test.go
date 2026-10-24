//go:build integration

// L.4 验收：断 ClickHouse 写 1000 条 audit，恢复后 count==1000 且无重复。
//
// 运行方式（需先起 ClickHouse 并建表）：
//
//	docker run -d --name blogx-ch-it -p 18123:8123 -p 19000:9000 \
//	  -e CLICKHOUSE_DB=blogx_logs -e CLICKHOUSE_SKIP_USER_SETUP=1 \
//	  -v "<repo>/deploy/docker/clickhouse/init:/docker-entrypoint-initdb.d" \
//	  clickhouse/clickhouse-server:25.3
//
//	BLOGX_CH_ADDR=127.0.0.1:19000 \
//	  go test -tags integration -run Integration ./internal/platform/log/persist/
package persist

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
)

const auditTable = "blogx_logs.action_audit_logs"

func openCH(t *testing.T) *sql.DB {
	t.Helper()

	addr := os.Getenv("BLOGX_CH_ADDR")
	if addr == "" {
		addr = "127.0.0.1:19000"
	}

	db := clickhouse.OpenDB(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
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

// auditLines 造 n 条符合 action_audit_logs 列名的 JSON 行（event_id 从 start 递增）。
func auditLines(start, n int) [][]byte {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	out := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, []byte(fmt.Sprintf(
			`{"event_id":%d,"ts":"%s","level":"info","message":"audit","log_kind":"action_audit",`+
				`"service":"blogx","env":"test","instance_id":"1","action_name":"create",`+
				`"target_type":"article","success":1}`,
			start+i, ts)))
	}
	return out
}

func countDistinct(t *testing.T, db *sql.DB) uint64 {
	t.Helper()
	var n uint64
	if err := db.QueryRow("SELECT count(DISTINCT event_id) FROM " + auditTable).Scan(&n); err != nil {
		t.Fatalf("查询去重数量失败: %v", err)
	}
	return n
}

func countFinal(t *testing.T, db *sql.DB) uint64 {
	t.Helper()
	var n uint64
	if err := db.QueryRow("SELECT count() FROM " + auditTable + " FINAL").Scan(&n); err != nil {
		t.Fatalf("查询 FINAL 数量失败: %v", err)
	}
	return n
}

func TestClickHouseWALReplayIntegration(t *testing.T) {
	db := openCH(t)
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "TRUNCATE TABLE "+auditTable); err != nil {
		t.Fatalf("清表失败: %v", err)
	}

	walDir := t.TempDir()

	const total = 1000
	const batchSize = 100
	const batches = total / batchSize

	// ── 阶段 1：模拟断库（sink 的 db 为 nil，写入必失败）→ 全部落 WAL ──
	brokenSink, err := NewClickHouseSink(nil, auditTable)
	if err != nil {
		t.Fatal(err)
	}
	wal, err := NewWALSink(brokenSink, walDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	// 每批内容只生成一次：阶段 4 的"重复投递"必须复用同一份字节，
	// 否则 ts 不同会让 ORDER BY 元组不同，ReplacingMergeTree 就无法去重。
	allBatches := make([][][]byte, batches)
	for i := 0; i < batches; i++ {
		allBatches[i] = auditLines(i*batchSize, batchSize)
		if err := wal.WriteBatch(ctx, allBatches[i]); err != nil {
			t.Fatalf("断库期间第 %d 批落盘失败: %v", i, err)
		}
	}

	pending, err := PendingBatches(walDir)
	if err != nil {
		t.Fatal(err)
	}
	if pending != batches {
		t.Fatalf("断库期间应积压 %d 个 WAL 批次，实际 %d", batches, pending)
	}
	if got := countDistinct(t, db); got != 0 {
		t.Fatalf("断库期间不应有数据进 CH，实际 %d", got)
	}

	// ── 阶段 2：恢复（换成可用 db）→ 重放 WAL ──
	goodSink, err := NewClickHouseSink(db, auditTable)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplayWAL(ctx, walDir, goodSink)
	if err != nil {
		t.Fatalf("重放失败: %v", err)
	}
	if replayed != batches {
		t.Fatalf("应重放 %d 批，实际 %d", batches, replayed)
	}

	// ── 阶段 3：不丢 ──
	if got := countDistinct(t, db); got != total {
		t.Fatalf("恢复后应有 %d 条唯一事件，实际 %d", total, got)
	}
	if pending, _ := PendingBatches(walDir); pending != 0 {
		t.Fatalf("重放后 WAL 应清空，实际剩 %d", pending)
	}

	// ── 阶段 4：不重（模拟"写完 CH、还没删 WAL 就崩溃"后的重复投递）──
	dupDir := filepath.Join(t.TempDir(), "dup")
	if err := os.MkdirAll(dupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < batches; i++ {
		if err := walToFile(dupDir, i, allBatches[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ReplayWAL(ctx, dupDir, goodSink); err != nil {
		t.Fatalf("重复重放失败: %v", err)
	}

	// ReplacingMergeTree 按 ORDER BY（含 event_id）去重：FINAL 视图应仍是 1000
	if got := countFinal(t, db); got != total {
		t.Fatalf("重复投递后 FINAL 去重应为 %d，实际 %d", total, got)
	}
	if got := countDistinct(t, db); got != total {
		t.Fatalf("重复投递后唯一事件数应仍为 %d，实际 %d", total, got)
	}
}

// walToFile 手工造一个 WAL 批次文件（模拟上次崩溃残留）。
func walToFile(dir string, seq int, lines [][]byte) error {
	name := fmt.Sprintf("%020d-%06d.jsonl", time.Now().UnixNano()+int64(seq), seq)
	path := filepath.Join(dir, name)

	buf := make([]byte, 0, len(lines)*160)
	for _, l := range lines {
		buf = append(buf, l...)
		buf = append(buf, '\n')
	}
	return os.WriteFile(path, buf, 0o644)
}
