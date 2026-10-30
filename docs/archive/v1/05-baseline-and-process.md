# 基线与过程（v1 归档）

> 归档自 `baseline.md`（2330 行，对应提交 `8246bf2` / tag `refactor-start`）+ `04-迁移路线图.md` + `05-代码质量补审.md` + `08-重构任务清单.md` + `09-M6决策清单.md` + `10-M7决策清单.md`。
> 压缩口径：**只保留关键数字与结论**。M3→M7 共 20 余节的逐步交付流水账整体丢弃，只留时间线。

**本文档分两部分**：

- **第一部分 基线**：起点时的真实数字，以及"这些数字是怎么量出来的"。
- **第二部分 过程**：M0→M7 做了什么、两轮决策留档、提交规范、收尾状态。

> **硬约束见 [overview](../../architecture/overview.md) §六**（A 组代码形状 + E 组提交协作），本文不再重复列清单。

---

# 第一部分：基线

## 0. 一句话结论

**代码本身是能编译、能跑的**：后端 77 个测试包全部通过，两个前端类型检查干净、71 个前端用例全绿。

问题不在"跑不起来"，而在**长期没有任何自动机制在看着它**：没有 CI、lint 从未执行过、架构守卫检查的是不存在的符号。M1 要做的就是把这两件事补上，并**证明新守卫真的会红**。

---

## 1. 工具链

| 组件 | 版本 | 说明 |
| --- | --- | --- |
| Go | **1.27.3** | `go.mod` 声明 `go 1.25.1`，实际用 1.27.3 编译通过 |
| gcc | 16.2.0（MinGW-w64） | **必需**：sqlite 测试走 CGO，没有它 77 个包全部失败 |
| Node | 22.22.2 | |
| pnpm | **10.33.0**（corepack 固定） | **不能用 12.x**，见环境坑第 2 条 |
| 平台 | Windows 11 / Git Bash | |

### 环境坑清单（全文档最有价值的部分，逐条保留）

| # | 现象 | 对策 |
| --- | --- | --- |
| 1 | Go 的 scoop 安装解压不完整：`scoop install go` 报成功，但 `src/` 缺 `unsafe`、`weak` 等目录，所有编译报 `package unsafe is not in std` | 直接从 scoop 缓存的 zip 补齐缺失文件（共 3707 个） |
| 2 | pnpm 12.5.1 无法为顶层依赖建链接：`node_modules` 里只有空的 `@scope` 目录，`nuxt`/`vue` 完全没链接，`nuxt prepare` 永远失败 | **必须用项目锁定的 pnpm 10.33.0**（`corepack prepare pnpm@10.33.0 --activate`）；两个 `package.json` 已加 `packageManager` 固定 |
| 3 | `nuxt typecheck` 在 Windows 上 OOM，默认堆不够，进程直接崩（连 V8 栈都没有） | `NODE_OPTIONS=--max-old-space-size=8192` |
| 4 | `go test ./...` 默认并行度偶发 build failed：`global_notif_api`、`site_api` 报 `[build failed]`，单独跑或 `-p 2` 正常 | 属本机资源限制，非代码问题；基线统一用 `go test -count=1 -p 2 ./...` |
| 5 | 行尾：仓库 blob 是 LF，工作区曾被写成 CRLF，`eslint --fix` 后十几个文件在 diff 里显示"整文件重写"（实际每处只改一个字符） | 全仓统一 LF + 新增 `.gitattributes`（`* text=auto eol=lf`）锁死 |
| 5b | 上条伴随现象：批量改文件后 `git status` 出现大量**假 M**（索引 stat 缓存失效） | **以 `git diff HEAD --name-only` 为准**，不信 `git status` |
| 6 | `nuxt prepare` 偶发 `EPERM`：`operation not permitted, open '.nuxt/eslint.config.mjs'`，Windows 文件被短暂占用，约 3 次中 1 次 | `scripts/check.sh` 已对这条错误自动重试（最多 3 次）；Linux CI 不受影响 |

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

