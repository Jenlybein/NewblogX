package writer

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	platformlog "myblogx/internal/platform/log"
	"myblogx/internal/platform/log/persist"
)

// kindCount 是平台日志类别的数量（Kind 是 0 起始的紧凑枚举）。
const kindCount = 6

// route 描述一类日志的落点：本地文件目录名与 ClickHouse 表名。
// 两者同名是刻意的——排查时不必在两套命名之间来回翻译。
type route struct {
	kind  platformlog.Kind
	dir   string
	table string
}

// routes 是五类结构化日志的落点；emitters 按 Kind 直接寻址。
// 用切片而不是定长数组：Kind 枚举里有一个 KindAccess 目前没有对应表，
// 数组会留下一个零值坑（表名为空），切片则只列出真实存在的类别。
var routes = []route{
	{platformlog.KindRuntime, RuntimeLogDirName, RuntimeLogTableName},
	{platformlog.KindAudit, ActionAuditLogDirName, ActionAuditLogTableName},
	{platformlog.KindLogin, LoginEventLogDirName, LoginEventLogTableName},
	{platformlog.KindCDC, CdcEventLogDirName, CdcEventLogTableName},
	{platformlog.KindReplay, ReplayEventLogDirName, ReplayEventLogTableName},
}

// PipelineConfig 是结构化日志管道的构造参数。
type PipelineConfig struct {
	Options    Options // 日志目录/应用名 + ClickHouse 连接
	WALDir     string  // 本地兜底目录；为空时用 <日志目录>/wal
	ClickHouse bool    // 是否把日志直接写进 ClickHouse
}

// pipeline 持有五条独立管道，一条对应一张日志表。
//
// 为什么要按类别分成五条：每类日志的落点（表 / 目录）不同，而 Sink 接口
// 一次只认一个目标。五条管道各自有独立的有界队列，互不挤占——
// 审计日志暴涨不该把 runtime 日志挤掉。
type pipeline struct {
	walDir   string
	emitters [kindCount]*platformlog.Emitter
}

var (
	pipelineMu sync.RWMutex
	activePipe *pipeline
	lazyPipeMu sync.Mutex
)

// InitStructuredLog 构造并安装结构化日志管道。
//
// 幂等性：可重复调用（第二次会替换并关闭上一条管道），用于测试与配置热更。
// 启动时会把上次遗留的 WAL 批次重放回主出口；重放失败只告警不阻断启动，
// 文件仍在磁盘上，下一次启动会再试。
func InitStructuredLog(cfg PipelineConfig) error {
	p, err := newPipeline(cfg)
	if err != nil {
		return err
	}

	pipelineMu.Lock()
	old := activePipe
	activePipe = p
	pipelineMu.Unlock()

	if old != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = old.close(ctx)
	}
	return nil
}

