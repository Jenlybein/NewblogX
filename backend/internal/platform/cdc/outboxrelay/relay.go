// Package outboxrelay 投递事务型发件箱（outbox_event_models）里的事件。
//
// 职责边界：
//   - 写侧（article）只负责在业务事务里 INSERT 一条 pending 事件行，不做任何分发；
//   - 本包负责把 pending 事件投递给订阅者。
//
// 这样"业务提交"与"副作用"解耦，换来三个性质：
//   - 进程崩溃不会丢事件（事件已随业务一起落库）；
//   - 下游失败可以重试，重试耗尽进死信队列；
//   - 下游不再与业务强耦合（不再是"通知发不出去就回滚整篇文章发布"）。
//
// 两条摄入路径：
//  1. CDC（低延迟）：river 订阅 outbox 表的 binlog INSERT，调 EnqueueID 入队；
//  2. 补偿扫描（兜底）：本包周期性扫描 pending/failed 且到点的行。
//     有它在，即使 canal/river 没启用，事件照样能投递出去。
//
// 幂等：投递语义是 at-least-once（崩溃重放、扫描重复命中、binlog 重放
// 都可能让同一条事件被投递多次），所以订阅者必须幂等。
// 中继侧另外用"原子领取"（条件 UPDATE）保证同一时刻只有一个 worker
// 能领到某一行，避免并发下的重复处理。
package outboxrelay

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/safego"
	"myblogx/models"
	"myblogx/models/ctype"
)

// Event 是一条待投递事件（落库形态）。
type Event struct {
	ID            ctype.ID
	EventName     string
	AggregateType string
	AggregateID   uint64
	Payload       string
	// Attempts 本次是第几次尝试（从 1 开始）。
	Attempts int
}

// Subscriber 处理一种事件。
//
// 必须幂等：中继是 at-least-once，同一条事件可能被投递多次。
// 返回 error 表示失败，会按退避重试，耗尽后进死信队列。
type Subscriber func(ctx context.Context, e Event) error

// Options 是中继参数；零值由 withDefaults 兜底，便于调用方只写关心的项。
type Options struct {
	// Workers 并发投递的 worker 数。
	Workers int
	// QueueSize 有界队列容量：满了就丢弃入队（靠补偿扫描捞回），
	// 避免下游变慢时把内存撑爆或把 canal 事件循环堵死。
	QueueSize int
	// MaxAttempts 最大尝试次数，超过即标记 dead 并写死信。
	MaxAttempts int
	// BackoffMS 退避基数（毫秒）；第 n 次失败后等待 BackoffMS*n。
	BackoffMS int
	// SweepInterval 补偿扫描间隔。
	SweepInterval time.Duration
	// SweepLimit 单次扫描最多取多少条。
	SweepLimit int
	// LeaseDuration 处理租约时长。
	//
	// worker 领取后状态变成 processing；若它在 markDone 之前崩溃，
	// 这行会一直卡在 processing。租约到期后补偿扫描会把这些行重新捞回，
	// 否则崩溃一次就永久丢一条事件。
	LeaseDuration time.Duration

	// RetentionDone 已完成（done）事件行保留时长，超期物理删除。
	//
	// done 的行已经投递过了，再留着没有投递价值，只占磁盘。
	// 不清理的话，每篇文章的创建/发布/点赞/收藏都会沉淀一行，长期只增不减。
	RetentionDone time.Duration
	// RetentionDead 死信（dead）事件行保留时长，通常比 done 长，便于事后排查。
	//
	// 死信在标记 dead 时已经同时写入 cdc_dead_letter_models，
	// 所以这里过期后物理删除是安全的：证据还在死信表里。
	RetentionDead time.Duration
	// CleanupInterval 保留期清理的检查间隔。
	CleanupInterval time.Duration
	// CleanupLimit 单批删除行数上限。
	//
	// 为什么要分批：一次 DELETE 几十万行会长时间持锁、把 undo log 撑爆。
	// 分批删能把每批的影响控制在可控范围。
	CleanupLimit int
}

func (o Options) withDefaults() Options {
	if o.Workers <= 0 {
		o.Workers = 2
	}
	if o.QueueSize <= 0 {
		o.QueueSize = 1024
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 5
	}
	if o.BackoffMS <= 0 {
		o.BackoffMS = 1000
	}
	if o.SweepInterval <= 0 {
		o.SweepInterval = 5 * time.Second
	}
	if o.SweepLimit <= 0 {
		o.SweepLimit = 100
	}
	if o.LeaseDuration <= 0 {
		o.LeaseDuration = 2 * time.Minute
	}
	if o.RetentionDone <= 0 {
		o.RetentionDone = 7 * 24 * time.Hour
	}
	if o.RetentionDead <= 0 {
		o.RetentionDead = 30 * 24 * time.Hour
	}
	if o.CleanupInterval <= 0 {
		o.CleanupInterval = time.Hour
	}
	if o.CleanupLimit <= 0 {
		o.CleanupLimit = 1000
	}
	return o
}

