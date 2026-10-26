package log

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync"
)

// ErrSinkClosed 表示向已关闭的 sink 写入。
var ErrSinkClosed = errors.New("log: sink already closed")

// Sink 是日志的最终去向（文件、ClickHouse、内存缓冲等）。
//
// WriteBatch 一次接收一批已编码好的 JSON 行；实现方应尽量一次性写入，
// 减少系统调用。返回 error 由 Emitter 计数，不影响调用方业务。
type Sink interface {
	WriteBatch(ctx context.Context, lines [][]byte) error
	Close() error
}

// WriterSink 把 JSON 行写入任意 io.Writer，每行一条，末尾带 '\n'。
//
// 这是 L.2 的文件出口；L.4 的 ClickHouse sink 走同一个接口。
type WriterSink struct {
	mu     sync.Mutex
	w      io.Writer
	closer io.Closer
	closed bool
}

// NewFileSink 以追加方式打开文件并返回 sink。
func NewFileSink(path string, perm os.FileMode) (*WriterSink, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, perm)
	if err != nil {
		return nil, err
	}
	return &WriterSink{w: f, closer: f}, nil
}

// WriteBatch 把一批行合并成一次写入。
func (s *WriterSink) WriteBatch(_ context.Context, lines [][]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.w == nil {
		return ErrSinkClosed
	}
	if len(lines) == 0 {
		return nil
	}

	var buf bytes.Buffer
	buf.Grow(len(lines) * 128)
	for _, line := range lines {
		buf.Write(line)
		buf.WriteByte('\n')
	}

	_, err := s.w.Write(buf.Bytes())
	return err
}

// Close 关闭底层文件（如果可关闭）。重复调用安全。
func (s *WriterSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}
