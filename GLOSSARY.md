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

**零 DDL（零 DDL 硬约束 = 零列变更 + 允许新增表）**：
不改、不删既有表结构（列名、类型、列顺序、删列全禁），只允许 `CREATE TABLE IF NOT EXISTS` 新建表；因为 **CDC 按列名解析 binlog 行**（ES 文章投影与 image-ref 两条链路都从行快照按列名取值，不是只有 ES 投影）。
例外边界写死在"新建表"这一侧：**新建表本身不违反零 DDL**，但必须由一条 ADR 指名立项，并走 `schema.sql` + testkit 三处登记。私信模块唯一经 ADR 指名新增的表是 `chat_block_models`（[ADR-0026](./docs/adr/0026-chat-send-side-risk-control.md)）；通用取景表 `picture_crop_models` 曾由 [ADR-0020](./docs/adr/0020-zero-ddl-exception-and-generic-framing-table.md) 立项、已随取景退场在 #205 物理下线（该 ADR 由 [ADR-0015](./docs/adr/0015-banner-framing-as-parameters.md) 取代）。**既有表的列变更没有任何例外**（含 ALTER、DROP 后重建、加列、删列）——说"没有登记例外"指的是这一侧。
位置：硬约束 A1，见 `docs/architecture/overview.md` §六、`README.md` §11.3 与 §十三；新增表仍走 `schema.sql` + testkit 三处登记。
_Avoid_: 把"迁移"理解成 ALTER——本项目没有 ALTER 通道；把"允许新增表"读成"允许重建既有表"；把已下线的通用取景表当作现行登记例外；把"没有登记例外"读成"不允许新增表"（新建表只需 ADR 指名，不是例外）。

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

**data 载荷（ListData / HasMoreListData / EmptyData）**：
三种 data 形状，必须按 handler 实际调用的 helper 选：`OkWithList`→`{list,count}`、`OkWithHasMoreList`→`{list,has_more}`、`OkWithMsg`→`{}`；九段运行时配置各有具名形状，不再使用自由键值块。
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
位置：表 `runtime_config_models`；九段 `site/ai/mail/storage/oauth/log/upload/deco/image_gc`，由 `backend/internal/platform/config/registry.go` 的段注册表一处登记，漏登记在写库前失败（[ADR-0029](./docs/adr/0029-config-section-registry-and-nine-named-sections.md)）。
_Avoid_: 旧表 `runtime_site_config_models`（已无代码读）、列化存储（零 DDL 下每加字段都要改表 = 不可行）、参数化 `/site/{name}` 与 `map[string]any` 自由键值块（已退场，九段各有具名响应/请求形状）。

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

**引用账本**：
`image_ref_models` 中"哪张图被谁引用"的明细行集合，由 image-ref 从 binlog 维护。
是**明细行**而不是计数：没有引用数字段，"被引用几次"要按 `image_id` 反查行数。
引用靠**从 URL 反解 `object_key`**建立，因此只覆盖"以图片存储前缀开头"的 URL；引用源目前是文章正文/封面、轮播封面、收藏封面、用户头像四类。
易混：账本**不是**图片表 `image_models` 的字段，它是独立的一张表。

**孤儿图**：
判定结果，指"`image_models` 里存在、但没有任何引用账本行指向它"的图。
不是 `image_models.status` 的一个值——状态里的"审核中/封禁"来自存储侧审核，与本判定无关。
典型成因：编辑器上传后未保存文章、文章创建失败、正文删掉图引用。
易混：与"零引用"不是一回事——账本可能漏记（CDC 有死信、引用靠 URL 反解），所以判定孤儿不等于确认无人使用。

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
位置：`bash scripts/check.sh [all|backend|frontend|gates]`，开关 `BLOGX_FAST` / `BLOGX_NO_CACHE` / `BLOGX_SKIP_INTEGRATION`；具体命令不写在这里，见「门禁清单 / 门禁 runner」。

**门禁清单 / 门禁 runner（scripts/gates/）**：
门禁清单只有一个来源 `scripts/gates/gates.json`；本地门禁、持续集成、门禁自检三处都经 `node scripts/gates/run.mjs` 调同一批命令，工具版本与公共安装步骤经 `scripts/gates/install.mjs` 读 `scripts/tool-versions.env`。
位置：`scripts/gates/`（`gates.json` / `run.mjs` / `install.mjs` / `tools.mjs` / `registry.mjs`）；加一道门禁只改 `gates.json` 一处，并登记它对应的演练节号。
_Avoid_: 在 `check.sh`、`ci.yml`、`verify-gates.sh` 各写一遍命令或版本字面量（口径必然漂移，且加一道门禁要改三处）。

**archcheck**：
Go AST 实现的架构守卫，替代会"永真通过"的 PowerShell 正则守卫；规则 R1–R12，每条有 ID 与 slug。
位置：`backend/tools/archcheck/`（`rules.go`、`rules_id.go`）；flag `--mode=error|warn`、`--rule`、`--max-violations`。
_Avoid_: 用 grep/正则匹配符号名做架构检查（别名与换行能绕过，且 CI 跑不了 PowerShell）。

**棘轮（--max-violations）**：
存量违规基线，只准减不准增；基线数字只在 `scripts/arch-baseline.env` 定义一次，经门禁清单里的 archcheck 门禁由 runner 读一次并填进命令。
位置：`scripts/arch-baseline.env`（当前 0），消费方是 `check.sh`、`verify-gates.sh`、`.github/workflows/ci.yml` 三处共用的 `node scripts/gates/run.mjs --group backend`。
_Avoid_: 把基线数字写死在别处（曾出现"本地降到 0、CI 还停在 250"）。

**规则自检（rules_test.go）**：
每条 archcheck 规则都必须能在"人造违规"上报错，否则说明这条规则失效。
位置：`backend/tools/archcheck/rules_test.go`；新增规则必须同时补人造用例。

**门禁自检（verify-gates.sh）**：
主动注入违规来证明每道门禁真的会红（14 道 canary），而不是永远绿；开跑前先核对「清单里的每道门禁都登记了演练」。
位置：`bash scripts/verify-gates.sh`（默认全跑；`--only <节号>` 只跑受影响演练）；CI 里由 `gates-canary` job 调用同一脚本，不另写一份注入逻辑。
_Avoid_: 给门禁自检加"整体跳过"开关（`--skip` / `BLOGX_SKIP_GATES` 之类一律拒绝；零项选中按失败处理）。

