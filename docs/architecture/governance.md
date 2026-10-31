# 治理与质量

> 归档自：`06-质量保障与治理.md`、`07-日志模块治理.md`、`12-重构收尾审查.md`。
> 内容与当前代码核对过（`.github/workflows/ci.yml`、`scripts/check.sh`、`scripts/arch-baseline.env`、`backend/tools/archcheck/rules.go`、两端 `vitest.config.ts`）。
> 本文保留**当前仍在运行**的门禁、守卫、测试体系、日志管道，以及 v1 收尾盘点出的产品缺陷与教训。
> 已删除：论证过程、被否决的备选方案、同一结论的重复表述、迁移阶段的过程记录。

**一句话**：v1 把"测试、守卫、日志"从"写了但没人跑"改成了"跑不动就红"，代价是每一次门禁撒谎都要再补一道自检。

> **硬约束见 [overview](./overview.md) §六 D 组**，本文不再重复列清单。

---

## 一、门禁

### 1.1 本地一键门禁：`scripts/check.sh`

用法：`./scripts/check.sh [all|backend|frontend|gates]`。

| 段 | 实际执行的命令 |
| --- | --- |
| 后端 | `go build ./...` → `go vet ./...` → `go test ./tools/archcheck/...`（规则自检）→ `go run ./tools/archcheck --mode=error --max-violations=$ARCH_MAX_VIOLATIONS .` → 再跑一遍 `--skip-tests=false` → `run_integration` |
| 集成 | `go test $race -tags integration $count_flag -timeout 25m $parallel ./...` |
| 契约类型 | `run_contract_types`：`openapi-typescript ../backend/docs/swagger.json -o /tmp/xxx.d.ts` 再 `diff` 与 `app/types/openapi.d.ts` |
| 前端（两端） | `nuxt prepare` → `pnpm lint` → `pnpm typecheck` → `pnpm test` → `pnpm test:coverage` |
| 自检 | `bash scripts/verify-gates.sh` |

环境变量（默认值即推荐值）：

| 变量 | 效果 |
| --- | --- |
| `BLOGX_FAST=1` | 跳过 `-race`。快约 12 秒，但**不做数据竞争检测；提交前别用** |
| `BLOGX_NO_CACHE=1` | 等价于 `-count=1`，强制全量真跑（CI 就是这个语义） |
| `BLOGX_SKIP_INTEGRATION=1` | 只跑单元测试，golden 与容器契约不验证 |
| `BLOGX_REQUIRE_INTEGRATION=1` | Docker 不可用时**直接失败**而不是跳过（CI 用） |
| `BLOGX_MYSQL_ADDR=h:p` | 复用已有 MySQL，省掉每次起临时容器（约 10 秒） |
| `ARCH_MAX_VIOLATIONS` | 棘轮基线，只从 `scripts/arch-baseline.env` 读 |
| `GOTOOLCHAIN=local` / `CGO_ENABLED=1` | sqlite 测试需要 CGO，脚本内固定导出 |
| `NODE_OPTIONS=--max-old-space-size=8192` | nuxt typecheck 在 Windows 上默认堆不够 |

### 1.2 CI：9 个 job（`.github/workflows/ci.yml`）

| # | job | 跑什么 | 关键点 |
| --- | --- | --- | --- |
| 1 | `backend` | `go build ./...` + `go vet ./...` | **不跑测试**——项目里没有任何测试用 `testing.Short()`，`-race -short` 与 integration job 是同一批用例 |
| 2 | `backend-integration` | `go test -race -tags integration -count=1 -timeout 25m ./...` | MySQL 8.0 service 供所有包共用；先断言 `docker info` 可用 |
| 3 | `archcheck` | 规则自检 + 棘轮两遍（含 `--skip-tests=false`） | 基线从 `arch-baseline.env` 读，**绝不写死数字** |
| 4 | `contract` | 装 `swag@v2.0.0-rc6` → `go mod download` → `make openapi` → `git diff --exit-code backend/docs/swagger.json` | 判据是"重新生成后 git 无差异"，不是"能生成" |
| 5 | `frontend-lint` | 矩阵 `[frontend, frontend_admin]`：`pnpm lint` | `TZ: Asia/Shanghai` |
| 6 | `frontend-typecheck` | 矩阵两端：`pnpm typecheck` | 同上 |
| 7 | `contract-types` | 矩阵两端：重新生成 d.ts 到 `/tmp` 再 `diff` `app/types/openapi.d.ts` | 补上 job 4 盖不到的那一层 |
| 8 | `frontend-test` | 矩阵两端：`pnpm test:coverage` | 曾经只跑 `pnpm test`，admin 0 用例 + `passWithNoTests` = 空转 |
| 9 | `gates-canary` | 仅当「PR 标题含 `[canary]`」**或「本次 PR 改到门禁自身」**时调用 `scripts/verify-gates.sh`；门禁自身 = `scripts/` 下的门禁脚本与基线、`.github/workflows/`、任一路径的 `vitest.config.ts` | **必须调用与本地同一个脚本** |

> job 数量演进：7（无一校验契约）→ 8（加 `contract`）→ 9（加 `contract-types`）。

### 1.3 门禁的已知坑（踩过才写进来）

