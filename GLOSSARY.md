# blogx 领域词汇表

本文件是本仓库**术语的单一权威**：文档、代码注释、评审意见与 engineering skills 都使用这里的写法，不要漂移到同义词（尤其是各词条 `_Avoid_` 里列的旧名）。

词条只回答"这个概念是什么、代码里在哪"。查"为什么这么设计、否掉了什么、代价是什么"去 `docs/adr/`（ADR-0001 起，一条决策一个文件；v1 的旧合集归档在 `docs/archive/v1/04-adr-collection.md`，只查历史），不在这里。

---

## 一、架构与模块

**合成根（Composition Root）**：
唯一允许 `new` 具体实现、把所有端口与适配器接线起来的地方。
位置：`backend/cmd/server/main.go`；模块内的装配函数叫 `wireXxx`（`internal/<模块>/adapter/wire.go`），`main` 只负责连线。

**端口（Port）/ 能力接口**：
由**使用方**定义、只暴露用得到的方法的窄接口（每个 1–3 个方法），取代把 `*gorm.DB` 层层装箱的传递方式；实现方不得反向定义。
位置：`internal/<模块>/app/port.go`（跨模块与外部能力）、`internal/<模块>/domain/port.go`（仓储）；例：`ArticleGate`、`DiggCounter`。
易混：端口与能力接口是同一概念的两个叫法，不要另造"服务接口""依赖包"。
_Avoid_: `Deps`（含 `type Deps struct`，archcheck R2 直接拦）、Service Locator。

**垂直模块（domain / app / adapter）**：
每个业务模块自包含的三段目录：`domain`（实体、值对象、不变量）、`app`（用例编排与事务边界）、`adapter`（HTTP 与持久化都算适配器），依赖单向 `adapter → app → domain`。
位置：`backend/internal/<模块>/`，样板是 `internal/article/`。
_Avoid_: v1 的水平目录 `api/` / `service/` / `repository/` / `models/`（已不存在，禁止往里加文件）。

**kernel**：
跨模块共用的值对象与契约类型包，只放 ID、分页、错误码等**零业务规则**的内容，可被所有模块 import。
位置：`backend/internal/kernel/`（`ctype`、`page`、`enum`、`envelope.go`）。
_Avoid_: `models/`（v1 的共享持久化模型包，已删除；row 结构现在归各模块 `adapter`）。

**UnitOfWork**：
事务契约：把"一个事务里要做的事"包成 `WithinTx(ctx, fn(ctx, Repos) error)`，事务边界在 app 层，`Repos` 由 app 定义。
位置：`internal/<模块>/app`；domain 与 app 都不得出现 `*gorm.DB`。
易混：是 v1 `TxManager` 的演进形态，不要再用旧名。

**outbox（事务型发件箱）**：
领域事件先与业务写在同一个事务里落库，再由中继异步投递，保证"业务写成功则事件不丢"。
位置：表 `outbox_event_models` + `backend/internal/platform/cdc/outboxrelay`；跨模块"写副作用"必须走它。
_Avoid_: 在 handler 里裸起 goroutine 发事件（v1 做法，已废）。

**safego**：
唯一允许起 goroutine 的入口（内部 recover + 结构化日志）；裸 `go` 语句由 archcheck R7 拦下。
位置：`backend/internal/platform/safego`。

**apperr（错误码）**：
应用错误码包：`Code` 用"领域.语义"命名（如 `article.not_found`），中文文案集中在一处；调用点只写错误码，判分支用 `CodeOf` / `IsCode`。
位置：`backend/internal/apperr`；archcheck R8 拦 `errors.New("中文")`。
_Avoid_: 按中文字符串匹配错误分支（改一个字就断）。

**零 DDL（零 DDL 硬约束）**：
不改、不删既有表结构（列名、类型、列顺序、删列全禁），只允许 `CREATE TABLE IF NOT EXISTS` 新建表；因为 ES 投影按列名解析 binlog 行。
位置：硬约束 A1，见 `docs/architecture/overview.md` §六、`README.md` §十二。
_Avoid_: 把"迁移"理解成 ALTER——本项目没有 ALTER 通道。

---

## 二、契约与类型

**契约（OpenAPI 契约）**：
后端注解生成的 `backend/docs/swagger.json`（OpenAPI 3.1），是三个应用之间**唯一**的耦合点；两端前端类型全部从它派生。
位置：`cd backend && make openapi` → `backend/docs/swagger.json` → 两端 `app/types/openapi.d.ts`。
_Avoid_: 手写 openapi 文档（v1 的 14,462 行已删除）。

