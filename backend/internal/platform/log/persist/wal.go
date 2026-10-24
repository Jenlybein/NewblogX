package persist

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	platformlog "myblogx/internal/platform/log"
)

var _ platformlog.Sink = (*WALSink)(nil)

// WALSink 给一个主出口加"本地 WAL 兜底"。
//
// 语义是 write-behind：先试主出口（通常是 ClickHouse），成功即返回；
// 失败时把这一批 JSON 行落到本地磁盘，等主出口恢复后再重放。
//
// 为什么不是 write-ahead：正常路径双写磁盘会让每次日志都吃一次 fsync，
// 代价不成比例；而"断库"场景下所有写都会失败，天然全部落盘，兜底效果一样。
//
// 落盘用「写 .tmp → fsync → rename」保证原子性：任何时刻磁盘上出现的
// *.jsonl 都是完整批次，不会出现"写了一半、重放时解析失败"的残缺文件。
type WALSink struct {
	inner   platformlog.Sink
	dir     string
	onError func(error)

	mu     sync.Mutex
	seq    uint64
	closed bool
}

// NewWALSink 构造 WAL 包装。dir 不存在时在首次落盘时创建。
// onError 可为 nil，用于把"主出口失败"上报给上层计数/告警，不影响写入流程。
func NewWALSink(inner platformlog.Sink, dir string, onError func(error)) (*WALSink, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("log/persist: WAL 目录不能为空")
	}
	return &WALSink{inner: inner, dir: dir, onError: onError}, nil
}

// WriteBatch 先试主出口，失败则落 WAL。
//
// 返回值只反映"是否被安全接收"：落 WAL 成功也算成功——数据没有丢，
// 只是延迟送达。只有连盘都写不进去时才返回错误。
func (s *WALSink) WriteBatch(ctx context.Context, lines [][]byte) error {
	if len(lines) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return platformlog.ErrSinkClosed
	}

	if s.inner != nil {
		if err := s.inner.WriteBatch(ctx, lines); err == nil {
			return nil
		} else if s.onError != nil {
			s.onError(err)
		}
	}

	return s.appendBatch(lines)
}

// appendBatch 原子落盘一批。
// 调用方已持有 s.mu。
func (s *WALSink) appendBatch(lines [][]byte) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("log/persist: 创建 WAL 目录失败: %w", err)
	}

	s.seq++
	finalName := fmt.Sprintf("%020d-%06d.jsonl", time.Now().UnixNano(), s.seq)
	finalPath := filepath.Join(s.dir, finalName)
	tmpPath := finalPath + ".tmp"

	var buf bytes.Buffer
	buf.Grow(len(lines) * 256)
	for _, line := range lines {
		buf.Write(line)
		buf.WriteByte('\n')
	}

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("log/persist: 创建 WAL 临时文件失败: %w", err)
	}

	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("log/persist: 写 WAL 失败: %w", err)
	}
	// fsync：否则进程崩溃时数据可能还在页缓存里，"不丢"就不成立。
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("log/persist: fsync WAL 失败: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("log/persist: 发布 WAL 失败: %w", err)
	}
	return nil
}

// Close 标记关闭（不再接收新批次）。未重放完的 WAL 文件保留在磁盘上，
// 由下次启动的 ReplayWAL 处理。
func (s *WALSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.inner != nil {
		return s.inner.Close()
	}
	return nil
}

// ReplayWAL 把 WAL 目录里的积压批次按时间顺序重新灌回 sink。
//
// 返回成功重放的批次数。任一批次失败即停止并保留剩余文件——宁可少放，
// 不可跳过：跳过会在日志链路上留下永久空洞。
//
// 幂等性：如果在"主出口已写成功、但文件还没删除"的瞬间崩溃，重放会造成
// 重复写入。这依赖 ClickHouse 侧的去重（ReplacingMergeTree + event_id），
// 所以 log 表引擎必须是 ReplacingMergeTree 而非 MergeTree。
func ReplayWAL(ctx context.Context, dir string, sink platformlog.Sink) (int, error) {
	if sink == nil {
		return 0, fmt.Errorf("log/persist: 重放目标 sink 为空")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			names = append(names, e.Name())
		}
	}
	// 文件名前缀是 20 位 unixnano，字典序即时间序。
	sort.Strings(names)

	replayed := 0
	for _, name := range names {
		path := filepath.Join(dir, name)

		lines, err := readWALBatch(path)
		if err != nil {
			return replayed, fmt.Errorf("log/persist: 读取 %s 失败: %w", name, err)
		}

		if len(lines) > 0 {
			if err := sink.WriteBatch(ctx, lines); err != nil {
				return replayed, fmt.Errorf("log/persist: 重放 %s 失败: %w", name, err)
			}
		}

		if err := os.Remove(path); err != nil {
			return replayed, fmt.Errorf("log/persist: 删除已重放的 %s 失败: %w", name, err)
		}
		replayed++
	}

	return replayed, nil
}

// readWALBatch 读一个批次文件，按行切分（空行忽略）。
func readWALBatch(path string) ([][]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	raw := bytes.Split(data, []byte{'\n'})
	lines := make([][]byte, 0, len(raw))
	for _, line := range raw {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// PendingBatches 返回 WAL 目录里待重放的批次数，用于启动时判断是否需要重放、
// 以及健康检查暴露积压量。
func PendingBatches(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			n++
		}
	}
	return n, nil
}