| 坑 | 原文要点 / 对策 |
| --- | --- |
| **`go vet ./...` 不编译 `//go:build integration` 的文件** | 默认跳过 integration 文件，`go build` 也**完全不编译 `_test.go`**。v1 里出现过"第一轮全绿是假的"——真跑 `check.sh` 时 `go vet` 与 `go test -tags integration` 同时红约 **90 处**，全在 integration 测试里。收集全量错误用 `go test -tags integration -run='^$' -gcflags=-e ./...`（不跑用例只编译，`-e` 不让编译器截断） |
| **子 shell 吃掉退出码** | 子 shell 的退出码只等于**最后一条**命令，前面某步失败会被吞掉。所以 `run()` 必须 `return 1`，且每个子 shell 末尾必须显式 `exit "$FAILED"`（backend / 两端前端各一处） |
| **测试缓存让门禁"看着绿"** | 命中缓存意味着**这次没有真正执行那批测试**。脚本把 `(cached)` 命中数打出来；CI 必须 `BLOGX_NO_CACHE=1`。实测带缓存 173s → 26s |
| **testkit 的 `sync.Once` 是包级的** | `go test ./...` 每个包是独立进程，不共用 MySQL 会让 12 个容器测试包各起一个，并行互相拖垮（Windows Docker Desktop 实测约五成概率就绪超时）。共用后实测 **10 分钟 → 1 分 40 秒**；没有共享实例时必须 `-p 1` |
| **容器"已启动" ≠ 可连接** | 必须用 `mysqladmin ping` 循环等待（脚本内 60 次 × 1s） |
| **静默跳过是门禁撒谎的老毛病** | `BLOGX_SKIP_INTEGRATION` 与"Docker 不可用"都必须打印醒目警告；CI 用 `BLOGX_REQUIRE_INTEGRATION=1` 把跳过变成失败 |
| **时区** | runner 默认 UTC，前端有用例断言"某个时刻显示成 3 点"，四个前端 job 统一钉 `TZ: Asia/Shanghai` |
| **swag 依赖未下全会假报错** | 不加 `go mod download` 会偶发 `cannot find all dependencies, <nil>`，而 swag 的报错只说"找不到依赖"，极易误判成注解写错 |
| **Windows：`nuxt prepare` 偶发 EPERM** | `.nuxt/eslint.config.mjs` 被杀软/索引短暂占用；脚本重试 3 次，只在日志含 `EPERM` 时重试 |
| **`git diff` 判据会假红** | 工作区常有未提交改动，`git diff --exit-code` 会假红、人习惯性忽略。契约类型改成"重新生成到临时文件再 `diff`" |
| **门禁自检的恢复不能用 `git checkout --`** | 那会把工作区里未提交的活一起冲掉。所有临时改动用**备份副本**恢复，`trap` 只兜底 |

### 1.4 门禁自检：`scripts/verify-gates.sh`（4 道 → 11 道）

> 存在的意义：**证明上面所有 job 真的会失败**。如果它们只是永远绿，整套 CI 等于不存在。

| # | 门禁 | canary 方式 |
| --- | --- | --- |
| 1 | 后端单测 | 注入必败测试 |
| 2 | archcheck 棘轮 | 注入 1 处违规，阈值从 `arch-baseline.env` 读 |
| 3 | archcheck 规则自检 | 跑通（规则测试自身即反例） |
| 4 | 前端 typecheck | 注入类型错误 |
| 5 | 契约未过期 | 改 `@title` → `make openapi` → 生成结果必须变化 |
| 6 | 前端契约类型未过期 | 往 d.ts 插一行 → 必须判为过期 |
| 7 | 契约 ↔ 路由对账 | 往 swagger.json 塞虚构端点 → 测试必须报 phantom |
| 8 | admin 鉴权 | **不注入**，只跑通（自带两条对照断言） |
| 9 | 菜单 ↔ 页面 | 往 `ADMIN_NAV` 加死链 → 用例必须红 |
| 10 | 前端 lint | 注入 `debugger`（两端都是 error） |
| 11 | 覆盖率门槛 | 往 `app/utils` 加 0 覆盖文件（100% → 85.7%） |

- 第 5 道不用 `git diff` 判定的理由见 §1.3。
- 第 8 道不注入的理由：要让它红必须临时摘掉某条后台路由的 `AdminMiddleware`，脚本一旦被中断就留下"后台无鉴权"。它本身已带 `TestProbeDistinguishesUnprotectedRoute` 与 `TestAdminOnlyListCoversContract` 两条对照断言。
- 验收：**11/11 全部正确报红，耗时 3 分 29 秒**；跑完 `git status` 与跑之前逐字节一致，无 canary 残留。
- CI 的 `gates-canary` 曾经在 YAML 里另写一份注入逻辑、只验证 1 道 —— 两处各写一份的必然结果是"本地 11 道、CI 1 道，没人知道该信哪个"。
- **门禁自检的触发条件曾经只看 PR 标题**：于是"改了门禁却没人验门禁"能一路飘过去。2026-10-31 实际发生过一次：`verify-gates.sh` 第 11 节（覆盖率门槛）的注入假设已过期（admin 覆盖率与文件数增长后，注入一个零覆盖文件掉不到 95 门槛），但因为没人跑它，PR 上的 `gates canary` 显示 pass 而 step 其实被 skip —— 门禁给了它并没有提供的保障。现在补了第二条触发：**本次 PR 改到门禁自身时无条件跑**。

---

## 二、架构守卫：`backend/tools/archcheck`

### 2.1 为什么是 Go AST

它替代了原来的两套实现（`scripts/arch_guard.ps1` 与 `test/architecture/`）——那两套检查的 **5 个符号在项目中根本不存在，因此永远通过，等于没有守卫**。

设计要点：

1. 用 `go/ast` 分析真实语法树，而不是 grep 字符串，**别名/换行/注释都绕不过**。
2. `--mode=warn`（只报告）/ `--mode=error`（违反即非零退出），便于存量渐进收紧。
3. `--max-violations` 棘轮：存量已知时允许一定数量，**只准减不准增**。
4. 每条规则都必须能在人造代码上报错 —— 由 `rules_test.go` 强制验证。

常用 flag：`--mode` / `--max-violations` / `--skip-tests`（默认 `true`）/ `--rule`（ID 或 slug，如 `R9` / `log-no-any`）/ `--list-rules`。

### 2.2 R1–R12 规则表

