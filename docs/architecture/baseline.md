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

**另一处"门禁会撒谎"的坑（2026-10-25 修复）**：`scripts/check.sh` 把后端步骤包在
子 shell `( ... )` 里，而 `run()` 只在子 shell **内部**置 `FAILED=1`。
子 shell 的退出码等于**最后一条命令**的成败，于是出现"archcheck 明明红了、
最后一条 `go test` 是绿的 → 整体打印 ✓ 全部门禁通过、退出码 0"。
比没有门禁更危险：它让人误以为验证过了。

修法：`run()` 失败时 `return 1`；每个子 shell 末尾显式 `exit "$FAILED"`。
并用"把棘轮上限临时改成 1"验证过——现在确实 `✗ 门禁未通过` + 退出码 1。

---

## 6. 下一步的对照点

后续任何一步做完，都应该回来对比这几个数字：

| 指标 | 现在 | 目标 |
| --- | --- | --- |
| archcheck 违规数 | **250**（见下方 S4.0 重标说明；S3.11 时是 236） | 单调递减至 0 |
| `type Deps struct` | 34 | 0 |
| api 层 `*gorm.DB` | 142 | 0 |
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

### M3-B / M3-D / M3-C1 / M3-C2：基础设施归位 + AI 客户端 + 超长文件拆分 + CDC 合并（已交付，棘轮 321→318）

| 批次 | 内容 | 结果 |
| --- | --- | --- |
| **M3-B（S2.2/S2.3）** | `service/redis_service` → `internal/platform/redis`；`service/es_service` → `internal/platform/es`；89 个文件 import 同步改写 | 纯路径迁移，行为零变化；移动包测试全绿；无残留旧路径 |
| **M3-D（S2.6）** | 新建 `internal/platform/llm`：超时 / ctx / 重试 / 连接池；流式协程改 `safego.Go`；`ai_service` 变薄委托层 | R7 −1（`ai_service/enter.go` 的裸 `go` 消失），棘轮收紧到 **320** |
| **M3-C1（S2.4）** | `internal/platform/es/article_sync.go`(1547) 按职责拆为 `article_sync.go` / `_update.go` / `_load.go`，再收进 `articlesync/` 子包（436 / 386 / 753 行） | 同包内拆分 + 子包归拢、语义不变；R6 −1，棘轮收紧到 **319** |
| **M3-C2（S2.5）** | `service/river_service` → `internal/platform/cdc/river`；`service/image_ref_river_service` → `internal/platform/cdc/imageref`；`service/cdc_dead_letter_service` → `internal/platform/cdc/deadletter`（消除 platform→service 反向依赖）；`river/sync.go`(852) 拆为 `sync.go`(223) / `sync_request.go`(397) / `sync_bulk.go`(265) | 两包互不依赖，纯路径迁移 + 同包内拆分；函数数精确对上 27 = 10+12+5；**R6 归零**，棘轮收紧到 **318** |

验收：`go test ./... -count=1` 全绿；llm 6 用例（成功/5xx重试/4xx不重试/ctx取消/流式/未开启）全绿；
canary 注入 3 处违规 → 323 > 320 正确失败。

M3-C2 追加验收：`go build` / `go vet` / `gofmt -l` 全干净；`go test ./... -count=1` 全绿；
`internal/platform/cdc/{river,river/rule,imageref,deadletter}` 编译与测试通过；
无残留旧路径引用；canary 注入 4 处违规 → **322 > 318 正确失败**，清理后 318 通过。

### 为什么"地基"要先做这四件

- **事务**：M4 的 `internal/article/app` 必须能在一个事务里完成"写文章 + 发事件"，
  并且能在没有数据库的情况下用 fake 跑回滚测试。M3 已交付 `TxManager`，但接口形态
  （回调交出 `*gorm.DB`）会让 app 反向依赖 gorm，**M4 步骤 0 需改造成 UnitOfWork**。
- **testkit**：现有 `test/testutil` 是全局单例（`InitGlobals()` + 包级 `testDB`），
  测试之间互相污染、无法并行。新模块若沿用这套，重构会继续积累技术债。
- **日志**：M3 之后所有模块都要写日志。先把"类型化 + 有界 + 不阻塞"的管道立起来，
  后续模块直接接入，避免再造一批 `SanitizeValue(any)`。
- **safego**：R7 有 17 处存量裸 `go`。要修它们，必须先有一个合法的替代品；
  否则规则只降不升的要求会让任何后台任务都无从写起。

### M3 进度盘点（按 08 §4 的 M3 范围）

| 项 | 状态 | 落点 / 说明 |
| --- | --- | --- |
| S2.1 | ✅ | `internal/platform/mysql`：`TxManager` 窄接口 + GORM 实现 + fake |
| S2.2 | ✅ | redis 10 子包 → `internal/platform/redis` |
| S2.3 | ✅ | `es_service` → `internal/platform/es` |
| S2.4 | ✅ | `article_sync.go`(1547) → `es/articlesync/` 三文件（436 / 386 / 753） |
| S2.5 | ✅ **结构 + 引擎唯一** | 两套 river + deadletter → `platform/cdc/{river,imageref,deadletter}`；新增 `cdc/canalbase` 单一引擎（连接/日志/表过滤/dump 范围），业务子包内已无 `canal.NewCanal`；`sync.go`(852) 拆三文件。行数 3,237（原 3,231，见下方结论） |
| S2.6 | ✅ | `internal/platform/llm`：超时 / ctx / 重试 / 连接池 |
| S2.7 | ✅ | JSON 抽取已收敛为 `UnmarshalJSONBlock` 单入口（另一处正则是去代码块，非 JSON 解析） |
| S2.8 | ✅ | 117 个业务 operation 全部补注解（article 30 / user 20 / log 11 / 其余 56），`swag init` 产出 `backend/docs/swagger.json`；`make openapi` 可复现。`/health` 为 infra 探针、不在 `/api` 下且返回空 body，不纳入业务契约 |
| S2.9 | ✅ | 两前端 `pnpm openapi:types`：`swagger2openapi`（2.0→3.0）→ `openapi-typescript` 生成 `app/types/openapi.d.ts`（117 operations）；`app/types/contract.ts` 提供 `Schema<>`/`Operation<>`；`api.ts` 保留为视图模型层 |
| L.1 / L.2 | ✅ | `internal/platform/log`：类型化事件 + 有界异步 + file sink |
| L.3 | ✅ | 生产入口已接线：删掉 `jsonLineSink`（同步 `json.Marshal` + 每条一次文件写 + 全局锁），改由 `log_service/pipeline.go` 五条有界队列承接；`EmitActionAudit` 等 4 类写入与 logrus 的 `FileDateHook` 全部改为投递 |
| L.4 | ✅ | `log/persist`：`ClickHouseSink` + `WALSink` + `ReplayWAL`；管道在启用 ClickHouse 时组合为 `WALSink(ClickHouseSink)`，并挂在启动时自动重放；5 张日志表引擎改 `ReplacingMergeTree` |
| L.5 | ✅ | 删掉 fluent-bit 服务与其 5 个 app tail（`Skip_Long_Lines` 会丢长行、`Read_from_Head` 会重放历史）。应用现在直接写 ClickHouse，中间不再有采集器 |
| L.10 | ✅ | 审计字段瘦身（选项 1 纯热库）：删 4 个 `*_raw` 列与字段；`request_body`/`response_body` 仅失败时写 + `CODEC(ZSTD(9))`；`ActionAuditInput.RequestBody/ResponseBody` 改 `[]byte`、`GinAuditInput` 去 body/header 入参；请求体边界 `io.LimitReader` 限 1MB；archcheck 新增 R9（slug `log-no-any`） |
| T.3 / T.4（tx） | ✅ | `testkit/`：Clock / SeqID / Golden / 独立 SQLite；`mysql.FakeTxManager` |
| T.8 | ✅ | 集成测试改 testcontainers：`internal/testkit/chcontainer` 自动起 ClickHouse 并执行 `01-logs.sql`；无 Docker 时 `t.Skip`；`BLOGX_CH_ADDR` 可复用已有实例 |

#### M3 阶段验收（2026-10-24 通过）

M3 = S2 全部 + L 全部（L.1–L.5 + 提前完成的 L.10）+ T 基建（T.3 / T.4 基建 / T.8），全部达标：

| 维度 | 项 | 状态 |
| --- | --- | --- |
| 契约 | S2.8 = 117 business operations；S2.9 = 两前端 `openapi.d.ts` + `Schema<>`/`Operation<>` | ✅ |
| 日志 | L.1–L.5 端到端异步管道（WAL + ClickHouse sink）；L.10 审计瘦身（默认不记全量 raw） | ✅ |
| 测试 | T.3 testkit / T.4 tx·outbox fake / T.8 testcontainers | ✅ |
| 门禁 | 全量 `scripts/check.sh` exit 0；archcheck 棘轮 318 未上升；门禁自检 4/4 | ✅ |
| 集成 | `go test -tags integration`（testcontainers 自动起 ClickHouse）两包 ok | ✅ |

刻意的范围边界（不是缺陷）：

- `/health` 不纳入 OpenAPI：infra 探针、不在 `/api` 下、返回空 body。
- `api.ts` 保留为**视图模型/适配层**；契约 DTO 的唯一来源是生成的 `openapi.d.ts`。
- **L.6–L.9**（删 `logsafe` 反射 / 去雪花 / TTL·压缩 / 日志规则）仍按原计划归 **M6**，
  不在 M3 范围。

#### M4 前置条件与七项决策（2026-10-24 拍板，代码未动）

M4 = S3，范围定为**全 7 个子域**（article 主体 / review / favorite / top / view_history /
category / tags），生产代码 3,547 行。开工前复核出四处与早期文档不符的实际口径：

| 早期文档 | 实测 |
| --- | --- |
| 194 处 handler SQL | **63 处**（grep）/ archcheck R1 命中 69 处（全 api 层 140/142） |
| `BeforeDelete` 级联上移 | **该钩子不存在**；级联在 `service/article_service/delete.go` 的 `DeleteArticles(tx *gorm.DB, ...)`，软删 **6 张表** + 本体，逐条 select+delete（N+1） |
| article 模块即主体 | 另有 **34 处跨模块调用**：鉴权 8 / 跨模块写 4（`StatApplyArticleDelta`）/ 读展示 8 / 图片 6 / 子域互调 8 |
| repository 有存量 | `repository/article_repo` 是**空壳**，S3.4 是从零实现 |