// CloseStructuredLog 停止接收日志、把队列刷完并关闭出口。
// 停机时必须在关闭 ClickHouse 连接之前调用（见 app.RegisterInfraCleanup 的注册顺序）。
func CloseStructuredLog(ctx context.Context) error {
	pipelineMu.Lock()
	p := activePipe
	activePipe = nil
	pipelineMu.Unlock()

	if p == nil {
		closeFallbackSinks()
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	err := p.close(ctx)
	closeFallbackSinks()
	return err
}

// StructuredLogStats 返回每条管道的健康快照，供健康检查与运维排查使用。
func StructuredLogStats() map[string]platformlog.Stats {
	pipelineMu.RLock()
	p := activePipe
	pipelineMu.RUnlock()

	out := make(map[string]platformlog.Stats, kindCount)
	if p == nil {
		return out
	}
	for _, rt := range routes {
		if em := p.emitters[rt.kind]; em != nil {
			out[rt.dir] = em.Stats()
		}
	}
	return out
}

// WALPending 返回每类日志尚未重放的批次数。
// 数值持续增长说明主出口一直写不进去（通常意味着 ClickHouse 不可用）。
func WALPending() map[string]int {
	pipelineMu.RLock()
	p := activePipe
	pipelineMu.RUnlock()

	out := make(map[string]int, kindCount)
	if p == nil {
		return out
	}
	for _, rt := range routes {
		n, err := persist.PendingBatches(filepath.Join(p.walDir, rt.dir))
		if err != nil {
			continue
		}
		out[rt.dir] = n
	}
	return out
}

// newPipeline 按配置逐类构造管道。
func newPipeline(cfg PipelineConfig) (*pipeline, error) {
	rootDir := ResolveLogDir(cfg.Options.LogConfig.Dir)
	appName := ResolveLogApp("", cfg.Options.LogConfig.App)

	walRoot := strings.TrimSpace(cfg.WALDir)
	if walRoot == "" {
		walRoot = filepath.Join(rootDir, "wal")
	}

	// 唯一判定点：出口选择与告警文案都以它为准，避免两处条件漂移。
	withCH := cfg.ClickHouse && cfg.Options.ClickHouse != nil

	p := &pipeline{walDir: walRoot}

	opts := platformlog.Options{
		Service:    appName,
		Env:        runtimeEnv(cfg.Options),
		InstanceID: strconv.Itoa(int(runtimeServerID(cfg.Options))),
	}

	for _, rt := range routes {
		sink, err := buildSink(cfg.Options, withCH, rootDir, appName, walRoot, rt)
		if err != nil {
			// 已经建好的管道要收掉，避免泄漏 goroutine。
			_ = p.close(context.Background())
			return nil, err
		}
		p.emitters[rt.kind] = platformlog.New(opts, sink)
	}
	return p, nil
}

// buildSink 装配一类日志的出口：主出口 + （仅 ClickHouse 模式下的）本地 WAL 兜底。
func buildSink(deps Options, withCH bool, rootDir, appName, walRoot string, rt route) (platformlog.Sink, error) {
	walDir := filepath.Join(walRoot, rt.dir)

	var primary platformlog.Sink
	if withCH {
		chSink, err := persist.NewClickHouseSink(deps.ClickHouse, rt.table)
		if err != nil {
			return nil, err
		}
		primary = chSink
	} else {
		// 没开 ClickHouse 时文件本身就是最终落点，不需要再套一层 WAL——
		// 往同一个磁盘再写一份"兜底"挡不住磁盘满，只会让失败路径更长。
		primary = newDailyFileSink(rootDir, rt.dir, appName)
	}

	// 上次进程遗留的 WAL 先灌回主出口（文件模式则直接补写进文件）。
	if _, err := persist.ReplayWAL(context.Background(), walDir, primary); err != nil {
		warnf(deps, "重放日志 WAL 失败（%s），文件保留待下次重试: %v", rt.dir, err)
	}

	if withCH {
		return persist.NewWALSink(primary, walDir, func(err error) {
			warnf(deps, "日志写入 %s 失败，已落本地 WAL 等恢复后重放: %v", rt.table, err)
		})
	}
	return primary, nil
}

// close 关闭全部管道；单个失败不阻塞其余管道收尾。
func (p *pipeline) close(ctx context.Context) error {
	var firstErr error
	for i := range p.emitters {
		if p.emitters[i] == nil {
			continue
		}
		if err := p.emitters[i].Close(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// emit 把一条已编码的行投递到对应类别的管道。
func (p *pipeline) emit(kind platformlog.Kind, level platformlog.Level, line []byte) {
	if p == nil || int(kind) >= kindCount {
		return
	}
	em := p.emitters[kind]
	if em == nil {
		return
	}
	em.EmitLine(context.Background(), level, line)
}

// getPipeline 返回当前管道；尚未初始化时惰性建一条"只写文件"的管道。
//
// 为什么会走到惰性分支：单测与命令行工具不会执行 app.InitInfra，
// 但它们仍会调用 EmitActionAudit。此时退化成原来的行为（写本地文件），
// 不 panic、不丢日志，也不需要每处调用都先初始化。
func getPipeline(deps Options) *pipeline {
	pipelineMu.RLock()
	p := activePipe
	pipelineMu.RUnlock()
	if p != nil {
		return p
	}

	lazyPipeMu.Lock()
	defer lazyPipeMu.Unlock()

	pipelineMu.RLock()
	p = activePipe
	pipelineMu.RUnlock()
	if p != nil {
		return p
	}

	p, err := newPipeline(PipelineConfig{Options: deps})
	if err != nil {
		warnf(deps, "初始化兜底日志管道失败，日志将被丢弃: %v", err)
		return nil
	}

	pipelineMu.Lock()
	activePipe = p
	pipelineMu.Unlock()
	return p
}

// emitRow 把一条表格式日志（runtime / audit / login / cdc / replay）送进管道。
//
// 编码放在业务侧而不是写协程里，理由有三：
//  1. 有界队列的预算是按字节算的，编码完才知道真实大小；
//  2. 这些结构体本身就是表的行形态，编码即最终形态，不需要再过一层映射；
//  3. platform/log 因此不必接受 any，Event 的字段集合保持封闭。
//
// 真正从请求路径上被移除的是文件写、全局锁与 fsync——这些才是阻塞项；
// json.Marshal 是纯 CPU、无锁、微秒级，留在原地不构成瓶颈。
func emitRow(deps Options, kind platformlog.Kind, level platformlog.Level, row any) {
	if row == nil {
		return
	}
	line, err := json.Marshal(row)
	if err != nil {
		// 编码失败只可能是结构体写错（例如塞了不可序列化的值），
		// 这条日志确实发不出去，必须留下痕迹而不是静默丢弃。
		errorf(deps, "结构化日志编码失败（%s）: %v", kind, err)
		return
	}
	getPipeline(deps).emit(kind, level, line)
}

// parseLevel 把日志级别字符串映射为平台级别，未知值按 info 处理。
func parseLevel(level string) platformlog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug", "trace":
		return platformlog.LevelDebug
	case "warn", "warning":
		return platformlog.LevelWarn
	case "error", "fatal", "panic":
		return platformlog.LevelError
	default:
		return platformlog.LevelInfo
	}
}

// warnf / errorf 在 logger 为空时静默降级，避免日志管道自身引发 panic。
func warnf(deps Options, format string, args ...any) {
	if deps.Logger != nil {
		deps.Logger.Warnf(format, args...)
	}
}

func errorf(deps Options, format string, args ...any) {
	if deps.Logger != nil {
		deps.Logger.Errorf(format, args...)
	}
}