### 关键事实：不开 CGO 时 77 个包全失败

第一轮基线（`CGO_ENABLED=0`）结果：通过 40、**失败 77**，失败原因 172 处，全部是同一句：

```
Binary was compiled with 'CGO_ENABLED=0', go-sqlite3 requires cgo to work. This is a stub
```

说明现有测试**高度依赖 sqlite 内存库**（`test/testutil` 的 `SetupSQLite`）。

⚠️ **这是"假安全"**：sqlite 与 MySQL 在 `ON CONFLICT`、`SELECT ... FOR UPDATE`、唯一键冲突、`AUTO_INCREMENT` 上行为不一致，sqlite 通过 ≠ 生产正确。这正是后来 ADR-006 里"SQLite 不冒充 MySQL、契约测必须跑真 MySQL"的由来。

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

**覆盖率门槛（只升不降）**：`frontend/vitest.config.ts` 已设 **语句 60 / 分支 50 / 函数 65 / 行 60**。取实测值向下取整作为基线；以后每补一批测试就上调，**永不回调**。

### lint 首次执行暴露的真实问题（已修，是"从来没跑过 lint"的直接证据）

| 文件 | 问题 | 处理 |
| --- | --- | --- |
| `nuxt.config.ts` | `node:fs` 被 import 两次 | 合并 |
| `app/services/image.ts` | `let f = 0; let k = 0` 初值被所有分支覆盖，死赋值 | 改为声明不赋初值 |
| `app/services/mermaid-runtime.mjs` | 正则里 `\"` 是无意义转义 | 去掉多余转义 |
| `server/routes/_backend/[...path].ts` | 取了 `requestPath` 却从未使用 | 删除 |
| `app/components/auth/Email{Login,Register}Form.vue` | `timer && clearInterval(timer)` 短路写法 | 改 `if (timer)` |
| `app/pages/users/[id].vue` 等 4 处 | 导入未使用的组件/函数（含一个 10 行死函数） | 删除 |
| `frontend_admin` | 缺 `@types/node`，8 处 `Cannot find name 'process'` | 补依赖 |
| 两端 | 都没显式声明 `typescript`，靠间接依赖碰运气 | 显式声明并锁 5.9.3 |

另有一条规则是**误报**：`vue/no-multiple-template-root` 是 Vue 2 遗留规则，Nuxt 4（Vue 3）下 `layouts/auth.vue` 只放一个 `<slot/>` 完全合法，已在管理后台配置中关闭并写明原因。

---

## 4. 架构基线：324 处违规

`backend/tools/archcheck` 首次全量扫描（`--mode=warn`）：

| 规则 | 含义 | 违规数 |
| --- | --- | --- |
| **R1** | api 层禁止直接使用 `*gorm.DB` | **144** |
| **R8** | 禁止 `errors.New` 用中文字面量 | **126** |
| **R2** | 禁止定义 `type Deps struct` | **35** |
| **R7** | 禁止裸 `go` 语句 | **17** |
| **R6** | 单文件不得超过 800 行 | **2** |
| R3 | service/domain 层禁止 import gin | 0 |
| R4 | domain 层禁止 `time.Now()` | 0（尚无 domain 层） |
| R5 | api 层禁止 import repository | 0 |
| | **合计** | **324** |

### 这组数字为什么重要

评审时的判断是"**archcheck 上线时必须报出 150+ 处真实违规，如果第一天全绿就说明规则无效**"。实测 **324 处**，且分布完全吻合现状评审结论：

- R2 的 35 处几乎全部落在 `api/*/enter.go` 与 `apideps/`；
- R7 的 17 处包含 `comment_create.go:119,130`、`article_digg.go:73`——正是评审里点出的"7 条能让进程直接退出的路径"；
- R6 的 2 处是 `service/es_service/article_sync.go`（1,547 行）与 `service/river_service/sync.go`（852 行）；
- R3/R5 为 0 说明当前确实没被违反，**不是规则失效**（`rules_test.go` 已验证它们能抓到人造违规）。