| # | Slug | 规则 | Level | 只查生产代码 |
| --- | --- | --- | --- | --- |
| R1 | — | api 层禁止直接使用 `*gorm.DB` | error | ✅（e2e/golden 要建表造数据） |
| R2 | — | 禁止定义 `type Deps struct`（改用能力接口） | error | — |
| R3 | — | service/domain 层禁止 import gin | error | — |
| R4 | — | domain 层禁止直接调用 `time.Now()`（应注入 Clock） | warn | — |
| R5 | — | api 层禁止 import repository（分层倒置） | error | — |
| R6 | — | 单文件不得超过 800 行 | warn | — |
| R7 | — | 禁止裸 `go` 语句（应使用 `safego.Go`） | warn | ✅ |
| R8 | — | 禁止 `errors.New` 使用中文字面量（应使用 apperr 错误码） | warn | ✅ |
| R9 | `log-no-any` | **日志输入禁 `any` / 禁同包类型 / `Extra` 必须 `map[string]string`** | error | — |
| R10 | `mod-no-models` | 业务模块禁止 import models（row 结构应放本模块 adapter） | warn | —（**必须查测试文件**） |
| R11 | `mod-no-cross-adapter` | 禁止跨模块 import 其他模块的 adapter | warn | —（**必须查测试文件**） |
| R12 | — | **ID 语义字段必须用 `ctype.ID`，禁止裸整型** | — | — |

`ProductionOnly` 的判据是"违规在测试文件里会不会造成线上风险"：R1/R7/R8 约束运行时行为，测试里造数据、起测试协程、写断言消息都不产生线上风险；R10/R11 约束**依赖方向**，测试文件同样会把依赖固化下来，甚至直接决定能不能删 `models`。

### 2.3 棘轮与基线数字

- 基线**只有一处定义**：`scripts/arch-baseline.env` 的 `ARCH_MAX_VIOLATIONS`。三处消费：`scripts/check.sh`、`scripts/verify-gates.sh`、`.github/workflows/ci.yml`。
  以前三处各写一遍，结果"本地降到 0、CI 还停在 250"与"自检脚本永远判定失效"同时发生。
- 演进：M1 基线 **324** → M2 **321** → M3-D **320** → M3-C1 **319** → M3-C2 **318** → S3.11 **236** → S4.0-A **233** → 新增 R10 重新标定 **250** → S4.10c **0** → M7-E6 新增 R12，落地时全仓已是 0 违规，基线保持 **0**。
- 棘轮跑**两遍**：第二遍 `--skip-tests=false`，专门盯依赖方向（R10/R11）。R1/R7/R8 标了 `ProductionOnly`，在测试文件上本来就不报，所以第二遍不会因为"测试里造数据用了 `*gorm.DB`"变红。

### 2.4 规则自检（防止"又一轮永真守卫"）

```go
func TestEveryRuleDetectsViolation(t *testing.T) {
    for _, r := range rules {
        if got := r.Check(violationFixture(r)); len(got) == 0 {
            t.Fatalf("规则 %s 无法检测人造违规——规则本身失效", r.Name)
        }
    }
}
```

**判断标准**：archcheck 上线时必须报出 **150+ 处真实违规**；若第一天全绿，说明又做了一遍表演。

---

## 三、测试体系

### 3.1 三条铁律

1. **规则测试必须零依赖**：domain 层测试不连 DB、不起 HTTP、不读环境变量，`go test` 毫秒级完成。
2. **一个测试只断言一件事**：不写"顺便也测了"；失败时能一眼看出哪条规则破了。
3. **测试不写全局状态**：没有 `InitGlobals`，没有包级变量覆盖；所有依赖从参数注入，测试可并行。

> 起点教训：当时 117 个测试文件 / 2,357 断言，但只有 **3 处** `t.Parallel()`；`article_test.go` 里断言的是"`validateRequest` 不返回 error"——**它验证了实现按预期跑，却没验证业务规则没被破坏**。
> **测试要断言"结果符合规则"，而不是"函数返回了预期类型"。**

### 3.2 金字塔与分层口径

```
        ▲  E2E（黑盒 HTTP）        ~10%   登录→发文→搜索→评论 主链路
       ╱ ╲
      ╱───╲  集成（app + 真 MySQL） ~20%   事务、仓储 SQL、CDC、ClickHouse 写入
     ╱     ╲
    ╱───────╲ 单元（domain + 纯逻辑）~70%  规则、值对象、脱敏、截断、采样
   ╱_________╲
```

按"**错了代价多大**"分配预算，不按"是否重要"平均用力：

| 测试 | 覆盖范围 | 抓什么 | 不抓什么 |
| --- | --- | --- | --- |
| **不变量矩阵** | 每模块必做 | 权限、状态机、"谁能改什么" | 编排错误、SQL 错误 |
| **app 用例测**（fake 端口） | 每模块必做 | 事务边界、事件发布、跨模块 port 调用 | 真实 SQL |
| **adapter 契约测**（真 MySQL） | 每个有写路径的模块 | SQL 正确性、mapper、软删/级联 | 业务规则 |
| **golden** | 仅 5 个只读模块 | 响应形状逐字节漂移 | 写接口 |
| **openapi 契约测试** | 全部模块 | 响应字段与 `swagger.json` 一致 | 业务语义 |
| **e2e 冒烟** | 全部模块 | 主链路能跑通、状态对 | 具体返回内容 |

- **做 golden 的 5 个模块**：`comment` / `search` / `site` / `notification` / `analytics`（只读、字段多、嵌套深、前端重度依赖）。article 已有 20×12，不重复做。
- **不做 golden 的**：写接口（ID/时间戳天然不同，会天天报红，最后所有人都会去关掉它）、WebSocket（`chat`，分片顺序与心跳不稳定）、文件上传（`media`，二进制与签名 URL 带时效）、SSE（`ai`，流式片段边界不稳定）。这三类只做 e2e 冒烟 + app 层用例测。
- **位置约定**：domain 在 `internal/<mod>/domain/*_test.go`（零外部依赖，`t.Parallel`）；app 在 `internal/<mod>/app/*_test.go` 且**必须 `package app_test`**（要 import 同模块的 `apptest`，否则 `app → apptest → app` 成环）；adapter 契约测带 `//go:build integration`；golden 数据在 `internal/<mod>/adapter/http/testdata/`；e2e 在 `test/e2e/`。

