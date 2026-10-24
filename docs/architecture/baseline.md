# 基线（M1 / S1.0）

> 生成时间：2026-10-24
> 对应提交：`8246bf2`（tag `refactor-start`）+ 本阶段改动
> 作用：**这是后续每一步"变好还是变坏"的唯一参照系。** 没有它，任何重构都无法判断对错。

---

## 0. 一句话结论

**代码本身是能编译、能跑的**：后端 77 个测试包全部通过，两个前端类型检查干净、71 个前端用例全绿。

问题不在"跑不起来"，而在**长期没有任何自动机制在看着它**：没有 CI、lint 从未执行过、架构守卫检查的是不存在的符号。M1 要做的就是把这两件事补上，并**证明新守卫真的会红**。

---

## 1. 工具链

| 组件 | 版本 | 说明 |
| --- | --- | --- |
| Go | **1.27.3** | `go.mod` 声明 `go 1.25.1`，实际用 1.27.3 编译通过 |
| gcc | 16.2.0 (MinGW-w64) | **必需**：sqlite 测试走 CGO，没有它 77 个包全部失败 |
| Node | 22.22.2 | |
| pnpm | **10.33.0**（corepack 固定） | 见下方"环境坑"第 2 条，**不能用 12.x** |
| 平台 | Windows 11 / Git Bash | |

### 环境坑（已解决，记录下来避免重复踩）

1. **Go 的 scoop 安装解压不完整**：`scoop install go` 报告成功，但 `src/` 缺 `unsafe`、`weak` 等目录，导致所有编译报 `package unsafe is not in std`。修复方式：直接从 scoop 缓存里的 zip 补齐缺失文件（共 3707 个）。
2. **pnpm 12.5.1 无法为顶层依赖建立链接**：`node_modules` 里只有空的 `@scope` 目录，`nuxt`/`vue` 等包完全没链接进来，`nuxt prepare` 因此永远失败。**必须用项目原本锁定的 pnpm 10.33.0**（`corepack prepare pnpm@10.33.0 --activate`）。两个 `package.json` 已加 `packageManager` 字段固定版本。
3. **`nuxt typecheck` 在 Windows 上会 OOM**：默认堆大小不够，进程直接崩（V8 栈都没有）。需要 `NODE_OPTIONS=--max-old-space-size=8192`。
4. **`go test ./...` 默认并行度会偶发 build failed**：`global_notif_api`、`site_api` 在默认并行下报 `[build failed]`，单独跑或 `-p 2` 就正常。属于本机资源限制，不是代码问题。
5. **行尾踩坑**：仓库 blob 是 LF，但工作区曾被写成 CRLF，导致 `eslint --fix` 之后十几个文件在 diff 里显示"整文件重写"（实际每处只改了一个字符）。已全仓统一为 LF 并新增 `.gitattributes`（`* text=auto eol=lf`）锁死。
   排查时注意：批量改文件后 `git status` 会出现大量**假 M**（索引 stat 缓存失效），**以 `git diff HEAD --name-only` 为准**。
6. **`nuxt prepare` 偶发 `EPERM`**：报 `operation not permitted, open '.nuxt/eslint.config.mjs'`，是 Windows 上文件被其他进程短暂占用，约 3 次中 1 次。`scripts/check.sh` 已对这条错误自动重试（最多 3 次），Linux CI 不受影响。

---

## 2. 后端基线

```bash
cd backend
export GOTOOLCHAIN=local CGO_ENABLED=1
go build ./...            # 通过
go vet ./...              # 通过，零告警
go test -count=1 -p 2 ./...   # 77 包 ok / 0 FAIL / 46 包无测试文件
```

| 指标 | 数值 | 备注 |
| --- | --- | --- |
| `go build` | **通过** | R0 搬目录没有破坏编译 |
| `go vet` | **零告警** | |
| 测试包通过 | **77** | |
| 测试包失败 | **0** | 前提是 `CGO_ENABLED=1` |
| 无测试文件的包 | 46 | 主要是不含逻辑的 `models/enum`、`buildinfo` 等 |