---

## 5. 门禁自检结果（最关键的一条结论）

`scripts/verify-gates.sh` 故意制造 4 类违规，实测：

```
✓ 门禁有效：backend 单测            正确失败
✓ 门禁有效：archcheck 棘轮          正确失败
✓ 门禁有效：archcheck 规则自检      通过（规则自检齐全）
✓ 门禁有效：前端 typecheck          正确失败
```

**这四行是本次重构与上一轮的实质区别。** 上一轮的守卫检查 `appctx`、`global`、`MustFromGin`、`DepsFromGin`、`mustApp` 五个符号，而这五个符号在项目里**一个都不存在**，所以 13 条规则全部永真通过。

佐证：旧 `test/architecture/no_global_refs_test.go` 用正则 `\bglobal\.` 扫全文，把新写的 `archcheck/rules.go` 注释里的 "global.DB" 判成违规——它连注释和字符串都分不清。该目录已删除。

### 另一处"门禁会撒谎"的坑

`scripts/check.sh` 把后端步骤包在子 shell `( ... )` 里，而 `run()` 只在子 shell **内部**置 `FAILED=1`；子 shell 退出码等于**最后一条命令**的成败，于是出现"archcheck 明明红了、最后一条 `go test` 是绿的 → 整体打印 ✓ 全部门禁通过、退出码 0"。**比没有门禁更危险**：它让人误以为验证过了。

修法：`run()` 失败时 `return 1`；每个子 shell 末尾显式 `exit "$FAILED"`。并用"把棘轮上限临时改成 1"验证过。

---

## 6. 下一步的对照点

后续任何一步做完，都应回来对比这几个数字：

| 指标 | 基线 | 目标 |
| --- | --- | --- |
| archcheck 违规数 | **324** | 单调递减至 0 |
| `type Deps struct` | 35 | 0 |
| api 层 `*gorm.DB` | 144 | 0 |
| 裸 `go` 语句 | 17 | 0 |
| 后端测试包失败数 | 0 | 保持 0 |
| 前端 lint error | 0 | 保持 0 |

---

# 第二部分：过程

## 一、迁移过程时间线（M0 → M7）