### 3.3 `testkit`（替代 `test/testutil`）

`test/testutil` 靠覆盖 `global.*` / `Deps`，是并行的根源障碍。

| 组成 | 内容 |
| --- | --- |
| `testkit/` | `clock.go` / `id.go` / `golden.go` / `db.go` |
| `internal/testkit/chcontainer` | testcontainers ClickHouse + 执行 `01-logs.sql` |
| `internal/testkit/mysqlcontainer` | testcontainers MySQL 8（契约测专用） |
| `http.go` / `redis.go` | `NewTestServer(router)` + 断言助手；miniredis 实例（返回 client，不覆盖全局） |
| `fake/` | `article_repo.go` / `comment_repo.go` / `user_repo.go` / `outbox.go` / `tx.go` |
| `fixture/` | `ArticleBuilder().Status(Draft).Author(u).Build()` 等 |
| `golden.go` / `assert.go` | golden 读写 + `UPDATE_GOLDEN=1`；领域断言 `RequireVisible(t, article, viewer)` |

**有状态 fake 优于 gomock**：gomock 只能验"调了哪个方法、传了什么参数"，无法模拟状态与故障；本项目最需要测的恰恰是状态。

```go
type ArticleRepo struct {
    mu     sync.Mutex
    byID   map[uint64]*article.Article
    failOn map[string]error // "Save" -> ErrConflict
    calls  int
}
```

用途：测"重复创建返回冲突"、"事务回滚后仓储无副作用"、"Outbox 发布失败时业务回滚"、"第二次读能读到第一次写"。

### 3.4 确定性、并行与 golden 口径

- 依赖注入：`clock Clock`（生产 `RealClock` / 测试 `FakeClock`）、`ids IDGen`（生产 ULID / 测试 `SeqID`）。domain 层**禁止**直接调 `time.Now()` / `rand` / 雪花（archcheck R4 等规则）。
- golden 用 `SeqID` 产生稳定 ID；时间相关规则用 `FakeClock.Advance` 精确验证。
- **golden 口径：写死种子，不做归一化**。种子 SQL 里显式写死 ID、`created_at`/`updated_at`、各类计数，并在种子内 `SET time_zone='+08:00'`；断言**逐字节比对**，不替换 `<id>` / `<ts>` 占位符。
  理由：golden 的价值不是"结构对不对"（那是契约测试的活），而是"**同一个请求，重构前后输出是否一字不差**"。一旦把 ID 和时间抹掉，就永远发现不了 `123` 变成 `"123"`、`+08:00` 变成 `Z` 这类**线上会炸、测试却绿**的错。
- 并行：所有测试默认 `t.Parallel()`；每个测试 `newArticleApp(t)` 构造独立 Service + fake；集成测试用每测试独立 schema（`test_<random>`）或事务回滚，禁止共享一个库；`t.Cleanup` 负责关闭容器/连接/临时目录。

### 3.5 覆盖率门槛

| 层 | 目标 |
| --- | --- |
| `*/domain/` | ≥ 90% |
| `*/app/` | ≥ 85% |
| `*/adapter/`（契约测） | 每个方法至少 1 条 |
| `internal/platform/log` | ≥ 90%（纯逻辑） |
| 整体 | **不设**（避免垃圾测试） |

前端（vitest `thresholds`）：

| 端 | 统计范围 | statements / branches / functions / lines | 备注 |
| --- | --- | --- | --- |
| `frontend` | `app/` 全量（排除 `app/types/**`） | **60 / 50 / 66 / 60** | 展示组件靠 e2e |
| `frontend_admin` | **只 `app/utils/**`**（纯逻辑层） | **95 / 90 / 95 / 95** | 实测 100 / 92.3 / 100 / 100 |

- 为什么 admin 只卡 `app/utils`：页面与组件要 mock 一整套 naive-ui 交互，投入产出比低。含义是"**已测过的不许退化**"，而不是用虚高数字粉饰整站 0 覆盖。
- `passWithNoTests` 必须从 `true` 改成 **`false`** —— 否则"测试文件被误删"和"全部通过"在门禁里长得一模一样。
- 变异测试（nightly，domain 关键规则：`go-mutesting` 或 `greece`），目标 mutation score **≥ 70%**。若"删掉某行规则测试仍全绿"，说明测试没守住规则。

### 3.6 Flaky 治理

| 来源 | 对策 |
| --- | --- |
| 时间 | 注入 `Clock`，禁 `time.Now()` |
| 随机 | 注入 `IDGen`，固定种子 |
| 端口/网络 | 用 `httptest` / 随机端口，禁用固定 8080 |
| 全局状态 | 测试不写包级变量；每测试独立实例 |
| 容器启动慢 | 集成测试单独 tag + 更长超时，不进 fast 档 |
| 顺序依赖 | 强制 `t.Parallel()` + 独立数据，禁止共享 fixture 可变状态 |

被标记 flaky 的测试：**隔离到 quarantine job**，限时修复，**不允许长期 `retry` 掩盖**。

---

## 四、日志模块（`internal/platform/log`）

### 4.1 六条硬约束（缺一不可）

