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
| archcheck 违规数 | **236**（S3.11 删旧 `api/article_api` 后由 318 收紧到 236） | 单调递减至 0 |
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

新增表 `outbox_event_models` 不违反零 DDL：该约束只禁止改/删**既有**表
（ES 投影按列名解析 binlog）。这张表还特意**排除在全量转储之外**，
否则 canal 启动时会把历史已完成事件按 INSERT 全量重放一遍。

验证：`outboxrelay` 8 个集成用例（成功 / 幂等 / 重试耗尽进死信 / 扫描 /
未到点跳过 / 租约恢复 / worker 消费）+ 写侧 2 个事务用例，全部打真 MySQL。

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
- **M5 = S4**：其余模块，各自随行交付本模块的有状态 fake
- **M6 = S5 + L6–L9**：删旧 Hook、去雪花、TTL 与压缩、日志规则（**L.10 审计字段瘦身已提前执行**，不占 M6 工期）

T.4 里 `article`/`comment`/`user` 的 fake 依赖 domain 实体，属依赖顺序而非成本，随各模块交付；
`outbox`/`tx` 的 fake 不依赖 domain，在 M3 交付。
| 前端覆盖率（语句） | 62.47 | 单调递增 |

**规矩**：违规数只准降不准升。一旦某次提交让任何一个数字变差，就是引入了回归。