### 关键事实：**不开 CGO 时 77 个包全失败**

第一轮基线（`CGO_ENABLED=0`）的结果是：通过 40、失败 77，失败原因 172 处，全部是同一句：

```
Binary was compiled with 'CGO_ENABLED=0', go-sqlite3 requires cgo to work. This is a stub
```

这说明现有测试**高度依赖 sqlite 内存库**（`test/testutil` 里的 `SetupSQLite`）。

⚠️ **这是"假安全"，必须在 S3 之前处理**：sqlite 与 MySQL 在 `ON CONFLICT`、`SELECT ... FOR UPDATE`、唯一键冲突、`AUTO_INCREMENT` 等行为上并不一致。sqlite 上通过 ≠ 生产正确。CI 里因此分成两档：fast 档用 sqlite，nightly 档用 testcontainers 起真 MySQL。

---

## 3. 前端基线

```bash
cd frontend        # 读者站
corepack pnpm exec nuxt prepare
corepack pnpm lint        # 0 error / 3 warning
corepack pnpm typecheck   # 通过
corepack pnpm test        # 21 文件 / 71 用例 全绿
corepack pnpm test:coverage
```

| 指标 | 读者站 | 管理后台 |
| --- | --- | --- |
| `nuxt prepare` | 通过 | 通过 |
| `lint` | **0 error**，3 warning | **0 error**，3 warning |
| `typecheck` | 通过 | 通过（需先补 `@types/node`） |
| `test` | **21 文件 / 71 用例全绿** | **0 用例**（已装 vitest 并留好入口） |
| 覆盖率 | 语句 62.47 / 分支 50.96 / 函数 67.14 / 行 62.62 | — |

### lint 首次执行时发现的真实问题（已修）

这些是"从来没跑过 lint"的直接证据，全部是真实缺陷，不是风格问题：

| 文件 | 问题 | 处理 |
| --- | --- | --- |
| `nuxt.config.ts` | `node:fs` 被 import 两次 | 合并 |
| `app/services/image.ts` | `let f = 0; let k = 0` 初值被所有分支覆盖，属死赋值 | 改为声明不赋初值 |
| `app/services/mermaid-runtime.mjs` | 正则里 `\"` 是无意义转义 | 去掉多余转义 |
| `server/routes/_backend/[...path].ts` | 取了 `requestPath` 却从未使用 | 删除 |
| `app/components/auth/Email{Login,Register}Form.vue` | `timer && clearInterval(timer)` 短路写法 | 改 `if (timer)` |
| `app/pages/users/[id].vue` 等 4 处 | 导入了未使用的组件/函数（含一个 10 行的死函数） | 删除 |
| `frontend_admin` | **缺 `@types/node`**，8 处 `Cannot find name 'process'` | 补依赖 |
| 两端 | **都没显式声明 `typescript`**，靠间接依赖碰运气 | 显式声明并锁 5.9.3 |

另有一条规则是误报：`vue/no-multiple-template-root` 是 Vue 2 遗留规则，在 Nuxt 4（Vue 3）下 `layouts/auth.vue` 只放一个 `<slot/>` 完全合法，已在管理后台配置里关闭并写明原因。

### 覆盖率门槛（只升不降）

`frontend/vitest.config.ts` 已设：语句 60 / 分支 50 / 函数 65 / 行 60。
取实测值向下取整，作为基线；以后每补一批测试就上调，**永不回调**。

---

## 4. 架构基线：324 处违规

`backend/tools/archcheck` 首次全量扫描（`--mode=warn`）：