1. **内存有界**：进程内日志缓冲有硬上限（条目数 + 字节数双限），超限**丢最旧或丢新并计数**，绝不无限增长。
2. **请求路径不阻塞**：`Emit` 只做"投递"，不做序列化、不做磁盘、不做网络。投递失败可丢（审计除外）。
3. **类型化事件**：日志字段是**封闭类型集合**（`string/int64/uint64/float64/bool/duration/time/[]string`），**禁止 `any`、禁止传结构体**。违反由 archcheck（R9）阻断。
4. **边界采集**：request/response body 在 HTTP 边界用 `io.LimitReader` **限量读取**，先截断再脱敏再序列化；**绝不"先反射整个对象再截断"**。
5. **不丢要可计损，要丢要可解释**：每类日志有明确的"可丢/不可丢"策略；所有丢弃都有计数器与采样告警。
6. **日志自身可观测**：队列深度、丢弃数、刷盘延迟、WAL 积压、写入错误率全部有指标；`/healthz` 能反映日志管道健康。

### 4.2 审计字段策略

**一句话**：审计日志默认**不记全量原始 body/header**；热库（ClickHouse）只存元数据 + 业务语义 + **失败请求**的脱敏限量体；不做冷存储分层。

| 项 | 决定 |
| --- | --- |
| `request_body_raw` / `response_body_raw` / `request_header_raw` / `response_header_raw` | **删除**（应用字段 + ClickHouse 列） |
| `request_body` / `response_body` | 保留，但**仅在请求失败（`success=0`）时填充**；成功请求留空；列加 `CODEC(ZSTD(9))` |
| header | **不再单独存储**；trace 信息由 `trace_id` / `request_id` / `span_id` 列承载 |
| 采集 | HTTP 边界 `io.LimitReader` 限量读（硬上限 **1MB**），再脱敏 + 截断到 **≤16KB**；**禁止 `any`、禁止传结构体** |
| 冷存储分层 | **不做**；`payload_ref` 列预留但默认不写 |
| 闸门 | archcheck R9 `log-no-any` |

理由：全量 raw 同时踩三个雷 —— ① 它是最长的行，采集/传输最先出问题；② 它逼着"反射整棵树再截断"，是爆炸路径的根源；③ 请求体里几乎必然含 PII，落库即有合规风险。而审计真正要回答的"谁、何时、对什么、做了什么、结果如何"由 `action_name` / `target_type` / `target_id` / `user_id` / `result` 这些**业务语义字段**承载。

### 4.3 管道结构

```
业务代码 → log.Emit(ctx, event)        ← 只传类型化事件，O(1) 投递
  ▼
Emitter（有界环形缓冲，双限额）
  - entries 上限 N（默认 8192）
  - bytes   上限 M（默认 8MB）
  - 满：runtime 丢弃+计数；audit 落 WAL+计数
  ▼ 单写协程（批量）
Batch（≤500 条 / 1s）
  ├─ 成功 → ClickHouse（HTTP 批量 INSERT ... FORMAT JSONEachRow）
  └─ 失败/超时 → 本地 WAL（append-only） → Replayer（退避重放）→ ClickHouse
```

| 参数 | 值 |
| --- | --- |
| `buffer_entries` / `max_bytes` | 8192 / 8MB |
| `flush_interval` / `batch_size` | 1s / 500 |
| ClickHouse 超时 / 重试 | 3s；100ms → 1s → 5s，最多 3 次 |
| WAL 单文件上限 | ≤64MB，按 kind + 日期分文件，写完 fsync 可配置 |
| body 采集上限 / 输出上限 | 1MB（限量读）/ ≤16KB |
| JSON 解析 | 限深 ≤8 层、限节点数 ≤2000；非 JSON 直接截断字节 |

### 4.4 类型化事件模型

```go
type Kind uint8
const ( KindRuntime Kind = iota; KindAccess; KindAudit; KindLogin; KindCDC; KindReplay )

type Field struct {
    Key string
    Val fieldValue // 内部私有 union：str/i64/u64/f64/bool/dur/time/strs
}
func Str(k, v string) Field
func Int(k string, v int64) Field
func Uint(k string, v uint64) Field
func Bool(k string, v bool) Field
func Dur(k string, v time.Duration) Field
func Time(k string, v time.Time) Field
func Strs(k string, v []string) Field // 有长度上限
```

`AuditEvent` 与 `Base` 全部是标量字段（`Base.Extra` 只能是 `map[string]string`，key 白名单 + value ≤256B + 总 ≤2KB）。
**关键差别**：没有 `any`，没有 `reflect`，序列化是直接写已知字段。

`Emit` 必须 O(1)：不做 `json.Marshal`（`encode` 只拼装字段 + 估算大小），不做磁盘/网络；`ctx` 只用于取 trace 字段与取消，**不用于阻塞等待**；缓冲满时走 `e.overflow(ev)`，**永不阻塞**。

### 4.5 可丢 / 不可丢

| 日志类 | 队列满 | ClickHouse 挂 | 说明 |
| --- | --- | --- | --- |
| runtime / access | **可丢**，计数 + 采样告警 | 可丢（或极短 WAL） | 运行日志允许损失，换取业务稳定 |
| audit / login | **不可丢**：落 WAL | 落 WAL，无限重试（有积压上限告警） | 合规要求 |
| cdc / replay | **不可丢**：落 WAL | 落 WAL | 用于对账 |
| security | 不可丢 | 落 WAL | 登录/越权事件 |

### 4.6 两处必须改掉的旧写法

- **去掉雪花 ID 依赖**：旧路径每条日志都调 `db_service.NextSnowflakeID()`。改为进程启动生成 `instance_id`（配置或随机）+ 进程内 `atomic.Uint64` 单调递增作 `event_id`；唯一性 = `(instance_id, event_id)`，ClickHouse 排序键已含 `service, ts, event_id`。跨实例需要全局唯一时用启动时生成的 ULID 前缀 + 本地序号，**仍然零 DB 调用**。
- **去掉全量调用栈**：生产默认 `SetReportCaller(false)`；仅 **error 级**按配置采样抓栈（`runtime.Caller` 或 `debug.Stack`，8KB 上限）；dev 可开全量但用 build tag / 配置隔离，不带到生产。

