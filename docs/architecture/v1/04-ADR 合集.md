# 04 · ADR 合集（v1 归档）

> 归档自 `docs/architecture/adr/` 下 9 份独立 ADR。合并口径：保留每条 ADR 的**决策骨架 + 关键参数 + 当前生效约束**；删除被否决方案的完整论证（压缩为一句理由）；删除与其他 v1 文档重复的长篇论述。
>
> **交叉索引（本节不展开的部分）**：
> - ADR-003 / ADR-007 → 详述见 [03-契约](./03-契约.md)
> - ADR-004 / ADR-005 / ADR-006 → 详述见 [02-治理与质量](./02-治理与质量.md)
> - ADR-001 / ADR-002 → 与 [01-架构总纲](./01-架构总纲.md) 重叠，本节只保留骨架 + 总纲未写的部分（如为什么不上 wire/dig）
> - **ADR-008 / ADR-009 → 仅本文收录，完整保留**

## 总览

| ADR | 标题 | 一句话结论 | 详述位置 |
| --- | --- | --- | --- |
| 001 | 用能力接口取代 `Deps` 结构体装箱 | 窄接口注入，`Deps` 全部删除 | 01-架构总纲 |
| 002 | 垂直模块切分与合成根 | 模块内 `domain/app/adapter`，合成根唯一连线 | 01-架构总纲 |
| 003 | 前后端契约以代码生成为单一来源 | Go struct → openapi → TS 生成 | 03-契约 |
| 004 | 架构守卫从符号匹配改为可执行 AST 规则 | `tools/archcheck`，AST + 类型信息 | 02-治理与质量 |
| 005 | 日志改为有界异步管道 + 类型化事件 | 有界队列 + 批量直写 ClickHouse + WAL | 02-治理与质量 |
| 006 | 测试体系改为金字塔 + testkit + 有状态 fake | 分层阈值，禁用 SQLite 冒充 MySQL | 02-治理与质量 |
| 007 | ID 类型统一为 string 底层（类型即真相） | `ctype.ID` 底层改 string，R12 守卫 | 03-契约 |
| 008 | 死信重投（DLQ Replay）闭环 | 人工触发、幂等执行器、`unsupported ≠ failed` | **本文** |
| 009 | 删除"通用规则"同步路径 | ES 同步只留文章搜索投影 | **本文** |

> 9 条全部**已交付**。v2 硬约束汇总见 [01-架构总纲](./01-架构总纲.md) §六，本文保留的是每条约束的**出处与理由**。

---

## 一、架构与装配

### ADR-001 用能力接口取代 `Deps` 结构体装箱

**问题**：一轮"去全局化"把 `global.DB` 换成 `Deps` 结构体传递，结果造出 32 个 `type Deps struct`（30 个含 `*gorm.DB`）、6 层传递链（`Infra → apideps.Deps → api.Deps → article_api.Deps → category.Deps → view_history.Deps`）、`api.New` 逐字段展开 15 个子 Deps。它不是 DI，而是**看起来像 DI 的 Service Locator**。

**决策**：
1. handler/service 构造函数只接收**窄接口**（每个 1-3 个方法），结构体 3-5 个字段；
2. 端口定义在**使用方**包内（`internal/article/app/port.go`），不是实现方；
3. `Deps` 结构体全部删除，archcheck 禁止新增；
4. 跨模块调用由**消费方**定义自己需要的接口（如 `comment` 定义 `ArticleGate`），不 import 别的模块；
5. 按模块渐进迁移，任何时刻代码都能编译。

**理由**：`Deps` 比裸 global 更糟——依赖"看起来可见"（有签名）但实际仍任取 DB，测试要构造 17 字段结构体更难，且**让人误以为重构已完成**；能力接口则真正受限（`DiggCounter` 只有一个 `Count`，调不到 `Delete`），拿不到 DB 连接想写 SQL 也写不了。
- 否决「保留 Deps 只瘦身」：只要里面还有 `*gorm.DB`，任何包都能查任何表，属于"换个尺寸的创可贴"；
- 否决「引入 wire / dig」：wire 让启动流程不可读、出错提示晦涩；dig 把编译期错误推到启动期；本项目 12 个模块，手写装配约 150 行足够。

