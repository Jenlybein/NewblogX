// Package persist 是日志的持久化出口：ClickHouse sink 与 WAL 兜底。
//
// 为什么单独成包：父包 log 只定义 Sink 抽象与内存/文件实现，不该被
// database/sql 这类重依赖污染；真正需要"落 ClickHouse 且不能丢"的场合，
// 在这里组合出「CH + WAL」的持久化链路。
package persist

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"

	platformlog "myblogx/internal/platform/log"
)

// 编译期确认满足父包的 Sink 契约。
var _ platformlog.Sink = (*ClickHouseSink)(nil)

// 表名白名单：只接受 [schema.]table 形态的标识符。
// 表名会拼进 SQL（ClickHouse 不支持表名占位符），必须挡住拼接注入。
var tableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// ClickHouseSink 把一批 JSON 行灌入 ClickHouse 的指定表。
//
// 关键约定：**JSON 字段名与表列名一致**。log_service 的 baseEvent 与各类
// 事件结构都是照 ClickHouse 表结构设计的（event_id / ts / level / ... /
// extra_json），所以这里走 `INSERT INTO <table> FORMAT JSONEachRow`，
// 不做逐列映射、不引入反射。
//
// 已知行为（实测 CH 25.3）：`input_format_skip_unknown_fields` 默认为 1，
// 因此
//   - JSON 里多出的字段会被**静默丢弃**，不会报错；
//   - JSON 里缺失的列取类型默认值（空串 / 0）；
//   - 类型不匹配（例如给 String 列传对象）会被序列化成 JSON 字符串。
//
// 也就是说"列名写错"不会响——这是 JSONEachRow 的固有语义，无法靠 INSERT
// 语句纠正。补这个缺口的是测试而不是运行时：`clickhouse_integration_test.go`
// 用 DESCRIBE TABLE 与各类事件结构体做对照，列名一漂移 CI 就会红。
// 我们把该设置显式写进连接（见 core/init_clickhouse.go），
// 不依赖服务端默认值，避免换版本后行为变化。
type ClickHouseSink struct {
	db    *sql.DB
	table string

	mu     sync.Mutex
	closed bool
}

// NewClickHouseSink 构造一个写入 table 的 sink。
// db 为 nil 时返回的 sink 在写入阶段返回错误（便于上层 WAL 接管），
// 不在构造期 panic——CH 未启用是合法状态。
func NewClickHouseSink(db *sql.DB, table string) (*ClickHouseSink, error) {
	if !tableNamePattern.MatchString(table) {
		return nil, fmt.Errorf("log/persist: 非法的 ClickHouse 表名 %q", table)
	}
	return &ClickHouseSink{db: db, table: table}, nil
}

// WriteBatch 把一批 JSON 行拼成一条 INSERT 语句写入。
//
// 用一次 Exec 提交整批，避免 N 次往返；lines 已经是编码好的 JSON 对象，
// ClickHouse 在 FORMAT JSONEachRow 模式下按行解析，不经过 SQL 词法分析。
func (s *ClickHouseSink) WriteBatch(ctx context.Context, lines [][]byte) error {
	if len(lines) == 0 {
		return nil
	}

	s.mu.Lock()
	closed := s.closed
	db := s.db
	s.mu.Unlock()

	if closed {
		return platformlog.ErrSinkClosed
	}
	if db == nil {
		return fmt.Errorf("log/persist: ClickHouse 未初始化")
	}

	// 1000 行 × 数百字节 ≈ 数百 KB，一次拼接的开销可接受，且只有一批。
	var b strings.Builder
	b.Grow(len(lines) * 256)
	b.WriteString("INSERT INTO ")
	b.WriteString(s.table)
	b.WriteString(" FORMAT JSONEachRow\n")
	for _, line := range lines {
		b.Write(line)
		b.WriteByte('\n')
	}

	_, err := db.ExecContext(ctx, b.String())
	return err
}

// Close 标记关闭。底层 *sql.DB 由装配方管理，这里不代为关闭——
// 同一个连接池通常被查询侧共用。
func (s *ClickHouseSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