**swag 注解 / `make openapi`**：
handler 上的 `// @Summary` / `@Param` / `@Success` 注释是契约的唯一来源；`make openapi` 用 swag v2 加 `--v3.1` 直出 OpenAPI 3.1。
位置：`backend/Makefile` 的 `openapi` target、`internal/*/adapter/http/`。
易混：验证生成成功不能只看 exit code——注解里写 `map[string]any` 时 swag 会静默跳过整份写入，退出码仍是 0。

**openapi.d.ts**：
`openapi-typescript` 从 `swagger.json` 生成的 TS 类型文件，禁止手改、禁止跨 app 共享。
位置：`frontend/app/types/openapi.d.ts`、`frontend_admin/app/types/openapi.d.ts`；命令 `pnpm openapi:types`。

**Envelope[T]**：
统一响应外壳 `{code,msg,data}`，**只用于 `@Success` 注解**，业务代码不要拿它构造响应。
位置：`backend/internal/kernel/envelope.go`；注解写法 `kernel.Envelope[kernel.ListData[http.XxxItem]]`。

**data 载荷（ListData / HasMoreListData / EmptyData / SiteConfigData）**：
四种 data 形状，必须按 handler 实际调用的 helper 选：`OkWithList`→`{list,count}`、`OkWithHasMoreList`→`{list,has_more}`、`OkWithMsg`→`{}`、`/site/{name}`→自由键值块。
位置：`backend/internal/kernel/envelope.go`。
易混：`ListData` 与 `HasMoreListData` 用错就是契约撒谎——前端读 `has_more` 恒为 `undefined`，编译器一句提示都没有。

**contract.ts 三层工具（Schema / RequiredSchema / Nullable / OptionalFields）**：
前端消费契约的正解：请求体用裸 `Schema<K>`，响应用 `RequiredSchema<K>`（递归去可选），后端会返回 null 的用 `Nullable<T,K>`，可能整个缺席的用 `OptionalFields<T,K>`。
位置：`frontend/app/types/contract.ts`、`frontend_admin/app/types/contract.ts`。
_Avoid_: 退回手写 API 类型——`api.ts` 里每个手写类型都必须能回答"为什么契约里没有它"。

**可空性三态**：
Go 字段三种写法对应三种 TS 表达：`T`→`T`、`*T`→`T | null`、`*T`+`omitempty`→字段整个不出现。
位置：契约硬约束 B6；`README.md` §七。
易混：只用 `RequiredSchema` 会把第三态逼成"必须编造一个值"，加载失败时的兜底对象直接编译不过。

**ctype.ID**：
ID 语义字段的唯一类型，底层是 string（JSON 里就是字符串，不会丢精度），需要数值时显式 `.Uint64()`。
位置：`backend/internal/kernel/ctype/id.go`；archcheck R12 拦裸整型 ID，基线 0。
_Avoid_: 裸 `uint64`/`int64` ID 字段、`ID(123)` 构造（用 `NewID`）、直接拿 ID 比大小（`"10" < "9"` 成立）。

**DTO（Req / Resp / Query）**：
跨网络传输的请求/响应结构，后缀固定，放模块 `adapter` 内，不进 domain。
位置：`internal/<模块>/adapter/dto.go`、`adapter/http/dto.go`；命名规范 A8。
_Avoid_: `article_types.go` 这类混杂命名的类型文件（v1 反例）。

**枚举别名（kernel/enum）**：
同一套状态值只在一处声明词表，其他模块引用别名而不是重新声明一份（否则契约会印出被收窄的单值枚举）。
位置：`backend/internal/kernel/enum/`，如 `kernel/enum.ArticleVisibilityStatus`。

---

## 三、配置分层

**配置三层**：
进程级（`settings.yaml` + 环境变量）→ 业务默认值（`site_default_settings.yaml`）→ 运行时配置（`runtime_config_models` 表）；判据一句话："改它会不会影响进程怎么跑"。
位置：`docs/architecture/configuration.md` §二、`README.md` §五。

**进程级配置**：
必须在启动前就知道的值（监听地址、DB/Redis/ES/ClickHouse、JWT、日志目录、CDC 位点目录），改了要重启，走环境变量与 `settings.yaml`。
位置：`backend/config/settings.yaml`、`deploy/compose/production/env.example`；`BLOGX_*` 现在只剩这一类。
_Avoid_: 把邮件 / AI / 对象存储 / QQ 登录等业务配置塞回环境变量（52 个已删）。

**业务默认值（site_default_settings.yaml）**：
站点业务配置的唯一默认值来源，全是字面量（没有 `${VAR}`），只在库里还没有该段时灌入。
位置：`backend/config/site_default_settings.yaml`。
_Avoid_: 在代码里再写一份默认值（两份默认值会互相打架）。

