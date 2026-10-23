package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

type stopHook struct {
	name string
	fn   func(context.Context) error
}

// Lifecycle 统一管理启动后生命周期（阶段 A 先做薄封装）。
type Lifecycle struct {
	logger      *logrus.Logger
	stopTimeout time.Duration
	stopHooks   []stopHook
}

func NewLifecycle(logger *logrus.Logger) *Lifecycle {
	return &Lifecycle{
		logger:      logger,
		stopTimeout: 5 * time.Second,
	}
}

func (l *Lifecycle) OnStop(name string, fn func(context.Context) error) {
	if fn == nil {
		return
	}
	l.stopHooks = append(l.stopHooks, stopHook{name: name, fn: fn})
}

// WaitForShutdown 阻塞等待退出信号并执行 stop hooks。
func (l *Lifecycle) WaitForShutdown() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	sig := <-signals
	if l.logger != nil {
		l.logger.Infof("收到退出信号: %s", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), l.stopTimeout)
	defer cancel()

	for i := len(l.stopHooks) - 1; i >= 0; i-- {
		hook := l.stopHooks[i]
		if err := hook.fn(ctx); err != nil && l.logger != nil {
			l.logger.Errorf("执行 stop hook 失败: %s, 错误=%v", hook.name, err)
		}
	}
}