七项决策：

1. **范围**：全 7 子域，可整体删 `api/article_api`；`models/` 保留到 S5.2，故 ai/search/cron 引用不必提前改。
2. **事务**：UnitOfWork —— `WithinTx(ctx, fn(ctx, Repos) error)`，`Repos` 由 app 定义。
   **M3 交付的 `TxManager`/`FakeTxManager` 需返工**：现状 `fn(ctx, *gorm.DB)` 会把 gorm 直接塞进 app，
   与 S3 验收「app 内不得出现 `*gorm.DB`」自相矛盾。
3. **目录**：水平一套三层 `internal/article/{domain,app,adapter}`，子域在 domain 内用子包，
   共享同一事务边界。
4. **golden**：写死种子（20 篇 × 6 关联表，ID/时间/计数全硬编码），响应**逐字节比、不归一化**；
   时区在种子内 `SET time_zone='+08:00'`。
5. **MySQL 契约测试**：新增 `internal/testkit/mysqlcontainer`（照 `chcontainer` 模式）；
   sqlite 不可用于此项。
6. **跨模块写**：`AuthorStatsPort` 同步、复用同一事务；不改异步（会改变"统计立即生效"的语义）。
7. **旧测试**：迁能迁的（业务规则 → domain 不变量矩阵），删绑死 DB/Gin 的；2,661 行不整体保留。

**硬约束：零 DDL。** ES 投影纯靠 CDC（`cdc/river/article_row_snapshot.go` 按列名解析 binlog），
`api/article_api` 内 0 处调用 ES；删列或改名会直接打穿投影。

#### M4 进度（2026-10-24，代码已落地）

| 步 | 内容 | 状态 |
| --- | --- | --- |
| S3.0 | `testkit/mysqlcontainer`（testcontainers MySQL 8，独立 schema，DSN 钉 +08） | ✅ |
| S3.1/S3.2 | `internal/article/domain` 全 7 子域 + 不变量矩阵 **28 组**（4 状态 + 已删除）× 角色 + 两种隐藏可见性 | ✅ |
| T.4 | `internal/article/apptest` 有状态 fake（7 仓储 + 4 端口，带故障注入/计数/快照回滚） | ✅ |
| S3.3 | `internal/article/app` 全用例（create/update/examine/delete/query/interaction/subdomain）+ `Repos`/`TxManager`/`AuthorStatsPort` | ✅ |
| S3.4/S3.8 | `adapter/mysql` 六仓储 + mapper（row 复用 models，零 DDL） | ✅ 真库契约测 8 条 |
| S3.5/S3.6 | `adapter/http` 30 端点 + 路由开关 + 装配；HTTP 层零 SQL | ✅ e2e 通过 |
| S3.9 | 16 个写接口埋审计 | ✅ |
| S3.10 | golden 机制（写死种子 + 逐字节，已验证可复现） | ✅ 20 篇 × 12 接口、32 个基线文件、并行连跑稳定 |
| S3.11 | 删旧 `api/article_api` / `article_service` / `repository/article_repo` | ✅ 按用户指令提前执行；棘轮 318→236 |
| 收尾① | `MdToSafe` 并发缺陷 | ✅ 每次新建引擎（池化修不了），golden 恢复 `t.Parallel` |
| 收尾③ | outbox 下游接回真实副作用 | ✅ 事务落表 + CDC 中继（含租约恢复/死信）；评分/通知/点赞/收藏已接回 |
| 收尾⑤ | 子域端点 e2e | ✅ 新增 `e2e_subdomain_integration_test.go` 覆盖读写端点 |

**M4 期间的两次棘轮回归（均已修复，记录以免重犯）**

1. domain 首版写了 27 处 `errors.New("中文")` → R8 125→152、合计 345。
   处理方式是补上规则一直指向却从未落地的 `internal/apperr`（错误码 + 集中文案表），
   而不是放宽规则。副产品：S5 清理存量 125 处时终于有统一去处。
2. `adapter/http` 首版直接用 gorm 句柄查图片反查与作者信息 → R1 142→147。
   收口方式：新增 `app.AuthorProfile` 用例 + `ImageResolver` 端口。
   这条恰好证明"收回 handler SQL"对新代码同样适用——新写的 handler 也会犯同样的错。

**M4 过程中发现的三个真实缺陷（不是文档估算偏差，是代码 bug）**

- `article_tag_models` 是 `many2many` 自动建的**纯 join 表**（只有 `article_id`/`tag_id`，
  无 `id` 无 `deleted_at`）。用 `models.ArticleTagModel` 结构体查会被 GORM 加上
  `deleted_at IS NULL` 而报 Unknown column；且软删同一对 (article, tag) 再次插入会撞唯一键。
  已改为 `Table(...)` 直连 + 硬删。
- GORM 对零值 `CreatedAt` 会自己填 `time.Now()`，导致落库时间不由用例层的可注入时钟决定。
  golden 会因此变成"每次都红"或"每次都绿"的假测试。mapper 已显式写 `CreatedAt`。
- 旧链路的 `ArticleExamineView` **从未被任何路由注册**（审核实际走 `article-review-tasks/:id/review`）。
  按"不照抄死代码"处理：新链路不实现该端点。

**golden 上线第一天抓到的并发缺陷（已修复）**

`utils/markdown.MdToSafe` 在并发调用下输出不稳定：同一份输入 `"正文A"`，
有时产出 `"正文A\n"`、有时 `"\n正文A\n"`；并发压测时甚至会直接 panic。
影响面：文章正文/摘要的落库值不稳定，进而影响 ES 投影与前端渲染。

根因（注意：不是"goldmark 没做并发隔离"）：`mdrenderer.Renderer` 把渲染中间状态
（`previousLineBlank` / `wasAtNL` / `bufs` 等）存放在渲染器**实例**上。
构造时 `previousLineBlank` 被初始化为 `true`（源码注释即
"document (conceptually) starts with a blank line"），
但 `Render` 结束时不会复位、开头也不会重置——
**复用实例必然继承上一次渲染留下的脏状态**。

由此得到两个结论：

1. `sync.Pool` 复用实例**修不了**这个问题——"归还再取出"只是换一个 goroutine
   来继承脏状态，并不能清零；而这些字段在 `internal` 包里是私有的，外部也无法
   手动重置。（实测：池化后 panic 消失，但仍有约 12% 的输出不一致。）
2. 正确做法是**每次调用新建一份干净引擎**。

修复：`MdToSafe` 改为每次调用 `newSafeEngine()`；`rawHTMLMarkdownEngine`
（goldmark 自带 html 渲染器，每次 Render 新建上下文）经并发 + `-race` 验证
可安全共享，保持单例。
实测开销：新建引擎约 12µs、`MdToSafe` 整体约 22–28µs，且只在文章创建/更新
两处写路径调用，不在读路径，代价可接受。

回归：`utils/markdown/md_concurrency_test.go`（32 goroutine × 100 次 × 8 种输入，
覆盖公式/代码块/列表/引用等分支）——修复前必失败（含 panic），修复后
普通与 `-race` 均稳定通过；`TestArticleGolden` 已恢复 `t.Parallel()`
并连跑 3 次全绿。

#### 事务型发件箱（outbox）：把"内存同步分发"换成"落表 + CDC 中继"

M4 收尾时新链路的事件出口其实是个空壳：`Emit` 在内存里同步调已注册 handler，
而 handler 体只有一行 debug 日志，且只挂了 published/deleted 两个事件。
若就这样上线，"评分 / 系统通知"两类副作用会整体丢失。

改造后的结构分四层：

1. **写侧（`internal/article/adapter/mysql/outbox.go`）**
   `repos.Outbox()` 用**当前事务句柄**构造写入器，`Emit` 就是往
   `outbox_event_models` 插一条 `status='pending'` 的行。
   于是事件与业务**同事务原子提交**：`事件存在 ⇔ 业务已提交`；
   事务回滚时事件一起消失（`TestOutboxWriterTxRollback` 守着这条）。

2. **中继（`internal/platform/cdc/outboxrelay`）**
   - 状态机 `pending → processing → done`；失败回 `failed`（带退避），
     重试耗尽转 `dead` 并写死信；
   - **原子领取**：靠 `UPDATE ... WHERE status IN (pending,failed) AND available_at<=now`
     的 `RowsAffected` 判定归属，保证同一条事件只被一个 worker 处理；
   - **租约恢复**：worker 领取后崩溃会把行卡死在 `processing`，
     所以 `claimable` 还接受"`processing` 且 `updated_at` 超过租约"的行，
     否则崩一次就永久丢一条事件；
   - **两条摄入路径**：river 订阅 outbox 表 binlog INSERT（低延迟），
     加周期性补偿扫描（兜底，river 没启用也能投递）。

3. **下游订阅者（`app/outbox_wiring.go`）**
   - 评分：`EnsureArticleScoreIfMissing`（同步版——中继本身已异步，
     用同步版才能拿到错误去重试；且它"缺失时才生成"，天然幂等）；
   - 通知：审核通过/拒绝对作者发系统通知；点赞/收藏通知作者。
     **点赞/收藏这两处是 ④ 删旧代码后暴露的真实回归**：旧实现是 handler 里的
     裸 goroutine，删掉旧代码后通知会静默消失。现已接回并加单测锁死
     （`TestDiggToggle` / `TestFavoriteAndUnfavorite` 断言事件必须发出）。

4. **不双写**：ES 投影继续由 article 表的 CDC 驱动（本来就在跑），
   outbox 中继不碰 ES，避免同一份数据出现两条写路径。

5. **保留期清理（TTL）**
   中继自带清理协程，周期性物理删除**已终结**的事件行：
   `done` 默认保留 7 天、`dead` 默认保留 30 天（后者更久，便于排查；
   死信在转 `dead` 时已同步写入 `cdc_dead_letter_models`，删掉不丢证据）。
   `pending/failed/processing` **永不清理**——它们代表尚未投递完，清掉等于丢事件。
   实现上先按 `updated_at < cutoff` 捞 ID、再按 ID 物理删除（`Unscoped`，
   因为模型内嵌软删列，软删回收不了磁盘），分批删（默认 1000/批、单轮最多 20 批），
   避免大事务长时间持锁。旋钮在 `config/settings.yaml` 的 `outbox` 段。
   没有这一步，每篇文章的创建/发布/点赞/收藏都会沉淀一行，表只增不减。

