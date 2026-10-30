package log

import (
	"fmt"
	"os"
	"time"
)

// 默认缓冲参数。数值来自 docs/architecture/governance.md §4.3 的设计：条目 8192、字节 8MB、
// 批量 500、间隔 1s。它们共同决定“内存上限”与“刷盘延迟”的折中。
const (
	DefaultBufferEntries = 8192
	DefaultMaxBytes      = int64(8 << 20)
	DefaultFlushInterval = time.Second
	DefaultBatchSize     = 500

	// DefaultTimeFormat 与现有 logrus 输出保持一致（L.2 要求“输出与现有一致”）。
	// 需要更高精度时可显式覆盖。
	DefaultTimeFormat = "2006-01-02 15:04:05.000"
)

// Options 是管道的构造参数。零值可用，缺省项会被补全。
type Options struct {
	Service    string // 服务名，写入每条日志
	Env        string // 环境名，如 dev / prod
	InstanceID string // 实例标识，默认 hostname-pid
	Level      Level  // 最低输出级别

	BufferEntries int           // 队列条目上限
	MaxBytes      int64         // 队列字节预算
	FlushInterval time.Duration // 定时刷盘间隔
	BatchSize     int           // 单批最大条数

	// TimeFormat 是 ts 字段的格式，零值使用 DefaultTimeFormat。
	TimeFormat string
}

// withDefaults 补全零值。
func (o Options) withDefaults() Options {
	if o.BufferEntries <= 0 {
		o.BufferEntries = DefaultBufferEntries
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultMaxBytes
	}
	if o.FlushInterval <= 0 {
		o.FlushInterval = DefaultFlushInterval
	}
	if o.BatchSize <= 0 {
		o.BatchSize = DefaultBatchSize
	}
	if o.BatchSize > o.BufferEntries {
		o.BatchSize = o.BufferEntries
	}
	if o.InstanceID == "" {
		o.InstanceID = defaultInstanceID()
	}
	if o.Env == "" {
		o.Env = "dev"
	}
	if o.TimeFormat == "" {
		o.TimeFormat = DefaultTimeFormat
	}
	return o
}

// defaultInstanceID 生成进程级实例标识：零 DB 调用、零网络。
func defaultInstanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}