### 4.7 采样、配置与 ClickHouse 侧

- 按 `event_name` 令牌桶（默认 **100/s**），超限合并为 1 条 `log_suppressed` 摘要（每 **10s** 一次）。
- access 日志：成功请求 1:1（低流量）或 1:10（高流量，可配）；4xx/5xx 全量。DEBUG 生产关闭。
- 配置块：`log.async.{enabled,buffer_entries,max_bytes,flush_interval,batch_size}`、`log.sink.{type,clickhouse.addr/database/timeout,wal_dir}`、`log.sampling.{default_rate,rules}`、`log.capture.{body_limit,header_allowlist,redact_keys}`、`log.retention.{runtime_days,audit_days,login_days}`。
- 启动校验：DSN 不通 → **fail fast**（不要带病启动）；`wal_dir` 不可写 → fail fast。热更新只允许 `level` 与 `sampling`（`atomic.Pointer`）。
- TTL：runtime **30** 天；audit / login **180** 天；cdc / replay **30** 天。大文本列加 `CODEC(ZSTD(3))`，body 列加 `CODEC(ZSTD(9))`。`request_body` / `response_body` 应用侧已 ≤16KB，DB 侧再加校验防旁路写入超限。

### 4.8 日志管道自身的可观测性

| 指标 | 含义 | 告警阈值 |
| --- | --- | --- |
| `log_queue_depth{kind}` | 环形缓冲当前深度 | > 80% 持续 1min |
| `log_dropped_total{kind,reason}` | 丢弃数（满/采样/超限） | 1min 内 > 0 |
| `log_flush_duration_seconds` | 批量刷盘耗时 | p99 > 1s |
| `log_write_errors_total` | ClickHouse 写入失败 | > 0 持续 1min |
| `log_wal_backlog_bytes` | WAL 积压 | > 256MB |
| `log_suppressed_total{event_name}` | 被限流合并数 | 突增 |

`/healthz` 增加 `log_pipeline: ok|degraded|failing`（队列、WAL、最近刷盘时间三者综合）。

### 4.9 日志管线怎么测

| 测什么 | 怎么测 |
| --- | --- |
| 丢弃策略 | `Emitter` 注入容量 1，灌 100 条，断言 `dropped_total==99` |
| 审计不丢 | Writer 注入"ClickHouse 失败"，断言全部落 WAL；恢复后重放条数一致 |
| 脱敏 | 表驱动：`password/token/cookie` 一律 `***`；body 限量 1MB、输出 ≤16KB；header 不采集 |
| 截断 | 20KB body → 输出 ≤16KB 且 `truncated=true` |
| 采样 | 令牌桶 100/s，灌 1000 条，断言保留量在 [90,110] |
| 序列化 | golden：给定事件 → 固定 JSON 行 |

`log.Emitter` 是接口，测试用 `testkit/log.Recorder` 捕获内存事件，断言**类型化字段**而不是字符串；纯逻辑（encode/redact/truncate/sample）零外部依赖可 `t.Parallel`；集成测试用 `//go:build integration` + testcontainers 起 ClickHouse 验证批量写入与 TTL。

---

## 五、v1 收尾盘点的产品缺陷（7 个）

> 时间：2026-10-26。方法：不靠回忆，跑工具 + 读代码（`deadcode`、前端符号可达性分析、契约 JSON 统计、`go.mod` ↔ `import` 对账）。
> 结论：**架构层面的活基本做完了，剩下的是"类型说真话之后暴露出来的产品缺陷"。** 7 个中 6 个已修、1 个有意撤 UI。

### 5.1 缺陷清单（症状 → 病根 → 处理）

