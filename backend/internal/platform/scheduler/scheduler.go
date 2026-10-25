// Package scheduler 是定时任务的统一入口：定时触发、单实例互斥、统一日志。
//
// 它只做"什么时候跑、能不能跑、跑完怎么记"，不知道任何业务。
// 具体跑什么由装配层以 Job 的形式传进来，因此这个包不依赖任何业务模块。
//
// 单实例互斥用 Redis 锁：同一定时任务在多个副本上只应有一个真的执行，
// 否则计数会被刷两遍（我们的刷盘是"取增量再加"，重复执行会让库里的数偏大）。
package scheduler

import (
	"context"
	"time"

	redis_service "myblogx/internal/platform/redis"

	"github.com/go-co-op/gocron/v2"
	"github.com/go-redis/redis/v8"
)

// Logger 是窄日志接口。
type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

// Options 是调度器的构造参数。
type Options struct {
	// Client 用于分布式锁；为 nil 时不加锁（单机部署）。
	Client *redis.Client
	Logger Logger
	// Location 是时区，nil 时用 Asia/Shanghai。
	Location *time.Location
}

// Job 是一个定时任务。
type Job struct {
	// Name 用于日志。
	Name string
	// LockKey 是互斥锁的 key；为空表示不加锁。
	LockKey string
	// LockTTL 是锁的持有时间，应大于任务最长耗时。
	LockTTL time.Duration
	// Run 返回本次处理的条目数（0 表示无变化，不打印成功日志）。
	Run func(ctx context.Context) (int, error)
}

// Scheduler 包装 gocron。
type Scheduler struct {
	inner gocron.Scheduler
	opts  Options
}

// New 创建调度器。
func New(opts Options) (*Scheduler, error) {
	loc := opts.Location
	if loc == nil {
		loaded, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			loc = time.UTC
		} else {
			loc = loaded
		}
	}
	inner, err := gocron.NewScheduler(gocron.WithLocation(loc))
	if err != nil {
		return nil, err
	}
	return &Scheduler{inner: inner, opts: opts}, nil
}

// Every 每 d 执行一次。
func (s *Scheduler) Every(d time.Duration, job Job) error {
	_, err := s.inner.NewJob(gocron.DurationJob(d), gocron.NewTask(func() {
		s.run(job)
	}))
	if err != nil && s.opts.Logger != nil {
		s.opts.Logger.Errorf("注册定时任务失败: 任务=%s 错误=%v", job.Name, err)
	}
	return err
}

// Daily 每天在指定时刻执行。
func (s *Scheduler) Daily(hour, minute, second int, job Job) error {
	_, err := s.inner.NewJob(
		gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(uint(hour), uint(minute), uint(second)))),
		gocron.NewTask(func() { s.run(job) }),
	)
	if err != nil && s.opts.Logger != nil {
		s.opts.Logger.Errorf("注册每日任务失败: 任务=%s 错误=%v", job.Name, err)
	}
	return err
}

// Start 启动调度。
func (s *Scheduler) Start() {
	s.inner.Start()
	if s.opts.Logger != nil {
		s.opts.Logger.Infof("定时任务已启动")
	}
}

// Shutdown 停止调度并等待在途任务结束。
//
// gocron v2 的 Shutdown 不带 context：它会一直等到在途任务返回。
// 因此任务自身必须尊重 ctx 的取消（见 Job.Run 的签名），否则停机时会被挂住。
func (s *Scheduler) Shutdown() error {
	return s.inner.Shutdown()
}

// run 执行单个任务：先抢锁，再跑，最后记日志。
func (s *Scheduler) run(job Job) {
	ctx := context.Background()

	if job.LockKey != "" {
		unlock, err := redis_service.LockArticleSync(
			redis_service.Options{Client: s.opts.Client, Logger: nil},
			ctx, job.LockKey, job.LockTTL,
		)
		if err != nil {
			if s.opts.Logger != nil {
				s.opts.Logger.Errorf("%s获取锁失败: 错误=%v", job.Name, err)
			}
			return
		}
		if unlock == nil {
			// 已有实例在跑这一轮。这不是错误，多副本部署下的正常现象。
			if s.opts.Logger != nil {
				s.opts.Logger.Infof("%s跳过，本轮已有任务在执行", job.Name)
			}
			return
		}
		defer unlock()
	}

	affected, err := job.Run(ctx)
	if err != nil {
		if s.opts.Logger != nil {
			s.opts.Logger.Errorf("%s执行失败: 错误=%v", job.Name, err)
		}
		return
	}
	if affected > 0 && s.opts.Logger != nil {
		s.opts.Logger.Infof("%s执行成功: 影响数量=%d", job.Name, affected)
	}
}
