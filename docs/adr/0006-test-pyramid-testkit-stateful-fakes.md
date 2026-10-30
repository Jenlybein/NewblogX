# ADR-0006：测试体系改为金字塔 + testkit + 有状态 fake

- 状态：已交付
- 日期：2026-10（v1 归档期）
- 来源：从 [04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md) 拆分

## 背景与问题

117 个 Go 测试文件、2,357 处断言，但 `t.Parallel()` 只有 **3** 处；测试普遍依赖 `testutil.InitGlobals` / `Deps` / 包级全局，无法并行无法隔离。

集成测试用 SQLite 冒充 MySQL（`ON CONFLICT` / `clause.Locking` / JSON 函数行为不同）→ **假安全**；`repository/` 449 行核心 SQL **0 测试**，`frontend_admin` 0 测试；无 CI，断言从不自动执行。

## 决策

金字塔 + testkit + 有状态 fake + golden/契约：

- domain 纯单测穷举规则；
- app 用 fake 端口测编排；
- adapter 用真库测 SQL；
- HTTP 测契约；
- E2E 只保主链路。

## 理由与已否决方案

业务规则散在 handler 的 `if` 里，没有 domain 层就写不出不变量测试，没有端口注入就 fake 不掉，因此测试体系必须与架构重构绑定推进。四条关键取舍：

1. **有状态 fake 而非 gomock**——"重复插入冲突""事务回滚""Outbox 失败"需要状态，gomock 表达不了；
2. **SQLite 不再冒充 MySQL**——用 testcontainers 起真 MySQL，`//go:build integration` 隔离；
3. **覆盖率分层设阈值**——domain ≥ 90%、app ≥ 85%，**不设整体数字**；
4. **golden + 契约双保险**——golden 守"行为没漂"，契约守"前后端没漂"。

已否决的方案：

- **在现有风格上补集成测试**：每个"状态 × 角色"组合都要起一次环境，必然退化为只测 happy path；
- **只做 E2E**：慢、脆、无法穷举规则组合。

## 代价 / 不做什么

- 一次性写 testkit 约 800 行与各模块 fake；
- 集成测试需要 Docker；
- golden file 带来 PR diff 噪音，需人工 review。

## 当前生效约束

1. `go test -race -parallel 8 ./internal/...` 常态化；
2. `go test ./internal/*/domain/...` 必须 < 2s（domain 零外部依赖）；
3. 门禁自检：CI 必须能因"注入的失败"而红，否则 CI 无效；
4. flaky 测试 quarantine，**不允许长期 retry**；
5. mutation testing 先覆盖 domain 关键规则，逐步扩大。

## 相关代码与文档

- 代码：[internal/testkit](../../backend/internal/testkit)（替代旧的 `test/testutil`）
- 文档：[治理与质量 · 测试体系](../architecture/governance.md)（金字塔、分层口径、覆盖率门槛、flaky 治理）
- 原始记录：[04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md)