| # | 位置 | 症状 | 病根 | 处理 |
| --- | --- | --- | --- | --- |
| 1 | 登录日志 / 用户列表的 `addr` | 恒为「未知地址」 | `deadcode` 查出 `core.InitIPDB` **零调用**，`ipv4Searcher/ipv6Searcher` 恒为 nil；仓库里的 `resources/ipbase/ip2region_v4.xdb`(11MB) + `_v6.xdb`(36MB) **从来没被打开过** | 在 `app/bootstrap.go` 调 `InitIPDB()`，失败只 `logger.Warnf`（`addr` 只是展示字段，不该因资源缺失让服务起不来）；路径向上探 3 层（容器 CWD 是 `/app`，`go test ./core/` 是 `backend/core`） |
| 2 | 首页「精选文章」区块 | **整块坏掉**：链接指向 `/article/undefined`，标题/封面全空 | `GET /articles/top` 返回的是**文章 ID 字符串数组**（`ListTops` → `[]domain.ArticleID`），前端却按 `ArticleTopItem[]`（对象）渲染 | `article/domain/top` 新增 `Card` 投影 + `Repository.ListCards`（置顶表 → 文章表排除软删 → 用户昵称 → 分类标题，一次 join）；`ListTops` 保留给"设置置顶"用；HTTP 改 `kernel.Envelope[kernel.ListData[ArticleTopCardResponse]]`；前端模板 `user_nickname` → `author_nickname`。golden `top_list.json` 从 `["1019",...]` 变成三张卡片 |
| 3 | 作者卡「点赞/收藏/评论」 | 三个数字**恒为 0** | 从未被建模：`user_stat_models` 只有 `view_count / fans_count / follow_count / article_visited_count / article_count` 五列，且**零 DDL 约束禁止加列** | **有意撤 UI**：`ProfileHeroCard` 只留「阅读」，`UserBaseInfo` 删掉三个伪造字段。理由：为三个虚荣指标引入一条跨模块读 + 一次 SUM 不划算。将来要恢复，先在后端 `user_stat` 建模 |
| 4 | admin 用户列表「角色」列 | **恒为空**（编辑弹窗里能改，列表看不到）→ 补上后立刻暴露**提权方向的 bug** | `AdminUserItemResponse` 没有 `role` / `status`；补上后发现**前后端角色数值是反的**（后端 `RoleAdmin=1 / RoleUser=2 / RoleGuest=3`；前端 `{普通用户 value:1} / {管理员 value:2}`），选「普通用户」提交 `role:1` 会把该用户**设为管理员** | 后端补字段（`domain.User` 本来就有，只是没带出边界）；新建 `frontend_admin/app/utils/roles.ts` 收口 `ROLE_ADMIN/ROLE_USER/ROLE_GUEST` 与 label/options，**文件头写明数值必须与 `backend/internal/user/domain/user.go` 逐字一致**；默认角色从 `1` 改成 `ROLE_USER`（缺省按最小权限）；`test/unit/roles.test.ts` 断言**具体数字** |
| 5 | admin「恢复已隐藏文章」 | 判断为"流程走不到"——**该判断本身是错的** | 真因只是**契约**：`myblogx_internal_search_domain.VisibilityStatus` 在搜索模块只声明了 `"visible"` 一个常量，swag 把枚举印成单值，前端只好用 `Omit<..., "visibility_status">` 手工放宽 | 把搜索模块的单值枚举**别名**到 `kernel/enum.ArticleVisibilityStatus`（三值），不再重新声明一份词表；契约枚举变成 `"visible" \| "user_hidden" \| "admin_hidden"`，前端 `Omit` 删掉 |
| 6 | admin 媒体库「下一页」 | 按钮**永久 disabled** | 后端 `res.OkWithList(items, total, c)` 只返回 `{list, count}`，**没有 `has_more`**；前端手写 `request<{ list?: ...; count?: number; has_more?: boolean }>`，编译器一句话不说 | 后端**保持 `{list, count}`**（契约对、前端错）；`ImageListItem` 改成契约派生；新增 `app/utils/pagination.ts` 的 `hasNextPage` + 4 个用例，页面不再自己算 |
| 7 | 管理端首页「访问量」 | 恒为 0 | `redis_site.SetFlow` **零调用**；`git log -S` 定位到 **S4.7 site 迁移**时漏搬了一行 | 在新 `PublicInfo` 的 `case "site"` 恢复调用（`Handler.onPublicSiteView`，装配层注入，nil 时跳过）+ 回归测试 `TestPublicSiteRecordsVisit` |

**第 2、6 项是同一个病根**（后端返回的形状与前端手写的形状不一致），第 2 项最严重——它不是"少个统计数字"，是**首页一个区块整体不可用**。

### 5.2 同类但非产品缺陷的两条

| 现象 | 病根 | 处理 |
| --- | --- | --- |
| 契约里 **6 个端点运行时 404**（`GET\|POST\|PUT /users`、`GET /users/{id}`、`PUT /users/settings`、`PUT /users/{id}/status`） | `user/adapter/http/handler.go` + `dto.go` 里 6 个方法（`PublicProfile`/`Update`/`UpdateSettings`/`List`/`Create`/`ChangeStatus`）零调用，路由实际走 `compat.go`，但它们**带着 swag 注解**——编译、vet、archcheck、覆盖率、契约生成没有一环能发现"注解写在一份死代码上" | 删 6 个方法 + `dto.go` 整文件；契约 paths 94→**90**、operations 123→**117**、schemas 208→**196** |
| admin 的 `UserDetailResponseData` 接错了对象 | 删掉那批死代码后 `domain.User` 从契约里消失，它立刻编译不过——`/users/detail` 返回的是 `app.DetailView`，**一直接错**，此前能编译通过纯粹因为 `domain.User` 恰好被幽灵端点的注解留在契约里 | 改为 `RequiredSchema<"myblogx_internal_user_app.DetailView">` |

### 5.3 教训（写给后来人）

1. **别用契约反推运行时行为**。§5.1-5 就是拿着"枚举只有 visible"去断定"恢复按钮走不到"，而真因是枚举本身写漏了。**判断"某条流程走不到"之前，必须读一遍实际调用链，不能只看类型。**
2. **补上一个字段，可能立刻暴露另一个 bug**。补 `role` 之后才看见前后端枚举值是反的；删死代码之后才看见 `UserDetailResponseData` 接错了对象。
3. **测试夹具缺数据时，断言"非零"才有意义**。`author_info` 的 golden 里三项统计都是 0，只断言"字段存在"的话这条缺陷可以再藏一年。`TestAuthorProfileIncludesStats` 必须**插入非零统计**再断言相等。
4. **回归测试的判据要能区分"没接上"和"真的是 0/兜底值"**。旧测试 `TestGetIpAddrBranches` 只断言非空，而 `GetAddr` 有兜底分支，库没加载时也会返回「未知地址」。新测试 `TestInitIPDBLoadsRealLibrary` 先 `InitIPDB()`，再断言公网 IP **不是**「未知地址」。
5. **`InsertUser` 会顺带建零值 `user_stat`**（GORM 关联），再 `Create` 会主键冲突，要 upsert（`testkit/schema.InsertUserStat`）。
6. **"注解写在死代码上"是契约类缺陷的新变种**。之前只想到"注解写错形状"，没想到"注解挂在没人调的方法上"——后者只能靠**路由表对账**，因为注解和路由是两份独立的事实。
7. **匿名响应类型等于契约失明**。后端不返回的字段，只要前端手写一个可选属性，编译器就一句话不说。
8. **`deadcode` 的输出要按文件看**，别只数条数：54 条生产条目里有 3 条是"整套 handler 没人调"这种高危项，其余是零散 helper。
9. **别用 role 分支推断功能缺失**。`main.go` 的 `role="all"` 只调 `WireWorker`，看着像"cron 不跑"；读下去才发现 `WireWorker = River + ImageRef + Cron`。
10. **一条 `.gitignore` 写错 = 同时关掉两道门禁，且两道都不吭声**。`frontend_admin/.gitignore` 里一行裸的 `logs` 匹配**任意层级**同名目录，于是 `app/pages/logs/` 被静默忽略：本地能跑、`git status` 什么都不提示、**CI 和别人的机器上就是 404**；而且 ESLint v9 默认拿 `.gitignore` 当忽略源，这两个页面连 lint 都没检查过。改成 `/logs` 的同一时刻 lint 就报出一个真实错误。
11. **"入口存在、目标不存在"有三种形态**：读端活写端死、菜单指向空页面、以及上面这种连版本控制都不提示的。前两种至少能在运行时或对账里看到。