**运行时配置（runtime_config_models）**：
管理员在后台改、下一次请求即生效的配置；表结构是 `(name, payload)`，**一段一行完整 JSON**。
位置：表 `runtime_config_models`；七段 `site/ai/mail/storage/oauth/log/upload`。
_Avoid_: 旧表 `runtime_site_config_models`（已无代码读）、列化存储（零 DDL 下每加字段都要改表 = 不可行）。

**Provider 闭包（现读）**：
消费点持有的是取值函数（`func() T`）而不是启动时的值快照，所以后台改完立刻生效。
位置：`internal/platform/config`；例 `middleware.Runtime.logConfig()`、`emailCodeBridge`、`qiniuStorage`。
_Avoid_: 把配置值拷进 struct 字段再长期持有（改完不生效，且是静默不生效）。

**深度合并（加字段零迁移）**：
读旧 JSON 补默认值的三条规则：`null` 不覆盖、两边都是对象则递归、数组整体替换。
位置：`backend/internal/platform/config/merge.go`。
_Avoid_: 数组逐元素合并（会变成"只增不减"，白名单删不掉）。

**SensitivePlaceholder（`"******"`）**：
出边界时密钥字段统一替换成的占位符；写回时占位符=不改、空串=真清除、其它字符串=覆盖。
位置：`domain.SensitivePlaceholder`；后台的 `AdminXxx` / `UpdateXxx` 成对出现。
_Avoid_: 把占位符原样写回库（密钥变成 `******`，下一次调用全部 401）。

---

## 四、CDC 与同步

**CDC（change data capture）**：
MySQL binlog 驱动的异步同步总称；本项目有两条链路：文章搜索投影（MySQL→ES）与图片引用关系。
位置：`backend/internal/platform/cdc/`。

**river**：
MySQL binlog → ES 文章搜索投影的消费循环，进程角色 `-role=river`；它"假装成一个 MySQL 从库"读 binlog。
位置：`internal/platform/cdc/river/`；`river.Run()` 失败只登记 health，不崩进程。
易混：river 是消费循环，canal 是它底下的 binlog 客户端，两者不是同一个东西。

**canal / canalbase**：
基于 go-mysql 的 binlog 客户端封装，负责连主库、解析位点、冷启动时做全量转储。
位置：`internal/platform/cdc/canalbase/`。
易混：`-role=river` 起的是消费循环；canal 只是其中一层，不要在讲"同步死没死"时混用。

**全量转储（dump）**：
canal 冷启动时用 `mysqldump` 把现有数据按 INSERT 重放一遍，让投影从空索引追平；`skip_master_data=false` 才会带上 binlog 坐标。
位置：dump wrapper（镜像内，转发前摘掉 `--skip-opt`）；另有一条 CLI 全量回填 `-es -s article-sync`。
_Avoid_: 用包名判断 dump 客户端来源（alpine 上 `apk add mysql-client` 装到的是 MariaDB 客户端，dump 会 exit 2）。

**位点（master.yaml）**：
river 记录在 `/app/var/master.yaml` 的 binlog 进度；删了会从头重扫，轻则重复同步、重则把旧数据覆盖回去。
位置：宿主机的 `deploy/state/api/var/`（备份必带）。

**投影（projection）**：
把 binlog 行按主键回库重读、重算成 ES 文档或引用记录的动作（不是"把这一行复制过去"）。
位置：`internal/platform/cdc/river/article_search_projection.go`、`projection_replay.go`。
_Avoid_: 配置驱动的"通用规则同步"（ADR-009 已删除；新增一张表要走三步代码登记）。

**outbox 中继（outboxrelay）**：
扫描 outbox 表把领域事件投递给消费方，失败按退避重试、耗尽后落死信。
位置：`internal/platform/cdc/outboxrelay/`；`-role=api` 也会起它。

**image-ref（图片引用 river）**：
维护"哪个业务对象引用了哪张图"的监听器，供图片引用统计与清理使用；进程角色 `-role=image-ref`。
位置：`internal/platform/cdc/imageref/`。

**死信（cdc_dead_letter）**：
同步重试耗尽后留下的证据行，只有 `pending`（还没补上）与 `resolved`（已人工补上）两个状态。
位置：表 `cdc_dead_letter`、`internal/platform/cdc/deadletter/`。
_Avoid_: 增设 `replaying` 中间态或 `gave_up` 状态（刻意没有，理由见 ADR-008）。