**代价 / 不做什么**：构造函数参数变长（4-5 个）；接口数增加到约 50 个（12 模块 × 约 4 端口）；领域实体与持久化模型分离，每模块约 200 行 mapper 样板；迁移期接口与 `Deps` 双轨并存。

**当前生效的约束**：
1. 禁止新增 `type Deps struct`；
2. `*gorm.DB` 只能出现在 `*/adapter/` 与 `platform/mysql/`，其余层出现数为 0；
3. 端口接口定义在使用方包内，实现方不得反向定义；
4. 模块之间不得互相 import，跨模块依赖一律通过消费方窄接口 + 合成根连线。

---

### ADR-002 垂直模块切分与合成根

**问题**：现状是水平切分（`api/` 170 包、`service/` 170 包、`repository/` 8 包、`models/` 48 文件），业务逻辑全压在 handler 上（`article_api` 6,012 行 vs `article_service` 99 行、`comment_service` 11 行），65 个 `enter.go` 共 1,327 行全是包装。**"每层都均匀有内容"这个隐含假设已被证伪。**

**决策**：
1. 采用垂直切分：每个业务模块自包含 `domain / app / adapter` 三段；
2. `adapter` 合并 http 与 store（不拆两个包），代码量用**文件粒度**控制：`http.go` <500 行、`mysql.go` <800 行；
3. 合成根唯一：`cmd/server/main.go` 是唯一允许 import 具体实现的地方，`main.go` 之外不得 `new` 具体实现；
4. 每个模块的 `adapter/wire.go` 提供一个 `wireXxx(infra)` 函数；无包级可变状态；
5. 跨模块协作三条规则：

| 需求 | 做法 | 禁止 |
| --- | --- | --- |
| 读别的模块数据 | 消费方定义窄接口（`ArticleGate`） | import 别的模块 domain |
| 被通知 | 发领域事件（outbox） | 直接调别的模块 service |
| 共享值对象 | 放 `internal/kernel` | 把业务实体放 kernel |

**理由**：改动局部化（一个功能在 `internal/article/` 内）、可按模块独立迁移上线、包数从 300+ 降到 36（12 模块 × 3）。http 与 store 合并是因为两者生命周期一致（都由合成根构造、都是"与外界打交道"），分开只增加 import 不增加约束。
- 否决「保持水平切分补空」：强制制造空壳，`comment_service` 11 行就是榜样；
- 否决「水平切分 + `internal/` 强制边界」：改善了症状没解决病因，5 个模块没有 service 会让边界规则形式化；
- 否决「完整 DDD（限界上下文 + 防腐层 + 领域事件）」：83k 行项目上过度设计。

**代价 / 不做什么**：目录认知成本；跨模块调用变啰嗦（要定义接口、在合成根接线）；`article` 数据被 `comment`/`search`/`ai` 使用时需各自定义只读接口；迁移期旧 `api/ service/` 与新 `internal/` 并存。

**当前生效的约束**：
1. 依赖单向 `adapter → app → domain`，编译器可校验；
2. 模块间零 import；
3. `internal/kernel` 只放 ID、分页、错误，不得放业务实体；
4. 每个模块迁移前录 golden file，保证行为不变。

---

## 二、契约与类型

### ADR-003 前后端契约以代码生成为单一来源

**问题**：`openapi/blogx.openapi.json` 是 14,462 行的手写文档，全仓引用数为 **0**；前端 1,005 行 interface 手写（`frontend/app/types/api.ts` 736 行 80 个 + admin 269 行 28 个）。118 个后端 operation ↔ 108 个前端 interface 零机械校验，字段改名只在运行时暴露为 `undefined` 或 500。

**决策**：Go handler + struct + swag 注解 → `backend/docs/swagger.json` → 各前端 `openapi-typescript` 生成 `app/types/openapi.d.ts`；S2 只做类型生成不动页面，S5 删除全部手写 API 类型；CI 校验"生成结果与提交一致"。

**理由**：漂移从"运行时 500"变成"编译期报错"，且让 openapi 从文档负债变成可做 Mock / SDK / 文档站的资产。
- 否决「tygo 直接出 TS」：只覆盖 struct，无法表达路由、状态码、参数位置；
- 否决「tRPC / Connect」：要改 Gin 路由层与前端调用方式，迁移成本远超收益。