**假绿 / 门禁撒谎**：
门禁"看起来通过但实际没执行"的失效形态：测试缓存命中、静默跳过、子 shell 吞掉退出码、生成器静默跳过整份写入仍 exit 0。
位置：`docs/architecture/governance.md` §1.3；对策是跳过的步骤必须打印警告、CI 把跳过变成失败。
理由：用「生成后有没有差异」当契约判据时，生成器静默跳过时差异为空，检查会误判为"契约未过期"而全绿；所以契约未过期判定改成"重新生成到临时文件逐字比对 + 条目数量不下降 + 本次新增结构在场"。
_Avoid_: 用 `git diff --exit-code` 当契约判据（生成器静默跳过时差异为空，且工作区脏会假红，人习惯性忽略）。

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
位置：读者站 `frontend/app/components/deco/CursorCharm.vue`（挂件组件）与 `frontend/app/composables/useCursorCharm.ts`（驱动它的 composable）；素材复用 `utils/brand.ts` 的 `logo` 资产（与 `favicon.ico` 同一张画）。
_Avoid_: 叫它「鼠标挂件」（触控板用户没有鼠标，本仓库一律说「指针」）；用「鼠标追随特效」「cursor effect」指代它（通用词，也指 `usePointerSpecular` 那类非挂件效果）。

**挂绳（tether）**：
光标挂件里连接指针与站标的那条绳：**指针端 = 指针热点 + 指针端偏移**（`deco.charm.pointer_offset_x` / `pointer_offset_y`，默认右下 3px，每轴 -24–24，可在后台调整；`0` 是合法值，等于指针端正好压在热点上），站标端自由，形状与摆动由物理模拟逐帧求出，不是一条静态描边。
位置：同「光标挂件」。这个偏移只在**指针端节点**一处换算，渲染层不得再单独偏一次 —— 否则弧垂与张力都算在错的原点上，快速拉扯时起点会与绳子脱节。
_Avoid_: 把它当成挂件的描边或阴影；用「绳子」「线」当正式名；说"指针端固定在光标上"（固定的是"热点 + 偏移"这个点）；把「指针端」叫「绳头」（旧叫法，已退役）。

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
**唯一的内容面例外**是「合集/项目文件夹卡」的前盖 flap —— 只有 flap 一层，背板与封面卡仍是不透明面，且必须带 `@supports not (backdrop-filter)` 与 `prefers-reduced-transparency` 降级；登记见 [ADR-0012 后续修订（2026-11-07）](./docs/adr/0012-neutral-glass-visual-system.md)。
_Avoid_: 在内容面/正文上叠玻璃（文件夹卡的 flap 是登记过的例外，别处一律不适用）；把玻璃当成"装饰性半透明"到处铺（`backdrop-filter` 会让元素成为 backdrop root，嵌套玻璃只会读成平灰）。
易混：玻璃层 ≠ 半透明卡片——前者是控制层材质，后者是非法用法。

**圆角阶梯（radius scale）**：
五个圆角档位 `--radius-sm/md/lg/xl/pill` 的统一口径（控件 8–14、卡片 20、浮层 28、胶囊 pill），取代旧的"圆角归零"；嵌套层的子圆角 = 父圆角 − 内边距。
位置：`tokens.css` 的圆角组；Naive `themeOverrides` 的 `borderRadius` 从它取值。
_Avoid_: 在组件里写 `rounded-[18px]` 一类裸圆角字面量。

**首屏封面（editorial hero）**：
首页顶部的**满幅图形封面**，是全站唯一的"编辑部式例外区"：横向色带、超大 h1、细网格、标题下发丝基线、封面四角裁切标记、下沿刻度、方括号/箭头记号、胶囊小签，视觉素材取自运营上传的首张轮播封面。它没有卡片边框与纸面底色，轮播控件并入其中；记号与下沿刻度构成右下角整组。
位置：`frontend/app/pages/index.vue` 首屏区块与 `frontend/app/components/article/BannerCarousel.vue`；取舍见 [ADR-0013](./docs/adr/0013-editorial-hero-cover.md)（含 2026-11-03「后续修订」：右栏线条场退役）。
_Avoid_: 叫它"首屏卡片 / hero 卡片"（卡片形态已废）；把它当成可复用的版式模板（只允许出现在首页首屏）；用「线条场」指代已删除的那 15 条横条（它是历史实现，不是版式元素）。

**轮播封面（banner cover）**：
首屏轮播面板里的那张图，是运营在**上传前就按 16:9 定形好**的成品文件；面板比例固定、不随屏宽变化，因此同一张图在任何屏幕上是同一个形状，读者端固定居中渲染、不读任何构图参数。
位置：后台「媒体运营」的轮播图列表（选图 → 定形预览 → 上传）与读者站首屏面板；上传后契约里只有封面 URL，没有焦点 / 缩放字段。
易混：轮播封面 ≠「首屏封面」——后者指整个编辑部式版式区块，轮播封面只是它里面的视觉素材。
_Avoid_: 叫它"横幅图 / banner 海报"（它是首屏面板里的封面图，不是通用横幅）；再说"重新取景 / 调焦点 / 改缩放"（取景体系已退场，改构图只能重选原图重新定形）。

**头像定形（avatar squaring）**：
个人头像在**上传到服务器之前**于客户端完成的文件级改动：把任意比例的图片**裁成 1:1 并降采样到固定边长（256）**，产出的是一张新 PNG，原图不上传、不入库。它改动的是**图片文件本身**；封面定形同理——轮播 / 文章 / 合集 / 项目封面也在上传前按 16:9 裁切并降采样（见 [ADR-0015](./docs/adr/0015-banner-framing-as-parameters.md)），两者是并列且互不包含的两个概念，都不再有"参数 + 渲染层复现"的中间态。**裁剪区域怎么来**：与目标比例相差 ≤1% 时自动居中裁；不合规时弹出**裁剪窗**由作者自己选区域（拖动 + 1–3× 缩放，确认后把结果烤进新文件，取消即中止本次上传），两条路径的产物都是新文件，任何裁剪参数都不入库。
位置：`frontend/app/pages/studio/settings.vue` 的头像上传入口与其客户端处理工具；封面对应在创作页 / 后台媒体运营 / 合集 / 项目的封面上传处；裁剪窗与定形工具由两端逐字节同源的 `imageShaping` 家族提供。取舍见 [ADR-0015](./docs/adr/0015-banner-framing-as-parameters.md)（封面上传前定形）。
_Avoid_: 叫它「取景」「crop」（取景体系已删除）；说成"改已有头像文件"（每次上传产生的是新文件）；把它当成服务端或存储侧裁切——裁剪窗只在浏览器里、上传前出现。

**品牌资产例外（brand asset exception）**：
`brand/logo-256.png`、`brand/jelly-flat.svg`、`apple-touch-icon.png`、`og-image.png` 允许自带品牌色，不受"只消费语义色 token"与漂移门禁约束（同 App 图标惯例）；但品牌资产**不随主题切换变体**。
位置：`frontend/app/utils/brand.ts` 的 `BRAND_ASSETS`（`logoWhite` 按 ADR-0012 批次 0 删除）。
_Avoid_: 再造一张"暗色专用 Logo"——`logo-512-white.png` 是无 alpha 通道的白底方块，正是不透明白边的来源。