**DLQ 重投（replay）**：
人工触发的死信补救：取记录 → 分派执行器 → 更新状态 → 写回放日志；"不支持"（`ErrUnsupported`）与"这次失败"是两种不同结果。
位置：`internal/platform/cdc/dlq/service.go`、`POST /logs/dead-letter/replay`。
易混：失败 = 仍 `pending` 且 `retry_count+1`（红色"失败"，可以再点）；不支持 = 状态与 `retry_count` 都不动（橙色"不支持重投"，再点一百次也一样）。
_Avoid_: 自动重投（定时扫 pending；死信多半是"改之前必然失败"，自动重投只会把 ES 打热）。

---

## 五、质量与门禁

**门禁（scripts/check.sh）**：
本地一键门禁（后端 + 集成 + 两端前端 + 门禁自检），口径必须与 CI 一致；跑过它等于本地结论与 CI 一致。
位置：`bash scripts/check.sh [all|backend|frontend|gates]`，开关 `BLOGX_FAST` / `BLOGX_NO_CACHE` / `BLOGX_SKIP_INTEGRATION`。

**archcheck**：
Go AST 实现的架构守卫，替代会"永真通过"的 PowerShell 正则守卫；规则 R1–R12，每条有 ID 与 slug。
位置：`backend/tools/archcheck/`（`rules.go`、`rules_id.go`）；flag `--mode=error|warn`、`--rule`、`--max-violations`。
_Avoid_: 用 grep/正则匹配符号名做架构检查（别名与换行能绕过，且 CI 跑不了 PowerShell）。

**棘轮（--max-violations）**：
存量违规基线，只准减不准增；基线数字只在 `scripts/arch-baseline.env` 定义一次，三处消费。
位置：`scripts/arch-baseline.env`（当前 0），消费方是 `check.sh`、`verify-gates.sh`、`.github/workflows/ci.yml`。
_Avoid_: 把基线数字写死在别处（曾出现"本地降到 0、CI 还停在 250"）。

**规则自检（rules_test.go）**：
每条 archcheck 规则都必须能在"人造违规"上报错，否则说明这条规则失效。
位置：`backend/tools/archcheck/rules_test.go`；新增规则必须同时补人造用例。

**门禁自检（verify-gates.sh）**：
主动注入违规来证明每道门禁真的会红（11 道 canary），而不是永远绿。
位置：`bash scripts/verify-gates.sh`；CI 里由 `gates-canary` job 调用同一脚本，不另写一份注入逻辑。

**假绿 / 门禁撒谎**：
门禁"看起来通过但实际没执行"的失效形态：测试缓存命中、静默跳过、子 shell 吞掉退出码、生成被打断仍 exit 0。
位置：`docs/architecture/governance.md` §1.3；对策是跳过的步骤必须打印警告、CI 把跳过变成失败。
_Avoid_: 用 `git diff --exit-code` 当契约判据（工作区脏会假红，人习惯性忽略）。

**golden 测试**：
写死种子、逐字节比对、**不做归一化**的响应快照测试；只给 5 个只读模块做（comment / search / site / notification / analytics）。
位置：`internal/<模块>/adapter/http/testdata/`、`testkit/golden.go`；`UPDATE_GOLDEN=1` 重写。
_Avoid_: 把 ID / 时间抹成占位符再比（`123`→`"123"` 这类线上会炸的错就再也发现不了）。

**testkit（含有状态 fake）**：
测试基建包：可注入的 Clock / IDGen、fixture、golden、容器夹具，以及**带状态**的假仓储（能模拟冲突、回滚、事件失败，gomock 表达不了）。
位置：`backend/internal/testkit/`（golden / fixture / mysqlcontainer）、`internal/<模块>/apptest/`（有状态 fake）。
_Avoid_: gomock（只能验"调了哪个方法、传了什么参数"）、`test/testutil` 的全局覆盖写法（已删）。

**契约测试（真 MySQL / 不假 Safe）**：
adapter 层用 testcontainers 起真 MySQL 验 SQL 与 mapper；SQLite 不冒充 MySQL。
位置：`//go:build integration` + `internal/testkit/mysqlcontainer`；`BLOGX_SKIP_INTEGRATION=1` 跳过时必须打印警告。
_Avoid_: 用 SQLite 跑 MySQL 的 SQL（`ON CONFLICT`、锁、JSON 函数行为不同 = 假 Safe）。

**覆盖率门槛（分层阈值）**：
按层设阈值而不是一个总数：domain ≥90%、app ≥85%、platform/log ≥90%、adapter 每方法至少 1 条，整体不设；前端读者站 60/50/66/60，admin 只卡 `app/utils/**` 的 95/90/95/95。
位置：两端 `vitest.config.ts`、CI `frontend-test`；`passWithNoTests` 必须为 `false`。
_Avoid_: 追求整体覆盖率数字（只会养出垃圾测试）。

