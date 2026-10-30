# ADR-0004：架构守卫从符号匹配改为可执行的 AST 规则

- 状态：已交付
- 日期：2026-10（v1 归档期）
- 来源：从 [04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md) 拆分

## 背景与问题

原有守卫（`scripts/arch_guard.ps1` 110 行 9 条 + `test/architecture/` 260 行 6 个测试）检查的符号 `appctx` / `global` / `MustFromGin` / `DepsFromGin` / `mustApp` **全部 0 处引用，13 条规则 + 6 个测试全部永真通过**；唯一有效的「api 禁止 import repository」也被 `type QueryService = repo.QueryService` 别名 re-export 绕过。

真正的问题（32 个 `Deps`、handler 直接查库 61 文件 194 处、65 个 `enter.go`）无人看守。

## 决策

采用 AST + 类型信息方案，建 `tools/archcheck/`（`main.go` / `load.go` / `rules.go` / `rules_test.go` / `report.go`），删除 ps1 与 `test/architecture/` 两套重复实现。

首批 8 条规则：

| # | 规则 | 判定方式 | 存量 |
| --- | --- | --- | --- |
| R1 | 导出结构体不得含 `*gorm.DB` | 类型检查 | 32 |
| R2 | `adapter`/`platform/mysql` 之外不得出现 `*gorm.DB` | 类型检查 | 61 文件 |
| R3 | 禁止 `type Deps struct` | 类型检查 | 32 |
| R4 | `domain` 不得 import gin/gorm/redis/es | import 图 | 迁移中 |
| R5 | 模块之间不得互相 import | import 图 | 迁移中 |
| R6 | 单文件 ≤ 800 行 | 行数 | 4 |
| R7 | `enter.go` ≤ 50 行 | 行数 | 65 |
| R8 | `adapter` 之外不得调用 `Begin` / `Commit` | 调用图 | 12 |

> 现状是 R1–R12，编号与含义已演进，以 `backend/tools/archcheck/rules.go` 为准，详见 [治理与质量 · 架构守卫](../architecture/governance.md)。

## 理由与已否决方案

类型检查看的是实际类型而非字面量，别名绕不过；跨平台 `go run` 可进 CI；输出 `文件:行号` 可定位。

已否决的方案：

- **修 PowerShell**：仍是正则匹配符号名，且 Linux CI 跑不了；
- **只用 Go 测试**：载体对了方法错，仍是 `regexp.MatchString`；
- **depguard / forbidigo**：只能约束 import 路径，不能约束结构体字段类型，覆盖约 30%。

## 代价 / 不做什么

- 需写约 400 行工具代码；
- `packages.Load` 启动开销 2–5s（可缓存）；
- 存量 150+ 处违规需跨阶段清理；
- 规则会误报，需要逃生口。

## 当前生效约束

1. 双轨运行：`--all --warn` 输出债务基线不阻断，`--changed-since=origin/main --error` 只拦新增，逐模块清零后转全量 `--error`；
2. **规则自检是地基**：`TestEveryRuleDetectsViolation`（对人造违规必须报错）+ `TestEveryRulePassesCleanCode`（干净代码不得误报），两条测试缺一不可；
3. 逃生口 `//nolint:archcheck R2`；
4. 规则数量控制在 8–12 条，每加一条都要问"它会误伤什么"。

## 相关代码与文档

- 代码：[tools/archcheck/rules.go](../../backend/tools/archcheck/rules.go)（规则表与判定逻辑）
- 代码：[tools/archcheck/rules_test.go](../../backend/tools/archcheck/rules_test.go)（规则自检）
- 代码：[tools/archcheck/rules_id.go](../../backend/tools/archcheck/rules_id.go)（R12 `id-must-be-ctype`，见 [ADR-0007](./0007-id-string-type-as-truth.md)）
- 文档：[治理与质量 · 架构守卫](../architecture/governance.md)
- 原始记录：[04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md)