**动效档位（motion tier）**：
动效预算的分级名字，现行是**档位 C**：三档时长 120 / 200 / 320ms + 单一缓动 + stagger 40ms，允许全站区块级滚动进场（位移 ≤12px）、视差 ≤10px、导航随滚动转玻璃；正文段落不进场，且强制尊重 `prefers-reduced-motion`。
位置：`tokens.css` 动效层与 `frontend/app/utils/motion.ts`（同一份预算，两处必须同改）。
档位 C 在 **JS 侧另有第二套预算**：`utils/motion.ts` 的「弹簧预设」，目前只有三处消费者（标题大纲飞行器、目录树沟槽圆点、消息中心接缝位移），是登记过的例外；CSS 侧仍只用三档时长 + 单一缓动。登记见 [ADR-0012 后续修订（2026-11-07）](./docs/adr/0012-neutral-glass-visual-system.md)。
_Avoid_: 用"档位 B"描述现状；在 `prefers-reduced-motion` 下仍保留进场（弹簧在该偏好下必须落位为静止，不是放慢）；在组件里内联弹簧数字。

**合集/项目文件夹卡（folder card）**：
读者侧合集列表与项目列表的卡片形态：一张文件夹（背板 + 前盖 flap）里插着**一张** 16:9 封面卡，hover / 键盘聚焦时卡片上抽、flap 前倾，点击直接进详情。里面的封面就是该合集/项目已有的单张 `cover`（空封面时后端已回退到首篇公开文章封面）。
位置：两页共用的文件夹卡组件（新建），消费方 `frontend/app/pages/collections/index.vue` 与 `frontend/app/pages/projects/index.vue`；形态与交互口径见[阅读页 UI grill 记录](./docs/notes/2026-11-07-reading-ui-grill.md) §三 3.6。
_Avoid_: 叫它"3D 卡片"（它是文件夹，不是可翻转卡片）；说"里面放三张封面"（只放一张，三张扇形方案已被否）；把「文件夹卡」与后台的树编辑器节点混为一谈。

**沟槽圆点（gutter dot）**：
阅读页左边缘抽屉目录里指示「当前文章」的圆点：横向位置随该行的层级落在**该层的发丝引导线上**（线从圆点中穿过），换文章时沿位移向量画弧滑过去；`hover` 不动它，`depth 0` 落在最外侧缩进起点。
位置：`frontend/app/components/outline/OutlineTree.vue`（文档树本身），驱动见 `frontend/app/utils/motion.ts` 的弹簧预设。
_Avoid_: 叫"指示器""小圆点"（压线是它的定义，不是装饰）；把它当成悬停反馈或滚动进度。

**接缝（seam）**：
消息中心 tab 条的一体化分段控制里，激活块与相邻块之间被拉开的那道**间隙**。张开量由弹簧驱动，`margin-left` 是该条唯一允许布局动画的地方；激活块两侧（敞口侧）给圆角，闭合接缝侧为 0，免得贴着时啃出缺口。
位置：`frontend/app/components/inbox/InboxSubNav.vue` + `frontend/app/utils/subnavNeck.ts`（`seam*` 几何）；弹簧预设见 `frontend/app/utils/motion.ts` 的 `SPRING_NECK`。
_Avoid_: 叫「颈部」——参考组件里那条「凹曲连接面 + 渐变」的颈部已在 2026-11-07 返工时被明确**去掉**：激活块现在是独立的圆角块，接缝里不画任何连接形状。也不要用它指代激活块本身。

**弹簧预设（spring preset）**：
`utils/motion.ts` 里集中定义的 stiffness / damping / mass 参数组，是弹簧动效的唯一来源；组件只能引用预设，不得内联数字。它与「动效档位」并列，是后者在 JS 侧的第二套预算，不是替代。
位置：`frontend/app/utils/motion.ts` 的 `SPRING_*`；由两条设计测试锁定（reduce 下必须落位、组件不得出现裸 stiffness/damping）。
_Avoid_: 在组件里写 `{ type: "spring", stiffness: 200 }`；把弹簧当成"档位 C 的替代"。

> 说明：本节新增词条（中性石墨阶、功能蓝、液态玻璃层、圆角阶梯、品牌资产例外、动效档位）出自 [ADR-0012](./docs/adr/0012-neutral-glass-visual-system.md)。该 ADR 已**交付**（落地分票 #18–#25 已全部合入），上述词条描述的都是当前生效的实现口径；「语义色 token」词条中两端逐字节同源的约定已由 `scripts/check-token-parity.mjs` 保证，两份 `tokens.css` 现逐字节一致。
> 「轮播封面 / 头像定形」两个词条源自封面与头像的上传前定形：封面口径见 [ADR-0015](./docs/adr/0015-banner-framing-as-parameters.md)（状态**已交付**，2026-11-03 正文整体重写）；词条只定义概念，取舍（为什么改成上传前定形、为什么不存参数、历史封面为什么不迁移）见该 ADR。旧的「取景 / 焦点 / 缩放」三条词条已随取景体系退场删除（2026-11-03，#205）；`crop` 契约字段、`banner.crop_invalid` 错误码与通用取景表在代码、契约与建表脚本中均不存在。

---

## 九、文章发布与运营位

**热门置顶（运营位）**：
首页首屏之后由管理员挑选并手动排序的文章位，最多 12 条；这 12 条按表里的行算，**失效行也占名额**（要在后台「热门置顶」页清理后才腾出来）。唯一真相是 `article_hot_top_models`（`article_id` 唯一 + `sort_order` + 软删），**不进 ES**；文章后来不再公开时置顶行保留、首页不展示，后台标「已失效」并可一键清理。公开读受「显示非管理员发布文章」开关约束：开关关闭时作者不是管理员的卡片不出现（与首页列表、搜索、详情同一口径）。
位置：表 `article_hot_top_models`；接口 `GET /articles/hot-top`（公开读卡片）、`GET /articles/hot-top/admin`（管理端全量，含失效行）、`POST|DELETE /articles/{id}/admin/hot-top`、`PATCH /articles/hot-top/order`；后台「热门置顶」页与文章管理列表的切换按钮（两者都读 admin 那份全量列表）；读者站 `frontend/app/components/home/HomePinnedSection.vue`。取舍见 [ADR-0017](./docs/adr/0017-publish-display-switches-and-hot-top-operator-slot.md) 决策 4–5。
_Avoid_: 「管理员置顶」（ES 旧派生字段 `admin_top` 的叫法，已删除）；「精选文章」（旧文案）；拿「热门置顶」指作者置顶。