// claimable 拼出"这一行现在可以被领走吗"的查询条件。
//
// 两种可领取情形：
//   - pending/failed 且 available_at 已到（正常首次投递、或失败后退避结束）；
//   - processing 且租约已过期（领取它的 worker 崩溃了，需要补偿）。
func (r *Relay) claimable(q *gorm.DB) *gorm.DB {
	return q.Where(
		"((status IN ? AND available_at <= ?) OR (status = ? AND updated_at <= ?))",
		[]string{models.OutboxStatusPending, models.OutboxStatusFailed},
		time.Now(),
		models.OutboxStatusProcessing,
		time.Now().Add(-r.opts.LeaseDuration),
	)
}

// Relay 投递 outbox 事件。
type Relay struct {
	db   *gorm.DB
	log  *logrus.Logger
	opts Options

	mu   sync.RWMutex
	subs map[string][]Subscriber

	queue     chan ctype.ID
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
}

// New 构造中继。db 为 nil 时中继退化为空转（不投递）。
func New(db *gorm.DB, log *logrus.Logger, opts Options) *Relay {
	opts = opts.withDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	return &Relay{
		db:     db,
		log:    log,
		opts:   opts,
		subs:   make(map[string][]Subscriber),
		queue:  make(chan ctype.ID, opts.QueueSize),
		ctx:    ctx,
		cancel: cancel,
	}
}

// On 注册某事件名的订阅者。nil 订阅者被忽略。
func (r *Relay) On(name string, s Subscriber) *Relay {
	if r == nil || s == nil {
		return r
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs[name] = append(r.subs[name], s)
	return r
}

func (r *Relay) subscribers(name string) []Subscriber {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Subscriber(nil), r.subs[name]...)
}

// EnqueueID 把一个事件 ID 放进队列（CDC 路径）。
//
// 队列满时**丢弃**并返回 false：绝不能阻塞 canal 的事件循环。
// 丢掉不影响正确性——补偿扫描下一轮会把这行重新捞回来。
func (r *Relay) EnqueueID(id ctype.ID) bool {
	if r == nil {
		return false
	}
	select {
	case r.queue <- id:
		return true
	default:
		return false
	}
}

// Run 启动 worker 与补偿扫描（非阻塞）。可重复调用，只有第一次生效。
func (r *Relay) Run() {
	if r == nil || r.db == nil {
		return
	}
	r.startOnce.Do(func() {
		// 后台协程一律走 safego：裸 go 一旦 panic 会直接终止进程
		// （gin 的 Recovery 管不到后台任务）。archcheck R7 会拦裸 go。
		handler := r.panicHandler()
		for i := 0; i < r.opts.Workers; i++ {
			r.wg.Add(1)
			safego.Go(handler, r.worker)
		}
		r.wg.Add(1)
		safego.Go(handler, r.sweepLoop)
		r.wg.Add(1)
		safego.Go(handler, r.cleanupLoop)
	})
}

// Close 停止中继并等待在途投递结束。
func (r *Relay) Close() {
	if r == nil {
		return
	}
	r.cancel()
	r.wg.Wait()
}

// panicHandler 把后台协程的 panic 记进日志，而不是让进程退出。
func (r *Relay) panicHandler() safego.PanicHandler {
	return func(recovered any, stack []byte) {
		if r.log == nil {
			return
		}
		r.log.Errorf("outbox 中继协程 panic: %v\n%s", recovered, stack)
	}
}

func (r *Relay) worker() {
	defer r.wg.Done()
	for {
		select {
		case id := <-r.queue:
			if err := r.Dispatch(r.ctx, id); err != nil && r.log != nil {
				r.log.Errorf("outbox 投递失败: id=%d 错误=%v", id, err)
			}
		case <-r.ctx.Done():
			return
		}
	}
}

func (r *Relay) sweepLoop() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.opts.SweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n, err := r.Sweep(r.ctx)
			if err != nil {
				if r.log != nil {
					r.log.Errorf("outbox 补偿扫描失败: %v", err)
				}
				continue
			}
			if n > 0 && r.log != nil {
				r.log.Debugf("outbox 补偿扫描入队 %d 条", n)
			}
		case <-r.ctx.Done():
			return
		}
	}
}

