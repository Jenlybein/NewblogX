// Package dlq 是"死信重投"的编排层。
//
// # 它解决什么
//
// 三个 CDC 子链路重试耗尽后都会把失败证据写进 `cdc_dead_letter`，状态一律 pending，
// 但以前没有任何代码去读它重做一次 —— 死信表只进不出，出问题只能人工进 MySQL 翻。
// 这个包把"照证据补救"补齐：取记录 → 按 stream 分派执行器 → 更新状态 → 写一条回放日志。
//
// # 职责边界
//
//   - 这里只有编排，不碰具体协议：怎么重投 ES、怎么重置 outbox 行，由各子包实现
//     `Replayer`（见 river / outboxrelay 里的 replay.go）。
//   - 也不碰数据库细节：状态流转走 deadletter 包的 MarkResolved / BumpRetry。
//   - 回放日志通过注入的 Emit 写，而不是直接调 writer：单测不该产生真实日志，
//     而且"日志管道长什么样"是装配层的事。
//
// 决策背景见 docs/adr/0008-dead-letter-replay.md。
package dlq

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/log/writer"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ErrSkipped 由执行器返回，表示"这条记录现在不需要重投"。
// 语义定义在 deadletter 包（执行器与编排层的公共依赖），这里只是给调用方一个就近的名字。
var ErrSkipped = deadletter.ErrSkipped

// ErrUnsupported 由执行器返回，表示"这条记录语义上不能重投"（= 不是失败）。
// 与"没有注册执行器"那一支同属 unsupported，区别只在这句话是执行器看完 payload 之后说的。
var ErrUnsupported = deadletter.ErrUnsupported

// 重投结果的取值。前三个与回放日志的 result 字段共用一套词。
const (
	ResultSuccess     = "success"
	ResultFailed      = "failed"
	ResultSkipped     = "skipped"
	ResultUnsupported = "unsupported"
)

// Replayer 是某一类死信的重投执行器。
//
// 实现必须是**幂等**的：同一条死信被重投两次不应该产生第二次副作用。
// 这是本包不引入互斥/中间态的前提（见 ADR-008「状态机」一节）。
type Replayer interface {
	// Streams 声明它能处理哪些 stream 值（死信表里的 stream 列）。
	Streams() []string
	// Replay 对一条死信执行重投：nil 表示成功，ErrSkipped 表示无需重投，
	// 其余 error 表示这次没成功（原因会写回死信记录并进回放日志）。
	Replay(ctx context.Context, record deadletter.Record) error
}

// Result 是一条死信的重投结果。
type Result struct {
	ID      ctype.ID
	Stream  string
	Status  string // 处理之后该记录的状态（unsupported / skipped 时是原状态）
	Result  string // success / failed / skipped / unsupported
	Message string
}

// Options 是组装参数。
type Options struct {
	DB     *gorm.DB
	Logger *logrus.Logger

	// Emit 写一条回放日志。为 nil 时不记录（单测、或未接日志管道的部署）。
	//
	// 为什么是函数而不是直接依赖 writer：这个包不该知道日志要去 ClickHouse 还是文件，
	// 也不该在单测里往磁盘写日志。装配层把 writer.EmitReplayEvent 包一层传进来。
	Emit func(input writer.ReplayEventInput)

	// Replayers 是各 stream 的执行器。同一个 stream 只允许被一个执行器声明。
	Replayers []Replayer
}

// Service 编排死信重投。
type Service struct {
	opts     Options
	byStream map[string]Replayer
	supports []string
}

// New 构造编排服务。
func New(opts Options) *Service {
	byStream := make(map[string]Replayer, len(opts.Replayers))
	for _, r := range opts.Replayers {
		if r == nil {
			continue
		}
		for _, stream := range r.Streams() {
			key := strings.TrimSpace(stream)
			if key == "" {
				continue
			}
			byStream[key] = r
		}
	}
	supports := make([]string, 0, len(byStream))
	for stream := range byStream {
		supports = append(supports, stream)
	}
	sort.Strings(supports)
	return &Service{opts: opts, byStream: byStream, supports: supports}
}

// SupportedStreams 返回当前支持重投的 stream（已排序，便于拼错误信息）。
func (s *Service) SupportedStreams() []string {
	out := make([]string, len(s.supports))
	copy(out, s.supports)
	return out
}

// Replay 批量重投。
//
// 单条失败不影响同批其他记录（运维要的是"这批里哪几条补上了"）。
// 只有"连记录都读不出来"才会让整体返回 error。
func (s *Service) Replay(ctx context.Context, ids []ctype.ID) ([]Result, error) {
	if s == nil || s.opts.DB == nil || len(ids) == 0 {
		return nil, nil
	}

	records, err := deadletter.ByIDs(s.opts.DB, ids)
	if err != nil {
		return nil, fmt.Errorf("dlq: 读取死信记录失败: %w", err)
	}

	results := make([]Result, 0, len(records))
	for _, record := range records {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return results, err
			}
		}
		results = append(results, s.replayOne(ctx, record))
	}
	return results, nil
}