**代价 / 不做什么**：118 个 operation 补注解是一次性工作量；生成产物入库带来 PR diff 噪音；swag 对泛型响应支持有限。**不追求消灭所有手写类型**——纯前端状态类型（`ThemeMode`、UI props）与后端未暴露的内部结构仍手写。

**当前生效的约束**：
1. **凡是跨网络传输的类型，都必须来自生成**；
2. 生成产物放两端 `app/types/openapi.d.ts`，与手写类型分文件；
3. 前端禁止从 `openapi.d.ts` 之外 import API 类型；
4. CI 三处一致性校验，防止有人手改产物。

---

### ADR-007 ID 类型统一为 string 底层（类型即真相）

**问题**：后端 `type ID uint64` 却在 `MarshalJSON` 里输出字符串，swag 只读 Go 类型声明（`uint64`），导致契约里 100 处 ID 印成 `number` 而实际传输是 `"372842048379686912"`（18 位，超 JS 安全整数 2^53-1）。只能靠 `fix-openapi-id-types.mjs` 做 138 处 integer→string 后处理，且全仓有 **10 套**并存的 ID 类型。

**决策**：
1. `ctype.ID` 底层改 `string`，提供 `Uint64()` / `IsZero()` / `Uint64OrZero()` / `ParseID()` / `Value()` / `Scan()`（入库出库仍转 int64，**零 DDL**）；
2. 其余 9 个 domain ID 类型收敛为 `ctype.ID` 的别名或同构 string 类型；
3. 89 处裸 `uint64` ID 字段收编进 `ctype.ID`；
4. 237 处非 ID 整型（`Page`/`Limit`/`DiggCount`/`Size`/`Score`）**一个都不动**；
5. 追加：swag 换 v2 + `--v3.1`，流水线 4 步 → 3 步 → **2 步**。

**理由**：类型即真相——隐式转换消失后 "Go 类型" 与 "JSON 形态" 永远是同一个东西，swag 不可能印错，`.swaggo` 与后处理脚本可删。
- 否决「`.swaggo` 全局覆盖」：只买"这次对了"，管不到裸 `uint64`；
- 否决「逐字段 `swaggertype:"string"`」：那些字段真的发数字，标注等于让契约撒谎；
- 否决「换 huma / go-fuego」：123 个接口全部重来。

**代价 / 不做什么**：实际改动约 490 处（预估 240），3-4 轮才收敛；**6 处 ID 大小比较会静默变语义**（字符串下 `"10" < "9"` 成立），必须改成显式 `a.ID.Uint64() < b.ID.Uint64()`；26 处按理由豁免（log 6 / testkit schema 17 / `ServerID` 4 / `initializedID` 1）；golden 需重生成。

**当前生效的约束**：
1. archcheck **R12 `id-must-be-ctype`**（`backend/tools/archcheck/rules_id.go`）：字段名以 `ID`/`IDs`/`Id`/`Ids` 结尾且类型为原生整型（含 `*` / `[]`）→ 违规，**基线 0**；
2. 验证口径必须与门禁一致（`go vet ./...` 不编译 `//go:build integration` 文件）；
3. `docs/` 只保留 `swagger.json` 一个产物，`docs.go` / `swagger.yaml` 已删。

---

## 三、治理与质量

### ADR-004 架构守卫从符号匹配改为可执行的 AST 规则

**问题**：原有守卫（`scripts/arch_guard.ps1` 110 行 9 条 + `test/architecture/` 260 行 6 个测试）检查的符号 `appctx` / `global` / `MustFromGin` / `DepsFromGin` / `mustApp` **全部 0 处引用，13 条规则 + 6 个测试全部永真通过**；唯一有效的「api 禁止 import repository」也被 `type QueryService = repo.QueryService` 别名 re-export 绕过。真正的问题（32 个 `Deps`、handler 直接查库 61 文件 194 处、65 个 `enter.go`）无人看守。

**决策**：采用 AST + 类型信息方案，建 `tools/archcheck/`（`main.go` / `load.go` / `rules.go` / `rules_test.go` / `report.go`），删除 ps1 与 `test/architecture/` 两套重复实现。首批 8 条规则：