// Sweep 扫描"可领取"的事件并入队（补偿路径），返回入队条数。
//
// 可领取 = 状态 pending/failed 且 available_at 已到且未耗尽重试。
func (r *Relay) Sweep(ctx context.Context) (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}

	var ids []ctype.ID
	q := r.claimable(r.db.WithContext(ctx).Model(&models.OutboxEventModel{}))
	err := q.
		Where("attempts < ?", r.opts.MaxAttempts).
		Order("id asc").
		Limit(r.opts.SweepLimit).
		Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}

	enqueued := 0
	for _, id := range ids {
		if !r.EnqueueID(id) {
			break // 队列满了，剩下的等下一轮
		}
		enqueued++
	}
	return enqueued, nil
}

// cleanupLoop 周期性删除超过保留期的已终结事件行。
func (r *Relay) cleanupLoop() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.opts.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n, err := r.Cleanup(r.ctx)
			if err != nil {
				if r.log != nil {
					r.log.Errorf("outbox 保留期清理失败: %v", err)
				}
				continue
			}
			if n > 0 && r.log != nil {
				r.log.Infof("outbox 保留期清理删除 %d 条已终结事件", n)
			}
		case <-r.ctx.Done():
			return
		}
	}
}

// maxCleanupBatches 单轮清理最多删多少批。
//
// 首次上线时表里可能已有存量，一轮删太久会拖慢停机；
// 删不完的留给下一轮，反正清理是幂等的。
const maxCleanupBatches = 20