**有界异步日志管道**：
日志写入模型：`Emit` 只投递（O(1)）→ 双限额有界队列 → 单写协程批量写 ClickHouse → 失败落 WAL，恢复后重放。
位置：`backend/internal/platform/log/`（`emit.go`、`event.go`）；本地兜底目录 `logs/wal`。
_Avoid_: 在请求路径里 `json.Marshal`、写文件、调雪花 ID（v1 病灶）；`any` 或结构体进日志字段（archcheck R9 拦）。

**日志 Kind（可丢 / 不可丢）**：
日志分类决定"队列满或 ClickHouse 挂掉时能不能丢"：runtime / access 可丢（计数 + 采样告警）；audit / login / cdc / replay 不可丢，必须落 WAL。
位置：`internal/platform/log/event.go` 的 `Kind`；ClickHouse 每类一张表，各有 TTL。
易混：Kind 是日志**分类**，不是日志**级别**（`Level`）；`ai_call` 是后来追加的第 6 类（见"AI"一节）。

---

## 六、AI

**eino**：
CloudWeGo 的 LLM 组件框架（eino-ext 提供各供应商适配）；本项目所有供应商调用最终都由它的组件发出。
位置：`internal/platform/llm/factory.go` 按 vendor 分发到 `model/openai`、`model/claude` 等组件。

**platform/llm**：
唯一真正发 HTTP 的 AI 网关层：报文映射、重试（网络错 / 429 / 5xx）、错误码、channel 契约都在这里。
位置：`backend/internal/platform/llm/`（`spec.go`、`factory.go`、`chat.go`）。
_Avoid_: 在业务包里自己拼 `Authorization: Bearer` 加 SSE 扫描（v1 那套 332 行手写实现已删）。

**ai.Chat / ai.ChatStream**：
业务侧唯一出口：6 个 AI 子包只调这两个函数（带用量的版本是 `ChatDetailed` / `ChatStreamDetailed`），换框架或供应商不动业务代码。
位置：`backend/internal/ai/llm.go`。

**ProviderSpec**：
从 `conf.AI.Active` 投影出来的供应商强类型入参（vendor / model / api_key / base_url / region / max_tokens …），是"配置字段 → 供应商字段"的翻译结果。
位置：`internal/platform/llm/spec.go`；构造函数 `specFromConfig` 在 `internal/ai/llm.go`。

**供应商（vendor）**：
八个可选后端：openai / claude / gemini / ark / deepseek / qwen / qianfan / openrouter；每家要哪些字段、哪些是 secret，由元数据描述。
位置：`internal/ai/app/vendors.go`、`GET /ai/vendors`。
易混：vendor 是"哪家协议"，model 是"哪个模型"；ark 的 model 填的是推理接入点 ID（`ep-xxx`），不是模型名。

**ai_provider_models**：
模型配置表：`name` / `vendor` / `model` / `enabled` 列化，其余字段进 `config` JSON（各家字段不同 + 零 DDL 无 ALTER 通道）；`enabled` 全局唯一靠服务层事务。
位置：表 `ai_provider_models`、`internal/ai/adapter/mysql/provider.go`。

**两层开关**：
AI 真正可调用要同时满足两层：站点总闸 `ai.enable`（runtime_config 的 ai 段）与条目级 `enabled`（表里启用的那一条）。
位置：`docs/architecture/ai-integration.md` §五。
易混：关总闸 vs 关条目是两件事，报错文案也不同（"整站 AI 不可用" vs "还没有启用的模型配置"）。

**Active（conf.AI.Active）**：
派生的"当前启用的模型配置"字段，`yaml:"-" json:"-"`，由装配层从 ProviderService 的内存快照填入，所以子包仍只收 `conf.AI` 一个参数。
位置：`backend/conf/`；读取点是 `internal/ai/llm.go` 的 `specFromConfig`。

**CallMeta（scene）**：
挂在 ctx 上的调用元数据（scene / user_id / request_id / trace_id），给 token 用量日志用；固定 6 个场景 `scoring / metainfo / overwrite / diagnose / search_rewrite / search_answer`。
位置：`internal/ai/callmeta.go`；`WithCallMeta`、`callCtx(c)`。

**KindAICall / ai_call_logs**：
AI 调用日志（场景、模型、token、耗时、失败原因），第 6 类平台日志；TTL 400 天、`ORDER BY` 以 ts 打头，**不记 prompt 与回复正文**。
位置：`internal/platform/log/event.go`、ClickHouse 表 `ai_call_logs`、`GET /logs/ai`、`GET /data/ai-usage`。

---

## 七、环境与部署

