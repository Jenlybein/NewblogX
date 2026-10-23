# blogx_monorepo 架构评审与重构设计

> 仓库：待重建（旧 `blogx_monorepo` 已废弃，重建为解耦的 `backend` / `frontend` / `frontend_admin` / `deploy`）
> 规模：约 **76,000 行**（Go 58.5k / Nuxt 20k；prototype 7.3k 在 S0 删除）
> 文档状态：**提案（Proposed）**，决策已收敛

---

## 一、执行摘要

### 规模

| 模块 | 文件 | 行数 | 说明 |
| --- | --- | --- | --- |
| `apps/api` | 541 Go | 58,508（非测试 37,931） | Gin + GORM 后端 |
| `apps/web` | 118 | 17,219 | Nuxt 读者站 |
| `apps/admin` | 24 | 2,835 | Nuxt 管理后台 |
| `prototype/web` | 20 | 3,312 | **死代码** |
| `prototype/admin` | 15 | 856 | **死代码** |
| `plugin/` | 1 | 711 | 仅 prototype 使用，**随 prototype 删除** |
| **合计（含待删 prototype）** | — | **≈ 83,000** | prototype 7.3k 将在 S0 删除 |

### 一句话诊断

> **这个项目最严重的问题不是"没有架构"，而是"架构是一场表演"。**

它有一整套看起来专业的东西：`app/` 装配层、`platform/` 抽象层、`repository/` 仓储层、`arch_guard.ps1` 架构守卫、`test/architecture/` 架构测试、monorepo + turbo。

**但它们几乎全部没有真正起作用**，而且最讽刺的是：

> `arch_guard.ps1` 和 `test/architecture/` 花了 370 行代码、13 条规则，
> 去禁止 `appctx`、`global`、`MustFromGin`、`DepsFromGin`、`mustApp` 这五个符号。
>
> **这五个符号在项目中一个都不存在。**

于是守卫全部永真通过，而真正的问题——32 个 `Deps` 结构体、61 个 handler 直接查库、空壳 repository、无 CI——**没有任何一条规则在看**。

### 五个 P0

| # | 问题 | 证据 | 为什么是 P0 |
| --- | --- | --- | --- |
| 1 | **伪依赖注入** | 32 个 `Deps`，30 个含 `*gorm.DB`；传递链最长 6 层 | 只是把 `global.DB` 改名成 `deps.DB`，God Object 换马甲 |
| 2 | **handler 直接查库** | `api/` 61 文件 import gorm，194 处调用 | 分层不存在，SQL 散落在 HTTP 层 |
| 3 | **repository 空壳** | 2,076 行（service 的 9%），`article_repo` 只有 `.gitkeep` | "仓储层"是装饰，连接仍由上层传入 |
| 4 | **架构守卫永真** | 检查的 5 个符号全不存在；`api import repository` 被 `type X = repo.X` 别名绕过 | 治理机制失效，腐化无人阻挡 |
| 5 | **无 CI / lint 空转** | 无 `.github`、无 git hook；`pnpm lint` 因 5 个包都没定义而**永远退出 0** | 2357 处测试断言、13 条规则，从不自动执行 |

### 三个"看起来做了其实没做"

| 表面 | 实际 |
| --- | --- |
| `platform/` 抽象层（3 个子包） | 共 **63 行**，每个都是 `return core.InitXxx(...)` 一行转发，无接口、无第二实现、无 mock 点 |
| `app/` 装配层（5 文件 370 行） | `app.Bootstrap` **从未被调用**；`Lifecycle.OnStop` **0 个调用点**，`stopHooks` 永远为空 |
| openapi 契约（14,462 行 / 118 operation） | 全仓**唯一引用是文件自身**，无 `openapi-typescript`，前端 80 个 interface 纯手写，契约与实现零机械校验 |

### 目标架构一句话

**合成根（Composition Root）+ 真实端口 + 单向依赖 + 可执行的架构规则 + 契约自动生成**，并且**取消 monorepo、拆成四个解耦的仓库级目录**：`backend/`（Go）、`frontend/`（Nuxt 读者站）、`frontend_admin/`（Nuxt 管理后台）、`deploy/`（Docker/compose/nginx）。

核心是把"依赖注入"从**结构体装箱**改成**接口注入**：handler 不再持有 `*gorm.DB`，而是持有 3-5 个方法的能力接口。配合 `archcheck`（Go AST 实现，跨平台、进 CI）替代 PowerShell 正则；openapi 由后端代码自动生成，前端各自派生类型。

### 迁移策略一句话

**不做大爆炸重写，但也不做"又一轮形式主义改造"**。

先删死代码（prototype 8,000 行 + 2.1MB dist）建立信心，再补 CI 让现有 2357 处断言真正跑起来，然后**用一个领域（article）把真 DI 走通**，验证后批量。每一步都必须有"可执行的验收规则"，不接受"看起来做了"。

---

## 二、文档导航

| 文档 | 内容 |
| --- | --- |
| [01-现状评审](./01-现状评审.md) | 分层、依赖、治理、前端、工程化五大块，逐项带行号证据 |
| [02-目标架构](./02-目标架构.md) | 合成根、端口设计、模块边界、前后端契约、守卫重做 |
| [03-物理目录与映射](./03-物理目录与映射.md) | 目标目录树 + 8.3 万行的去向表 |
| [04-迁移路线图](./04-迁移路线图.md) | S0–S5，每阶段可独立上线 |
| [05-代码质量补审](./05-代码质量补审.md) | 重复代码、超大文件、缺陷与浪费清单 |
| [06-测试体系重构与质量保障](./06-质量保障与治理.md) | 测试金字塔、testkit、有状态 fake、golden/契约、确定性、并行、CI 三档、变异测试 |
| [07-日志模块治理](./07-日志模块治理.md) | 日志被打爆的根因、有界异步管道、类型化事件、WAL、ClickHouse 批量、可观测性 |
| [08-重构任务清单](./08-重构任务清单.md) | M0–M6 里程碑、可验收细任务、依赖、提交规范、第一步 |