| 里程碑 | 主题 | 交付了什么 | 关键结果数字 |
| --- | --- | --- | --- |
| **M0 仓库基线**（tag `refactor-start`） | R0 目录解耦 | monorepo 拆成四目录；删 `prototype`/`plugin`/`scripts`/`openapi` 与 monorepo 根文件；`git init` 重建，首提交即目标形态 | 净减 **8,000 行 + 2.1 MB** |
| **M1 建闸**（tag `m1-gates`） | S1 + T1 | 首次接入 CI；`tools/archcheck`；eslint 与 `lint` 脚本真跑；`typecheck` 进 build 链路；`rules_test.go` 规则自检；`buildinfo` 改 ldflags；`BLOGX_*` 密钥命名空间拆分 | archcheck 首批规则**必须报出 ≥150 处真实违规**；**2,357 处断言首次自动执行** |
| **M2 止血**（tag `m2-stop-bleed`） | S0 剩余 | 删 `apideps/`；修 `Lifecycle.OnStop` 并接入 main（优雅停机生效）；删 `kafka_service` + `init_kafka.go`；修 admin Dockerfile 层序；nginx 4 份合 1 模板 + envsubst | nginx 约 **420 行 → 约 120 行**；删 kafka 链路 135 行 |
| **M3 地基**（tag `m3-foundation`） | S2 + L + T 基建 | `platform/{mysql,redis,es}` 真实化（`TxManager` + fake）；AI 网关 `platform/llm` 重写；**日志端到端管道一次性交付**；CDC 双 river 合并为「单一 canal 引擎 + 各业务映射器」；openapi 自动生成流水线；testkit + 有状态 fake | `article_sync.go` **1,547 行 / 42 函数 → 5-6 个文件**；两套 canal **3,231 行 → 约 1,800 行** |
| **M4 试点**（tag `m4-article`） | S3 + T3 | article 全 7 子域走通 `domain`/`app`/`adapter`；`TxManager` 改 UnitOfWork；`testkit/mysqlcontainer`；不变量矩阵先于实现 | 生产代码 **3,547 行**；收回 **63 处** handler SQL（早期"194 处"口径不成立）；golden **20 篇 × 12 接口**；审计 **16 个写接口 → 100%** |
| **M5 铺开**（tag `m5-all-modules`） | S4 | S4.0 公共前置 + 9 个业务批次（auth·user / comment / media / search / chat·social / notification / site / ai / analytics）+ S4.10 平台收尾 | 吃掉约 **29,000 行**存量（是 M4 的 8 倍）；archcheck 棘轮 **236 → 0** |
| **M6 收尾**（tag `m6-done`） | S5 + L.6–L.9 | 删 `models/` 目录；9 个旧目录消失；单上游 `/api`；生产 compose；日志收尾；审计字段瘦身 | `write.vue` **2,045 → 361 行**（前端 71 → 104 用例）；nginx location 族 **4 → 3**；`api.ts` **1,020 → 405 行** |
| **M7 契约根治**（`m7-contract`，交付时未打 tag） | 方案 E：ID 类型即真相 | 10 个 ID 类型底层统一为 `string`；删 `.swaggo` 与 `fix-openapi-id-types.mjs`；新增 archcheck 规则 R12 | 全量约 **240 处**；契约里 integer 型 ID **= 0**；流水线 **4 步 → 2 步**；后端 **82 包全绿**；R12 基线 **0** |

**顺序不可换**：S1 之前的任何重构都没有验证手段，等于重复上一轮"改完没人知道对不对"的老路。L（日志）与 T（测试）是并行轨道，但 **T1 建闸必须与 M1 同时完成**。

**判断本次是否成功的唯一标准**：`archcheck -mode=error ./...` 能通过，且**它曾经报出过真实的存量违规**。如果从第一天就是绿的，说明又做了一遍表演。

---

## 二、起点质量账：代码质量补审的结论

补审的对象是重构前的旧代码。它回答的是"哪些债值得先还"，逐条清单本身已失效，结论如下：