新增表 `outbox_event_models` 不违反零 DDL：该约束只禁止改/删**既有**表
（ES 投影按列名解析 binlog）。这张表还特意**排除在全量转储之外**，
否则 canal 启动时会把历史已完成事件按 INSERT 全量重放一遍。

验证：`outboxrelay` 9 个集成用例（成功 / 幂等 / 重试耗尽进死信 / 扫描 /
未到点跳过 / 租约恢复 / worker 消费 / 保留期清理）+ 写侧 2 个事务用例，全部打真 MySQL。

**M4 过程中的一次棘轮回归与修复**：S3.1 首次提交时在 domain 里写了 27 处
`errors.New("中文")`，archcheck 从 318 涨到 345（R8 125→152），违反"只准降不准升"。
处理方式不是放宽规则，而是补上规则一直指向却从未落地的 `internal/apperr`：

- 错误码为稳定契约（`article.not_found` / `taxonomy.tag_disabled` …），文案集中在
  `apperr.messages` 一处，调用点只写码；
- `apperr` 只依赖 stdlib，因此 domain 仍能满核并行测试；
- 修完 R8 回到 125、合计回到 318。

副产品：S5 清理存量 125 处中文 error 时，终于有统一去处。

#### L.3 的接线方式：一条管道，五张表，两种出口

改造前：`EmitActionAudit` → `jsonLineSink.write` → 每个请求线程里 `json.Marshal`
再 `file.Write`（一把全局 `sync.Mutex` 串住所有写）。HTTP 访问日志走 logrus 的
`FileDateHook`，同样是「每条一次 write」。

改造后只有一条路径：

```
业务调用 ──► emitRow(kind, level, row) ──► json.Marshal ──► Emitter.EmitLine
                                                        │  有界队列（条目 + 字节双限）
                                                        ▼
                                              写协程：攒批 → 1s 或 500 条触发
                                                        │
                              ┌─────────────────────────┴──────────────────────┐
                              ▼                                                ▼
                   ClickHouse 开启                            ClickHouse 关闭
                   WALSink(ClickHouseSink)                    dailyFileSink
                   ├ 成功：直接进表                            └ 与改造前完全同路径、同字节
                   └ 失败：落 <wal_dir>/<类别>/*.jsonl
```

要点：

- **五条独立队列**：每类日志一个 `Emitter`，审计日志暴涨不会把 runtime 挤掉。
- **编码留在业务侧**：预算是按字节算的，编码后才知道真实大小；且这些结构体本身
  就是表的行形态，`platform/log` 因此不必接受 `any`，`Event` 的字段集合保持封闭。
  请求路径上被拿掉的是文件写、全局锁与 fsync——这些才是阻塞项。
- **未启用 ClickHouse 时文件就是最终落点**，不套 WAL：往同一个磁盘再写一份"兜底"
  挡不住磁盘满，只会让失败路径更长；落点与文件名（`<dir>/<类别>/<日期>/<app>.log`）
  一个字节都没变，外部采集器不需要调整。
- **启动即重放**：`InitStructuredLog` 先把上次遗留的 WAL 灌回主出口；失败只告警
  不阻断启动（文件还在，下次再试）。
- **停机顺序**：日志管道最后注册、最先执行，必须先把队列刷完再关 ClickHouse 连接
  （见 `app.RegisterInfraCleanup`）。
- **惰性兜底**：单测与命令行工具不走 `app.InitInfra`，首次写日志时自动建一条
  "只写文件"的管道，不 panic、不丢日志。

**`log_middleware` 的位置没变**：它仍然通过 `logrus` 打点，只是 `FileDateHook`
换成了 `core.StructuredLogHook`——三行转发，不持文件句柄、不做 IO。条目到列的映射
集中在 `buildRuntimeRow`：表里有的列按名取，表里没有的字段一律进 `extra_json`
（宁可多带一列 JSON，也不静默丢字段）。

#### L.4 / L.3 的验证方式

**1) 断库不丢（`persist` 集成测试）**

```bash
docker run -d --name blogx-ch-it -p 18123:8123 -p 19000:9000 \
  -e CLICKHOUSE_DB=blogx_logs -e CLICKHOUSE_SKIP_USER_SETUP=1 \
  -v "<repo>/deploy/docker/clickhouse/init:/docker-entrypoint-initdb.d" \
  clickhouse/clickhouse-server:25.3

BLOGX_CH_ADDR=127.0.0.1:19000 go test -tags integration \
  ./internal/platform/log/persist/ ./service/log_service/
```

`persist`：断库期间 1000 条全部积压成 WAL 批次、CH 内 0 条；恢复后重放 →
`count(DISTINCT event_id)` = 1000；再重复投递一遍 → `count() FINAL` 仍为 1000。

`log_service`（L.3 验收）：1000 条 `EmitActionAudit` 真的进了 `action_audit_logs`，
且 `method/path/status_code/success/user_id` 五个扩展列都对得上；登录 / CDC / 回放
各 1 条进各自表；runtime 经 logrus 桥进 `runtime_logs`，HTTP 列与 `extra_json` 都对。

**2) 断库不丢（无 Docker 也能跑的单元级验证）**

`TestPipelineFallsBackToWALWhenClickHouseDown` 用指向 `127.0.0.1:1` 的连接模拟断库：
5 条审计日志必须变成 WAL 批次，且每行都是合法 JSON。这条测试在普通 `go test` 里就跑。

**3) 列名契约（补 `JSONEachRow` 的静默缺口）**

实测 CH 25.3 的 `input_format_skip_unknown_fields` 默认为 1：**多出的字段被静默丢弃**，
缺列取默认值，类型不匹配的还会被序列化成字符串。也就是说"列名写错"不会响。
补法有两个：

- 连接层显式声明该设置（`core/init_clickhouse.go`），不依赖服务端默认值；
- `TestIntegrationEventStructsMatchTableColumns`：拿 5 个事件结构体的 JSON 键与
  `DESCRIBE TABLE` 的列做对照，要求 **JSON 键 ⊆ 表列**。

这条契约测试当场抓到一个真实漂移：`cdc_event_logs` / `replay_event_logs` 缺
`user_id`、`ip` 两列，而 `baseEvent` 会带出来。已把这两列补进建表脚本——
现在五张表都覆盖 `baseEvent` 的全部列。

#### L.5 为什么可以直接删掉采集器

原来的链路是「应用写文件 → fluent-bit tail 文件 → HTTP 灌进 ClickHouse」。这条路有两个
结构性问题，都来自 tail 的语义：

- `Skip_Long_Lines On`：超过 `Buffer_Max_Size` 的行被**直接丢掉**，而审计日志的
  `request_body_raw` / `response_header_raw` 恰恰是最长的行；
- `Read_from_Head On`：容器重建、state 文件丢失时会**从头重读**，把历史日志重复灌一遍。

