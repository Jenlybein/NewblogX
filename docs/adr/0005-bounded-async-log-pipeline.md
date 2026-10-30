# ADR-0005：日志改为有界异步管道 + 类型化事件

- 状态：已交付
- 日期：2026-10（v1 归档期）
- 来源：从 [04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md) 拆分

## 背景与问题

日志链路五个病灶：

1. `utils/logsafe/sanitize.go:52` 对任意 `any` 反射遍历，无深度/大小上限；
2. `write_action_audit.go:49` 的 `RequestBody any` / `ResponseBody any` 允许整对象进入；
3. `write_sink.go:25` 在请求路径**同步** `json.Marshal` + 全局锁 `file.Write`；
4. 每条日志调 `NextSnowflakeID()`；
5. `fluent-bit.conf:12` 5 个 tail、`Skip_Long_Lines On`（丢）、`Read_from_Head On`（重）。

结果：并发一高内存尖刺、RT 抖动，ClickHouse 慢时丢日志且不可见。

## 决策

自建**类型化 Emitter + 有界队列 + 批量直写 ClickHouse + WAL**：

- `Emit` O(1)；
- 单写协程批量；
- 失败落 WAL 重放；
- 去掉 fluent-bit 的 app tail（它继续采容器 stdout/stderr 与基础设施日志）。

## 理由与已否决方案

六条硬约束全满足，审计可做到不丢，日志管道自身可观测。关键取舍三条：

1. **runtime 可丢，audit/login 不可丢**——队列满时 runtime 丢弃并计数，audit 落 WAL（"业务稳定优先"的显式选择）；
2. **不做全量调用栈**——生产默认关闭 `ReportCaller`，仅 error 采样抓栈；
3. **去掉 fluent-bit 的业务日志采集**——消除"丢/重"单点。

已否决的方案：

- **保留 logrus 只修截断**：同步写在请求路径没解决；
- **logrus + 异步 Hook**：半成品，`any` 与 fluent-bit 问题仍在。

OpenTelemetry Collector 保留为**可选导出器**（`log.Emitter` 面向"记录"而非具体后端），不在本次强制。

## 代价 / 不做什么

- 新增约 1,200 行基础设施与一个 WAL 目录；
- ClickHouse 批量大小 / 超时 / 退避需调参；
- 一次性迁移 5 类日志写入调用点。

## 当前生效约束

1. 日志字段不得出现 `any` / 结构体（archcheck `--rule=log-no-any`）；
2. `internal/platform/log/emit.go` 请求路径内不得出现 `json.Marshal` / `os.OpenFile`；
3. WAL 积压 > **256MB** 告警；ClickHouse TTL：runtime **30d** / audit **180d**；
4. 有界性验收：1 万 QPS 压测 1 分钟 RSS 平稳且 `log_dropped_total` 有值；断 ClickHouse 写 1000 条 audit，恢复后 count == 1000 且无重复。

## 相关代码与文档

- 代码：[platform/log](../../backend/internal/platform/log)（`emit.go` / `sink_file.go` / `event.go` 等）
- 文档：[治理与质量 · 日志模块](../architecture/governance.md)（六条硬约束、管道结构与可观测性）
- 原始记录：[04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md)