**合集（collection）**：
由**管理员**维护的、有标题 / 描述 / 封面 / 展示开关与顺序的文章集合（"Nuxt 部署实战"这类专题）。与「热门置顶」是两套数据：合集没有条数上限，且主体是"一个有名字的集合"而不是首页排序位；作者不参与创建，读者只读。唯一真相是主表 `collection_models`；**收录关系与层次由「合集目录树」承载**（见本节「合集目录树」词条，[ADR-0028](./docs/adr/0028-named-outline-trees-as-the-only-shape.md)），旧的扁平中间表 `collection_article_models` 已下线（#225：代码不再登记，生产库由部署时手工 DROP）。失效口径沿用原来那套：文章后来不再公开时树节点**保留**，读者侧靠 JOIN 过滤、后台标「已失效」并可清理。合集**不进 ES**（运营编排不是可检索内容）。读者侧列表只出「展示中」的合集，篇数只算仍公开的文章；未展示的合集对读者按**不存在**处理（不返回"这个合集未发布"），管理员仍可打开。封面为空时读模型回退到首篇公开文章的封面（即树中深度优先的第一个文章节点）。
位置：表 `collection_models` + `collection_outline_node_models`（DDL 见 `backend/internal/platform/mysql/ddl/schema.sql`）；接口 `GET /collections`、`GET /collections/{id}`（公开读，detail 回嵌套树）、`GET|PUT /collections/{id}/outline-tree`、`GET /collections/admin`、`POST /collections`、`PATCH /collections/{id}/display`；后端 `internal/article/domain/collection`（规则与读模型）、`app/collection.go`、`adapter/mysql/collection_repo.go`、`adapter/http/handler_collection.go`；读者站 `frontend/app/pages/collections/` 与顶栏第四项 `frontend/app/utils/chrome.ts`。
_Avoid_: 「专辑」「专栏」「系列」（同义词，一律写合集）；用「合集」指用户自己的收藏夹（那是 `favorite_models`，与合集无关）；用「合集」指 ADR 归档（`docs/archive/v1/04-adr-collection.md` 是旧文档合集，不是这个功能）。

**项目（project）**：
读者站顶栏第五项所指向的内容实体，与「合集」是**两套独立数据、两个入口**（合集仍在第四项）。在内容语境下「项目」专指这个实体：它的列表页、卡片、空态文案一律写「项目」，不混用「合集」字样；页面版式与组件复用合集的读者侧实现，但数据来源独立。
易混：`backend/conf/site/enter.go` 的 `Project` 段是**站点项目配置**（站点项目名 / 图标 / 前端入口路径），契约字段 `project_title` 属站点配置域，不是内容实体；文档里「本项目 / 这个软件项目」一律指**本仓库**，写作时改成「本仓库」。
位置：表 `project_models` 与 `project_outline_node_models`（收录与层次由「项目目录树」承载，旧中间表 `project_article_models` 已下线，#225 之后代码不再登记它、生产库由部署时手工 DROP）；接口 `GET /projects`、`GET /projects/{id}`、`GET|PUT /projects/{id}/outline-tree`；读者站 `frontend/app/pages/projects/`、后台「项目管理」与树编辑器。读者侧版式与合集复用（同网格、同卡片、同空态），数据来源与表各自独立。
_Avoid_: 用「项目」指站点项目配置或本仓库；用「项目」指顶栏第四项的「合集」；把两者当成同一份数据的两个入口。

**合集目录树（collection outline）**：
合集内部的**层次与顺序**：由「文件夹节点」（只分组、不可阅读、不可点击）与「文章节点」（指向一篇文章）组成，可任意深度嵌套。一篇文章可以同时出现在多棵树里，所以"属于哪棵树"是**节点**的属性，不是文章的属性（同一棵树内最多一个节点）。读者侧合集详情页就是这棵树——点文章节点跳到阅读页并带 `?from=collection:<id>` 来路；阅读页左侧显示同一棵树（当前节点高亮 + 上一篇/下一篇），窄屏收进「目录」抽屉。后台用一个树编辑器编排。
位置：表 `collection_outline_node_models`（`owner_id` / `parent_id` / `kind` / `title` / `article_id` / `sort_order` / 软删）；接口 `GET|PUT /collections/{id}/outline-tree`；后端 `internal/article/domain/outline`（两个实体共用的纯规则）、`internal/article/app/named_outline.go`、`adapter/mysql/named_outline_repo.go`、`adapter/http/handler_named_outline.go`；读者站合集详情页与阅读页左栏、后台合集编辑弹窗的树抽屉。取舍见 [ADR-0028](./docs/adr/0028-named-outline-trees-as-the-only-shape.md)。
易混：文章内的标题大纲叫**标题大纲**（`ArticleTocAnchor`），不是合集目录树；「收录」现在就是"把一个文章节点挂到树上"，不再有第二份成员表。
_Avoid_: 「目录树」（旧正名，已退役：它曾指一张同时服务合集与项目的共用表，见 [ADR-0023](./docs/adr/0023-outline-tree-for-collections-and-projects.md)）；「侧栏 / 目录组件」（那是标题大纲）；把文件夹当可读内容；把它与「项目目录树」当成同一份数据。

**项目目录树（project outline）**：
项目内部的**层次与顺序**：节点种类、嵌套规则、失效行口径与「合集目录树」共用一份纯规则，但数据各自独立、归属由表名表达。读者侧项目详情页就是这棵树——点文章节点跳到阅读页并带 `?from=project:<id>` 来路；阅读页左侧显示同一棵树，窄屏收进「目录」抽屉。后台用一个树编辑器编排。
位置：表 `project_outline_node_models`（`owner_id` / `parent_id` / `kind` / `title` / `article_id` / `sort_order` / 软删）；接口 `GET|PUT /projects/{id}/outline-tree`；后端 `internal/article/domain/outline`、`internal/article/app/named_outline.go`、`adapter/mysql/named_outline_repo.go`、`adapter/http/handler_named_outline.go`；读者站项目详情页与阅读页左栏、后台项目编辑弹窗的树抽屉。取舍见 [ADR-0028](./docs/adr/0028-named-outline-trees-as-the-only-shape.md)。
易混：文章内的标题大纲叫**标题大纲**（`ArticleTocAnchor`），不是项目目录树；读者侧版式与合集复用同一份组件，数据来源与表各自独立。
_Avoid_: 「目录树」（旧正名，已退役：它曾指一张同时服务合集与项目的共用表，见 [ADR-0023](./docs/adr/0023-outline-tree-for-collections-and-projects.md)）；「侧栏 / 目录组件」（那是标题大纲）；把文件夹当可读内容；把它与「合集目录树」当成同一份数据。

**作者置顶**：
作者把自己的一篇文章钉在**个人主页**的置顶关系；只有作者本人能设置，与热门置顶是两套数据、两套存储。读者站与后台当前都**没有**消费点（用户页 / 个人主页的文章列表走搜索 type 3），接口保留但不在「显示」开关的读者读面清单内（ADR-0017 决策 4）。
位置：表 `user_top_article_models`、`GET /articles/top?user_id=`、`POST|DELETE /articles/top`、`backend/internal/article/domain/top`；规则 `domain.CanBeTopped`。
_Avoid_: 裸词「置顶」（两种置顶都要带限定语：热门置顶 / 作者置顶）。