| # | 规则 | 判定方式 | 存量 |
| --- | --- | --- | --- |
| R1 | 导出结构体不得含 `*gorm.DB` | 类型检查 | 32 |
| R2 | `adapter`/`platform/mysql` 之外不得出现 `*gorm.DB` | 类型检查 | 61 文件 |
| R3 | 禁止 `type Deps struct` | 类型检查 | 32 |
| R4 | `domain` 不得 import gin/gorm/redis/es | import 图 | 迁移中 |
| R5 | 模块之间不得互相 import | import 图 | 迁移中 |
| R6 | 单文件 ≤ 800 行 | 行数 | 4 |
| R7 | `enter.go` ≤ 50 行 | 行数 | 65 |
| R8 | `adapter` 之外不得调用 `Begin`/`Commit` | 调用图 | 12 |

> 现状是 R1–R12，编号与含义已演进，以 `backend/tools/archcheck/rules.go` 为准，见 [02-治理与质量](./02-治理与质量.md) §二。

**理由**：类型检查看的是实际类型而非字面量，别名绕不过；跨平台 `go run` 可进 CI；输出 `文件:行号` 可定位。
- 否决「修 PowerShell」：仍是正则匹配符号名，且 Linux CI 跑不了；
- 否决「只用 Go 测试」：载体对了方法错，仍是 `regexp.MatchString`；
- 否决「depguard / forbidigo」：只能约束 import 路径，不能约束结构体字段类型，覆盖约 30%。

**代价 / 不做什么**：需写约 400 行工具代码；`packages.Load` 启动开销 2-5s（可缓存）；存量 150+ 处违规需跨阶段清理；规则会误报需逃生口。

**当前生效的约束**：
1. 双轨运行：`--all --warn` 输出债务基线不阻断，`--changed-since=origin/main --error` 只拦新增，逐模块清零后转全量 `--error`；
2. **规则自检是地基**：`TestEveryRuleDetectsViolation`（对人造违规必须报错）+ `TestEveryRulePassesCleanCode`（干净代码不得误报），两条测试缺一不可；
3. 逃生口 `//nolint:archcheck R2`；
4. 规则数量控制在 8-12 条，每加一条都要问"它会误伤什么"。

---

### ADR-005 日志改为有界异步管道 + 类型化事件

**问题**：日志链路五个病灶——`utils/logsafe/sanitize.go:52` 对任意 `any` 反射遍历无深度/大小上限；`write_action_audit.go:49` 的 `RequestBody any` / `ResponseBody any` 允许整对象进入；`write_sink.go:25` 在请求路径**同步** `json.Marshal` + 全局锁 `file.Write`；每条日志调 `NextSnowflakeID()`；`fluent-bit.conf:12` 5 个 tail、`Skip_Long_Lines On`（丢）、`Read_from_Head On`（重）。结果：并发一高内存尖刺、RT 抖动，ClickHouse 慢时丢日志且不可见。

**决策**：自建**类型化 Emitter + 有界队列 + 批量直写 ClickHouse + WAL**；`Emit` O(1)；单写协程批量；失败落 WAL 重放；去掉 fluent-bit 的 app tail（它继续采容器 stdout/stderr 与基础设施日志）。

**理由**：六条硬约束全满足，审计可做到不丢，日志管道自身可观测。关键取舍三条：
1. **runtime 可丢，audit/login 不可丢**——队列满时 runtime 丢弃并计数，audit 落 WAL（"业务稳定优先"的显式选择）；
2. **不做全量调用栈**——生产默认关闭 `ReportCaller`，仅 error 采样抓栈；
3. **去掉 fluent-bit 的业务日志采集**——消除"丢/重"单点。

否决「保留 logrus 只修截断」（同步写在请求路径没解决）；否决「logrus + 异步 Hook」（半成品，`any` 与 fluent-bit 问题仍在）；OpenTelemetry Collector 保留为**可选导出器**（`log.Emitter` 面向"记录"而非具体后端），不在本次强制。

**代价 / 不做什么**：新增约 1200 行基础设施与一个 WAL 目录；ClickHouse 批量大小/超时/退避需调参；一次性迁移 5 类日志写入调用点。

**当前生效的约束**：
1. 日志字段不得出现 `any` / 结构体（archcheck `--rule=log-no-any`）；
2. `internal/platform/log/emit.go` 请求路径内不得出现 `json.Marshal` / `os.OpenFile`；
3. WAL 积压 >**256MB** 告警；ClickHouse TTL：runtime **30d** / audit **180d**；
4. 有界性验收：1 万 QPS 压测 1 分钟 RSS 平稳且 `log_dropped_total` 有值；断 ClickHouse 写 1000 条 audit，恢复后 count == 1000 且无重复。