| 规则 | 含义 | 违规数 |
| --- | --- | --- |
| **R1** | api 层禁止直接使用 `*gorm.DB` | **144** |
| **R2** | 禁止定义 `type Deps struct` | **35** |
| **R8** | 禁止 `errors.New` 用中文字面量 | **126** |
| **R7** | 禁止裸 `go` 语句 | **17** |
| **R6** | 单文件不得超过 800 行 | **2** |
| **R3** | service/domain 层禁止 import gin | **0** |
| **R4** | domain 层禁止 `time.Now()` | **0**（尚无 domain 层） |
| **R5** | api 层禁止 import repository | **0** |
| | **合计** | **324** |

### 这组数字为什么重要

评审时的判断是"archcheck 上线时必须报出 150+ 处真实违规，如果第一天全绿就说明规则无效"。
实测 **324 处**，且分布完全吻合现状评审的结论：

- R2 的 35 处几乎全部落在 `api/*/enter.go` 与 `apideps/`
- R7 的 17 处包含 `comment_create.go:119,130`、`article_digg.go:73`——正是评审里点出的"7 条能让进程直接退出的路径"
- R6 的 2 处是 `service/es_service/article_sync.go`（1,547 行）和 `service/river_service/sync.go`（852 行）

R3/R5 为 0 说明这两条当前确实没被违反（不是规则失效——`rules_test.go` 已验证它们能抓到人造违规）。

---

## 5. M1 交付物与验收

| 项 | 内容 | 状态 |
| --- | --- | --- |
| S1.0 | 本文件（基线） | ✅ |
| S1.1 | `.github/workflows/ci.yml` | ✅ |
| S1.2 | CI 跑 `go vet` + `go build` + `go test -race -short` | ✅ |
| S1.3 | 两端装 eslint 并定义 `lint`（不再永真退出 0） | ✅ |
| S1.4 | CI 跑 `pnpm typecheck` | ✅ |
| S1.5 | **门禁自检**：注入违规，断言门禁会红 | ✅ 4/4 |
| S1.6 | `backend/tools/archcheck`（Go AST） | ✅ |
| S1.7 | 首批 8 条规则，`--warn` 起步 + 棘轮 324 | ✅ |
| S1.8 | `rules_test.go` 规则自检 | ✅ |
| S1.9 | 删除重复且失效的 `test/architecture/` | ✅ |
| S1.10 | `buildinfo` 改 ldflags 注入 + `--version` | ✅ |
| S1.11 | `BLOGX_*` 命名空间白名单 | ✅ |
| S1.12 | vitest 覆盖率门槛 | ✅ |

### 门禁自检结果（最关键的一条）

`scripts/verify-gates.sh` 故意制造 4 类违规，实测：

```
✓ 门禁有效：backend 单测            正确失败
✓ 门禁有效：archcheck 棘轮          正确失败
✓ 门禁有效：archcheck 规则自检      通过（规则自检齐全）
✓ 门禁有效：前端 typecheck          正确失败
```

**这四行是本次重构与上一轮的实质区别。** 上一轮的守卫检查 `appctx`、`global`、`MustFromGin`、`DepsFromGin`、`mustApp` 五个符号，而这五个符号在项目里一个都不存在，所以 13 条规则全部永真通过。

顺带一个佐证：旧 `test/architecture/no_global_refs_test.go` 用正则 `\bglobal\.` 扫全文，把新写的 `archcheck/rules.go` 注释里的 "global.DB" 判成了违规——它连注释和字符串分不清。该目录已删除。

---

## 6. 下一步的对照点

后续任何一步做完，都应该回来对比这几个数字：

| 指标 | 现在 | 目标 |
| --- | --- | --- |
| archcheck 违规数 | **321**（M2 后，棘轮已收紧到 321） | 单调递减至 0 |
| `type Deps struct` | 35 | 0 |
| api 层 `*gorm.DB` | 144 | 0 |
| 裸 `go` 语句 | 17 | 0 |
| 后端测试包失败数 | 0 | 保持 0 |
| 前端 lint error | 0 | 保持 0 |

---