**发布与展示开关（site.article 三布尔）**：
控制「非管理员能不能发布 / 能不能存草稿 / 非管理员作者的文章是否展示」的三个布尔，全部默认 `true`，后台同一张「发布与展示」卡片维护；判定在 app 层与读过滤处，不靠前端隐藏。
位置：`backend/conf/site/enter.go` 的 `Article`、`backend/config/site_default_settings.yaml`、`frontend_admin` 站点配置页；取值在 `internal/article/adapter/http/handler.go`（`writePolicy()` / `onlyAdminAuthors()` 每请求现读），判定规则在 `internal/article/domain/rule.go`（`CanBeSeenByReader`），生效点 `internal/article/app/`（`create.go`、`update.go`、`query.go`、`subdomain.go`、`interaction.go`、`hottop.go`）与 `internal/search/app/search.go`。
_Avoid_: 「站点模式」（已删除的 1 社区 / 2 博客，从未生效）；把「允许发布」当成「允许存草稿」。

**受限访客（guest / role=0）**：
鉴权层视同未登录的账号：能浏览公开内容，不能创建、发布、互动，也看不到自己的历史内容；`role=3` 与库里未入枚举的 `role=0` 同待遇。
位置：`backend/app/auth_verifier.go`（每请求重建身份的收敛点）、`backend/internal/article/domain/viewer.go` 的 `RoleGuest`。
_Avoid_: 把 guest 当普通用户（旧行为，已废）。

**浏览历史（view history）**：
读者"读过哪些文章"的个人记录：同一篇文章只留一条（再次浏览刷新时间并顶到最前），因此它是**最近一次浏览**的列表、不是流水账；数据只服务于本人，不参与推荐、不公开。保留期是**数据策略**：超过 90 天的记录由每日清理任务**软删**（与用户手动"移出历史"同一语义，可回滚），改动保留期要同步发布说明。
位置：后端 `backend/internal/article/domain/viewhistory`（`Retention = 90 天`）与 `backend/app/cron_wiring.go` 的每日清理任务；表 `user_view_article_history_models`（`UNIQUE(user_id, article_id)` + upsert）；接口 `GET /api/articles/history`（标题搜索 + `before_updated_at` / `before_id` 游标）、`DELETE /api/articles/history`（按 id 选删）、`DELETE /api/articles/history/all`（清空）——三条删除接口都是软删；读者站独立页 `frontend/app/pages/history.vue`（按日历天分组、搜索、清空、批量选删）。
_Avoid_: 叫它"浏览记录接口"（它是一个用户可见的数据域与页面，不是一组接口的别称）；把它当成流水账（去重后只保留最近一次）；说"历史永久保留"（90 天前的记录会被每日清理软删）。

---

## 十、读者站渲染与缓存

**渲染边界（rendering boundary）**：
读者站"哪些页面付服务端渲染代价"的白名单，唯一来源是 `frontend/shared/readerRenderingBoundary.ts`：服务端渲染只保留 `/` 与 `/article/**`（首页是站点门面，文章页靠它出分享卡片与被收录）；`/users/**`、`/search`、`/studio/**`、`/collections/**`、`/projects/**`（运营编排，不进首屏与匿名缓存）、`/history` 与 `/messages/**`（私人数据，不索引也不进匿名共享缓存）显式 `ssr: false`；`/articles` → `/search`、`/studio/history` → `/history` 由服务端 301 承担。`nuxt.config.ts` 的 `routeRules`、匿名 SSR 缓存的缓存范围、两道渲染边界门禁都从这张表派生；新增 SSR 页面必须先改 [ADR-0019](./docs/adr/0019-reader-rendering-and-cache-strategy.md) 再进白名单。
位置：`frontend/shared/readerRenderingBoundary.ts`、`frontend/nuxt.config.ts`（`routeRules`）、`frontend/test/gates/rendering-boundary.test.ts`。
_Avoid_: 把它当成"哪些页面能进缓存"（缓存范围从它派生，但缓存另受"匿名 / TTL / 整页 HTML"三条约束）；按页面随手加 `ssr: true` 或随手删 `ssr: false`。

**30s SWR（客户端 30 秒 SWR 缓存）**：
"先给旧值、后台静默刷新"的客户端内存缓存：**只有首屏（`cause === initial`）**在 30 秒内返回或重进时直接命中；手工 `refresh()` / 钩子刷新一律绕过缓存、真发请求（ADR-0024，否则 hydration 预热进缓存的匿名 payload 会让登录用户永远拿不到自己的用户态）。超过 30 秒先把旧值渲染出来，再后台刷新、界面不闪。TTL 30 秒、不落盘、不做 per-user 持久化；缓存键含路由参数（文章 id、分页、搜索词）；写缓存前剔除 `is_digg` / `is_favor` / `relation` 等用户私有字段。
位置：`frontend/app/utils/readerSwrCache.ts`（缓存实现）、`frontend/app/composables/useReaderSwrAsyncData.ts`（`useAsyncData` 接线）、`frontend/app/composables/usePagedResourceCache.ts`（分页列表接线）。
_Avoid_: 说它"持久化 / 离线缓存"（它只在本页内存里、刷新即空）；把服务端那层也叫 SWR（那层见「匿名 SSR 缓存」）。

**匿名 SSR 缓存（匿名服务端渲染缓存）**：
对匿名 SSR 结果的**进程内 30 秒缓存**。前提是 SSR 的 HTML 只含匿名内容（点赞/收藏/关注等用户态一律客户端获取），因此登录读者与匿名读者命中同一份缓存，不需要按 cookie 分流、也不会串号。缓存键只有 `pathname + search`，只存 200 + `text/html` + 不带 `Set-Cookie` 的整页响应，TTL 30 秒、内存表上限 500 条；响应头 `x-ssr-cache: HIT/MISS` 供验收与排障。缓存范围从「渲染边界」派生。
位置：`frontend/server/utils/ssrAnonymousCache.ts`、`frontend/server/plugins/ssrAnonymousCache.ts`（Nitro 的 `render:before` / `render:response` 钩子）；门禁 `frontend/test/gates/ssr-anonymous-cache.test.ts`、`ssr-anonymity.test.ts`。
_Avoid_: 说它"按用户缓存"或"登录用户单独一份"（违反 ADR-0019 约束 3，等于把共享缓存变成串号缓存）；用 `routeRules` 的 `swr` 表达它（Nuxt 渲染器注册在 `/**`，node-server 预设下写 `swr` 不生效、只给"已缓存"的假信号）。

**骨架优先（skeleton first）**：
路由切换时**不等数据齐才挂新页**：先用与真实版式同构的骨架占位（`LoadingSkeleton` 的文本条 / 卡片块；文章页另有 `ArticlePageSkeleton`），数据到达后逐块替换。SSR 与首屏 hydration 仍等首屏数据（HTML 不能缺内容），客户端跳转不 await。骨架只吃语义 token、**不做动画**——数据到了就该立刻被真内容替换。
位置：`frontend/app/components/common/LoadingSkeleton.vue`、`frontend/app/components/article/ArticlePageSkeleton.vue`；渲染环境判定在 `frontend/app/utils/readerRenderMode.ts`（`server` / `hydration` / `navigation`）；分页列表用 `usePagedResourceCache` 的 `initialLoad: block | background`。
_Avoid_: 把骨架当装饰动效（闪动 / 渐变不是它，它一秒都不该多留）；用「加载中…」文案代替结构占位（版式会跳）。