**role（-role）**：
进程角色开关：`api` / `river` / `image-ref` / `cron` / `worker` / `all`；CDC 消费者只在 worker 类角色里起。
位置：`./server -role=all`（生产单机）；`README.md` §八。
易混：只起 `-role=api` 就没有 CDC 消费者——新文章永远进不了 ES，而且不报错。

**entrypoint 自举**：
容器首次启动的自动初始化：以"`runtime_config_models` 表是否存在"判断是否已经初始化过，缺表才跑 `-db` 与 `-es -s ensure`。
位置：`deploy/docker/api/entrypoint.sh`。
易混：新增的表**不会**被自动创建（只要那张判断表已在），要手动 `-db`。

**存活（liveness）/ 就绪（readiness）**：
两个语义严格分工的端点：`/health` 只看进程能否处理请求（容器 healthcheck 用它）；`/health/ready` 有 `failed` 组件就返回 503（部署验收用它）。
位置：`/health`、`/health/ready`（刻意只在内网可达）；compose healthcheck 必须留在 `/health`。
_Avoid_: 把 compose healthcheck 改成 `/health/ready`（CDC 一挂会连带 nginx 起不来、全站不可用）。

**health.Registry（组件状态注册表）**：
进程内并发安全的组件状态表：`starting` / `running` / `stopped` / `failed` / `disabled`；只有 `failed` 会让就绪检查变红。
位置：`backend/internal/platform/health/registry.go`；`WireWorker` 里每个失败出口都要登记。
易混：`disabled` 必须记，否则"CDC 没启用"和"CDC 挂了"在观测上完全无法区分。

**verify-deploy.sh（部署验收）**：
部署的最后一步：必须 `healthy:true` **且** `cdc_river` 状态是 `running`，否则不算部署成功。
位置：`scripts/verify-deploy.sh`；`BLOGX_REQUIRE_CDC=0` 供纯 api 部署关掉第一项检查。
易混：只看 `healthy` 不够——`disabled` 也是 healthy。

**compose 服务名 / 容器名**：
compose 服务名用下划线（`blogx_server` / `blogx_web` / `blogx_admin`），容器名用连字符（`blogx-server` / `blogx-admin`）；命令里用错会报 `no such service` 或 `No such container`。
位置：`deploy/compose/production/docker-compose.yml`。

**BLOGX_NGINX_ENV（nginx 四形态）**：
一个变量决定 nginx 形态：`local`（只监听 80）/ `test` / `production` / `api-only`，由 `render-config.sh` 推导域名、证书、upstream 与子站开关。
位置：`deploy/docker/nginx/nginx.conf.tmpl`、`deploy/docker/nginx/render-config.sh`。

**profile（前端 env profile）**：
前端读 `env/<profile>.env` 并叠加 `env/common.env`，profile 由 `pnpm dev` / `pnpm build` 脚本注入：`local-local` / `local-test` / `test` / `production`。
位置：`frontend/env/`、`frontend_admin/env/`；容器部署时运行期映射成 `NUXT_*`，所以换环境不用重建镜像。
_Avoid_: 让前端读 `.env`（不读；且每份 env 只能写本应用前缀的键，白名单外直接忽略）。

**BLOGX_APP_ENV**：
环境标识 `dev` / `test` / `production`；代码只把 `production` 当作生产，它给 refresh Cookie 加 `Secure`。
位置：`deploy/compose/production/.env`。
易混：历史写法 `prod` 仍被兼容（启动日志会提示迁移），但新配置一律写 `production`；http 环境必须是 `dev`，否则表现是"能登录、刷新后掉登录"。

---

## 八、前端呈现与品牌

**Jelly（捏捏）**：
读者站右下角常驻的果冻方块软体角色，同时是读者站 **AI 的唯一入口**：点它打开 AI 面板，面板从它所在的位置生长出来、打开期间它被压成面板右下角的把手（非遮罩，可边读边问）；创作区 `/studio/**` 不渲染它，后台也没有它。
位置：读者站 `frontend/app/` 下的角色组件（只负责外观与指针交互）与状态 composable（唯一状态真相源，在 `composables/`）；AI 面板复用既有 `frontend/app/services/ai.ts` 流式服务层。取舍见 [ADR-0011](./docs/adr/0011-jelly-replaces-traditional-ai-entry.md)。
_Avoid_: 叫它"AI 对话框""聊天机器人"（它是入口角色，不是对话组件）；旧入口"首页右栏 AI 卡片 + 居中模态框"也不指代它。