| 类别 | 发现了什么 | 结论 / 处置 |
| --- | --- | --- |
| **重复代码** | `api/` 内部 >0.78 相似度 **191 对**、`service/` 内部 >0.75 **563 对**；`type Deps struct` **32 个**、`enter.go` **65 个 / 1,327 行**；前端双份维护约 **500 行** | 根源是"没有抽象层可放"，不是人的习惯问题 → 由 M4/M5 的三段结构吸收；前端双份按解耦决策**保留** |
| **超大文件** | `article_sync.go` **1,547 行 / 42 函数**（承担 7 个职责）、`sync.go` 852、`query_read.go` 655、`enter.go` 517；前端 `write.vue` **2,043 行**、`api.ts` 736 行 80 个手写 interface | **判断标准不是行数，是职责数**：`enter.go` 517 行但只做 ES 客户端封装，可以留；`article_sync.go` 做 7 件事必须拆 |
| **依赖注入导致测试不可行** | 30 个 `Deps` 含 `*gorm.DB` → 任何测试都要构造 17 字段的 `Deps`；因此 **`repository/` 449 行核心 SQL 层 0 测试**；全项目只有 3 处 `t.Parallel` | **这不是"懒得写测试"，是结构不允许** → 能力接口 + UnitOfWork |
| **架构守卫造成的虚假安全感** | `test/architecture/no_global_refs_test.go:40` 检查 `"myblogx/global"` —— 该包不存在，测试永远通过 | 守卫必须有**自检**：规则要能检测人造违规，且上线必须打到存量真违规 |
| **复制粘贴造成的审计缺口** | `category_create_update.go` 有 `EmitActionAuditFromGin`，`favorite_*.go` 复制时没带 → 全仓 **21 处埋点 / 138 个 api 文件 = 15% 覆盖** | 这是**合规风险不是代码风格问题** → 埋点覆盖率作为模块级验收项，M4 起写接口 100% |
| **优雅停机失效** | `lifecycle.go:32` 的 `OnStop` 零调用点 → SIGTERM 后直接退出，不等在途请求 | 列为**优先级 1**：发版丢数据的直接原因，修复成本极低（M2 修） |
| **版本不可追溯** | `buildinfo/version.go` 硬编码 `Version = "1.0.0"`，无 ldflags 注入 | 改 ldflags 即可（M1 修）；配合 CI 让线上实例能回答"这是哪个 commit" |
| **前端 WebSocket 无背压** | `state.queue` 无上限 + 无 ping/pong + `upstream.on("error")` 只 `closePeer` 不 `terminate()` | 内存泄漏 + 开发/生产行为不一致 → WS 接 `platform/http/ws`（M5 chat 批次处理） |
| **前端 store 循环依赖** | 登出流程横跨 `auth`/`message`/`chat` 三个 store | M6 按方案 B 解环：`auth` 不 import 任何 store，下游自己 `watch(auth.isLoggedIn)` |
| **可直接回收的浪费** | prototype（web 3,312 + admin 856 + dist）、vite plugin（711）、`arch_guard.ps1`（110）、永真的 `test/architecture/`（260）、nginx 重复 conf（约 412）等 | **S0 一次回收约 8,000 行 + 2.1 MB**；S2–S5 再回收约 1,400 行（river）+ 约 500 行（前端） |

**补审最重要的一条排序结论**：优先级前四项（优雅停机、审计缺口、WS 背压、版本不可追溯）**都不在"架构"范畴，而是运维可靠性**。分层重构要几个月，这几项是几小时的事——它们比分层问题更该先修。

---

## 三、M6 决策留档

> 保留的是**当时选了什么、为什么**；任务分解与执行顺序已丢弃。

| 决策项 | 拍板 | 结论与理由 |
| --- | --- | --- |
| **models 删到什么程度** | **B**（row + ctype/enum 全迁） | row 迁到使用者自持，`ctype`/`enum` 迁 `internal/kernel/`，`models/` 目录消失。放弃 C（换强类型 ID）：500+ 处改动会把 M6 拖成 M7，与 M6 出口条件无关 —— 该议题后来独立成为 M7 |
| **平台投影 row 归属** | **A**（各包自持） | `imageref` 自持 `imageRefRow`/`imageRow`，`articlesync` 自持 `articleRow`。代价是同一张表可能有两份定义 → 靠**列名契约测试**兜底。否决 C（`models` 改名搬走）："目录变了，问题原样保留"，是典型假重构 |
| **`router/`**（1,298 行 / 17 文件）怎么收 | **A**（目录平移 `internal/platform/http/router`） | "router 认识所有模块"不是缺陷 —— composition root 本来就该认识所有人。B（注册下放各模块）的收益只是概念纯度，代价是引入一类**只有 e2e 能发现**的错误 |
| **`write.vue`（2,045 行）拆到什么程度** | **A**（子组件 + composable） | 结果 2,045 → **361 行**；验收口径同轮由"主文件 <200 行"改为 **单文件 ≤800 行**，由两端 eslint `max-lines`(error) 兜住。否决 B（顺带换编辑器）：风险是 A 的数倍 |
| **日志 `event_id` 用什么** | **A**（时间戳 + 进程内单调计数） | 单进程严格单调，多实例靠注入 `System.ServerID` 错开；零依赖几十行。保留雪花等于日志路径仍与 ID 服务耦合 —— 正是要解的问题 |
| **零 DDL 口径** | **A2 零列变更** | 见 [overview](../../architecture/overview.md) §六 A1 |
| **chat 假外键 `ChatMsgModel.SessionList`** | **B2**（直接删，零引用） | 库侧靠重建生效，不需要 `ALTER` |
| **S5.7 stores 循环依赖** | **B**（下游 watch 上游） | 否决事件总线：SSR 下订阅会跨请求累积 |