换行尾的 `\` 继续换行还可能把一条完整 JSON 拆成两条。这些都不是配置能调好的问题，
是「先落盘、再猜增量」这个模型本身的限制。

L.3 之后应用已经直接写 ClickHouse（`WALSink(ClickHouseSink)`），采集器成了纯中间层，
所以整块删掉：`deploy/docker/fluent-bit/` 两个配置文件 + compose 里的 `fluent-bit` 服务。

副作用要说清：**ClickHouse 开启时不再产生日志文件**。若把 `clickhouse.enabled` 关掉，
应用退回写每日文件（L.3 的文件出口），此时没有采集器把文件送进 CH——这是刻意的：
关掉 CH 就表示"这台实例不查 CH 日志"，而不是"用采集器补上"。

#### L.10 审计字段策略：默认不记全量 raw（选项 1 · 纯热库）

落地口径见 [07 §0](./07-日志模块治理.md)。要点：

- **删除** `request_body_raw` / `response_body_raw` / `request_header_raw` / `response_header_raw`
  四个字段与 ClickHouse 列——它们既是采集器会丢的"最长行"，又是 `MarshalAuditValue → SanitizeValue`
  反射整棵树的爆炸源，还带 PII 合规风险。
- `request_body` / `response_body` 保留，但**只在失败请求时写**，列加 `CODEC(ZSTD(9))`。
- **header 不再采集**：4 个列删掉后没有落点，原始头一律不入库，trace 由 `trace_id/request_id/span_id` 承载。
- `ActionAuditInput` 去掉 `RequestBody any` / `ResponseBody any`，改受控的 `[]byte`（已脱敏 + ≤16KB）；
  `GinAuditInput` 不再接收 body/header，body 统一由 `CaptureLog` 在 HTTP 边界采集。
- 请求体用 `io.LimitReader` **限量 1MB**，再脱敏 + 截断到 16KB——先限量读，绝不整个读进内存。
- archcheck 新增 **R9（slug `log-no-any`）**：审计输入禁 `any`、禁结构体，防止老毛病复发。
- **不做冷存储分层**（选项 1）。`payload_ref` 列预留但默认不写，将来要"原始报文冷存储"再启用。

迁移：`ReplacingMergeTree` 支持 `ALTER TABLE ... DROP COLUMN`，旧数据那几列变默认空；
CH 的 `input_format_skip_unknown_fields=1` 会让旧 WAL 里多出的 `*_raw` 键被静默忽略，重放不受影响。

#### L.4 三个组件各自负责什么

| 组件 | 职责 | 关键设计 |
| --- | --- | --- |
| `ClickHouseSink` | 把一批 JSON 行灌进 CH | 走 `INSERT ... FORMAT JSONEachRow`，不逐列映射（JSON 字段名与列名本就直接对齐）；表名走白名单正则——CH 不支持表名占位符，只能拼进 SQL |
| `WALSink` | 主出口失败时落盘兜底 | write-behind：正常路径不碰磁盘，失败才写。落盘用 `.tmp → fsync → rename`，保证磁盘上出现的 `*.jsonl` 一定是完整批次，不会重放到半截 JSON |
| `ReplayWAL` | 恢复后重放 | 按文件名（20 位 unixnano 前缀）时间序重放；单批失败即停并保留剩余文件——宁可少放，不可跳过，跳过会在日志链路留永久空洞 |

表引擎从 `MergeTree` 改为 `ReplacingMergeTree`：日志是「至少一次」语义，
重放可能重复投递，靠 ORDER BY 里的 `event_id` 去重。**前提是重放投递同一份字节**
（`ts` 等列完全相同）；若日志被重新生成，会被当成新事件。

运维上的一个已知特征：断库期间每次刷盘失败都会落一个 WAL 文件
（默认刷盘间隔 1s，五类日志各一条管道）。断库 1 小时约 1.8 万个文件，
恢复后按序重放并逐个删除。要压这个数量就调大 `FlushInterval`，
代价是日志可见延迟变大。

#### 两个必须说清的结论

**1. S2.5 的 `<2000 行` 目标不成立，已改为架构约束（2026-10-24 拍板）**

08 文档原写"3,231 → ~1,800"。但实测两套 river 的 `river.go` 差异达 **694 行**（两文件共 766 行），
业务逻辑本质不同——一套把文章行事件翻译成 ES bulk 请求，另一套追踪图片引用关系。
**它们不是同一份代码的两份拷贝。** 真正逐字重复的只有 `logger.go`（两份 216 行完全相同）
与 canal 初始化骨架。抽完这两块，总行数只从 3,244 降到 3,237——**不是没合并，是代码本来就这么多**。

要压到 2000 以下只能删除实际功能，属于行为变更，与"行为等价"直接冲突。
所以验收已改为**架构约束**（04 §S2 验收第 2 条 / 08 §4.1 S2.5）：

> 引擎唯一——业务子包内不得出现 `canal.NewCanal` 与各自独立的批量/重试实现；
> 单文件 ≤800；CDC 功能等价。

**已达成**：`cdc/canalbase` 是唯一持有 `canal.NewCanal` 的地方（连接参数 / 日志适配 /
表过滤正则 / dump 范围），`river/` 与 `imageref/` 只剩各自的"行事件 → 文档"映射，
连两份重复的日志适配器也合并成 `canalbase.NewLogAdapter` 一份。
行数不再当门禁，降级为"只降不升"的观察项。

**2. S2.8 / S2.9：契约从代码长出来，前端类型从契约长出来**

swag 链路已落地：`make openapi`（= `swag init --parseDependency --parseInternal`）
从代码注解生成 `backend/docs/swagger.json`。

- 覆盖 **117 个业务 operation**（article 30 / user 20 / log 11 / 其余 56）。
  `/health` 是 infra 探针、不在 `/api` 下且返回空 body，刻意不纳入业务契约。
- 响应统一写成内联 `object{code=int,data=<Type>,msg=string}`。
  **踩过的坑**：`@Success 200` 与 `@Failure 200` 用同一状态码时，后者会覆盖前者——
  本项目失败也返回 HTTP 200 + 业务 code，所以约定**只写 `@Success 200`**。
- 前端类型链路：`pnpm openapi:types` = `swagger2openapi`（2.0→3.0）+ `openapi-typescript`
  → `app/types/openapi.d.ts`。openapi-typescript 7 只吃 OpenAPI 3.x，故必须转换这一步。
- `app/types/contract.ts` 提供 `Schema<"...">` / `Operation<"...","get">`；`api.ts` 保留为
  **视图模型/适配层**（组件消费面向 UI 的形状），不再镜像原始 HTTP 契约。

#### 文档口径已统一（2026-10-24 拍板）

原先 08 文档里 L.3–L.5 与 T.4 各在两处出现、归属冲突。按"架构完整性优先、不为压缩里程碑而降标准"
的原则收敛为一处口径：

- **M3 = S2 全部 + L 全部（L.1–L.5）+ T 基建（T.3 / T.4 基建 / T.8）**
  日志是端到端管道：只做 L.1/L.2 会让"平台层已建好、生产路径仍同步落盘"的新旧并存状态
  持续整个阶段，且 L.3 单独做会丢审计日志——必须先有 L.4 的 WAL 兜底。
- **M4 = S3**：article 全链路（全 7 子域）+ article 及各子域的有状态 fake（七项决策见上）
- **M5 = S4**：其余 11 个模块（含 `ai`）+ 平台归位 + 棘轮归零，各自随行交付本模块的有状态 fake。
  **四项决策（2026-10-25 拍板，详见 [04 §4.2](./04-迁移路线图.md#42-模块间协作通则2026-10-25-拍板读同步写事件) / [06 §4.1](./06-质量保障与治理.md#41-测试分层口径2026-10-25-拍板)）**：
  1. **范围=全包**：S4.0 公共前置 + 9 个业务批次 + S4.10 平台收尾。M5 结束时 `archcheck --error ./...` 全绿、
     `models/` 只剩空壳，M6 不再夹带迁移（只剩删目录 / 前端 / 部署这类可整体回滚的事）。
  2. **模块协作=读同步、写事件**：读别的模块数据走窄 port 同步直调；通知/统计/投影等副作用落 `outbox` 事件；
     "写结果本身"（如文章计数 +1）同步同事务。port 接口定义在被调用方的 `app/port.go`。
  3. **切换=直接替换**：一模块一 commit，**不保留** `BLOGX_<MODULE>_NEW_PIPELINE`。M4 证明开关只带来
     "两套路径并存 + 最后再删一次"，出问题整模块回滚一个 commit 更干净。
  4. **测试=分层**：不变量矩阵 + app 用例测**每模块必做**；golden **只给 5 个只读模块**
     （comment / search / site / notification / analytics）；全模块补 openapi 契约测；
     WebSocket / 文件上传 / SSE 只做 e2e 冒烟（分片顺序与时效天然不稳定，逐字节比无意义）。
  **row 归属口径**：row 结构（GORM 标签 / 列映射 / `TableName`）**复制进各模块 `adapter/mysql` 包内部**，
  不进 domain、不跨模块共享；雪花 `BeforeCreate` 钩子上提 `platform/mysql/hooks.go`，避免被复制 9 份。
  M4 的 article adapter 目前仍直接复用 `models.Article` 作 row（8 处），S4.0 一并改。
- **M6 = S5 + L6–L9**：删旧 Hook、去雪花、TTL 与压缩、日志规则（**L.10 审计字段瘦身已提前执行**，不占 M6 工期）

T.4 里 `article`/`comment`/`user` 的 fake 依赖 domain 实体，属依赖顺序而非成本，随各模块交付；
`outbox`/`tx` 的 fake 不依赖 domain，在 M3 交付。
| 前端覆盖率（语句） | 62.47 | 单调递增 |

**规矩**：违规数只准降不准升。一旦某次提交让任何一个数字变差，就是引入了回归。

---

## 8. M5 交付记录（S4.0 起）

### S4.0-A 公共前置：拆工具包 + 平台能力归位

**背景**：S4.0 定的三处结构债里，`service/db_service` 与 `service/read_service` 都是
"别人在用的转发壳 / 工具包"。动手前先做了一次调用者盘点，结果比预期好——M4 删掉
`api/article_api` 之后，有一整批包已经变成孤儿。

**删掉的死代码（约 790 行）**：

| 包 / 符号 | 行数 | 判定依据 |
| --- | --- | --- |
| `service/favorite_service` | 11 | 纯类型别名 + 转发到 `favor_repo`，全仓 **0 个调用者** |
| `service/top_service` | 8 | 同上 |
| `repository/favor_repo` | 401 | 只被上面那个孤儿 service import |
| `repository/top_repo` | 218 | 同上 |
| `read_repo` 的 5 个符号 | ~150 | 删掉上面两个 repo 后调用者归零：`LoadCategoryTitleMap`、`LoadArticleTagTitlesMap`、`ArticleCounterReader`（含 `ArticleBatchCounters` / `NewArticleCounterReader`）、`SyncArticleFavorSnapshots` |

`read_repo` 从 446 行降到 276 行。剩下的三块各有明确归属，随模块走：
`UserDisplay` / `LoadUserDisplayMap` / `SyncUserDisplaySnapshots` → S4.1（user）；
`CommentCounterReader` → S4.2（comment）；`LoadArticleBaseMap` / `NormalizeIDs` → S4.2 顺带
（comment 是它唯一的读者）。

**两个平台能力归位**：

- `service/db_service/snowflake.go` → `internal/platform/idgen/`
  新增并发锁（原实现两个 goroutine 同时 `Init` 有竞态）、错误信息改英文、补 5 个用例
  （非法机器号 / 未初始化不兜底 / 幂等且拒绝换号 / 8×500 并发无重复 / 严格递增）。
  改了 12 个调用点；`service/db_service/` 整个目录删除。
- `service/db_service/restore_create_unique.go` → `internal/platform/mysql/restore.go`
  逻辑照搬，错误改英文常量（`ErrRestoreNilValue` / `ErrRestoreEmptyMatch`）。
  测试从 `test/testutil` 改为自建 sqlite（平台包不该反向依赖测试工具包）；
  顺手补了一条原测试没有的断言：**活记录已存在时不能改写它的字段**。

**新增两条架构规则**：

| 规则 | 内容 | 当前命中 |
| --- | --- | --- |
| **R10** `mod-no-models` | 业务模块禁止 import `myblogx/models`（放行 `models/ctype`——它是值类型，不是 row） | 17 |
| **R11** `mod-no-cross-adapter` | 禁止跨模块 import 别人的 `adapter`（跨模块只能走 port 或 outbox） | 0 |

两条都配了人造违规用例，`TestAllRulesHaveSelfCheck` 同步更新。R11 现在 0 命中属于
"暂未违规"而非"规则无效"——它的用例证明规则会红。

**棘轮为什么从 236 变成 250**：

250 = 233（删死代码让 R8 从 111 → 108）+ 17（R10 首次统计出的 article 存量）。

那 17 处一直是真违规，只是此前没有任何规则统计它。**重标不是放宽，是把隐藏债务显性化**。

**进一步查了这 17 处的构成，结论比"row 放哪"更严重**：

| 位置 | 处数 | 实质 |
| --- | --- | --- |
| `adapter/mysql/ports.go` | 2 | **article 直接读写 `CommentModel`（级联软删评论）和 `UserModel`（查作者信息）**——跨模块表穿透，不是 row 归属问题 |
| `adapter/mysql/*_test.go` | 4 | 集成测试里 `AutoMigrate` 建别的模块的表，属测试脚手架 |
| `adapter/mysql/*.go`（article 自有表） | 4 | 真正的 row 归属问题 |
| `adapter/http/*.go` | 7 | `models.IDRequest`/`IDListRequest`（通用请求）+ 审核任务枚举 |

也就是说：**R10 抓到的核心问题是模块边界穿透，而不是"row 该放哪个包"**。
单纯把 572 行 model 复制进 adapter 只是换皮，边界照样穿透。

**正确的收口顺序**（不是一次复制能解决的）：

1. S4.1 建 `user` 模块时，同时立 `UserPort`（读用户展示信息 / 同步快照），article 的 `ports.go` 改调 port；
2. S4.2 建 `comment` 模块时，同时立 `CommentPort`（级联删除），article 不再直写评论表；
3. `adapter/http` 的 7 处独立于上面两步：`IDRequest`/`IDListRequest` 迁 `internal/kernel`，
   审核任务枚举复用 article domain 已有的类型；
4. 三步做完 R10 归零，棘轮 250 → 233。

**先记下不做的理由**：在 comment/user 模块还不存在时把 row 复制进 article adapter，
会让"article 能直接写评论表"这件事变得**更难被发现**——因为它不再叫 `models` 了。
规则的价值在于暴露问题，不是让人绕过它。

**一个刻意没做的事**：文档原本写"R7 转 error"，读代码后放弃——
`archcheck` 当前只按 `--mode` + `--max-violations` 判定，`Rule.Level` 只用于
`-list-rules` 显示，**不参与失败判定**。现在改 R7 的 Level 不会有任何效果，属于"看起来做了"。
正确做法是等棘轮清零时（M6）让 `Level` 真正参与判定，届时一并处理。

## 9. M5 交付记录 · S4.1d（认证链路）

**2026-10-25**：`api/user_api/auth_api`（13 文件）删除，认证 13 条路由改由 `internal/auth` 提供。

- 适配器：`adapter/mysql`（会话表）、`adapter/redis`（黑名单 + 登录限流）、
  `adapter/security`（刷新令牌随机串 + SHA256）、`adapter/http`（13 条接口）。
- 用例：在核心 Login / Refresh / Logout / Authenticate 之外，补邮箱验证码登录与注册、
  QQ 登录、改密 / 重置 / 绑定邮箱；依赖一律走窄端口（Directory / EmailCodes / OAuth）。
- user 侧补账号生命周期：注册建档、第三方建档、改密、绑定邮箱；
  仓储新增 `UpdatePassword`（令牌版本 +1）与 `UpdateEmail`。
- **JWT 签发器放在装配层**：载荷映射需要 `models/enum`，业务模块禁止 import models（R10）。
- 邮箱验证码沿用旧实现的**反枚举**语义：注册时邮箱已存在也返回成功和 id，但不发信、不存码。
- 棘轮 **250 → 240**（删掉 auth_api 的中文 error 与跨层引用）。
- 未做：`profile_api` / `user_man_api` / `log_api` 仍在旧 `api/user_api` 下，
  下一刀做 user 路由等价替换后整包删除。

## 10. M5 交付记录 · S4.1e（用户资料与管理端）

**2026-10-25**：`api/user_api/profile_api` 与 `user_man_api` 已删，路由路径与响应字段保持不变。

- user app 新增：`BaseInfo`（关注关系 / 主页浏览去重 / 头像图片 ID）、
  `UpdateInfo`（资料 + 偏好合并）、`AdminUpdate`（角色或状态变化 → 令牌版本 +1 + 吊销全部会话）、
  `AdminList`（附最后登录信息）。
- 可选外部能力收进 `app.Integrations`（Relation / HomeView / Sessions / LastLogin / Tags），
  装配层用旧 service 适配，避免用户模块直接 import 对方的表。
- 装配期循环依赖用 `SessionRevokerFunc` 闭包延迟绑定（user 服务先建、auth 服务后建）。
- `user_api` 只剩 `log_api`（登录日志，归 S4.9）；auth / profile / user_man 三个子包已迁。
- 棘轮 **240 → 224**。

## 11. M5 交付记录 · S4.2（comment）

**2026-10-25**：`api/comment_api` + `repository/comment_repo` + `service/comment_service` 已删，
`/comments` 6 条路由改由 `internal/comment` 提供。

- `internal/comment/{domain,app,adapter/{mysql,http}}`：实体 + 状态 + 删除权限规则；
  用例 Create / ToggleDigg / Remove / ListRoot / ListReplies / ListManaged。
- **两级评论**：回复二级评论时 RootID 仍指向同一个一级评论，不再加深层级。
- 计数 = 表内基数 + Redis 增量（`read_repo.CommentCounterReader`），
  发布/删除时回写 `redis_article.SetCacheComment` 与 `redis_comment.*`。
- 通知从裸 goroutine 改为同步 `Notifier` 端口（失败只记日志），S4.6 再换 outbox。
- 点赞仓储拆成 `DiggRepo` 独立类型：两个端口的 `Create` 签名不同，同一类型不能同名方法。
- 棘轮 **224 → 214**；`internal/comment/...` 零违规。

## 12. M5 交付记录 · S4.3（media）

**2026-10-25**：`api/image_api` + `service/image_service` 已删，`/images` 7 条路由改由 `internal/media` 提供；
七牛 SDK 调用迁入 `platform/storage/qiniu`。

- `internal/media/{domain,app,adapter/{mysql,redis,http}}` + `apptest` 内存替身。
- 上传任务完整保留：创建（秒传判断 + 凭证 + 对象 key）、手动确认、七牛回调确认、
  状态轮询、内容审核回调（含"回调先到、图片后入库"的 Redis 暂存）。
- 校验链未简化：哈希、大小、bucket、图片格式白名单、真实 imageInfo 尺寸。
- `article` / `user` / `banner` 的图片反查改为 `internal/media/adapter/mysql` 的
  `FindIDByURL` / `ResolveURLByID`，不再依赖旧 `image_service`。
- 平台包不依赖业务 domain：`qiniu.Storage` 由装配层的 `qiniuStorage` 适配成 media 端口。
- 棘轮 **214 → 186**；`internal/media/...` 零违规。

## 13. M5 交付记录 · S4.4（search）

**2026-10-25**：`api/search_api` 与搜索实现文件已删，`GET /search/articles` 改由 `internal/search` 提供。

- `internal/search/{domain,app,adapter/{redis,mysql,http}}`：请求归一化、ES DSL 构造、
  结果解析、计数叠加、偏好标签加权。
- DSL 语义原样保留：function_score（时间衰减 + 点赞/评论/收藏/浏览 log1p 加权）、
  published/visible 过滤、作者/分类/标签/关键词条件、两种分页模式（has_more/count）。
- 计数仍是「ES 基数 + Redis 增量」。
- **AI 模块仍复用搜索响应类型与 SearchArticles**（S4.8 才迁）：保留一个极薄的
  `service/search_service` 兼容层（类型别名 + 包装），AI 只改了两处 `PageInfo` 类型。
- 删 `repository/user_repo`（最后一个引用随偏好标签读取迁入 `internal/search/adapter/mysql` 消失）。
- 棘轮 **186 → 171**；`internal/search/...` 零违规。

## 14. M5 交付记录 · S4.5a（social / 关注关系）

**2026-10-25**：`api/follow_api` 已删，`/follow` 与 `/fans` 4 条路由改由 `internal/social` 提供。

- `internal/social/{domain,app,adapter/{mysql,http}}`：关注实体、关系枚举（1..4 与旧契约一致）、
  列表分页、隐私可见性校验、快照补齐。
- `Relation` / `RelationBatch` 作为 **跨模块读端口**：user 的主页关系、comment 的列表关系
  都改成调 social 服务，不再直接 import `follow_repo` / `follow_service`。
- 关注/取关保留「软删恢复」「重复关注提示」「取关未关注提示」语义。
- **已知债**：关注数/粉丝数统计从"同事务"退化为"关注成功后再更新"（失败只记日志）。
  S4.6 会把"更新统计"改成 outbox 事件恢复最终一致。
- `repository/follow_repo` 与 `service/follow_service` **暂留**：chat 模块仍在用，
  随 chat 迁移一并删除。
- 棘轮 **171 → 165**；`internal/social/...` 零违规。

## 15. M5 交付记录 · S4.5b（chat）

**2026-10-25**：`api/chat_api`、`service/chat_service`、`repository/chat_repo`、
`repository/follow_repo`、`service/follow_service` 全部删除；`/chat` 7 条路由（含 WS）改由 `internal/chat` 提供。

- 新增 `internal/platform/http/ws`：升级（Origin 策略注入）、Conn（写锁+超时+心跳）、
  Hub（多连接在线池，推送前复制快照，不持锁写 socket）。
- 新增 `internal/chat/{domain,app,adapter/{mysql,redis,http}}`：
  - 发消息一个事务：建/恢复双方会话 → 插消息 → 更新双方 last_msg / 接收方未读；
    `RowsAffected` 必须匹配（自聊 1、双人 2），否则回滚。
  - 已读：只命中"我作为接收方且未读"的消息，事务内同时递减会话未读（下限 0），
    再按 (发送者, 会话) 聚合推 WS 已读回执。
  - 单条删除写 `chat_msg_user_states`（不动原消息）；会话删除推进 `clear_before_msg_id` 水位。
  - 限流：分钟级 30/60 + 自然周（陌生人 1、单向 3、好友无限）；落库成功才提交预占，失败回滚。
  - WS：ticket 存 Redis 1 分钟，先试 Authorization 再试 ticket；读限制 4K、pong 60s、ping 54s。
  - 后台心跳用 `safego.Go`，不再是裸 goroutine。
- **技术债**：旧 chat/follow 测试（约 1770 行）未迁移，目前只有 chat domain 的
  `BuildSessionID` / `Summary` 与 social 枚举测试；消息事务、未读递减、水位删除缺少回归保护。
- 棘轮 **165 → 135**；`internal/chat/...` 零违规。

## 16. M5 交付记录 · S4.6（notification）

**2026-10-25**：`api/global_notif_api`、`api/sitemsg_api`、`service/message_service` 全部删除；
`/global_notif` 5 条与 `/sitemsg` 6 条路由改由 `internal/notification` 提供。

- `internal/notification/{domain,app,adapter/{mysql,http}}` + `apptest` 内存替身。
- 读侧：站内信分组列表/单条已读/分组已读/单条删除/分组删除/未读汇总；
  公告列表（用户视角 vs 管理员视角）、批量已读、用户删除、管理员发布/删除。
- 写侧：`NotifyComment` / `NotifyReply` / `NotifyCommentDigg` / `NotifyArticleDigg` /
  `NotifyArticleFavor` / `NotifySystem`。**错误不再吞掉**，交给 outbox 中继重试；
  去重（同一人对同一对象只通知一次）原样保留，所以重试幂等。
- 公告可见性三段规则原样保留（所有用户 / 发布前已注册 / 发布后注册），
  已删除公告改用 `NOT IN` 子查询排除，不再把用户全部状态读进内存。
- 评论与 outbox 中继的生产路径改为调用 `notificationService`，不再直连消息表。
- **一处有意修正**：站内信列表旧实现没有 `ORDER BY`，翻页会出现重复/漏行，
  现在固定 `created_at DESC, id DESC`。
- 保留并改造了 `router/global_notif_router_test.go` 与 `sitemsg_router_test.go`（9 个用例，
  真实 sqlite），它们直接覆盖新适配器的 SQL。**测试还抓到一个真 bug**：
  `user_conf_models` 没有 `updated_at` 列，写入它会报错。
- 棘轮 **135 → 116**；`internal/notification/...` 零违规。

## 17. M5 交付记录 · S4.7（site）

**2026-10-25**：`api/site_api` + `api/banner_api` 已删，`site_service` 迁为 `internal/platform/config`；
`/site` 6 条与 `/banners` 4 条路由改由 `internal/site` 提供。

- `internal/platform/config`：运行时配置服务（原 `service/site_service`）整体搬迁，
  **这是 middleware 与多个模块装配都依赖的设施**，所以必须放在 platform 而不是业务模块。
  4 处中文 error 同时改成错误码，模块内 R8 归零。
- `internal/site/{domain,app,adapter/{mysql,http}}` + `apptest` 内存替身。
- 公开读：QQ 登录地址、SEO、AI 信息、`/site/site`（含构建版本号）；
  管理端读：站点配置、AI 配置（密钥用 `******` 遮蔽）；写：按 name 分发到 site / ai。
- **AI 密钥保留语义**：请求体里密钥等于占位符表示"没改"，沿用当前值。
- 轮播图：列表（按 show 精确过滤 + 回填封面图片 ID）、创建、更新、批量删除；
  封面地址由图片 ID 解析，图片不可用时拒绝写入。
- 审计事件（banner_create / banner_update / banner_remove / site_runtime_update）逐条保留。
- 棘轮 **116 → 105**；`internal/site/...` 与 `internal/platform/config/...` 零违规。

## 18. M5 交付记录 · S4.8（ai）

**2026-10-25**：`api/ai_api` + `service/ai_service`（27 文件）已删，`service/search_service`
兼容层也随之删除；`/ai` 6 条路由改由 `internal/ai` 提供。

- 目录：`internal/ai`（LLM 客户端与 JSON 工具）+ `scoring` / `metainfo` / `overwrite` /
  `diagnose` / `search` 五个子包（提示词与结果校验），加 `domain` / `app` / `adapter/{mysql,http}` / `apptest`。
- **业务子包不再碰数据库**：元信息候选、标签标题、文章快照、评分记录全部改成端口注入，
  SQL 只出现在 `internal/ai/adapter/mysql`。
- 评分语义逐条保留：公开摘要隐去总分/文章类型/维度原因/评语，作者视图全量返回；
  重新评分校验作者身份；`CreateIfAbsent` 把"查重 + 写入"收进同一个事务。
- 智能搜索：先带标签搜一次、再不带标签搜一次、按 ID 去重——顺序与原实现一致；
  直接复用 `internal/search/domain` 的请求/响应类型，因此 S4.4 留下的
  `service/search_service` 兼容层被删除。
- outbox 的"补评分"订阅改为调用 `aiService.EnsureScoreIfMissing`，AI 未启用时静默跳过。
- 9 处中文 error 改为错误码，`internal/ai/...` 零违规。
- **技术债**：`api/ai_api` 的 1181 行测试未迁移；新增的 app 层测试覆盖评分可见性、
  作者校验、搜索去重与"AI 未启用不评分"，但 SSE 流式与真实 LLM 调用仍无回归保护。
- 棘轮 **105 → 83**。

## 19. M5 交付记录 · S4.9（analytics）

**2026-10-25**：`api/data_api`、`api/log_api`、`api/user_api/log_api` 全部删除（`api/` 只剩 `captcha_api`）；
`/data` 3 条、`/logs` 11 条、`/users/login/log` 改由 `internal/analytics` 提供。

- `internal/analytics/adapter/clickhouse`：原 `log_service` 的三份查询代码
  （runtime/login/action 三类日志 + CDC/replay + 链路串联）整体搬入，`Deps` 改为 `Options`。
- `internal/analytics/adapter/mysql`：站点汇总、按区间统计已发布文章、按天统计注册数。
- `internal/analytics/domain`：`StatsRepository` / `FlowCounter` / `DeadLetterReader` 三个端口。
- `internal/analytics/adapter/http`：17 个 handler，**请求/响应结构、文案、路由逐字保留**。
- 访问量仍来自 Redis（`redis_site.GetFlow/GetRecentFlow`），登录数仍来自 ClickHouse；
  ClickHouse 未启用时登录数按 0 兜底，不影响其它统计。
- **只切了查询侧**：`service/log_service` 的写入管道（pipeline / write_*）一行未动，
  它是 M3 建好的端到端日志链，留给 S4.10 归 `platform/log`。
- 顺手修掉一个越界隐患：访问量缓存为空时，`/data/growth` 取"今天/昨天"会数组越界，
  现在补 7 天零值骨架。
- 棘轮 **83 → 69**；`internal/analytics/...` 零违规。

## 20. M5 交付记录 · S4.10a（R7 + R8 清零）

**2026-10-25**：棘轮 **69 → 27**，R7（裸 goroutine）与 R8（中文 errors.New）双双归零。

- **R7 6 处 → 0**：`app/wiring_worker` 两处、`core/init_image_ref_river`、`core/init_mysql_es`、
  `cdc/river` 的同步循环、`router/enter` 的 HTTP 服务协程，全部改走 `safego.Go`，
  并在 panic 处理器里记结构化错误日志。后台协程 panic 不再拖垮进程。
- **R8 36 处 → 0**：`common/list_query`、`cdc/river`、`platform/llm`（7）、
  `platform/storage/qiniu`（5）、`service/user_service/auth_errors`（6）、
  `utils/{info_check,jwts,maps}`（14）、`models/user_model`（1），
  统一换成 `apperr.New/Wrap`；`%w` 包装改成 `apperr.Wrap(code, cause)`，错误链仍可用 `errors.Is` 穿透。
- 一处文案微调：`user_model` 的"用户状态异常"并入既有 `user.status_invalid`，文案变为"账号状态不合法"，
  语义未变。

**剩余 27 处，均需真实重构，不是替换能解决的**：
- **R2 10 处**（`type XxxDeps struct`）：`api/captcha_api`、`core`×2、`flags`、`middleware/deps.go`、
  `platform/cachex`、`internal/platform/redis`、`internal/platform/es/articlesync`、
  `service/log_service`、`service/user_service`。这些是**装配根的依赖容器**，
  改窄接口会连带改所有 Wire 调用点，属于独立一刀。
- **R10 17 处**（article 模块 import models）：M4 时期的遗留，需要把 `models.ArticleModel`
  等 row 结构与 `models/enum` 常量复制进 `internal/article/adapter/{mysql,http}`，
  是纯体力但要逐字段核对，风险在"漏一个列名就静默写错"。

## 21. M5 交付记录 · S4.10b（R10 清零，棘轮 27 → 10）

**2026-10-25**：`internal/article/adapter/{http,mysql}` 不再 import `myblogx/models`。

- `adapter/http`：新增 `enums.go`，复制 `ArticleStatus` / `ArticleVisibility` /
  `ReviewTask{Status,Stage,Source}` 五个枚举（取值与库、与 domain 三者一致）；
  `models.IDRequest`/`IDListRequest` 换成 `kernel.IDRequest`/`IDListRequest`。
- `adapter/mysql`：新增 `rows.go`，复制 12 个行结构（article / article_tag / article_digg /
  comment / user / favorite / user_article_favor / user_top_article /
  user_article_view_history / review_task / category / tag / outbox），
  含 `TableName()`、`BaseRow.BeforeCreate`（雪花主键）与状态常量。

**集成测试暴露的两个真问题（都已修）**：
1. **内嵌行结构必须导出**。`baseRow` 用小写时 GORM 反射写不进去，主键与时间列**静默变成零值**
   ——编译通过、接口 200、数据全空。改名 `BaseRow` 后恢复。
2. **匿名结构体不能内嵌带 `TableName()` 的行结构**。内嵌会把 `TableName()` 一起提升，
   GORM 按"表模型"解析 schema，导致 `SELECT f.*` 扫不进字段。
   收藏夹列表、分类列表两处改成显式列字段。

**行为未变的两处**：`FavoriteModel.BeforeDelete` / `TagModel.BeforeDelete` 的级联清理由
`Delete(&Model{})` 触发，钩子收到的模型 ID 为 0，**原本就是空操作**；
删除后行为与原实现完全一致（集成测试覆盖）。这是一处既有隐患，未在本刀顺手改语义。

验证：`go test -tags integration ./internal/article/...` 全绿，
含 20 篇文章 × 12 接口的 golden 逐字节比对。

## 22. M5 交付记录 · S4.10c（R2 清零，棘轮 10 → 0）

**2026-10-25**：`archcheck --mode=error ./...` 全绿，**M5 收口**。

- **真正的依赖袋**只有两个，如实收窄：
  - `middleware.Deps`（21 字段）→ `middleware.RuntimeOptions`（10 字段）。
    `NewRuntime` 实际只读 10 个，其余 11 个是"顺手塞进来的"。
  - `api.New(deps middleware.Deps)` → `api.New(runtimeSite, captchaStore)`；
    `captcha_api.Deps` 改成包内私有 `options`。`api` 包从此不依赖 middleware。
- **其余 8 处是构造参数，不是依赖袋**，统一改名 `Options` 并补注释说明区别：
  `internal/platform/redis`、`platform/cachex`、`service/log_service`、
  `service/user_service`、`flags`、`core/init_{image_ref_river,mysql_es}`、
  `internal/platform/es/articlesync`。
- **踩坑记录**：批量把 `Deps` 重命名为 `Options` 时，把 `tools/archcheck/rules.go`
  里的**规则实现本身**也改了（`name != "Deps"` → `name != "Options"`），
  守卫因此变成"拦截 Options 结构体"，违规数反而从 10 涨到 17。
  已 `git checkout` 还原。教训：**改代码的脚本必须排除守卫自己的源码**。

## M5 完成状态

- 九个业务批次 + 平台收尾全部完成，13 个绿色提交。
- 架构守卫棘轮 **324 → 0**，`--mode=error` 无违规。
- `backend/api/` 只剩 `captcha_api`（一个验证码接口，无业务状态）。
- `bash scripts/check.sh backend` 全绿；`-tags integration` 的 article golden（20 篇 × 12 接口）
  与 analytics 路由测试全绿。

## 23. M5 收口补漏 · 阶段一：集成测试进流水线 + 棘轮单一来源

**2026-10-25**：M5 名义收口后复查发现流水线本身在撒谎，本阶段修掉。

- **2844 行集成测试从未在 CI 跑过**。`ci.yml` 注释写着"集成测试由 nightly 档跑"，
  但仓库里只有 `ci.yml` 一个 workflow，**根本没有 nightly**。12 个
  `//go:build integration` 文件（golden 421 / article_repo 411 / relay 396 /
  pipeline 379 / user_repo 346 / e2e 222+265 …）只在开发机手动跑过。
  现已新增 `backend-integration` job：断言 `docker info` 可用，再跑
  `go test -tags integration -count=1 -timeout 20m ./...`。
- **棘轮在三处各写一遍，已经漂移**：`scripts/check.sh` 是 0、`ci.yml` 停在 250、
  `verify-gates.sh` 也停在 250。后果有两个：
  1. CI 允许新增 250 处违规，棘轮形同虚设；
  2. **门禁自检自己坏了** —— 它注入 1 处违规后按 250 判断，1 ≤ 250 不会红，
     `expect_fail` 于是报"门禁失效"。`check.sh gates` 实际是 3/4 通过、
     整体失败，而 M5 收口时只跑了 `backend`，没跑 `gates`，所以没发现。
  修法：基线抽到 `scripts/arch-baseline.env` 单一来源，check.sh / verify-gates.sh /
  ci.yml 三处都读它。自检里改用 `--max-violations=$ARCH_MAX_VIOLATIONS`
  （注入 1 处后总数 = 基线 + 1，必然超限）。
- `check.sh` 新增 `run_integration()`：Docker 不可用时**显式警告跳过**（不静默），
  设 `BLOGX_REQUIRE_INTEGRATION=1` 则直接失败；`BLOGX_SKIP_INTEGRATION=1` 可显式跳过。
- 验证：`check.sh backend` 全绿（含集成，约 2.5 分钟）；
  `check.sh gates` **4/4 通过**（修复前是 3/4）。

## 24. M5 收口补漏 · 阶段二：captcha 迁入 platform，删除 backend/api

**2026-10-25**：`backend/api/`（最后一个 api 包）整目录删除，`router.Run` 不再接收容器参数。

- `api/captcha_api`（170 行）→ **`internal/platform/captcha`**。没有建业务模块：
  它无领域状态、无业务规则，只是把 base64Captcha 包一层 + 读一个开关。
  为 170 行造 domain/app/adapter 三层属于为结构而结构。
- `captcha.Settings` 窄接口（`CaptchaEnabled() bool`）代替直接依赖
  `RuntimeConfigService`；装配层 `captchaSettings` 适配。
- **修掉一处靠巧合成立的耦合**：生成在 `platform/captcha`，校验在
  `middleware.CaptchaMiddleware`（登录、发邮箱验证码两条路由挂了它）。
  两边原先都各自取 `base64Captcha.DefaultMemStore` 包级单例 —— 恰好是同一个对象，
  所以能跑通；但任何一方改成 `NewMemoryStore()` 就会静默断掉（接口都返回 200，
  只是登录永远报"验证码错误"）。现在 `WireHTTP` 只创建一个 store 实例，
  同时传给 `RuntimeOptions.ImageCaptchaStore` 与 `WireCaptcha`。
- 测试从旧 `image_captcha_test.go` 迁移为 `platform/captcha/handler_test.go`，
  3 个用例：开关关闭不写 store、正常生成返回 id+base64、**生成的数据能从外部
  store 查到**（守住上面那条耦合，防止 Handler 内部偷偷 new 一个 store）。
- 路由：`CaptchaRouter(nr)` / `UserRouter(nr, mwRuntime)` / `router.Run(opts)` —— 
  `api.Api` 参数全部消失。
- 验证：`check.sh backend` 全绿（含集成），棘轮仍为 0。

## 25. M5 收口补漏 · 阶段三：read_repo 迁 platform/readmodel，删 read_service 与 repository/

**2026-10-25**：`backend/repository/` 整目录删除，`backend/service/read_service`（19 行）删除。

- `repository/read_repo` → **`internal/platform/readmodel`**。它是「跨模块共享的只读投影」
  （chat / comment / social 的装配层都要读用户展示信息、评论计数），
  放 platform 是诚实的，而 `repository/` 这个目录在 M5 之后不该再存在。
- `service/read_service`（19 行纯转发）**零外部引用**，是彻底的死代码，直接删。
- 剩余 `service/`：`cron_service`（1107 行）、`log_service`（写入管道）、`user_service`（878 行）。

## 26. M5 收口补漏 · 阶段四：认证单一化（middleware 摘掉旧 Authenticator）

**2026-10-25**：全站认证只剩一份实现（`internal/auth`），`user_service` 的认证代码删除。

### 发现的问题

S4.1d 把认证迁进 `internal/auth` 时，只换了**路由层**的 handler。
`middleware.AuthMiddleware` / `OptionalAuthMiddleware` 仍然持有
`*user_service.Authenticator` —— 也就是说：

- 走 `authHandler` 的接口用新逻辑（查黑名单、令牌版本、会话有效性）
- 走 `AuthMiddleware` 的接口用旧逻辑

两套并存意味着"登出后 token 还能用"这类问题只会在部分路由上出现，
且很难复现（取决于该路由挂的是哪个认证）。

### 改法

- 新增 `middleware.TokenVerifier` 窄接口（`VerifyAccessToken(ctx, raw) (*jwts.MyClaims, error)`），
  middleware 不再依赖任何具体认证实现。
- `RuntimeOptions.Authenticator` 换成 `Verifier TokenVerifier`；
  实现由装配层 `app/auth_verifier.go` 提供，内部调 `authService.Authenticate`。
  用延迟绑定（`authService` 全局变量在 `WireUser` 赋值），因为 opts 在它之前构造。
- 去掉两处**只写不读**的上下文数据：`auth_user`（整行 UserModel）、
  `auth_session`（整行 SessionModel）、`_authenticator`。全仓无任何读取方，
  属于把整行数据塞进请求上下文却没人用的死代码。
- 删除 `user_service` 的 `auth_session*.go`（3 文件）与 `auth_errors.go`；
  `stat_view.go` 里"DB 未初始化"误用 `ErrAuthInvalid` 的说法改成 `common.db_required`。

### 测试上的两个坑（都踩了）

1. **测试 verifier 不能只验签名**。`TestAuthAndAdminMiddleware` 明确断言
   "被拉黑的 token 必须拦截"，简化 verifier 一上就红。所以
   `testutil.AuthVerifier` 保留了黑名单检查（`redis_jwt.HasTokenBlack`）。
   这不是测试挑剔，是它守住了真实行为。
2. **`HasTokenBlack` 的返回语义反直觉**：第二个返回值 `ok=true` 表示
   **"不在黑名单、可以放行"**，命中黑名单返回 `false`，Redis 不可用/读取出错
   也返回 `false`（fail-closed）。按名字写成 `if black { 拒绝 }` 会把
   所有正常 token 拒掉。已在代码注释里写明。

## 27. M5 收口补漏 · 阶段五：user_service 删除（跨模块写改函数注入）

**2026-10-25**：`backend/service/user_service`（878 行）整体删除，`service/` 只剩 `cron_service` 与 `log_service`。

- 统计写入迁进 **`internal/user/adapter/mysql/stat.go`**：
  `ApplyArticleDelta` / `ApplyFollowDelta` / `EnsureStatRows` / `RecordViewDaily` / `InitDefaults`。
- **article 的跨模块写改成函数注入**：`NewTxManager(db, counterOptions, AuthorStatsApplier)`。
  article 侧出**当前事务句柄**，user 侧出**写入逻辑**，由装配层把两者接起来。
  这样既保住"删文章与作者文章数 -1 同事务"（改成异步会让用户看到旧数字），
  又让 article 不 import 任何 user 模块的实现包。
- `stat_view.go` 的"主页浏览按天去重"拆成两段：Redis 挡板 + 数据库唯一索引兜底，
  装配在 `app/user_wiring.go` 的 `homeViewBridge`；事务失败会回滚 Redis 标记，
  否则用户当天第二次真正访问会被误判成"已看过"。
- integration 测试传**真实**统计实现而不是 nil：传 nil 能让门禁变绿，
  但那是把"删文章后作者文章数 -1"的覆盖悄悄丢掉。

## 28. 门禁提速：去掉重复测试轮次（5分20秒 → 3分49秒 / 快速模式 2分04秒）

**2026-10-25**：实测各阶段耗时后重排 `check.sh`。

### 先量化，别猜

| 阶段 | 耗时 |
| --- | --- |
| `go build ./...` | 10s |
| `go vet ./...` | 9s |
| **`go test -race -short ./...`** | **193s** |
| `go test -tags integration ./...` | 98s |
| `archcheck` | 3s |

最大的一笔不是容器，也不是 golden —— 是那 193 秒。

### 根因：`-short` 在本项目完全没生效

全仓**没有任何测试使用 `testing.Short()`**（`grep -rl 'testing.Short()' --include=*_test.go` 结果为空）。
所以 `-race -short ./...` 与 `-tags integration ./...` 跑的是同一批用例，
后者只是多编译了 integration 测试文件 —— 两遍加起来 291 秒，其中 193 秒是纯重复劳动。

### 改法

- **合并成一次**：`go test -race -tags integration ./...`（215s）。
  `-tags integration` 已经包含全部非 integration 用例，不需要先单独跑一遍单元测试。
- 新增 `BLOGX_FAST=1`：跳过 `-race`（93s），迭代时用；提交前跑完整版。
  race 默认保留 —— 关掉等于少一类真实缺陷的防线。
- `BLOGX_SKIP_INTEGRATION=1` 时退回"仅单元测试"，避免一个测试都不跑。
- **`user/adapter/mysql` 的 9 个测试加 `t.Parallel()`**：它们本来就是各自独立 schema，
  只是漏了并行标记 —— 26s → 2.5s。race 下已验证无数据竞争。
- CI 同步：`backend` job 只留 build + vet（快反馈），测试全部集中在
  `backend-integration` job（`-race -tags integration`），不再重复跑两遍。

### 顺带发现的一个坑：testcontainers 容器泄漏

调试期间设过 `TESTCONTAINERS_RYUK_DISABLED=true`，之后发现 **8 个 `mysql:8.0` 容器
还活着**（ryuk 是 testcontainers 的回收器，关掉就没人清）。它们占着内存，
会让后续测试整体变慢。已手动清理。
结论：**不要为了"省一次容器启动"去关 ryuk**；要复用实例应该用 `BLOGX_MYSQL_ADDR`。

### 还能压吗（实测过，没有便宜可捡）

- 并行度：`-p 4` 129s、`-p 8` 94s、默认（16 核）93s。默认已最优，降低反而慢。
- `article/http` 单跑只要 4s，全量时 45s —— 是和其他包争 CPU/DB 的表现，不是它自己慢。
  压缩它只能靠减测试量，不可取。
- 剩下的 10s build + 9s vet 是静态检查，删掉不划算（编译错误会晚一步才暴露）。

## 29. 为什么慢：72% 的时间在链接测试二进制，不是跑测试

**2026-10-25**：上一节只做到"去掉重复轮次"，这一节找到了真正的成本结构。

### 量化

| 场景 | 耗时 |
| --- | --- |
| `go test -tags integration -run '^$' ./...`（只编译不执行） | **67–75s** |
| 同上但完整执行 | 93s |
| 第 2 次再编译一遍（代码未变） | **67s**（没有变快） |

也就是说：**跑测试只花 26 秒，另外 67 秒全在编译和链接**。
而且这 67 秒每次都付 —— Go 的构建缓存对"测试二进制"没有帮上忙。

原因有三条叠加：
1. **160 个包，80 个有测试文件**，每个包都要产出一个独立的测试二进制；
2. **单个测试二进制 76 MB**（`go test -c` 实测）—— gorm / gin / ES 客户端 / AWS SDK
   全链打进去，且默认带完整调试信息；
3. **Windows 的链接器本来就更慢**，加上杀毒软件对新建 exe 的实时扫描。

### 真正见效的一招：别关掉 Go 的测试结果缓存

`-count=1` 是 Go 的"禁止复用测试结果缓存"开关。`check.sh` 一直带着它，
于是每次都要重新链接 80 个 76MB 的二进制。

去掉之后的实测（代码未改动）：

| 模式 | 首次（真跑） | 第二次（缓存） |
| --- | --- | --- |
| 无 race | 74s | **7.5s** |
| 带 race | 173s | **26s** |

现在 `check.sh` 默认**允许缓存**，并且把命中数打出来：

```
其中 84 个包命中缓存（未重新执行），改动过的包已真跑
要全部真跑：BLOGX_NO_CACHE=1
```

这句话是刻意加的：**缓存命中意味着这次没有真正执行那批测试**，结论来自上次。
不说清楚就又是"门禁看着绿"。CI 用 `-count=1`（等价 `BLOGX_NO_CACHE=1`）。

### 最终耗时

| 配置 | 首次全量 | 稳态（改一个包后） |
| --- | --- | --- |
| 默认（含 race） | 3分15秒 | **48 秒** |
| `BLOGX_FAST=1` | 1分36秒 | **36 秒** |
| 改动前 | 5分20秒 | 5分20秒（每次都全量） |

### 试过但没用 / 不能用的

- **降低包级并行度**：`-p 4` 129s / `-p 8` 94s / 默认 16 核 93s。默认最优。
- **关掉 ryuk 省容器启动**：会泄漏容器（实测残留 8 个），反而更慢。
- **`article/http` 单独只要 4 秒，全量时 45 秒**：不是它慢，是在与其他包抢 CPU/DB；
  压缩它只能减测试量。
- **合并测试包减少二进制数量**：能从根上省掉大部分链接时间，但需要把 80 个包重组，
  属于独立的工程改造，暂不做。

## 30. M5 收口补漏 · 阶段六：log_service 迁入 platform/log/writer

**2026-10-25**：`backend/service/log_service`（20 文件 / 2687 行）整体迁入
`internal/platform/log/writer`，`service/` 只剩 `cron_service`。

### 为什么是"子包"而不是并进 platform/log

`internal/platform/log`（2132 行）是日志 SDK：`Event` / `Field` / `Emitter` / `Sink` / `Options`，
对外只提供"造事件、发事件"的能力，不含"本项目有哪些日志"。

`log_service` 是**建在它之上的写入层**：五类业务日志（runtime / login / action / cdc / replay）
的字段组装、审计 body 裁剪、按天落文件、WAL、批量进 ClickHouse。

放在 `platform/log/writer` 子包，依赖方向是 `writer → log`，符合项目既定的
"子包可依赖父包、父包不得反向依赖子包"。并进同一个包会让 2132 + 2687 行混在一起，
且写入层引用的 gorm/CH 客户端会污染 SDK 的依赖面。

### 连带改动：规则的作用范围必须跟着代码走

`archcheck` 的 R9（日志/审计层禁 `any`）原来用
`strings.HasPrefix(rel, "service/log_service/")` 圈定范围。目录一搬，这层就**静默失去保护** ——
规则不会报错，只是不再检查。已改为 `internal/platform/log/`，覆盖 SDK 与写入层整个子树。

同时修正 `rules_test.go` 里 R9 的两条人造用例路径（它们用旧路径，改完立刻红了 ——
说明规则自检确实在起作用，不是摆设）。

### 其他

- `docs/docs.go`（swagger 生成物）里的 21 处 `log_service.XxxRecord` 对齐为 `writer.`。
  没有重跑 `swag init`：注解本身还不全（S2.8 遗留 118 个 operation），重生成的风险大于收益。
- **本次集成测试未执行**：Docker Desktop 守护进程未运行，`check.sh` 按设计跳过并告警。
  单元测试（`internal/platform/log/...` / `core` / `middleware` / `cdc`）全绿，
  archcheck 0 违规。日志管道的端到端（CH 落盘、WAL 回放）待 Docker 恢复后补验。

## 31. M5 收口补漏 · 阶段七：cron_service 拆成四个职责，service/ 目录清空

**2026-10-25**：`backend/service/cron_service`（1107 行）删除，`backend/service/` 整个目录消失。

### 为什么不能整包搬

`cron_service` 里混了四件彼此无关的事：

| 内容 | 归属 |
| --- | --- |
| gocron 调度、Redis 分布式锁、任务日志 | `internal/platform/scheduler` |
| Redis 双缓冲刷盘算法（RENAME / 解析 / 失败回补） | `internal/platform/rediscounter` |
| 写 `article_models` / `tag_models` 的计数列 | `internal/article/adapter/mysql` |
| 写 `comment_models` 的计数列 | `internal/comment/adapter/mysql` |
| 日志日期目录清理 | `internal/platform/log/writer`（它持有目录约定） |
| "哪个 key 刷到哪张表" | 装配层 `app/cron_wiring.go` |

整包搬进 `internal/platform/cron` 会让平台层 import 所有业务模块 —— 那是把
"上帝模块"换个位置而已。拆开之后，平台层不认识任何业务表。

### 顺带消灭的一类静默错误

原来四个指标的 `syncKey` 是手写的（`article_favorite:syncing` 等）。
复制粘贴漏改一个，两个指标就会共用同一个 sync key 互相覆盖 ——
不报错、不 panic，只表现为"某个计数偶尔不涨"。

现在 `SyncKey()` 从 `ActiveKey` 派生（`ActiveKey + ":syncing"`），这类错误不可能发生，
并由 `app/cron_wiring_test.go` 锁定这条规则。

### 写清了一个原本没写清的设计

`Flush` 遇到"sync key 已存在"时**继续执行而不是跳过**。这是恢复语义：
上一轮在 RENAME 之后、写完库之前崩掉，残留增量只存在于 sync key 里，跳过就永久丢数据。
正确性依赖外层分布式锁（同一时刻只有一个实例在跑）—— 没有锁时两个实例会把同一笔
增量加两遍。这条依赖关系原来只在代码里隐含，现在写在 `rediscounter` 的包注释里。

### 测试

删掉的 5 个测试（约 500 行）重写为：
- `rediscounter/flusher_test.go`（7 例）：正常刷盘、无增量、**写库失败回补**、
  非法值跳过、**双缓冲隔离**（刷盘期间的新增量不丢）、崩溃残留恢复、无 Redis 降级。
  其中"回补"与"双缓冲隔离"原测试**没有覆盖**。
- `writer/cleanup_test.go`（5 例）：时间解析、过期目录删除、非日期目录保留、
  五子目录一起清、保留天数下限。
- `article|comment/adapter/mysql/counter_flush_test.go`（7 例）：落库、0 下限、
  **目标行不存在时返回 hit=false 而不是 error**（若返回 error，已删除的文章会让任务无限重试）。
- `app/cron_wiring_test.go`（2 例）：映射表 key/列不重复、sync key 派生规则。

**集成测试仍未执行**（Docker 守护进程未运行）。本次改动有装配层新增代码
（`app/cron_wiring.go`），端到端（真实 Redis → MySQL）待 Docker 恢复后补验。