---

### ADR-006 测试体系改为金字塔 + testkit + 有状态 fake

**问题**：117 个 Go 测试文件、2,357 处断言，但 `t.Parallel()` 只有 **3** 处；测试普遍依赖 `testutil.InitGlobals` / `Deps` / 包级全局，无法并行无法隔离；集成测试用 SQLite 冒充 MySQL（`ON CONFLICT` / `clause.Locking` / JSON 函数行为不同）→ **假安全**；`repository/` 449 行核心 SQL **0 测试**，`frontend_admin` 0 测试；无 CI，断言从不自动执行。

**决策**：金字塔 + testkit + 有状态 fake + golden/契约——domain 纯单测穷举规则；app 用 fake 端口测编排；adapter 用真库测 SQL；HTTP 测契约；E2E 只保主链路。

**理由**：业务规则散在 handler 的 `if` 里，没有 domain 层就写不出不变量测试，没有端口注入就 fake 不掉，因此测试体系必须与架构重构绑定推进。四条关键取舍：
1. **有状态 fake 而非 gomock**——"重复插入冲突""事务回滚""Outbox 失败"需要状态，gomock 表达不了；
2. **SQLite 不再冒充 MySQL**——用 testcontainers 起真 MySQL，`//go:build integration` 隔离；
3. **覆盖率分层设阈值**——domain ≥90%、app ≥85%，**不设整体数字**；
4. **golden + 契约双保险**——golden 守"行为没漂"，契约守"前后端没漂"。

否决「在现有风格上补集成测试」（每个"状态 × 角色"组合都要起一次环境，必然退化为只测 happy path）；否决「只做 E2E」（慢、脆、无法穷举规则组合）。

**代价 / 不做什么**：一次性写 testkit 约 800 行与各模块 fake；集成测试需要 Docker；golden file 带来 PR diff 噪音需人工 review。

**当前生效的约束**：
1. `go test -race -parallel 8 ./internal/...` 常态化；
2. `go test ./internal/*/domain/...` 必须 < 2s（domain 零外部依赖）；
3. 门禁自检：CI 必须能因"注入的失败"而红，否则 CI 无效；
4. flaky 测试 quarantine，**不允许长期 retry**；
5. mutation testing 先覆盖 domain 关键规则，逐步扩大。

---

## 四、CDC 与同步（仅本文收录）

### ADR-008 死信重投（DLQ Replay）——让"同步失败"有闭环

**问题**：三个 CDC 子链路（`es_projection` / `outbox_relay` / `image_ref_river`）在重试耗尽后都会调 `deadletter.SaveBatch` 写 `cdc_dead_letter`，状态一律 `pending`；但**没有任何代码把状态改成别的值、也没有任何代码去读它重做一次**——死信表是只进不出的黑匣子，出问题只能人工进 MySQL 翻、手动补数据。展示端更误导：`replay_event_logs` 的读端（analytics `/logs/replay`）与写端 `writer.EmitReplayEvent` 都写好了，但**没有任何地方调用它**，表永远空着。一句话：**"失败留证据"做了，"照证据补救"没做。**

**决策**：建一套最小但完整的死信重投闭环，分四层：

```
管理端页面（列表 + 勾选重投 + 回放日志）
      ↓
POST /logs/dead-letter/replay        GET /logs/dead-letter
      ↓ 端口 domain.DeadLetterReplayer
platform/cdc/dlq.Service  ── 编排：取记录 → 分派执行器 → 更新状态 → 写回放日志
      ├── es_projection   → 按死信里的行主键重算投影文档（幂等）
      ├── image_ref_river → 删除类：重新清理该对象的图片引用
      └── outbox_relay    → 把 outbox 行从 dead 重置为 pending（交给中继自己重投）
```

**1. 状态机只用已有列，零 DDL**：

```
pending ──重投成功──> resolved（终态，不再参与重投）
pending ──重投失败──> pending（retry_count+1、error_code/error_msg 覆盖为本次失败）
```