---

## 四、M7 决策留档：方案 E（ID 类型即真相）

> 拍板时间 2026-10-26，P0 / E1–E6 全部交付。技术细节见 [contracts](../../architecture/contracts.md) §7、[ADR-0007](../../adr/0007-id-string-type-as-truth.md)。本节只留决策与数量账。

**一句话**：让 Go 里的 ID 类型和它在 JSON 里的样子是同一个东西 —— 类型是 `string`，JSON 就是 `"123"`，`swag` 读类型声明时不可能印错。数据库侧照旧走 int64，**零 DDL**。

**为什么不选更省事的方案 A（`.swaggo` 覆盖）**：

| 维度 | 方案 A | 方案 E（采用） |
| --- | --- | --- |
| 契约一次生成就对 | ✅ 但只覆盖 `ctype.ID` 那 **61 处** | ✅ 全部 |
| 能删后处理脚本 | ❌ 还差 75 处 | ✅ |
| "类型数字 / 传输字符串"的隐式转换层 | **还在**（以后谁新写个 `uint64` ID，契约会再错一次） | **消失** |
| 工作量 | 约 119 处 | 约 **240 处**（约 A 的 2 倍） |

**E 比 A 多付的一半工作量，买的是"根因消失"。**

**关键发现：192 → 22 → 175。** 只改 `ctype.ID` 时残留 192 处转换错误，10 个 ID 类型统一后塌缩到 22 处 —— 说明"10 套 ID 类型并存"本身就是债。后续实测又把这一项修正为 **175 处**（散落在 `HomeStyleID`、`account.ID`、`row.UserID`、JWT claims 等），最终收编 **149 个**，**26 个按理由豁免**。

**26 个豁免的口径（不是"懒得改"，每条都有理由）**：

| 豁免范围 | 数量 | 理由 |
| --- | --- | --- |
| `internal/platform/log/**` | 6 | ClickHouse 落盘事件结构，列是 `UInt64`；外部真相在 `01-logs.sql`。Go 侧改成 string 会与列类型不一致，可能让 `JSONEachRow` 摄入失败 —— **那是"日志丢数据"，比契约印错严重得多** |
| `internal/testkit/schema/**` | 17 | 同上，CH 表测试行结构 |
| `ServerID`（conf ×3 + canalbase ×1） | 4 | MySQL 复制协议的 `server_id` / 雪花机器号，是 uint32 节点编号，**不是业务实体 ID，也不进 HTTP 契约** |
| `initializedID`（platform/idgen） | 1 | 雪花 worker 编号 |

**唯一编译器抓不到的风险**：6 处 ID 大小比较（字符串语义下 `"10" < "9"` 成立）。过程中靠人工审逐处确认，并顺带修掉两个隐性缺陷 —— `chat.BuildSessionID` 字符串比较会导致双方算出不同 session_id、**消息串号**；`site.Banner.CoverImageID` 早期人工审计漏判，是 R12 上线当天抓出来的。

---

## 五、提交规范

```
<任务ID>: <一句话>

背景/取舍（可选）
验收：<跑过的命令/结果>
```

示例：

```
S1.2: ci 跑 go vet + go test -race -short

验收：PR 上 fast job 绿；注入失败测试后变红
```

**里程碑 tag**：