### ADR

| 编号 | 决策 |
| --- | --- |
| [ADR-001](./adr/ADR-001-依赖注入的真实形态.md) | 用接口注入取代 Deps 结构体装箱 |
| [ADR-002](./adr/ADR-002-合成根与模块边界.md) | 合成根放在哪、模块如何划边界 |
| [ADR-003](./adr/ADR-003-前后端契约单一来源.md) | openapi 从手写文档改为代码生成源 |
| [ADR-004](./adr/ADR-004-架构守卫可执行化.md) | 守卫从 PowerShell 符号匹配改为 AST 规则 |
| [ADR-005](./adr/ADR-005-日志有界异步管道.md) | 日志改为有界异步管道 + 类型化事件 |
| [ADR-006](./adr/ADR-006-测试体系分层.md) | 测试改为金字塔 + testkit + 有状态 fake |

---

## 三、已确认的架构决策（用户拍板，不再作为待确认项）

基于现状评审，以下决策已经确定，构成本文档的总约束：

1. **彻底拆解 monorepo，拆成四个解耦目录**。根目录只剩 `backend/`（Go）、`frontend/`（Nuxt 读者站）、`frontend_admin/`（Nuxt 管理后台）、`deploy/`（Docker / compose / nginx）。**不再有 `pnpm-workspace.yaml` / `turbo.json` / `packages/`**。三部分各自独立、`go.mod` / `package.json` 自洽，唯一的耦合点是后端自动生成的 openapi 契约。
2. **`Deps` 结构体直接废弃，不留过渡态**。32 个 `Deps` 全部删除，handler 只持有 3-5 个方法级能力接口，拿不到任何数据库连接。代码量会增加，但依赖关系第一次可读、可测。
3. **`prototype/` 删除**（84 文件 / 7,300 行 / 2.1MB dist，零 API 调用，硬编码 `demo-password`），其唯一消费者 `plugin/`（711 行）一并删除。
4. **`scripts/` 删除**。`arch_guard.ps1` 等 PowerShell 守卫全部移除，改为 `backend/tools/archcheck`（Go AST，跨平台、进 CI）。
5. **openapi 改为"像 FastAPI 那样自动维护"**。已删除手写 14,462 行文档。后端用 `swag` 注解 handler，构建时生成 `openapi.json`（构建产物，非手写）；两个前端各自用 `openapi-typescript` 从这份契约生成类型。**契约单一来源是后端代码，"代码即契约"**。
6. **前后端只保留单一上游**。两个前端各自只代理一个后端（`/api` → `API_BASE_URL` 环境变量），`_backend` / `_origin` 别名删除；反向代理的**唯一 upstream 定义在 `deploy/nginx`**（基础设施，不在 app 代码里）。
7. **admin 与 web 不合并为一个 app**。两个 Nuxt app 保持独立；它们之间唯一的"共享"是各自从后端 openapi 生成的类型，不存在共享代码包。
8. **重建 git 仓库**。删除旧 `.git`，重新 `git init`，首提交即目标形态，避免新旧仓库交叉。
9. **测试体系完全重构**。改为测试金字塔：domain 纯单测穷举规则、app 用有状态 fake 测编排、adapter 用真 MySQL(testcontainers) 验 SQL、HTTP 契约测、E2E 只保主链路。配套 `testkit`（clock/id/http/fake/fixture/golden），全部测试可并行、可复现。详见 [06](./06-质量保障与治理.md)、[ADR-006](./adr/ADR-006-测试体系分层.md)。
10. **日志模块彻底治理**。禁止把结构体塞进日志（字段封闭类型、禁 `any`）；改为**有界异步管道**（双限额队列 + 单写协程批量 + ClickHouse 直写 + WAL 兜底），请求路径不序列化/不落盘；去掉每条日志调雪花 ID 与全量调用栈。详见 [07](./07-日志模块治理.md)、[ADR-005](./adr/ADR-005-日志有界异步管道.md)。

---

## 四、不做什么

- **不拆微服务**。单体后端 + 两个前端 app 的形态是合理的。
- **不换 Go 框架**。Gin + GORM 没问题，问题在用法。
- **不重写前端**。Nuxt 4 选型正确，问题是页面过大与类型手写。
- **不保留 monorepo / workspace / `packages/` 共享包**。三部分解耦，唯一共享是后端自动生成的 openapi 契约。
- **不追求覆盖率数字**。重点是让已有的 2357 处断言**真的执行**。
- **不为了"看起来规范"再加一层抽象**。这次的目标恰恰相反：**删掉表演性的抽象**。

---

## 五、术语

| 术语 | 含义 |
| --- | --- |
| **合成根（Composition Root）** | 唯一允许 `new` 具体实现、组装依赖的地方。本项目应只有 `cmd/server/main.go` |
| **端口（Port）** | 由使用方定义的窄接口，如 `ArticleReader`、`ArticleWriter` |
| **Deps 装箱** | 现状模式：把 DB/Redis/ES/Logger 塞进结构体层层传递，用的时候自己取 |
| **能力接口** | 目标模式：`interface { FindByID(ctx, id) (*Article, error) }`，只暴露用得到的方法 |
| **可执行规则** | 能用 `go test` 或 CI 判定通过/失败的规则，而非文档约定 |