**JellyBin（果冻盒）**：
读者站与后台的**用户可见品牌名**（英文 JellyBin、中文副标「果冻盒」）；旧展示名 `BlogX` 不再使用。
位置：两端各一份品牌模块（读者站 `frontend/app/`、后台 `frontend_admin/app/`），集中导出英文名、中文名、字标结构、助手显示名、站点一句话介绍与版权行；改造前这些品牌串仍是散落硬编码。
_Avoid_: 把内部标识当展示名——仓库名 `blogx`、Go module `myblogx`、`BLOGX_*` 环境变量前缀、compose 服务名 `blogx_web` / `blogx_admin` / `blogx_server`、`blogx-theme` cookie 与 `blogx_access_token` / `blogx_profile_snapshot` 存储键都不改名。

**光标挂件（cursor charm）**：
读者站的**纯装饰**角色：站标用一条「挂绳」系在指针上，随指针被拖行、摆动。它不响应点击、不抢焦点、不承载任何功能——读者站唯一的功能性入口仍是「捏捏」。
位置：读者站 `frontend/app/` 下的挂件组件与驱动它的 composable（待落地）；素材复用 `utils/brand.ts` 的 `logo` 资产（与 `favicon.ico` 同一张画）。
_Avoid_: 叫它「鼠标挂件」（触控板用户没有鼠标，本仓库一律说「指针」）；用「鼠标追随特效」「cursor effect」指代它（通用词，也指 `usePointerSpecular` 那类非挂件效果）。

**挂绳（tether）**：
光标挂件里连接指针与站标的那条绳：指针端固定、站标端自由，形状与摆动由物理模拟逐帧求出，不是一条静态描边。
位置：同「光标挂件」。
_Avoid_: 把它当成挂件的描边或阴影；用「绳子」「线」当正式名。

**语义色 token**：
呈现层颜色的唯一消费口径，分两层：**原始层**锁死五个色相（奶油白 `#FDFBF6`、暖灰 `#F2EEE6`、包豪斯黄 `#F5C518`、珊瑚橙 `#FF6B4A`、功能蓝 `#0066CC`/`#2997FF`，各带品牌前缀，不新增第六个）→ **语义层**（底色、纸面、墨色、结构线、高亮底、危险、链接、焦点环、阴影、玻璃、圆角等），组件只消费语义层。暗色是同一组语义 token 的第二份取值，由站点根类承载，不是另一套色板；带透明度的取值一律由 `rgb(var(--*-rgb) / α)` 三元组派生。
位置：`frontend/app/assets/css/tokens.css` 与 `frontend_admin/app/assets/css/tokens.css`（两端**逐字节同源**，由同步门禁保证）；两端 Tailwind 配置承载色板类名，Naive `themeOverrides` 从 token 取值。漂移门禁扫描组件与样式里的裸色字面量（含 `.ts` 里的 CSS 字符串）。
_Avoid_: 在组件或样式里写裸十六进制 / `rgba()`（token 文件、品牌资产、OG 生成物除外）；写死 `rgba(r,g,b,α)` 常量而不走 `--*-rgb` 三元组；只改一端 token 而不走同步门禁。
易混：旧口径"克莱因蓝作主色、结构线一律 1px 蓝"已退役（见 [ADR-0012](./docs/adr/0012-neutral-glass-visual-system.md)），不要再用它描述主色。

**中性石墨阶（graphite scale）**：
语义层里墨色与结构线所用的无彩灰阶，由暖灰去彩派生，因此**不算新增色相**；亮色是暖石墨，暗色是同一条明度阶梯上的中性暗阶（刻意不取暖黑，暖黑压上白玻璃会发浊）。
位置：`tokens.css` 语义层的 `--color-ink` / `--color-ink-muted` / `--color-ink-faint` 与 `--color-line` 三档。
_Avoid_: 把它当成第六个色相（它是暖灰的派生层级）；继续用"黑 = 深蓝"的旧取值。

**功能蓝（interactive blue）**：
只承担"这里可以点"的交互指向——文字链接与焦点环；不做铺底、不做 CTA 填充、不做墨色。亮色 `#0066CC`（对白 4.5:1）、暗色 `#2997FF`（苹果只在暗色用它，对白底仅 2.7:1，不能作亮色链接文字）。
位置：语义层 `--color-link` / `--color-focus`，取值见 `tokens.css` 原始层。
_Avoid_: 把它当品牌色或主色；把 `#2997FF` 用在亮色下的文字上。

**液态玻璃层（glass layer）**：
只允许出现在**导航/控制层**的半透明材质（顶栏、浮层、抽屉、捏捏面板、胶囊、悬浮控件），两档 `--glass-thin` / `--glass-thick`；正文、卡片、列表等内容面一律不透明实体面。
位置：`tokens.css` 的玻璃组 token；取舍与降级规则见 [ADR-0012](./docs/adr/0012-neutral-glass-visual-system.md) 与改造方案第 3 批。
_Avoid_: 在内容面/正文上叠玻璃；把玻璃当成"装饰性半透明"到处铺（`backdrop-filter` 会让元素成为 backdrop root，嵌套玻璃只会读成平灰）。
易混：玻璃层 ≠ 半透明卡片——前者是控制层材质，后者是非法用法。