> 说明：本节四个词条出自 [ADR-0019](./docs/adr/0019-reader-rendering-and-cache-strategy.md)（状态**已交付**，落地票 #80–#87 已全部合入）；词条只定义概念，取舍（为什么只有首页与文章页保留 SSR、为什么两层缓存都取 30 秒、代价是什么）见该 ADR。

---

## 十一、私信与会话

**消息中心（message center）**：
读者站的顶级页面 `/messages`，把读者可能收到的消息并列为**五个子项**：评论与回复、点赞与收藏、系统通知、全局通知、私信；五个子项各是独立路由（`/messages/{comments,digg,system,global,chat}`，入口 `/messages` 自身落到默认的「评论与回复」）。它不拥有自己的数据：每个子项只是对应模块列表的一个视图；未读数取统一的未读汇总（`GET /api/sitemsg/user` 的五个字段）。
位置：读者站 `frontend/app/pages/messages/**` 与顶栏入口（悬浮展开五子项、子项右侧显示未读数，窄屏收进图标 / 折叠面板）；旧入口 `/studio/inbox` 按 `tab` / `group` 语义重定向到对应子项。后端不新增消息表，只复用 `internal/notification` 与 `internal/chat`。
_Avoid_: 把「消息中心」当作私信页的新名字（私信只是它五个子项之一）；继续用「消息中心」指 `/studio/inbox`（那是旧位置，已迁移并重定向）。

**陌生人私信（stranger DM policy）**：
是否允许陌生人给我发私信由**两级**决定：站点总闸（`site.chat.stranger_msg_enabled`，缺省显式关闭）**且**接收方个人开关（账号设置里的「允许陌生人给我发私信」，默认开启）。任一关闭即拒绝；自己发给自己、双方已有关注或既存会话关系时不做陌生人判定。拒绝时判定结论带**哪一级**：站点级与接收方级各有错误码（`chat.stranger_disabled` / `chat.recipient_stranger_disabled`）与不同文案，不把"对方不想收"说成站点策略。
位置：`backend/internal/chat/domain/chat.go` 的 `StrangerPolicy`（`CheckStranger` 带接收方 id、返回 `StrangerVerdict`）、`backend/internal/chat/module.go`（两级取值）、读者站账号设置页的隐私区、`PUT /users/info` 的可选字段 `stranger_msg_enabled`。
_Avoid_: 说它"只由站点开关决定"（那是旧口径，已废止）；把"陌生人"理解成"没关注我的人"或"没聊过的人"之一——判定按既有关注/会话关系整体算，不是单一维度；把所有拒绝都归到站点级（接收方关掉自己开关时找管理员查不出任何问题）。

**屏蔽（block）**：
我对某个人的单向设置：只挡"对方发给我"（对方发送被拒），我仍能看到与他的历史会话。它只归我所有、方向性、可软删。
位置：`backend/internal/chat/`；表 `chat_block_models`。
_Avoid_: 把它与「拉黑」当同义词（拉黑还额外隐藏会话）；把它当成「陌生人私信」开关（后者是不特定对象的两级准入）；把它当成「会话软删」（后者只清自己那份会话视图，不挡对方发送）。

**拉黑（hard block / mute）**：
屏蔽的更强一级：既挡"对方发给我"，也让我看不到与他的会话（会话列表不出现）。同样只归我所有、方向性、可软删。
位置：`backend/internal/chat/`；表 `chat_block_models`。
_Avoid_: 把它与「屏蔽」当同义词（屏蔽仍保留历史会话可见）；把它当成「陌生人私信」开关或「会话软删」；说成双向生效（A 拉黑 B 不影响 B 那侧）。

**私信（direct message）**：
登录用户之间一对一发送的文本 / 图片消息；发送与接收双方各持一份会话视图，删除只影响自己那一份。它不是系统通知，也不是 AI 对话。
位置：`backend/internal/chat/`（`domain` / `app` / `adapter` / 模块装配 `module.go`）、接口 `/api/chat/*`、WebSocket `/api/chat/ws`；读者站 `frontend/app/components/inbox/InboxChatPanel.vue`（挂在消息中心的私信子项下）；表 `chat_msg_models` / `chat_msg_user_state_models` / `chat_session_models` / `chat_block_models`。
_Avoid_: 用裸词「消息」「聊天」指代私信（站内信、通知、AI 面板都叫消息）；把「站内信」当作私信（那是系统通知面板 `InboxSitePanel`）；把「捏捏」的 AI 面板当作会话。

**会话（chat session）**：
两名用户之间私信的**有序序列**，由双方 ID 双向一致地求出同一个 `session_id`（`domain.BuildSessionID`），因此 A→B 与 B→A 落在同一会话；会话有自己的软删与未读计数，删除会话只清自己那侧。
位置：`backend/internal/chat/domain/chat.go`、`adapter/mysql/chat.go` 的 `ListSessions`；表 `chat_session_models`。
_Avoid_: 把会话当成「消息列表」（后者是会话里的逐条内容）；把「会话」说成「对话窗口 / 聊天室」（前者是数据，后者是 UI）；把还没发出首条消息时的空占位当成另一种会话。

**会话可见集（session visible set）**：
某用户在某条会话里能看到的全部消息：`send_time` 在清空水位之上、未被本人删除，**包含自己发出的消息**（不是只算收到的）。会话列表、消息列表、整会话已读、消息中心角标与 30 分钟并账都消费同一份实现，不再各写一份判定式。
位置：`backend/internal/chat/adapter/mysql/visibility.go`（唯一实现）· `backend/internal/chat/adapter/mysql/chat.go`（消息列表与清空水位）· [ADR-0027](./docs/adr/0027-message-center-ordering-pagination-and-read-receipt.md)。
_Avoid_: 旧名「会话可见与未读真相」（已退役，正名拆成「会话可见集」与「会话未读数」）；把「可见集」当成「未读数」（后者是前者的子集）；把它当成「会话列表」；只算收到的消息（自己发的也在可见集里）。

**会话未读数（session unread count）**：
会话可见集里再加一个条件——接收者是我、且消息状态未读——得到的条数；会话 badge、消息中心角标、整会话已读与并账必须给出同一个数。
位置：`backend/internal/chat/adapter/mysql/visibility.go`、`backend/internal/chat/adapter/mysql/unread_reconcile.go`、消息中心聚合（`backend/internal/notification`）· [ADR-0027](./docs/adr/0027-message-center-ordering-pagination-and-read-receipt.md)。
_Avoid_: 旧名「会话可见与未读真相」（已退役，正名拆成「会话可见集」与「会话未读数」）；把未读数说成可见集；用推送增量或客户端缓存当真相；把站内信 / 系统通知的未读混进来。