## 7. M3-A：平台层地基（已交付）

M3 的 17 项里，先落地"所有后续模块都要用"的四件套。它们全部是新包、无历史包袱，
因此可以做到**零新增违规**，并把测试写成可并行、无全局状态。

| 项 | 落点 | 说明 |
| --- | --- | --- |
| **S2.1** | `internal/platform/mysql` | `TxManager` 窄接口 + GORM 实现 + 可注入失败的 fake；已接入 `middleware.Deps.Tx` |
| **T.3（核心）** | `testkit/` | Clock / SeqID / Golden / 独立内存 SQLite，**无包级状态**，可在 `t.Parallel()` 下用 |
| **L.1** | `internal/platform/log` | 类型化事件：`Field` 是封闭联合（str/int/uint/float/bool/dur/time/strs），无 `any` |
| **L.2** | 同上 + `WriterSink`/`NewFileSink` | 有界异步管道（条目 8192 + 8MB 双限，满则丢弃计数）+ 批量写 JSON Lines，输出字段/时间格式与现有 logrus 一致 |
| **R7 正解** | `internal/platform/safego` | 唯一允许写裸 `go` 的地方；archcheck 对此目录**精确豁免**（并有反向用例证明其他 platform 包仍被拦） |

### 验收实测

| 检查 | 结果 |
| --- | --- |
| `go build ./...` / `go vet ./...` / `gofmt -l` | 全部干净 |
| `go test ./internal/... ./testkit/... -race` | 全绿（含 8 路并行 sqlite 隔离、5000 条灌入不阻塞、关闭 drain） |
| `go test ./middleware/... ./app/... ./router/... ./api/...` | 全绿 |
| archcheck | **321 ≤ 321**，零新增；R7 豁免有正反用例 |
| 棘轮 | `check.sh` / `verify-gates.sh` / `ci.yml` 同步收紧到 321 |

### 为什么"地基"要先做这四件

- **事务**：M4 的 `internal/article/app` 必须能在一个事务里完成"写文章 + 发事件"，
  并且能在没有数据库的情况下用 fake 跑回滚测试。没有 `TxManager`，M4 无法开始。
- **testkit**：现有 `test/testutil` 是全局单例（`InitGlobals()` + 包级 `testDB`），
  测试之间互相污染、无法并行。新模块若沿用这套，重构会继续积累技术债。
- **日志**：M3 之后所有模块都要写日志。先把"类型化 + 有界 + 不阻塞"的管道立起来，
  后续模块直接接入，避免再造一批 `SanitizeValue(any)`。
- **safego**：R7 有 17 处存量裸 `go`。要修它们，必须先有一个合法的替代品；
  否则规则只降不升的要求会让任何后台任务都无从写起。

### 尚未覆盖（M3 剩余批次）

| 批次 | 内容 | 规模 | 风险 |
| --- | --- | --- | --- |
| M3-B | S2.2 redis 拆 counter/guard/cache + S2.3 es 承接 | ~5,500 行，~15 处 importer | 中（机械迁移，但面广） |
| M3-C | S2.4 拆 `article_sync.go`(1547) + S2.5 合并 river(3231→~1800) | ~4,800 行 | 高（行为等价需回归） |
| M3-D | S2.6 `platform/llm` 重写 + S2.7 结构化输出 | ~2,800 行 | 中（需超时/ctx/重试） |
| M3-E | S2.8 swag 生成 + S2.9 前端类型生成 | 工具链 | 低-中（契约先行） |
| M3-F | L.3–L.5 生产写入口替换 + ClickHouse 批量/WAL | 跨模块 | 高（合规：audit 不可丢） |
| M3-G | T.4 有状态 fake + T.8 testcontainers | 测试 | 中（需 Docker） |
| 前端覆盖率（语句） | 62.47 | 单调递增 |

**规矩**：违规数只准降不准升。一旦某次提交让任何一个数字变差，就是引入了回归。
