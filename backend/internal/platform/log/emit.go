package log

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"myblogx/internal/platform/safego"
)

// record 是入队的内部包装，size 在 Emit 时算好，出队时直接扣减预算。
type record struct {
	ev   Event
	size int64
}

// Emitter 是有界异步日志投递器。
//
// 生命周期：New → Emit(...)*n → Close。Close 之后再 Emit 会被静默丢弃（不会 panic）。
type Emitter struct {
	opts Options
	sink Sink

	ch   chan record
	stop chan struct{}
	done chan struct{}

	closed   atomic.Bool
	stopOnce sync.Once
	doneOnce sync.Once
	sinkOnce sync.Once
	sinkErr  error

	seq      atomic.Uint64
	buffered atomic.Int64

	emitted   atomic.Uint64
	dropped   atomic.Uint64
	written   atomic.Uint64
	batches   atomic.Uint64
	writeErrs atomic.Uint64
}

// New 构造并启动写协程。sink 为 nil 时只统计、不输出（用于压测与单测）。
func New(opts Options, sink Sink) *Emitter {
	o := opts.withDefaults()
	e := &Emitter{
		opts: o,
		sink: sink,
		ch:   make(chan record, o.BufferEntries),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	// 写协程 panic 不能让日志管道无声消失：记一次写错误并收尾。
	safego.Go(func(any, []byte) {
		e.writeErrs.Add(1)
		e.finish()
	}, e.run)

	return e
}

// Emit 投递一条日志。
//
// 这是请求路径上唯一的日志调用：只做级别判断、字段补全、大小估算和一次非阻塞入队。
// 队列满或字节预算超限时丢弃并计数，绝不阻塞调用方。
//
// 契约：ev.Fields 的底层数组在 Emit 返回后**不得再被修改或复用**。
// 这是为了不做防御性拷贝（避免每次投递都分配），与 zap/logrus 的约定一致。
// 需要复用同一个切片时，请每次传新的切片或先复制。
func (e *Emitter) Emit(ctx context.Context, ev Event) {
	if e == nil || e.closed.Load() {
		return
	}
	if !ev.Level.enabled(e.opts.Level) {
		return
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			e.dropped.Add(1)
			return
		default:
		}
	}

	if ev.TS.IsZero() {
		ev.TS = time.Now()
	}
	if ev.Service == "" {
		ev.Service = e.opts.Service
	}
	if ev.Env == "" {
		ev.Env = e.opts.Env
	}
	if ev.InstanceID == "" {
		ev.InstanceID = e.opts.InstanceID
	}
	if ev.EventID == 0 {
		ev.EventID = e.seq.Add(1)
	}

	sz := ev.size()
	if e.buffered.Load()+sz > e.opts.MaxBytes {
		e.dropped.Add(1)
		return
	}
	e.buffered.Add(sz)

	select {
	case e.ch <- record{ev: ev, size: sz}:
		e.emitted.Add(1)
	default:
		// 队列满：退还预算并计损
		e.buffered.Add(-sz)
		e.dropped.Add(1)
	}
}

// Stats 返回管道健康快照，供 /healthz 与指标导出使用。
type Stats struct {
	Emitted       uint64
	Dropped       uint64
	Written       uint64
	Batches       uint64
	WriteErrors   uint64
	QueueDepth    int
	BufferedBytes int64
}

// Stats 取当前统计。
func (e *Emitter) Stats() Stats {
	if e == nil {
		return Stats{}
	}
	return Stats{
		Emitted:       e.emitted.Load(),
		Dropped:       e.dropped.Load(),
		Written:       e.written.Load(),
		Batches:       e.batches.Load(),
		WriteErrors:   e.writeErrs.Load(),
		QueueDepth:    len(e.ch),
		BufferedBytes: e.buffered.Load(),
	}
}

// Close 停止接收新日志，把队列里剩余的条目刷完，然后关闭 sink。
// ctx 控制等待上限；超时返回 ctx.Err()，不阻塞进程退出。
func (e *Emitter) Close(ctx context.Context) error {
	if e == nil {
		return nil
	}
	e.closed.Store(true)
	e.stopOnce.Do(func() { close(e.stop) })

	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-e.done:
	case <-ctx.Done():
		return ctx.Err()
	}

	if e.sink == nil {
		return nil
	}
	e.sinkOnce.Do(func() { e.sinkErr = e.sink.Close() })
	return e.sinkErr
}

// run 是唯一的写协程：批量聚合、定时刷盘、退出前 drain。
func (e *Emitter) run() {
	ticker := time.NewTicker(e.opts.FlushInterval)
	defer ticker.Stop()
	defer e.finish()

	batch := make([]record, 0, e.opts.BatchSize)

	for {
		select {
		case rec := <-e.ch:
			batch = append(batch, rec)
			e.buffered.Add(-rec.size)
			if len(batch) >= e.opts.BatchSize {
				e.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				e.flush(batch)
				batch = batch[:0]
			}
		case <-e.stop:
			e.drain(&batch)
			if len(batch) > 0 {
				e.flush(batch)
			}
			return
		}
	}
}

// drain 把当前队列里剩余条目全部取到 batch，直到队列为空。
func (e *Emitter) drain(batch *[]record) {
	for {
		select {
		case rec := <-e.ch:
			*batch = append(*batch, rec)
			e.buffered.Add(-rec.size)
			if len(*batch) >= e.opts.BatchSize {
				e.flush(*batch)
				*batch = (*batch)[:0]
			}
		default:
			return
		}
	}
}

// flush 在写协程里序列化并交给 sink。这里的 json 编码发生在后台，不在请求路径上。
func (e *Emitter) flush(batch []record) {
	if len(batch) == 0 {
		return
	}
	if e.sink == nil {
		e.batches.Add(1)
		e.written.Add(uint64(len(batch)))
		return
	}

	lines := make([][]byte, len(batch))
	buf := make([]byte, 0, 256)
	for i := range batch {
		buf = encodeEvent(buf[:0], &batch[i].ev, e.opts.TimeFormat)
		line := make([]byte, len(buf))
		copy(line, buf)
		lines[i] = line
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	e.batches.Add(1)
	if err := e.sink.WriteBatch(ctx, lines); err != nil {
		e.writeErrs.Add(1)
		return
	}
	e.written.Add(uint64(len(lines)))
}

// finish 保证 done 只关一次（正常退出与 panic 退出都会走到）。
func (e *Emitter) finish() {
	e.doneOnce.Do(func() { close(e.done) })
}
