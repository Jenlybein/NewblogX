package log_service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	platformlog "myblogx/internal/platform/log"
)

// 编译期确认满足平台日志的 Sink 契约。
var _ platformlog.Sink = (*dailyFileSink)(nil)

// dailyFileSink 把一批 JSON 行追加到「根目录/类别/日期/应用.log」。
//
// 这就是 L.2 之前 jsonLineSink 的落点与命名规则，一个字节都没改：
// 未启用 ClickHouse 时，落到磁盘上的内容与改造前完全一致，fluent-bit
// 这类外部采集器不需要任何调整（关掉它是 L.5 的事）。
type dailyFileSink struct {
	rootDir string
	dirName string
	appName string

	mu   sync.Mutex
	file *os.File
	date string
	// lastRoot/lastApp 记录上次解析出的落点，配置变更时触发切换。
	lastRoot string
	lastApp  string
	closed   bool
}

// newDailyFileSink 构造一个按日期切割的文件出口。
func newDailyFileSink(rootDir, dirName, appName string) *dailyFileSink {
	return &dailyFileSink{rootDir: rootDir, dirName: dirName, appName: appName}
}

// WriteBatch 把一批行合并成一次写调用。
// 批量写是这套管道的收益之一：原来每条日志一次 Write，现在最多每秒一次。
func (s *dailyFileSink) WriteBatch(_ context.Context, lines [][]byte) error {
	if len(lines) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return platformlog.ErrSinkClosed
	}

	file, err := s.ensureFile(time.Now())
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.Grow(len(lines) * 256)
	for _, line := range lines {
		buf.Write(line)
		buf.WriteByte('\n')
	}

	_, err = file.Write(buf.Bytes())
	return err
}

// Close 关闭当前文件句柄，重复调用安全。
func (s *dailyFileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		return err
	}
	return nil
}

// ensureFile 确保当前日期对应的文件已打开，并在跨天/换目录/换应用名时切换。
// 调用方必须持有 s.mu。
func (s *dailyFileSink) ensureFile(now time.Time) (*os.File, error) {
	date := now.Format("2006-01-02")
	if s.file != nil && s.date == date && s.lastRoot == s.rootDir && s.lastApp == s.appName {
		return s.file, nil
	}

	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}

	dir := filepath.Join(s.rootDir, s.dirName, date)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, err
	}

	filename := filepath.Join(dir, s.appName+".log")
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	// 显式 chmod：OpenFile 的权限位受 umask 影响，采集器需要稳定的 0644。
	if chmodErr := os.Chmod(filename, 0o644); chmodErr != nil {
		_ = file.Close()
		return nil, chmodErr
	}

	s.file = file
	s.date = date
	s.lastRoot = s.rootDir
	s.lastApp = s.appName
	return s.file, nil
}

// touchDailyFile 预先创建当天的日志文件，避免采集器在文件尚未出现时持续告警。
//
// 这里用的是全新的一次性 sink，没有并发访问，因此不加锁、直接关掉即走。
func touchDailyFile(rootDir, dirName, appName string, now time.Time) error {
	sink := newDailyFileSink(rootDir, dirName, appName)
	if _, err := sink.ensureFile(now); err != nil {
		return err
	}
	return sink.Close()
}