**不引入 `replaying` 中间态**：重投是人工、低频、单实例的管理操作，中间态会带来"进程崩溃留下僵尸 replaying"的恢复逻辑，而它防的问题（两人同时点同一个按钮）最坏结果是重投两次——两个执行器都是幂等的（ES 按文档 id index/delete；outbox 重置是纯状态写），重复执行无害。**用"幂等执行器"换掉"分布式互斥"，代价低得多。**

**2. 支持范围按 payload 自洽性划定**：

| stream | 死信 payload 里有什么 | 重投怎么做 | 支持 |
| --- | --- | --- | --- |
| `outbox_relay` | `{payload}` + `CdcJobID=outbox:<id>` | 把该 outbox 行 `dead → pending`、`attempts=0`，由中继按自己的退避重投 | ✅ |
| `es_projection` | `{schema,table,action,rowCount,ids}` | 按 `ids` 重算投影（`SyncArticleSearchProjection`），只动受影响的文档 | ✅ |
| `image_ref_river` · 删除 | `{owner_id, ref_type}` | 直接重新清理该对象的图片引用（payload 自洽） | ✅ |
| `image_ref_river` · 新增/更新 | `{table,row}` / `{table,before,after}` | `row` 是 `[]any`，JSON 往返后数字变 float64、时间变字符串，重建行快照会**静默算错**候选图 | ❌ 返回 `unsupported` + "重新保存该业务对象" |

**`es_projection` 的 payload 必须带 `ids`。** 没有主键就只能全量重建索引（几分钟、期间新旧文档混杂）；带上主键后，重投是"重算这几篇"而不是"重建全库"。（历史 stream `es_river` 的 payload 是完整 ES bulk body，可原样重发，但该路径已随 ADR-009 删除；死信页保留该 stream 名仅为历史记录可读。）

**3. "不支持"与"失败"必须是两种结果**——`deadletter.ErrUnsupported` 这个哨兵就是为这条边界存在的：

| 执行器返回 | 结果 | 状态 / retry_count | 界面 |
| --- | --- | --- | --- |
| `nil` | `success` | `resolved` | 绿色"成功" |
| `ErrSkipped` | `skipped` | 不动 | 灰色"跳过" |
| `ErrUnsupported` | `unsupported` | **不动** | 橙色"不支持重投" |
| 其它 error | `failed` | 仍 `pending`，`retry_count+1` | 红色"失败" |

不支持的流不是"以后再说"，而是**当场告诉操作员该改用什么办法**：`POST /replay` 返回 `unsupported` + 一句人话，**不改数据库、不涨 `retry_count`**——"不支持"和"这次失败"是两件事，前者再点一百次也是同一个结果，记成失败只会诱导人反复点。

**4. 回放日志：把闲置的写端接上**——每次重投尝试（成功/失败/跳过/不支持）都 `writer.EmitReplayEvent` 写一条到 ClickHouse `replay_event_logs`，字段取自死信记录：`cdc_job_id/stream/source_table/action/target_key/retry_count/result`。收益：管理端「回放日志」页第一次真正有数据；同一条死信的"失败→重投→再失败→再重投→成功"有完整时间线（`cdc_job_id` 可串）；"点了没反应"也能查（跳过与不支持同样留痕）。

**5. 幂等与安全边界**：
- 已 `resolved` 的记录不再重投，返回 `skipped`（避免重复打 ES）；
- 更新状态用条件更新（`WHERE id=? AND status<>'resolved'`），不引入锁；
- **不自动重投**——死信可能是"代码 bug 导致必然失败"，自动重投只会把 ES 打热；
- 重投不修改死信里的 payload，失败证据保持原样便于事后比对；
- 删除类（`image_ref_river`）重投是"清掉该对象**当前**的所有引用"，不校验对象是否还在。**先重新保存过对象、再点旧死信，会把刚重建的引用一起删掉**——幂等 ≠ 无害，这条只通过对操作口径的约束（重建过就别点旧死信）来规避。