// Cleanup 删除超过保留期的已终结事件行（done / dead），返回删除条数。
//
// 只删 done 与 dead：pending/failed/processing 都代表"还欠一次投递"，
// 无论多老都不能删，否则等于把事件丢掉。dead 之所以能删，是因为它在
// 进入 dead 时已经把证据写进 cdc_dead_letter_models。
//
// 用 Unscoped 硬删：模型内嵌了软删的 deleted_at，软删并不回收磁盘，
// 而保留期清理的目的就是回收磁盘，必须物理删除。
func (r *Relay) Cleanup(ctx context.Context) (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}

	now := time.Now()
	// 再跑一次 withDefaults：万一有调用方绕过 New 直接构造 Relay，
	// 也不会因为 Retention 为 0 把 done/dead 全删光。
	opts := r.opts.withDefaults()
	total := 0
	for _, spec := range []struct {
		status string
		before time.Time
	}{
		{models.OutboxStatusDone, now.Add(-opts.RetentionDone)},
		{models.OutboxStatusDead, now.Add(-opts.RetentionDead)},
	} {
		n, err := r.purge(ctx, spec.status, spec.before, opts.CleanupLimit)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// purge 按状态分批删除 updated_at 早于 before 的行。
//
// 先查 ID 再按 ID 删，而不是 DELETE ... LIMIT：
//   - 不依赖 MySQL 方言（sqlite 默认不支持 DELETE ... LIMIT）；
//   - 单批删除量严格可控，且能准确拿到 RowsAffected。
func (r *Relay) purge(ctx context.Context, status string, before time.Time, limit int) (int, error) {
	deleted := 0
	for batch := 0; batch < maxCleanupBatches; batch++ {
		var ids []ctype.ID
		err := r.db.WithContext(ctx).Model(&models.OutboxEventModel{}).
			Where("status = ? AND updated_at < ?", status, before).
			Order("id asc").
			Limit(limit).
			Pluck("id", &ids).Error
		if err != nil {
			return deleted, err
		}
		if len(ids) == 0 {
			break
		}

		res := r.db.WithContext(ctx).Unscoped().
			Where("id IN ? AND status = ? AND updated_at < ?", ids, status, before).
			Delete(&models.OutboxEventModel{})
		if res.Error != nil {
			return deleted, res.Error
		}
		deleted += int(res.RowsAffected)

		if len(ids) < limit {
			break // 不足一批，说明已经清完
		}
	}
	return deleted, nil
}

// Dispatch 领取并投递一条事件。
//
// 流程：读行 → 原子领取（条件 UPDATE）→ 执行订阅者 → 记账（done / failed / dead）。
// 任何"不该由我处理"的情形都返回 nil 而非错误——那是并发下的正常竞争结果，
// 不是故障（例如已被别的 worker 领走、已经完成、还没到可重试时间）。
func (r *Relay) Dispatch(ctx context.Context, id ctype.ID) error {
	if r == nil || r.db == nil {
		return nil
	}

	var row models.OutboxEventModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	claimed, err := r.claim(ctx, id)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	dispatchErr := r.runSubscribers(ctx, row)
	if dispatchErr == nil {
		return r.markDone(ctx, id)
	}
	return r.markFailed(ctx, row, dispatchErr)
}

// claim 原子领取：只有成功把状态从 pending/failed 改成 processing 的 worker 才继续。
//
// 这是幂等的第一道防线：并发投递同一行时，条件 UPDATE 保证只有一个赢家。
func (r *Relay) claim(ctx context.Context, id ctype.ID) (bool, error) {
	q := r.claimable(r.db.WithContext(ctx).Model(&models.OutboxEventModel{}).Where("id = ?", id))
	res := q.
		Where("attempts < ?", r.opts.MaxAttempts).
		Updates(map[string]any{
			"status":     models.OutboxStatusProcessing,
			"attempts":   gorm.Expr("attempts + 1"),
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *Relay) runSubscribers(ctx context.Context, row models.OutboxEventModel) error {
	subs := r.subscribers(row.EventName)
	// 没人订阅不算失败：事件已经发生，只是当前没有下游关心它。
	if len(subs) == 0 {
		return nil
	}

	e := Event{
		ID:            row.ID,
		EventName:     row.EventName,
		AggregateType: row.AggregateType,
		AggregateID:   row.AggregateID,
		Payload:       row.Payload,
		Attempts:      row.Attempts + 1,
	}

	var errs []error
	for _, s := range subs {
		if err := s(ctx, e); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Relay) markDone(ctx context.Context, id ctype.ID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&models.OutboxEventModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":        models.OutboxStatusDone,
			"dispatched_at": now,
			"last_error":    "",
			"updated_at":    now,
		}).Error
}

func (r *Relay) markFailed(ctx context.Context, row models.OutboxEventModel, dispatchErr error) error {
	attempts := row.Attempts + 1
	msg := truncate(dispatchErr.Error(), 500)

	if attempts >= r.opts.MaxAttempts {
		// 重试耗尽：这个事件交给人工处理（死信队列），不再自动重试。
		if err := r.db.WithContext(ctx).Model(&models.OutboxEventModel{}).
			Where("id = ?", row.ID).
			Updates(map[string]any{
				"status":     models.OutboxStatusDead,
				"last_error": msg,
				"updated_at": time.Now(),
			}).Error; err != nil {
			return err
		}
		return r.saveDeadLetter(row, attempts, msg)
	}

	backoff := time.Duration(r.opts.BackoffMS*attempts) * time.Millisecond
	return r.db.WithContext(ctx).Model(&models.OutboxEventModel{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"status":       models.OutboxStatusFailed,
			"available_at": time.Now().Add(backoff),
			"last_error":   msg,
			"updated_at":   time.Now(),
		}).Error
}

func (r *Relay) saveDeadLetter(row models.OutboxEventModel, attempts int, msg string) error {
	return deadletter.SaveBatch(r.db, []deadletter.Item{{
		Stream:      "outbox_relay",
		CdcJobID:    fmt.Sprintf("outbox:%d", row.ID),
		SourceTable: "outbox_event_models",
		Action:      row.EventName,
		TargetKey:   fmt.Sprintf("%s:%d", row.AggregateType, row.AggregateID),
		Payload:     map[string]any{"payload": row.Payload},
		RetryCount:  attempts,
		Status:      "pending",
		ErrorCode:   "OUTBOX_DISPATCH_FAILED",
		ErrorMsg:    msg,
	}})
}

// truncate 截断字符串，避免超长错误把列写爆（last_error 列宽 512）。
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

var (
	currentMu sync.RWMutex
	current   *Relay
)

// SetCurrent 注册进程内唯一的中继实例。
//
// 为什么是进程级单例而不是到处传参：river（CDC 摄入）和独立 worker
// （补偿扫描）必须共用同一个实例，否则会跑出两套 worker 重复扫描同一个表。
// 一个进程本来就只需要一个 outbox 中继，用单例换取"不用改 NewRiver 签名"。
func SetCurrent(r *Relay) {
	currentMu.Lock()
	current = r
	currentMu.Unlock()
}

// Current 返回已注册的中继；未注册返回 nil（此时 CDC 路径静默跳过，靠扫描兜底）。
func Current() *Relay {
	currentMu.RLock()
	defer currentMu.RUnlock()
	return current
}