**圆角阶梯（radius scale）**：
五个圆角档位 `--radius-sm/md/lg/xl/pill` 的统一口径（控件 8–14、卡片 20、浮层 28、胶囊 pill），取代旧的"圆角归零"；嵌套层的子圆角 = 父圆角 − 内边距。
位置：`tokens.css` 的圆角组；Naive `themeOverrides` 的 `borderRadius` 从它取值。
_Avoid_: 在组件里写 `rounded-[18px]` 一类裸圆角字面量。

**首屏封面（editorial hero）**：
首页顶部的**满幅图形封面**，是全站唯一的"编辑部式例外区"：横向色带、超大 h1、细网格与下沿刻度、方括号/箭头记号、胶囊小签，视觉素材取自运营上传的首张轮播图。它没有卡片边框与纸面底色，轮播控件并入其中。
位置：`frontend/app/pages/index.vue` 首屏区块与 `frontend/app/components/article/BannerCarousel.vue`；取舍见 [ADR-0013](./docs/adr/0013-editorial-hero-cover.md)。
_Avoid_: 叫它"首屏卡片 / hero 卡片"（卡片形态已废）；把它当成可复用的版式模板（只允许出现在首页首屏）。

**品牌资产例外（brand asset exception）**：
`brand/logo-256.png`、`brand/jelly-flat.svg`、`apple-touch-icon.png`、`og-image.png` 允许自带品牌色，不受"只消费语义色 token"与漂移门禁约束（同 App 图标惯例）；但品牌资产**不随主题切换变体**。
位置：`frontend/app/utils/brand.ts` 的 `BRAND_ASSETS`（`logoWhite` 按 ADR-0012 批次 0 删除）。
_Avoid_: 再造一张"暗色专用 Logo"——`logo-512-white.png` 是无 alpha 通道的白底方块，正是不透明白边的来源。

**动效档位（motion tier）**：
动效预算的分级名字，现行是**档位 C**：三档时长 120 / 200 / 320ms + 单一缓动 + stagger 40ms，允许全站区块级滚动进场（位移 ≤12px）、视差 ≤10px、导航随滚动转玻璃；正文段落不进场，且强制尊重 `prefers-reduced-motion`。
位置：`tokens.css` 动效层与 `frontend/app/utils/motion.ts`（同一份预算，两处必须同改）。
_Avoid_: 用"档位 B"描述现状；在 `prefers-reduced-motion` 下仍保留进场。

> 说明：本节新增词条（中性石墨阶、功能蓝、液态玻璃层、圆角阶梯、品牌资产例外、动效档位）出自 [ADR-0012](./docs/adr/0012-neutral-glass-visual-system.md)。该 ADR 已**交付**（落地分票 #18–#25 已全部合入），上述词条描述的都是当前生效的实现口径；「语义色 token」词条中两端逐字节同源的约定已由 `scripts/check-token-parity.mjs` 保证，两份 `tokens.css` 现逐字节一致。

---

## 维护规则

1. **什么时候新增词条**：一个词开始被多处代码或文档引用、且读者可能叫错它时（出现同义词、旧名、缩写）才加。一次性数字、某次故障的过程、实现细节都不进本文。
2. **什么时候改词条**：概念本身变了才改（比如 `ctype.ID` 底层类型变化、某个状态机增删状态）。代码搬目录只改"位置"那一行。
3. **选词原则**：同一个概念只留一个正名，其余写进 `_Avoid_`；"易混"只写原文有明确区分说法的那几对，不自行发挥。
4. **与 ADR 的分工**：**词条 = 概念是什么**（不含取舍）；**ADR = 为什么这么选、否决了什么、代价是什么**。ADR 在 `docs/adr/`（一条决策一个文件，编号递增、不覆盖旧决策）；v1 的旧合集归档在 `docs/archive/v1/04-adr-collection.md`，只查历史。词条正文引用 ADR 编号即可，不要把决策论证抄进来。
5. **不收什么**：一般编程概念（超时、错误类型、通用设计模式）、纯 UI 文案、会过期的数量指标（当前 CI job 数、覆盖率的即时数值）——数字写进文档或代码注释，本文只记录稳定概念。
6. **同步义务**：README 与 `docs/architecture/*` 不复制词条正文；首次出现处链回本文即可，避免两处定义漂移。