**理由（变好的）**：同步失败处置从"进 MySQL 手改"变成"页面上勾选重投，结果立刻可见"；死信有了终态，运维能区分"还没处理"与"已补上"；`replay_event_logs` 从死代码变成有数据的时间线；**投影失败不再拖垮整个同步**——因为死信带主键、补得回来，`OnRow` 从"失败即 `cancel()`"改成"记死信 + 继续跑"，此前一篇文章投影失败会让 MySQL→ES 同步永久停摆，而 API 照常服务、没人会知道。
- 否决「只给页面、重投仍靠人工 SQL」：看得见补不上，等于把问题挪个地方；
- 否决「自动重投（定时扫描 pending）」：死信多半是"改之前必然失败"的（字段不兼容、索引不存在），自动重投会在故障期间把 ES 打出一片 400 并掩盖真实原因，而人工触发天然带"我先看看为什么失败"这一步；
- 否决「重投走消息队列」：引入新基础设施，当前人工处理的死信量级完全用不上。

**代价 / 不做什么**：多 2 个契约接口与 2 个管理端页面需维护；`outbox_relay` 的重投是"重置状态"而非"立即执行"，最长要等一个中继轮询周期（默认 2s 退避起步）——这是有意的，复用中继自己的重试与背压；**重投的力度是"重算"不是"重放一行"**（改 1 篇文章只影响 1 篇，改一个用户昵称会影响他名下几十到上百篇），这是"展示字段冗余进 ES 文档"换来的代价；**`es_projection` 可重投依赖"投影与业务表在同一个库"这一前提**（文档组装要跨 `article / user / category / tag / article_tag / user_top_article` 六张表做应用层批量查询，不是 SQL JOIN），**一旦拆库或拆服务，这条路径必须改成"订阅各服务事件 + 本地读模型"，不能靠改连接串对付过去**；旧版本无 `ids` 的 `es_projection` 死信仍只能全量重建，接口会明确告知。

**当前生效的约束**：
1. 零 DDL：只用 `cdc_dead_letter` 已有列（`status` / `retry_count` / `error_*` / `updated_at`）；
2. 新增契约接口的响应必须用 `kernel.Envelope[ListData[T]]`（后端注解引用真实 DTO）；
3. `TestContractMatchesRoutes` 会自动要求这 3 个新接口在路由表里存在；
4. 新增 stream 时，执行器必须显式返回 `ErrUnsupported` 而不是默认"支持"；
5. 重投执行器必须幂等（这是不引入 `replaying` 中间态的前提，破坏幂等必须先补中间态）。

---

### ADR-009 删除"通用规则"同步路径，ES 同步只留文章搜索投影

**问题**：`internal/platform/cdc/river` 里长期并存两条 "MySQL → ES" 路径：

| | 通用规则路径 | 文章搜索投影 |
| --- | --- | --- |
| 配置 | `river.rule`：表 → 索引 + 字段白名单 + 类型转换 | 代码里固定 6 张表 |
| 数据来源 | 只用 binlog 那一行 | 回库按主键重读并 join 用户/分类/标签/置顶 |
| 文档形状 | 这张表自己的列 | 搜索卡片需要的一整篇 |
| 失败证据 | 完整 bulk body（可原样重发） | 曾是"表名+动作+行数"（不可重发） |
| 重试 | `doBulkWithRetry`（2 次 + 200ms） | 无 |

**决策**：删除整条通用规则路径。核查确认了三件事，是本次删除的依据：
1. **通用路径对本项目的每一张表都不可达**——`OnRow` 先把 outbox、再把 6 张源表交给 `handleArticleSearchProjectionEvent` 并提前 `return`；配置里 `source` 的 6 张表全在投影清单里，唯一的 `rule` 指的也是 `article_models`；
2. **它拼不出搜索文档**——白名单只有 8 个列，缺作者名、分类名、标签、各项计数，即使可达，产出的文档也不是搜索结果要的那份；
3. **它独有的两项能力都不再被需要**——"删除类死信重投"已由投影侧的 `ProjectionReplayer`（带行主键、按主键重算）覆盖；`doBulkWithRetry` 从未作用于投影路径。

删除清单：
- **代码**：`river/rule/`（整包）、`sync_request.go`、`sync_bulk.go`（只留 `saveProjectionErrorToDLQ` 与 `buildCDCJobID`，已并入 `sync.go`）、`replay.go`（`ESReplayer`），以及 `river.go` 里的 `rules` / `prepareRule` / `parseSource` / `updateRule` / `ruleKey`；
- **同步循环**：`sync.go` 的批量合并与重试循环一并删除，`syncCh` 从 `chan interface{}` 收窄成 `chan posSaver`，只剩"按 3 秒节流落位点"；
- **配置**：`conf.River.Rules`、`flush_bulk_time`、`river.retry`、两份 `settings.yaml` 的 `rule:` 段，以及只为它存在的 `BLOGX_RIVER_INDEX`（搜索索引一直取 `es.index`）；
- **装配**：`analytics_wiring` 不再注册 `es_river` 执行器；顺带删除因此变成死代码的 `es_service.Bulk`。