---

## 六、遗留项与 v2 待办

### 6.1 明确不做（有意取舍，不是遗漏）

| 项 | 理由 |
| --- | --- |
| 给响应 struct 加 `binding:"required"` 提升 required 比例 | 949 个字段的人工判断（当前必填 44 / 4.6%），而 `RequiredSchema` 已在消费侧解决；**注解写错比没有契约更糟** |
| 契约里 192 处内联 properties 具名化 | 纯体力活，收益只是前端少写十几个手写类型 |
| `deadcode` 进硬门禁 | 输出需要人工区分"死代码"与"未接线的功能"，适合做**定期审查工具** |
| `SLICE` 级别的整体重写 | 剩余问题都在"某个字段没接上"这一级，架构分层本身没有暴露新问题 |
| 让"没权限"可辨识 | `AdminMiddleware` 仍返回"权限错误"（与校验失败同码、HTTP 200）。代价：降级的管理员不会被前端统一踢出。有意不做——不让用户从错误码看出"普通账号能登录" |
| ES 别名 + 双索引重建 | 重建空窗在这个量级可接受 → 索引大到"重建要十分钟"时再考虑 |
| MySQL ↔ ES 对账任务 | 成本大于收益 → 出现"文档悄悄丢了"的实际故障时再考虑 |
| 展示字段去冗余（作者名读时回填）/ 扇出改异步队列 | 扇出成本 = 低频 × 有界（几十篇），秒级 → 单作者上万篇 / 批量改名 / 投影耗时 > 1s 时再考虑 |

### 6.2 未修 / 未决（v2 起步时可先看这几条）

| 项 | 状态 |
| --- | --- |
| 从库 `read_only` 仍为 0 | `slave/init.sh` 里 `SET GLOBAL read_only = 1` 是在**临时服务器**阶段执行的，正式启动后回到默认值；要持久必须写进 `my.cnf`。与 MySQL 版本无关（5.7 同样如此） |
| `default-authentication-plugin = mysql_native_password` | 有意保留（Go 驱动非 TLS 连 `caching_sha2_password` 需要 `allowPublicKeyRetrieval`）；**8.4 移除后必须改 TLS 或连接参数** |
| `deadcode` 34 条 | 全部是 `testkit` 辅助函数，生产代码 0 条 |
| 建议但未做的门禁：读者站"菜单 path ↔ 页面文件"对账 | admin 端已用 `app/utils/menu.ts` + vitest 双向对账覆盖；读者站未做 |
| admin 测试 | 39 个用例，仍集中在 `app/utils/**`；约 50 个页面（用户管理、文章审核、日志查询、媒体库）中"错了代价大"的（auth / 审核）尚未铺开 |

### 6.3 指标终值（v1 收尾快照）

| 指标 | 起点 | v1 终值 |
| --- | --- | --- |
| CI job | 0（起点无 CI） | **9** |
| archcheck 规则 / 违规基线 | 0 条 / — | **12 条（R1–R12）/ 0** |
| 门禁自检覆盖 | 4 道 | **11 道**（11/11 正确报红） |
| `deadcode`（不带 `-test`） | — | **595**（生产 0） |
| 契约 paths / operations / schemas | — | **92 / 119 / 203**（归档时实测） |
| 契约幽灵端点 | 6 | **0** |
| 前端 `api.ts` 两端合计 | 1020 行 | **407 行** |
| 服务层匿名响应类型 / admin 手写响应类型 | 2 / 1 | **0 / 0** |
| admin 用例 | 0 | **39** |
| 单文件超 800 行 | — | 最大 `article/[id].vue` **715 行** |
| `TODO` / `FIXME`、`as any` / `@ts-ignore` | — | 全仓 **0** |

---

## 【归档备注】仅具历史价值、v2 不需要再看

- 起点诊断的"数量不缺、结构错位"统计（117 测试文件 / 2,357 断言 / 3 处 `t.Parallel` / `repository/` 449 行 0 测试）——那是重构前的事实，已全部被替换。
- 日志现状六病灶 L1–L6（反射序列化、同步写文件、雪花 ID、`SetReportCaller(true)`、fluent-bit 采集侧）与"2MB 响应 → 8 档 leaf 截断"的爆炸路径推演 —— 对应的旧代码（`service/log_service`、`utils/logsafe`、`init_logrus.go` 两个 Hook）已在 L3 阶段拆除，留着只是解释"为什么当初这么定"。
- archcheck 违规基线的完整演进链与"上线时必须报出 150+ 处真实违规"这个一次性判断标准 —— v2 只需知道**当前基线是 0、只准减不准增**。
- `frontend_admin` 覆盖率从 0 到 23/27/35/39 个用例的爬坡过程、Kafka 清零的具体文件清单、MySQL 5.7→8.0 的三处废弃参数与复制语法改动细节。
- `EmitReplayEvent` 与死信重投的方案选择过程——结论已落地为 ADR-008，见 [ADR-008](../adr/0008-dead-letter-replay.md)。
- 06 号文档里的 R1–R10 是**早期草案编号**，与实现不一致（本文已按 `rules.go` 实际定义纠正）；"CI 三档流水线（fast/full/nightly）"是未落地的方案，实际仓库只有单档 9 job。