**私信发送资格（chat send eligibility）**：
一次发送前必须判完的五项准入：站点总闸（`site.chat.stranger_msg_enabled`）、接收方个人开关、屏蔽 / 拉黑分级、按对限流预占、图片来源（仅图片消息）。判定点唯一，依赖不可用或查询异常一律失败关闭（拒绝发送），不存在"跳过这一项"的放行分支。管理员视图不过滤拉黑是**有意例外**，不属于发送资格。
位置：`backend/internal/chat/app/eligibility.go`（`sendGuard`）、`backend/internal/chat/app/service.go`（`Send`）、`backend/internal/chat/module.go`（五项依赖的装配）· [ADR-0026](./docs/adr/0026-chat-send-side-risk-control.md)。
_Avoid_: 「发送侧风控」（旧名，已退役）；把发送资格说成「陌生人私信」（那只是其中两级开关）；依赖缺失时放行；把管理员视图的拉黑例外当成漏判。

---

**互动条（article action bar）**：
阅读页里点赞 / 收藏 / 评论 / 分享 / 底部五个动作的那一条**纯图标 + 数字**的按钮带。它有两个**互斥**形态：`aside`（≥1024px 落在右栏，五等分铺满 320px）与 `hero`（<1024px 挂在 hero 下方、随正文滚动）。两种形态是同一个组件、同一份 props，任何视口只出现一份可见；2026-11-04 起底部固定条整体退役（用户反馈过"底部多出一条同样的容器"）。
位置：`frontend/app/components/article/ArticleActionsBar.vue`（文案在 `utils/articleActions.ts`）、样式在 `frontend/app/assets/css/tailwind.css` 的 `.article-actions-bar*`。
_Avoid_: 说它是"工具栏"或"操作按钮组"（容易与编辑器工具栏混淆）；把它当成状态持有者（它只吃 props、只抛事件，真相在 `useArticleDigg` 与正文）。

**评论输入面板（comment composer sheet）**：
从**窗口底部居中滑出**的一块纸面，宽 `min(92vw, 640px)`，内含评论输入本体与唯一的提交按钮；面板外是一层**透明**点击捕获层（不是暗色遮罩，读者要一边看正文一边写）。三条关闭途径：点面板外、Esc、右上 ×。打开时不锁 body 滚动，上限 `60dvh`。它是**唯一**的评论撰写入口——评论区的内嵌 textarea 已退役，改成「去发表评论」按钮；互动条的「评论」与楼层「回复」也打开同一个面板。
位置：`frontend/app/components/comment/CommentComposerSheet.vue`（壳）+ `CommentComposer.vue`（输入本体），样式 `.comment-sheet*`。
_Avoid_: 叫它"弹窗 / modal"（它无遮罩、无居中卡片、不锁滚动）；把它与「去发表评论」按钮混为一谈（后者只是入口）。

**到底按钮（jump-to-end action）**：
互动条第五个动作，图标-only、无计数，点它滚到评论区起点（= 正文末尾，页脚不算内容）。它**只负责滚动**；"写评论"是「评论」按钮与「去发表评论」按钮的职责，两者语义不要混。
位置：`ArticleActionsBar.vue` 的 `data-action="bottom"`，滚动实现是页面里的 `handleScrollToEnd → scrollToComments`。
_Avoid_: 说它"跳到页脚 / 回到顶部"；把它的 aria 名称写成"评论"。

**计数账本（count ledger）**：
点赞 / 收藏数的**派生值**口径：这两个数不再靠增量累积，并账时用 `COUNT(关系行)` 直接覆盖整列（`digg_count` / `favor_count`）；两次并账之间，权威值 = `持久列 + Redis 增量`，增量是短命的、不参与持久化。视图数与评论数仍是另一套口径（`列 += 增量`）。并账周期是**运行期配置**（`site.reconcile.interval_minutes`，默认 30 分钟），文章计数与私信未读跑在同一个调度任务里、失败彼此隔离。权威值只有一个读入口（`app.CountLedger`）；旧的「每个读点各自把列与增量相加」是**被取代口径**。因此"临时 ±1"不会累积成漂移，CDC 实时并账是明确的非目标。
位置：`backend/internal/article/adapter/mysql/counter_flush.go`（并账）、`backend/app/cron_wiring.go`（任务装配与周期来源）、`backend/internal/article/app/ledger.go`（唯一权威读入口）；决策见 [ADR-0025](docs/adr/0025-count-ledger-and-reconciliation.md)。
_Avoid_: 把"列 + 增量"当成一处**读路径口径**（它只是账本主值的构成：读取一律经账本入口）；用"实时对账 / CDC 并账"描述它（那是被否掉的方向）；把并账周期写回代码常量。

---

## 十二、发布治理（闸门 / 配额 / 机审）

> **本节词条全部处于「已定、未交付」状态**（2026-11-07 grill 决定的形态，代码尚未落地）。词条只定义概念，"为什么这么选、否掉了什么、代价是什么"见 [发布闸门与机审 grill 记录](./docs/notes/2026-11-07-publish-gate-and-moderation-grill.md)；落地后本节各词条的"位置"再补代码路径。

**发布质量闸门（quality gate）**：
"提交后先评分、分数够才允许继续"的这道**硬闸**，与人工审核是**两道彼此独立的开关**：闸门管"有没有资格进队列"，人工审核管"进了队列放不放行"。判定发生在提交那一刻，且必须在**数据库事务之外**（评分是一次 5–30 秒的模型调用，占着事务等于钉死连接池）。
位置：概念见 [grill 记录](./docs/notes/2026-11-07-publish-gate-and-moderation-grill.md) §2.1；决定见 ADR-0030（提议）。
易混：它与「人工审核」不是同一条流水线的两段，而是可以分别开关的两道门；它也不是评分本身——评分是"给出分数"，闸门是"拿分数做决定"。
_Avoid_: 把「闸门」当成评分的同义词；说"提交即评分即发布"（中间还有一道分数判定）；把闸门判定写进事务里。

**评分失败（score unavailable）**：
"没拿到可用分数"这一种结局：网络错、超时、429/5xx、返回非 JSON、JSON 里没有总分。它与「评分未达标」是**两个不同的错误码**——前者是系统没给出结论，后者是系统给出了"不够好"的结论。站点策略是失败关闭（拒绝本次投稿），但**因故障失败的投稿不消耗当日配额**。
位置：概念见 [grill 记录](./docs/notes/2026-11-07-publish-gate-and-moderation-grill.md) §2.1 决策 5–7。
易混：低分（哪怕 23 分）**不是**评分失败，它是有效结论。
_Avoid_: 把"AI 给了低分"说成"评分失败"；把超时与低分报成同一个错误码。