**保留**：`master`（位点）与 canal 的 dump 配置——全量转储会把行按 INSERT 重放一遍，投影分支照常把它们变成文档，**首次部署的数据初始化仍然可用**。

**理由**：死信侧语义已随 ADR-008 收敛（投影失败不再停掉整个 canal，而是"记死信 + 继续跑"，且死信带 `ids` 可重投），**旧路径存在的最后一个理由（"只有它能把 bulk body 原样重发"）随之消失**。
- 否决「保留但标注'当前不产生数据'」：成本是一个永不出数据的执行器 + 一份会误导人的 `rule` 配置；上一轮就是这么选的，结论是"标注"挡不住下一个人照着它改配置；
- 否决「把通用路径改成'配置 + 投影函数'」：本质是给一个只有一个实现的场景加一层间接，且仍要为每张表写代码。

**代价 / 不做什么（必须接受的）**：
- **加一张"不需要 join 的表"进 ES，现在要写代码，不能只改配置。** 步骤固定三步：① 在 `article_search_projection.go` 的分派里加一个 case；② 在 `projection_replay.go` 的 `projectionReplayEvents` 里登记 `(表, 动作) → 事件类型`；③ 补一条映射用例。约半小时，比"配置 + 猜字段"更可控；
- **ES 写入没有重试了。** 投影路径本来就是一次 `IndexBulk` 失败即落死信（以前也一样），所以这不是本次回归；但"想加重试"的人应该加在 `articlesync` 的批量写里，**而不是恢复一条独立的同步路径**；
- **历史 `es_river` 死信不再可重投。** 页面上仍显示流名（保留词表是为了历史记录可读），但点重投会得到 `unsupported`；需要补历史数据时用全量重建；
- 删掉的那批 `es_service` 单文档 API（`CreateDocument` / `Search` / `GetMapping` 等）在本次之前就已无生产调用方，属既有历史包袱。

**变好的**：少约 1,000 行、10 个文件；表清单从两份（`source` + `rule`）收敛成一份；消灭一类"配置看起来在生效"的陷阱——`rule` 段解析时是**非严格**的，删了字段却留着 yaml 只会静默失效；死信只剩 3 条流，死信页上不会再出现一条永远不会有数据的 `es_river`。

**当前生效的约束**：
1. ES 同步**只有文章搜索投影一条路径**，禁止再引入配置驱动的通用同步（新增表走三步代码登记）；
2. 表清单单一来源：只认 `article_search_projection.go` 里的分派，不认 `settings.yaml` 的 `rule:` 段；
3. 重试若需加强，加在 `articlesync` 的批量写里，不得恢复独立同步路径；
4. 补历史 `es_river` 数据用全量重建命令 `./server -es -s article-sync`，不要用页面重投（必然返回 `unsupported`）；
5. `river` 保留 `master` 位点与 canal dump 配置，用于首次部署的全量初始化。

---

## 【归档备注】仅具历史价值、v2 不需要再看

| 内容 | 原因 |
| --- | --- |
| ADR-001 / 002 / 003 / 005 / 006 的"现状病灶"数据与迁移路径 | 迁移已完成，数字（32 个 `Deps`、170 包、2,357 断言、14,462 行 json）只说明当时为何动手 |
| ADR-004 的存量违规基线（150+ 处、R6=4、R7=65）与"逐模块清零"节奏 | 存量已清零，基线当前值是 0 |
| ADR-007 的预估 vs 实际偏差表、3-4 轮收敛过程 | 已落地，教训本身已沉淀为"验证口径必须与门禁一致" |
| ADR-008 的四层架构图与 ADR-009 的删除清单逐项 | 代码已在位，清单只用于核对历史 diff |
| 被否决方案的完整论证（wire/dig、depguard、tRPC、SQLite 冒充 MySQL 等） | 已压缩为一行，v2 选型时不必重读论证 |