| tag | 含义 |
| --- | --- |
| `refactor-start` | 重构起点（四目录解耦 + 设计文档基线） |
| `m1-gates` | 建闸完成（CI + archcheck + 测试真跑） |
| `m2-stop-bleed` | 止血完成 |
| `m3-foundation` | 地基完成（platform / log / testkit） |
| `m4-article` | article 试点完成 |
| `m5-all-modules` | 12 模块迁移完成 |
| `m6-done` | 收尾完成 |
| `m7-contract` | 契约根治完成（**补记的 tag 名，M7 交付时没有打 tag**；按里程碑回溯用 `git log --oneline` 里 `M7` 开头的提交） |

---

## 六、收尾状态

**当前没有任何"等排期的重构欠账"**。下表是"曾经挂着、现在什么状态"，避免有人照着旧清单重复劳动：

| 曾经挂着的项 | 现在的状态 |
| --- | --- |
| `contract.ts` 与 `api.ts` 的取舍 | ✅ 已定：`api.ts` 只留视图模型，其余从契约派生；理由写在两个 `types/api.ts` 文件头 |
| `contract.ts` 接进业务代码 | ✅ 已完成：两端全仓在用；`api.ts` 合计 423 行（起点 1,020） |
| `InboxChatPanel` 富文本协议联调 | 🟡 代码侧已通（`msg_type` = 2 / 7），**实际收发仍需后端联调确认** —— 属联调任务不是重构任务 |
| S2.8 字段级注解（`@Param` 的 example/required） | ✅ 不再追：`required` 比例不作为目标（响应 struct 没有"一定返回"的表达载体） |
| 换 swag v2 直出 OpenAPI 3.1 | ✅ 已完成：流水线 2 步，删掉 `docs.go` / `swagger.yaml` / `swag` 依赖 |

**死信重投闭环**（M7 之后补的产品级决策，见 [ADR-008](../../adr/0008-dead-letter-replay.md)）：管理端「同步死信」页 → `POST /logs/dead-letter/replay` → `platform/cdc/dlq.Service` 按 stream 分派 → 更新 `cdc_dead_letter`（pending → resolved）→ `writer.EmitReplayEvent` → 管理端「日志中心 → 回放日志」看时间线。

仍有两类**无法自动重投**，接口返回原因与替代做法，**不静默失败**：

| stream | 为什么不支持 | 该怎么处置 |
| --- | --- | --- |
| `es_projection`（旧记录） | 2026-10-26 之前落库的死信只有表名与动作、**没有主键** | `./server -es -s article-sync` 全量重建文章投影；新记录带 `ids`，可直接重投 |
| `image_ref_river`（新增 / 更新类） | 行数据经 JSON 往返后**数值类型无法可靠还原** | 重新保存对应业务对象触发重建（删除类可直接重投） |

这两种返回 **`unsupported` 而不是 `failed`**：不改库、不涨 `retry_count` —— "这条本来就不该重投"和"这次没成功"是两件事。

---

## 【归档备注】仅具历史价值、v2 不需要再看

- §6 之后全部 20 余节 M3→M7 逐步交付流水账：M3-A/B/C1/C2/D 各批次改了哪些文件、S4.1d 认证链路、S4.2–S4.9 各业务模块交付记录、S4.10a/b/c 清规则、M5 收口补漏阶段一到七、M6 各开工项与剩余清单。
- 棘轮数字的逐步演进（324 → 321 → 320 → 319 → 318 → 236 → 233 → 250 → 0），已压缩为 §一 时间线里的结果数字。
- 每阶段的"验收实测"逐条描述、"改了哪些文件"清单、提交号与 tag 对应关系。
- 门禁提速的量化过程（5分20秒 → 3分49秒之类）——属一次性调优记录。
- 各阶段"发现的问题 / 改法 / 踩坑"的细节推演（如 `MdToSafe` 并发缺陷根因、store 解耦的四个方案对比）。
- 05 补审里的逐条证据（`fans_list.go` vs `follow_list.go` 相似度 0.97 之类）。
- 决策文档里被否决选项的完整论证，只留"选了哪个 + 一句话否决理由"（见 §三 / §四）。