**灰带（gray band）**：
发布阈值附近的一段**不算通过、也不算终局**的分数区间（当前定义为阈值 ±2）。落进灰带的文章不发布，文案要明说"差几分"，让作者按评审意见改一版再投；它的存在是为了不让 49 与 51 变成"随机打回"。站点关了人工审核时，灰带**不**自动转人工，也不放宽阈值。
位置：概念见 [grill 记录](./docs/notes/2026-11-07-publish-gate-and-moderation-grill.md) §2.1 决策 9 与 §七。
易混：灰带不是"宽容带"（不会偷偷把阈值降 2 分），也不是"待审区"（不产生审核任务）。
_Avoid_: 用灰带指代人工审核队列；说"灰带内可以发布"。

**投稿配额（submit quota）**：
每名普通用户每个自然日允许**提交**的次数上限（默认 15），用来把灌水成本关在门外。计量单位是**提交动作**而不是"发布成功的文章"：被打回也计，因为每次提交都会真实触发一次模型调用与一条审核任务。管理员豁免；因评分故障而失败的提交不计。刷新靠"按日期分桶的计数键自然过期"完成，不依赖定时任务。
位置：机制沿用私信的同名做法（`internal/platform/redis/redis_chat/risk_limit.go`）；概念见 [grill 记录](./docs/notes/2026-11-07-publish-gate-and-moderation-grill.md) §2.3；决定见 ADR-0032（提议）。
易混：它不是"发布上限"（被打回也计），也不是"存草稿的上限"（草稿不消耗）。
_Avoid_: 把「投稿」与「发布」「存草稿」混用；说"每天能发 15 篇"。

**三层机审（three-layer moderation）**：
人工审核之前的**自动筛查**，按成本从低到高分三层：① 纯代码高危词匹配 ② 从已有评分结果派生的风险标签（低分 / 维度塌陷 / 篇幅异常） ③ 模型合规判定。前两层是**给定文本就能重算的纯函数**，不落库；只有第三层（花钱、有延迟）才落一条风险记录。三层都能拒稿，但只有 ① 的"阻断级"与 ③ 的高置信违规是自动拒，② 与"观察级"只标红交人。
位置：概念见 [grill 记录](./docs/notes/2026-11-07-publish-gate-and-moderation-grill.md) §2.4；决定见 ADR-0031（提议）。
易混：它不是"机审替代人工"（第三层只给结论、裁决权仍在人工与阻断级词表）；② 也不是"再评一次分"（它复用已有分数，零新增调用）。
_Avoid_: 把三层说成"三次 AI 调用"；把风险标签当成文章的字段（它是风险记录，文章表不新增列）。

**阻断级词表 / 观察级词表**：
高危词的两个档：**阻断级**是"命中即自动拒"的小而准清单，**观察级**只标红、把决定交给人。之所以要分档，是因为字符串匹配没有语境（"六四"能命中"六十四位"），而自动拒是**不可申诉**的结局。词表是**运行期数据**、由运营热改；"怎么判"的规则才是代码或策略文本。
位置：概念见 [grill 记录](./docs/notes/2026-11-07-publish-gate-and-moderation-grill.md) §2.4 决策 22、27。
易混：它与「运行时策略文本」分工不同——词表是**判哪些词**（数据），策略文本是**怎么判**（规则）。
_Avoid_: 把两级词表合成一个"敏感词库"；把词表写进代码常量。

**风险记录（risk record）**：
一篇文章**最近一次**模型合规判定的结论：结论档（干净 / 存疑 / 违规）、风险级别、命中的类别与依据、判定时的策略版本与模型名。它只存"当前结论"，不存历史；审核列表的红点从它读，文章表与审核任务表**都不加列**（零 DDL 硬约束）。
位置：概念见 [grill 记录](./docs/notes/2026-11-07-publish-gate-and-moderation-grill.md) §四；决定见 ADR-0031（提议）。
易混：它不是「评分记录」——评分判"写得好不好"，风险记录判"能不能发"；两者可以互相矛盾（评分 82 但命中违规），矛盾时以风险结论为准。
_Avoid_: 把它当成文章的 `risk_level` 列；把它的历史价值当成已实现（刻意不存历史）。

**运行时策略 skill（`skills/`）**：
放在仓库根 `skills/<name>/SKILL.md`、随产品进程**启动时读盘**并注入模型调用的文本策略。格式遵循 **Agent Skills 开放规范**（`name` + `description` 为必填，可选 `license` / `compatibility` / `metadata` / `allowed-tools`；`name` 必须等于父目录名），**不自造字段**——产品私有信息只写进 `metadata`（`map[string]string`），这样同一份 skill 脱离本项目也能被任何兼容客户端直接使用。"哪个场景用哪份 skill"是**产品接线**，写在运行期配置里，不进 skill 本体。读不到、格式不合规、或引用了未知场景，一律**启动失败**，不存在内置兜底文本；版本号取 SKILL.md 字节哈希，落进调用记录。
位置：目录形态、加载器契约与校验规则见 [grill 记录](./docs/notes/2026-11-07-publish-gate-and-moderation-grill.md) §五；决定见 [ADR-0033](./docs/adr/0033-runtime-policy-skills.md)；规范见 <https://agentskills.io/specification>。
易混：它与 `AGENTS.md` 里那套「Agent skills」**格式相同、消费者不同**——那三个 `docs/agents/*.md` 是编码 agent 读本仓配置的入口，`skills/` 里这两份是产品运行时读的策略文本；与「阻断级词表」分工也不同（词表是数据、走运行期配置热改；skill 是规则、走代码版本）。
_Avoid_: 把它叫「prompt 目录」（它不只装 prompt 片段，还带规范校验与版本）；在 frontmatter 里自造 `applies_to` / `version` 这类顶层字段（规范只允许 `metadata` 承载私有信息）；在代码里再写一份兜底文案。

---

## 维护规则

1. **什么时候新增词条**：一个词开始被多处代码或文档引用、且读者可能叫错它时（出现同义词、旧名、缩写）才加。一次性数字、某次故障的过程、实现细节都不进本文。
2. **什么时候改词条**：概念本身变了才改（比如 `ctype.ID` 底层类型变化、某个状态机增删状态）。代码搬目录只改"位置"那一行。
3. **选词原则**：同一个概念只留一个正名，其余写进 `_Avoid_`；"易混"只写原文有明确区分说法的那几对，不自行发挥。
4. **与 ADR 的分工**：**词条 = 概念是什么**（不含取舍）；**ADR = 为什么这么选、否决了什么、代价是什么**。ADR 在 `docs/adr/`（一条决策一个文件，编号递增、不覆盖旧决策）；v1 的旧合集归档在 `docs/archive/v1/04-adr-collection.md`，只查历史。词条正文引用 ADR 编号即可，不要把决策论证抄进来。
5. **不收什么**：一般编程概念（超时、错误类型、通用设计模式）、纯 UI 文案、会过期的数量指标（当前 CI job 数、覆盖率的即时数值）——数字写进文档或代码注释，本文只记录稳定概念。
6. **同步义务**：README 与 `docs/architecture/*` 不复制词条正文；首次出现处链回本文即可，避免两处定义漂移。