// replayOne 处理单条记录。
func (s *Service) replayOne(ctx context.Context, record deadletter.Record) Result {
	out := Result{ID: record.ID, Stream: record.Stream, Status: record.Status}

	// 已经补上的不再重投：重复点按钮不该把 ES 再打一遍，也不该刷掉第一次的时间点。
	if record.Status == deadletter.StatusResolved {
		out.Result = ResultSkipped
		out.Message = "该记录已是已解决状态，跳过"
		s.emit(record, out, "")
		return out
	}

	replayer, ok := s.byStream[strings.TrimSpace(record.Stream)]
	if !ok {
		out.Result = ResultUnsupported
		out.Message = s.unsupportedMessage(record.Stream)
		s.emit(record, out, "")
		return out
	}

	err := replayer.Replay(ctx, record)
	switch {
	case err == nil:
		marked, markErr := deadletter.MarkResolved(s.opts.DB, record.ID, record.RetryCount+1)
		out.Result = ResultSuccess
		out.Status = deadletter.StatusResolved
		if markErr != nil {
			// 重投本身成功了，只是状态没写进去。不能报 failed（会诱导人再点一次、
			// 让幂等执行器白干一遍），但要如实说"状态没落库"。
			out.Message = fmt.Sprintf("重投成功，但状态回写失败: %v", markErr)
			s.logf("warn", "死信重投成功但状态回写失败: id=%s err=%v", record.ID.String(), markErr)
		} else if !marked {
			out.Message = "重投成功（该记录已被其他操作标为已解决）"
		} else {
			out.Message = "重投成功"
		}
		s.emit(record, out, "")
		return out

	case errors.Is(err, ErrSkipped):
		out.Result = ResultSkipped
		out.Message = "业务侧已经处理过该记录，无需重投"
		s.emit(record, out, "")
		return out

	case errors.Is(err, ErrUnsupported):
		// 不写库：status 与 retry_count 保持原样。
		// 理由同"没有注册执行器"那一支 —— 这不是一次失败的尝试，而是"这条不该被重投"；
		// 记成 failed 会让运维把它当成"再点几次就好了"，而它点一百次也是同一个结果。
		// 文案用执行器给的那句（它知道该改用什么办法），不用 unsupportedMessage 的通用话。
		out.Result = ResultUnsupported
		out.Message = err.Error()
		s.emit(record, out, err.Error())
		return out

	default:
		out.Result = ResultFailed
		out.Status = deadletter.StatusPending
		out.Message = err.Error()
		if bumpErr := deadletter.BumpRetry(s.opts.DB, record.ID, record.RetryCount+1, "DLQ_REPLAY_FAILED", err.Error()); bumpErr != nil {
			s.logf("error", "死信重投失败且状态回写失败: id=%s err=%v", record.ID.String(), bumpErr)
		}
		s.emit(record, out, err.Error())
		return out
	}
}

// unsupportedMessage 告诉操作员"这条为什么不能重投、该改用别的什么办法"。
//
// 只返回"不支持"是没用的 —— 人还得回来问代码。所以这里按已知 stream 给出替代动作。
func (s *Service) unsupportedMessage(stream string) string {
	supported := "无"
	if len(s.supports) > 0 {
		supported = strings.Join(s.supports, " / ")
	}
	switch strings.TrimSpace(stream) {
	case "es_projection":
		// 正常情况下这条走不到：投影执行器已在装配层注册，旧记录会由它返回 ErrUnsupported
		// 并给出自己的说明。走到这里说明装配缺了一环（比如某个部署漏调了 WireAnalytics）。
		return "当前部署没有注册投影重投执行器（装配缺失），无法就地重算；" +
			"临时办法：`./server -es -s article-sync` 全量重建文章搜索投影。" +
			"当前支持重投的流：" + supported
	default:
		return "没有为该 stream 注册重投执行器。当前支持重投的流：" + supported
	}
}

// emit 写一条回放日志。
//
// 成功/失败/跳过/不支持都记：运维关心的是"谁在什么时候点过这条、结果如何"，
// 只看成功的话，"点了没反应"就没法排查。
func (s *Service) emit(record deadletter.Record, out Result, errMsg string) {
	if s.opts.Emit == nil {
		return
	}
	level := "info"
	errorCode := ""
	if out.Result == ResultFailed {
		level = "warn"
		errorCode = "DLQ_REPLAY_FAILED"
	}
	s.opts.Emit(writer.ReplayEventInput{
		Level:        level,
		Message:      "死信重投: " + out.Result,
		ErrorCode:    errorCode,
		ErrorMessage: errMsg,
		ErrorType:    "dlq_replay",
		CdcJobID:     record.CdcJobID,
		Stream:       record.Stream,
		SourceTable:  record.SourceTable,
		Action:       record.Action,
		TargetKey:    record.TargetKey,
		RetryCount:   record.RetryCount + 1,
		Result:       out.Result,
	})
}

func (s *Service) logf(level, format string, args ...any) {
	if s.opts.Logger == nil {
		return
	}
	switch level {
	case "warn":
		s.opts.Logger.Warnf(format, args...)
	case "error":
		s.opts.Logger.Errorf(format, args...)
	default:
		s.opts.Logger.Infof(format, args...)
	}
}
