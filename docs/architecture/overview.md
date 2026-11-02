# 架构总纲

> **来源**：v1 阶段压缩归档（2026-10-28，由 `README.md` / `01-现状评审.md` / `02-目标架构.md` / `03-物理目录与映射.md` 合并压缩）。v1 已交付（M0–M7 全部完成）；本文记录**"当时为什么这么设计"**——决策背景、被否决的方案、曾经踩过的坑。**§六 是当前仍生效的硬约束总表**。
> 仓库形态：`backend` / `frontend` / `frontend_admin` / `deploy` 四目录解耦（旧 `blogx_monorepo` 已废弃）。

**文档地图**（本目录是现行架构与治理文档的入口）：

| 文件 | 内容 | 什么时候看 |
| --- | --- | --- |
| **overview**（本文） | 诊断、目标架构、目录映射、10 条决策、**硬约束总表** | 开工先看 §六 |
| [governance](./governance.md) | 门禁、archcheck、测试体系、日志管道、产品缺陷 | 改门禁 / 加测试 / 动日志时 |
| [contracts](./contracts.md) | 契约流水线、swag v2 的坑、`contract.ts` 三层工具 | 动 API / 改注解 / 改前端类型时 |
| [configuration](./configuration.md) | 配置分层、后台可改配置、合并规则 | 加配置项 / 排查"改了不生效" |
| [ai-integration](./ai-integration.md) | eino 调用链、模型配置表、供应商映射 | 加供应商 / AI 不可用 |
| [ADR 索引](../adr/README.md) | 20 条 ADR 的决策骨架、理由与当前约束（0010–0019 为呈现层 / 前端行为 / 装饰层 / 渲染与性能策略，0020 为零 DDL 例外授权与通用取景表） | 想改某条规则前，先读它为什么存在 |
| [v1 ADR 合集（归档原文）](../archive/v1/04-adr-collection.md) | 拆分前原文，保留合并口径与归档备注 | 需要对照历史时 |
| [v1 基线与过程](../archive/v1/05-baseline-and-process.md) | 起点数字、M0→M7 时间线、两轮决策留档 | 想知道"这个数字原来是多少" |

---

## 一、执行摘要

### 1.1 规模（评审时基线）

| 模块 | 文件 | 行数 | 说明 |
| --- | --- | --- | --- |
| `apps/api` | 541 Go | 58,508（非测试 37,931） | Gin + GORM 后端 |
| `apps/web` | 118 | 17,219 | Nuxt 读者站 |
| `apps/admin` | 24 | 2,835 | Nuxt 管理后台 |
| `prototype/web` | 20 | 3,312 | **死代码** |
| `prototype/admin` | 15 | 856 | **死代码** |
| `plugin/` | 1 | 711 | 仅 prototype 使用，**随 prototype 删除** |
| **合计（含待删 prototype）** | — | **≈ 83,000** | prototype 7.3k 在 S0 删除后约 76,000（Go 58.5k / Nuxt 20k） |

测试侧：117 个测试文件 / 20,577 行 / **2,357 处断言**；架构违规基线 **324 处**（见 [05](../archive/v1/05-baseline-and-process.md) §一.4）。

### 1.2 一句话诊断

> **这个项目最严重的问题不是"没有架构"，而是"架构是一场表演"。**

它有一整套看起来专业的东西：`app/` 装配层、`platform/` 抽象层、`repository/` 仓储层、`arch_guard.ps1` 架构守卫、`test/architecture/` 架构测试、monorepo + turbo。**但它们几乎全部没有真正起作用**。

最讽刺的一点：`arch_guard.ps1` 和 `test/architecture/` 花了 370 行代码、13 条规则，去禁止 `appctx`、`global`、`MustFromGin`、`DepsFromGin`、`mustApp` 这五个符号——**这五个符号在项目中一个都不存在**。于是守卫全部永真通过，而真正的问题（32 个 `Deps`、61 个 handler 直接查库、空壳 repository、无 CI）**没有任何一条规则在看**。

### 1.3 五个 P0

| # | 问题 | 证据 | 为什么是 P0 |
| --- | --- | --- | --- |
| 1 | **伪依赖注入** | 32 个 `Deps`，30 个含 `*gorm.DB`；传递链最长 6 层 | 只是把 `global.DB` 改名成 `deps.DB`，God Object 换马甲 |
| 2 | **handler 直接查库** | `api/` 61 文件 import gorm，194 处调用 | 分层不存在，SQL 散落在 HTTP 层 |
| 3 | **repository 空壳** | 2,076 行（service 的 9%），`article_repo` 只有 `.gitkeep` | "仓储层"是装饰，连接仍由上层传入 |
| 4 | **架构守卫永真** | 检查的 5 个符号全不存在；`api import repository` 被 `type X = repo.X` 别名绕过 | 治理机制失效，腐化无人阻挡 |
| 5 | **无 CI / lint 空转** | 无 `.github`、无 git hook；`pnpm lint` 因 5 个包都没定义而**永远退出 0** | 2,357 处断言、13 条规则，从不自动执行 |

### 1.4 三个"看起来做了其实没做"

| 表面 | 实际 |
| --- | --- |
| `platform/` 抽象层（3 个子包） | 共 **63 行**，每个都是 `return core.InitXxx(...)` 一行转发，无接口、无第二实现、无 mock 点 |
| `app/` 装配层（5 文件 370 行） | `app.Bootstrap` **从未被调用**；`Lifecycle.OnStop` **0 个调用点**，`stopHooks` 永远为空 |
| openapi 契约（14,462 行 / 118 operation） | 全仓**唯一引用是文件自身**，无 `openapi-typescript`，前端 80 个 interface 纯手写，契约与实现零机械校验 |

### 1.5 目标架构一句话

**合成根（Composition Root）+ 真实端口 + 单向依赖 + 可执行的架构规则 + 契约自动生成**，并且**取消 monorepo、拆成四个解耦的仓库级目录**：`backend/`（Go）、`frontend/`（Nuxt 读者站）、`frontend_admin/`（Nuxt 管理后台）、`deploy/`（Docker/compose/nginx）。

核心是把"依赖注入"从**结构体装箱**改成**接口注入**：handler 不再持有 `*gorm.DB`，而是持有 3-5 个方法的能力接口。配合 `archcheck`（Go AST 实现，跨平台、进 CI）替代 PowerShell 正则；openapi 由后端代码自动生成，前端各自派生类型。

### 1.6 迁移策略一句话

**不做大爆炸重写，但也不做"又一轮形式主义改造"。**

先删死代码（prototype 8,000 行 + 2.1MB dist）建立信心，再补 CI 让现有 2,357 处断言真正跑起来，然后**用一个领域（article）把真 DI 走通**，验证后批量。每一步都必须有"可执行的验收规则"，不接受"看起来做了"。

### 1.7 违规证据分类汇总（324 处）

> 不逐条罗列，按类汇总；每类保留 2-3 条最有力的具体证据（旧路径 + 行号，仅供感受当时的严重程度）。

| 类 | 规模 | 代表性证据 |
| --- | --- | --- |
| **A. 分层失效（越层 SQL）** | api 194 处 / 61 文件；service 91 处 / 40 文件，111 个函数首参是 `*gorm.DB` | `api/article_api/article_detail.go:85-87` `h.App.DB.Model(&models.ArticleDiggModel{}).Where(...).Count(...)`；`api/data_api/data_sum.go:21-31` handler 内拼接 6 个子查询的原生 SQL；`api/article_api/article_write_service.go`（279 行）**在 api 包内又造了一个 service** 绕开 service 层 |
| **B. 伪依赖注入** | 32 个 `Deps`，30 个含 `*gorm.DB`，最长 6 层传递 | `apideps/deps.go:17`（18 字段）与 `api/enter.go:32`（17 字段）**逐字重复**；`app/wiring_http.go:18-37` 与 `:39-57` 同一函数里手工抄两遍；`api.New` 从 `enter.go:70` 到 `:167` 逐字段展开拆出 15 个子 Deps |
| **C. 空壳抽象** | `platform/` 63 行；`app/` 2 条死路径；`repository/` 2,076 行（service 的 9%） | `platform/dbx/init.go:13` `func Init(...) *gorm.DB { return core.InitDB(...) }`；`app/bootstrap.go:89` `Bootstrap` **0 调用点**、`app/lifecycle.go:32` `OnStop` **0 调用点**；`repository/article_repo/` **只有 `.gitkeep`（2 字节）** |
| **D. 治理永真** | 13 条规则 + 6 个测试全部永真；唯一有效规则被别名绕过 | grep `appctx`/`global`/`MustFromGin`/`DepsFromGin`/`mustApp` **计数均为 0**；`service/top_service/query.go:1-8` `type QueryService = repo.QueryService` 纯别名 re-export，于是 `api/comment_api/comment_list_man.go:45` 实际直接构造 repository 的 QueryService |
| **E. 重复与巨石** | api 191 对 / service 563 对相似文件；41 处 20 行以上完全重复块；65 个 `enter.go` / 1,327 行 | `fans_list.go` vs `follow_list.go` 相似度 **0.97**；`service/es_service/article_sync.go` **1,547 行 / 42 函数**；`category_delete.go` 补了 `EmitActionAuditFromGin`，`favorite_delete.go` 复制时**没带过来**⇒ 审计覆盖 21/138 = 15%，哪条链路有审计取决于当初从哪个文件复制 |
| **F. 前端与契约** | 同一后端 3 个别名 + nginx 3 份 location；13,408 行幽灵 lockfile；80 个手写 interface | `apps/web/server/routes/{api,_backend,_origin}/[...path].ts` **三份都指向同一个 apiUpstream**；`nginx.production.conf:74/:115/:124` 三份 location 全指向 `blogx_api_upstream`；`apps/web/pnpm-lock.yaml` 与 `apps/admin/pnpm-lock.yaml` **md5 相同（aaba985a12cd34ae26f1505e21fc8304）** |
| **G. 工程化空转** | 60 次提交从未出现 `.github`/husky/eslint；3 处 `t.Parallel` | `"lint": "pnpm -r --if-present lint"` 而 5 个包**无一处定义 lint** ⇒ 永远退出 0；`repository/`（449 行核心 SQL）**0 测试**、`apps/admin` **0 测试**；3 处 `t.Parallel` 全在 `cron_service/log_cleanup_test.go` |
| **H. 数据层与安全** | 两套 canal 3,231 行；AI 手写 HTTP；112 个 `BLOGX_*` 环境变量 | `service/ai_service/enter.go:109` `client := &http.Client{}`——无超时、无 ctx、无重试、无连接池；`turbo.json:3-6` `globalPassThroughEnv: ["BLOGX_*"]` **密钥与前端变量同前缀并通配透传**；`buildinfo/version.go` 46 字节 `const Version = "1.0.0"`，无 `-ldflags` 注入 |

**一句话归因**：这个项目的代码不是"能力不足"，是**"缺少会让错误暴露的机制"**。一个人能写出单通路的 CDC 同步、死信处理、自包含的测试基建，说明工程能力在线；但因为没有 CI、守卫永真、lint 空转，所有问题都能安静地活到线上。

---

## 二、目标架构形态

### 2.1 仓库四目录划分（取消 monorepo）

仓库根**不再有 `pnpm-workspace.yaml` / `turbo.json` / `packages/`**。三部分各自 `go.mod` / `package.json` 自洽，**唯一的耦合点是后端自动生成的 `openapi.json` 契约**。

```
┌──────────────────────────────┐   ┌──────────────────────────────┐
│  frontend/  (Nuxt 读者站)      │   │  frontend_admin/ (Nuxt 后台)    │
│  ── 只做 UI + BFF 代理         │   │  ── 只做 UI + BFF 代理         │
│  ── 类型由 openapi 生成，不手写 │   │  ── 类型由 openapi 生成，不手写 │
└───────────────┬──────────────┘   └───────────────┬──────────────┘
                │ HTTP / SSE / WS（契约：openapi.json）  │
                └──────────────────┬───────────────────┘
┌───────────────────────────────▼───────────────────────────────┐
│  backend/  (Go)                                                 │
│  cmd/server/main.go  ← 唯一合成根，唯一允许 new 具体实现的地方     │
│  internal/article/    ← 垂直模块（domain / app / adapter）         │
│  internal/{comment,user,auth,chat,social,media,search,ai,      │
│            notification,site,analytics}/                       │
│  internal/platform/   ← 真实适配器（有接口 + fake；含 llm 网关）    │
└─────────────────────────────────────────────────────────────────┘
                │
   MySQL / Redis / ES / ClickHouse / LLM / 七牛

┌───────────────────────────────────────────────────────────────┐
│  deploy/  (基础设施，不属于任一 app)                              │
│  ── docker / compose / nginx；反向代理唯一 upstream 在此定义        │
└───────────────────────────────────────────────────────────────┘
```

### 2.2 合成根（Composition Root）

```go
// cmd/server/main.go —— 唯一允许 import 所有具体实现的地方
func main() {
    cfg := config.MustLoad(flagFile)

    infra := platform.New(cfg)          // DB / Redis / ES / CH / Logger
    defer infra.Close()

    // 每个模块的装配是一个函数，main 只负责连线
    articleSvc  := wireArticle(infra)
    commentSvc  := wireComment(infra, articleSvc.Gate)   // ← 跨模块通过接口
    aiSvc       := wireAI(infra, articleSvc.Projection)

    router := httpiface.NewRouter(articleSvc, commentSvc, aiSvc, ...)
    server.Run(cfg.System, router)
}
```

- `wireXxx` 函数放在各模块的 `adapter/wire.go`，**依赖图完全可读**。
- **约束**：`main.go` 之外任何地方不得 `new` 具体实现；不得存在 `Deps` 这类聚合结构体；不得有包级可变状态。
- **不上 wire / dig**：模块数约 12 个，手写装配约 150 行。代码生成会让启动流程不可读、出错提示晦涩；反射把编译期错误推后到启动期。手写 150 行换来"启动流程就是一张图"。

### 2.3 端口与能力接口

**端口由使用方定义**，不是由实现方定义：

```
internal/article/domain/port.go    ← 仓储端口（聚合根自己的持久化契约）
internal/article/app/port.go       ← 用例需要的外部能力（跨模块、外部服务）
```

```go
// internal/article/app/port.go
type Reader interface {
    FindByID(ctx context.Context, id kernel.ID) (*article.Article, error)
    ListPublished(ctx context.Context, q ListQuery) ([]article.Article, int, error)
}
type DiggCounter interface {
    CountByArticle(ctx context.Context, id kernel.ID) (int64, error)
}

// internal/article/app/service.go
type Service struct {
    reader Reader          // 只读这篇文章，不能写
    diggs  DiggCounter     // 只能数点赞，不能删点赞
    tx     TxManager
    outbox Outbox
}
```

跨模块调用时消费方定义接口（这是**模块解耦的关键：模块之间不 import，只在合成根连线**）：

```go
// internal/comment/app/port.go
type ArticleGate interface {
    CanAcceptComment(ctx context.Context, id kernel.ID) (bool, error)
}
```

相比 `Deps` 装箱的四点收益：**依赖可见**（4 个字段一眼看完）、**能力受限**（`DiggCounter` 调不到 `Delete`）、**可测试**（fake 只实现 1 个方法）、**不可绕过**（没有 DB 连接，想写 SQL 也写不了）。

> v1 落地时 `TxManager` 已演进为 **UnitOfWork**（`WithinTx(ctx, fn(ctx, Repos) error)`，`Repos` 由 app 定义），事务边界在 app 层，Outbox 与业务同事务。

### 2.4 模块划分（12 + platform）

| 模块 | 聚合根 | 现状来源 |
| --- | --- | --- |
| `article` | Article / Category / Tag / Favorite / Top / ViewHistory | `api/article_api`(55) + `article_service`(1) + 3 个 repo |
| `comment` | Comment | `api/comment_api`(15) + `comment_service`(1) |
| `user` | User / Session / Profile | `api/user_api` + `user_service` |
| `auth` | Identity（QQ/邮箱/密码） | `api/user_api/auth_api` |
| `chat` | ChatSession / ChatMessage | `api/chat_api` + `chat_service` |
| `social` | Follow | `api/follow_api` + `follow_service` |
| `media` | Image / ImageRef | `api/image_api` + `image_service` |
| `search` | （查询侧无状态） | `api/search_api` + `search_service` + `es_service` |
| `ai` | （能力型） | `api/ai_api` + `ai_service`(27) |
| `notification` | GlobalNotif / SiteMsg | `api/{global_notif,sitemsg}_api` + `message_service` |
| `site` | SiteConfig / Banner | `api/{site,banner}_api` + `site_service` |
| `analytics` | （只读投影） | `api/data_api` + `log_service`(17) |
| `platform` | — | `platform/` + `core/` |

### 2.5 分层与依赖方向（编译器强制）

```
adapter  →  app  →  domain
   ↓                  ↑
platform ─────────────┘（domain 定义端口，platform 实现）
```

| 层 | 可 import | 禁止 |
| --- | --- | --- |
| `*/domain` | 标准库、`internal/kernel`、`internal/apperr` | app、adapter、platform、gin、gorm、redis、es |
| `*/app` | 本模块 domain、`internal/kernel` | 本模块 adapter、其他模块、gin、gorm |
| `*/adapter` | 本模块 app/domain、`internal/platform`、第三方库 | 其他模块的内部包 |
| `platform` | 标准库、第三方库、`internal/kernel` | 任何业务模块 |
| `cmd` | 全部 | — |

**跨模块三条规则**：

| 需求 | 做法 | 禁止 |
| --- | --- | --- |
| 读别的模块的数据 | 消费方定义窄接口（如 `ArticleGate`） | import 别的模块的 domain 实体 |
| 被别的模块通知 | 发领域事件（Outbox） | 直接调用别的模块的 service |
| 共享值对象 | 放 `internal/kernel`（ID、分页、时间） | 把业务实体放 kernel |

**唯一例外**：`kernel` 可被所有模块 import，但只能有值对象，不能有业务规则。

**从"水平分层"改为"垂直模块"**：现状 `api/` 170 包 / `service/` 170 包 / `repository/` 8 包，切得极不均匀（`article_api` 55 文件 6,012 行 vs `article_service` **1 文件 99 行**，61×）。目标每段模块只分三个包（不是五个）：`domain`（必须独立，可零依赖测试）/ `app`（必须独立，可注入 fake 端口测试）/ `adapter`（HTTP + DB 都属适配器，生命周期一致、都只被合成根构造）。取舍：`adapter` 包会比较大（单模块 2,000-4,000 行），用文件粒度控制；换来包数可控（12 模块 × 3 = 36）。

### 2.6 `backend/` 目标目录树

> **⚠ 读目录树前先看这条**：上面是**设计期目标形态**，M1–M7 落地时有几处调整，**结构以代码为准**。

| 设计里写的 | 实际落地的 |
| --- | --- |
| `platform/cdc/{canal.go, checkpoint.go, projection/}` | `platform/cdc/{canalbase, river, outboxrelay, imageref, deadletter, dlq}` |
| `platform/outbox/` | `platform/cdc/outboxrelay/` |
| `platform/es/projection/` | `platform/es/articlesync/` |
| `platform/redis/` 的 `counter/guard/cache` | `platform/{redis,rediscounter}/` + 业务名子包（`redis_article` 等） |
| `platform/http/response.go` | `platform/http/res/` |

> 其中 `platform/cdc/dlq` 设计期不存在：它是"死信读端活、写端没了"被发现之后补的编排层（见 ADR-008）。

```
backend/
├── cmd/
│   ├── server/main.go                   # ★ 唯一合成根
│   └── cli/{main,migrate,es,user}.go    #   原 flags/（交互式运维命令）
├── internal/
│   ├── kernel/                          # 跨模块值对象（零业务规则）
│   │   ├── id.go                        #   原 models/ctype/id.go
│   │   ├── page.go                      #   原 common/list_query.go 的分页部分
│   │   └── errors.go                    #   ErrNotFound / ErrConflict
│   ├── platform/                        # 基础设施适配器（有接口 + fake）
│   │   ├── config/{static,runtime,loader}.go      # 原 conf/ 22 文件 + core/init_cfg.go
│   │   ├── mysql/{db,tx,hooks}.go                 # 原 core/init_db.go + platform/dbx；hooks = 雪花 ID
│   │   ├── redis/client.go + {counter,guard,cache}/  # 原 redis_service/redis_{article,comment,tag,user,email,jwt}
│   │   ├── es/{client,search}.go + mapping/       # 原 core/init_es.go + platform/searchx
│   │   ├── clickhouse/                            # 原 core/init_clickhouse.go + log_service 查询
│   │   ├── log/                                   # ★ 有界异步日志管道（见 02）
│   │   │   └── {event,emit,writer,wal,redact,sample,sink_clickhouse,metrics}.go
│   │   ├── llm/{client,structured,metrics}.go     # ★ AI 网关（替代 ai_service 手写 HTTP）
│   │   ├── storage/qiniu/                         # 原 image_service/qiniu_*.go
│   │   ├── mail/  ├── oauth/                      # 原 email_service / qq_service
│   │   ├── cdc/                                   # ★ 两套 river 合并（3,231 行 → 约 1,800）
│   │   │   ├── {canal,checkpoint}.go
│   │   │   └── projection/{article_search,image_ref}.go
│   │   ├── outbox/                                # 事件投递（替代 handler 内裸 goroutine）
│   │   └── http/
│   │       ├── server.go                          # ★ http.Server + 超时配置
│   │       ├── middleware/{auth,cors,log,recover,bind,trace}.go
│   │       ├── response.go                        # 原 common/res
│   │       └── sse.go                             # 原 ai_api/ai_sse.go
│   ├── article/                         # ★ 垂直模块样板
│   │   ├── domain/{entity,value,rule,port}.go     #   rule.go = 业务不变量（谁能看/改/删）
│   │   ├── app/{create,update,query,examine,favorite,top,digg,view,port}.go
│   │   └── adapter/{http,dto,mysql,es,wire}.go
│   ├── comment/  ├── user/  ├── auth/  ├── chat/  ├── social/
│   ├── media/    ├── search/ ├── ai/   ├── notification/
│   └── site/     └── analytics/        # 同样三段结构
├── testkit/                             # ★ 测试基建（替代 test/testutil 的全局写法）
│   └── {db,redis,tx,http,clock}.go + fixture/{article,user,comment}.go
├── tools/archcheck/                     # ★ AST 架构规则（替代 scripts/arch_guard.ps1）
├── resources/                           # 原 resources/ipbase
├── var/                                 # 运行时数据（.gitignore）
├── buildinfo/version.go                 # ★ 改为 ldflags 注入 commit + buildtime
├── openapi.json                         # ★ swag 生成产物（CI 校验一致性）
└── go.mod / go.sum                      # module myblogx（保持不变）
```

### 2.7 三条不可动摇的原则

1. **依赖必须是接口，不是结构体。** 现状 30 个 `Deps` 都含 `*gorm.DB`，等于每个包都能操作整台数据库。目标：handler 只持有 3-5 个方法的能力接口，**拿不到 DB 连接**。
2. **规则必须可执行，不能靠文档。** `arch_guard.ps1` 的教训：符号匹配会空转，PowerShell 在 CI 跑不了，规则写在脚本里没人看。目标：规则写成 `go test`，进 CI，违规即失败。
3. **抽象必须能替换，否则删掉。** `platform/dbx` 只有一行 `return core.InitDB(...)`，无接口无第二实现——它的唯一作用是增加一层文件。目标：要么定义接口并有 fake，要么直接用 `core.InitDB`。

### 2.8 已知取舍

| 取舍 | 代价 | 换来的 |
| --- | --- | --- |
| 领域实体与持久化模型分离 | 每模块多约 200 行 mapper | 改 DB 不影响 API；领域规则有落脚点 |
| 垂直切分 | 跨模块共享数据要定义接口 | 改动局部化；模块可独立测试 |
| 端口由消费方定义 | 同一能力可能在两处定义相似接口 | 模块间零 import，彻底解耦 |
| 只分 3 个子包（domain/app/adapter） | `adapter` 包较大（2-4k 行） | 包数可控（12 模块 × 3 = 36） |
| 手写装配 | main.go 约 150 行 | 启动流程即依赖图，可读可调试 |
| 所有规则走 AST + CI | S1 阶段要写约 400 行工具 | 规则不再空转，腐化有刹车 |

---

## 三、物理目录映射

### 3.1 顶层四目录与删除清单

```
blogx/                              ← 仓库根（无 workspace / 无 turbo）
├── backend/                        # Go 后端（原 apps/api）
│   ├── cmd/{server,cli}/           #   ★ cmd/server/main.go 唯一合成根
│   ├── internal/                   #   垂直模块（article / comment / ... / platform）
│   ├── tools/archcheck/            #   ★ 新建：AST 架构规则
│   ├── testkit/                    #   ★ 测试基建
│   ├── go.mod                      #   module myblogx（保持不变）
│   ├── docs/swagger.json           #   ★ 生成产物（swag init），CI 校验生成一致性
│   └── Makefile                    #   make openapi / make test / make build
├── frontend/                       # Nuxt 读者站（原 apps/web）
│   ├── server/routes/api/[...path].ts   # 唯一代理入口（→ $API_BASE_URL）
│   ├── app/types/openapi.d.ts           # ★ 生成，禁止手改
│   └── package.json                # 自洽，不引用 sibling 包
├── frontend_admin/                 # Nuxt 管理后台（原 apps/admin）
│   ├── server/routes/api/[...path].ts
│   ├── app/types/openapi.d.ts
│   └── package.json
├── deploy/                         # 基础设施（不属于任一 app）
│   ├── compose/{local,production}/docker-compose.yml
│   ├── docker/{api,web,admin,nginx,...}
│   └── nginx/
│       ├── nginx.template.conf     # ★ 4 份合并为 1 模板 + envsubst
│       └── envsubst.sh
├── .github/workflows/ci.yml        # ★ 新建（当时完全没有）
├── docs/                          # 文档入口（架构 / ADR / 笔记）
└── README.md
```

| 删除目标 | 规模 | 理由 |
| --- | --- | --- |
| `prototype/` | 84 文件 / 7,300 行 + 2.1MB dist | 死代码：零 API 调用，硬编码 `demo-password` / `captcha="7Y2P"` |
| `plugin/` | 711 行 + 入库 dist | 唯一消费者是 prototype，删后成孤儿 |
| `apps/{web,admin}/pnpm-lock.yaml` | 13,408 行 | md5 完全相同，幽灵文件 |
| `scripts/arch_guard.ps1` | 110 行 | 被 `backend/tools/archcheck` 取代 |
| `backend/apideps/` | 1 文件 | 与 `api.Deps` 重复，随 Deps 一起删 |
| `backend/platform/` | 4 文件 / 63 行 | 一行转发壳，并入 `internal/platform` |
| 根 `pnpm-workspace.yaml` / `turbo.json` / `package.json` / `pnpm-lock.yaml` | — | monorepo 工具链，解耦后不再需要 |
| `openapi/`（手写） | 14,462 行 | 改为后端 `swag` 自动生成 |

**净删除约 8,000 行源码 + 2.1 MB 二进制 + monorepo 配置，并消除一处 git 历史包袱（重建仓库）。**

### 3.2 8.3 万行的去向

#### `backend/api/`（170 文件 / 10,494 行）→ 各模块 `adapter/http.go`

| 现有 | 新位置 | 说明 |
| --- | --- | --- |
| `api/enter.go`（Deps + Api 聚合） | **删除** | 由 `cmd/server/main.go` 取代 |
| `api/article_api/*`（55 文件 / 6,012 行） | `internal/article/adapter/http.go` + `dto.go` | 按用例拆多个文件，单文件 <500 行 |
| `api/article_api/article_write_service.go`（279 行） | `internal/article/app/` | **核心问题文件**：api 包内自己造 service |
| `api/article_api/{category,tags,top,favorite,view_history}/` | `internal/article/adapter/` 下的文件 | 子包扁平化（5 个包 → 5 个文件） |
| `api/comment_api/*`（15 / 1,832） | `internal/comment/adapter/` | `comment_service` 只有 11 行 |
| `api/user_api/{auth_api,profile_api,user_man_api}` | `internal/auth/adapter/` + `internal/user/adapter/` | 按职责拆两个模块 |
| `api/chat_api/*` | `internal/chat/adapter/` | `chat_ws.go` → `platform/http/ws.go` |
| `api/ai_api/*`（12 / 1,824） | `internal/ai/adapter/` | `ai_sse.go` → `platform/http/sse.go` |
| `api/search_api/*` | `internal/search/adapter/` | |
| `api/image_api/*` | `internal/media/adapter/` | |
| `api/{banner,site}_api/*` | `internal/site/adapter/` | |
| `api/{global_notif,sitemsg}_api/*` | `internal/notification/adapter/` | |
| `api/{data,log}_api/*` | `internal/analytics/adapter/` | |
| `api/follow_api/*` | `internal/social/adapter/` | |
| `api/captcha_api/*` | `internal/auth/adapter/captcha.go` | |

#### `backend/service/`（131 文件 / 17,732 行）→ 拆到 app 与 platform

| 现有 | 新位置 |
| --- | --- |
| `service/article_service/`（1 文件 99 行） | `internal/article/app/`（扩张为真正的业务层） |
| `service/comment_service/`（1 文件 11 行） | `internal/comment/app/` |
| `service/ai_service/*`（27 / 2,835） | `internal/ai/app/`（用例）+ `platform/llm/`（HTTP 客户端） |
| `service/ai_service/article_score_service/` | `internal/ai/app/scoring/` |
| `service/es_service/article_sync.go`（**1,547 行 / 42 函数**） | `platform/es/projection/` 拆 5-6 个文件 |
| `service/es_service/enter.go`（517 行） | `platform/es/client.go`（原样迁移，质量好） |
| `service/river_service/`（2,222 行） | `platform/cdc/` |
| `service/image_ref_river_service/`（1,009 行） | `platform/cdc/projection/image_ref.go`（与上合并，省约 1,400 行） |
| `service/cdc_dead_letter_service/` | `platform/cdc/deadletter.go` |
| `service/log_service/`（17 / 2,489） | `platform/clickhouse/`（查询）+ `platform/log/`（写入） |
| `service/redis_service/*`（10 子包） | `platform/redis/{counter,guard,cache}` |
| `service/image_service/*` | `internal/media/app/` + `platform/storage/qiniu/` |
| `service/site_service/*` | `internal/site/app/` + `platform/config/runtime.go` |
| `service/user_service/*` | `internal/auth/app/` + `internal/user/app/` |
| `service/chat_service/*` | `internal/chat/app/` |
| `service/{follow,favorite,top,read}_service/*` | 并入 `internal/{social,article}/app/` |
| `service/message_service/*` | `internal/notification/app/` |
| `service/{email,qq}_service/*` | `platform/{mail,oauth}/` |
| `service/db_service/snowflake.go` | `platform/idgen/` |
| `service/kafka_service/*` | **删除**（135 行死代码，`main.go` 已注释） |
| `service/cron_service/*` | `platform/scheduler/jobs/` |

#### `backend/repository/`（11 文件 2,076 行）——不是迁移，是**重写**

| 现有 | 新位置 | 说明 |
| --- | --- | --- |
| `repository/article_repo/`（**只有 .gitkeep**） | `internal/article/adapter/mysql.go` | 从零实现，不是迁移 |
| `repository/comment_repo/`（451 行） | `internal/comment/adapter/mysql.go` | 签名去掉 `db *gorm.DB`，改持连接 |
| `repository/{favor,top}_repo/` | `internal/article/adapter/mysql.go` | 合并 |
| `repository/{follow,chat}_repo/` | `internal/social/adapter/` + `internal/chat/adapter/` | |
| `repository/read_repo/`（446 行，被 5 个 repo import） | 拆分到各模块 adapter | **已退化为工具包，必须拆** |
| `repository/user_repo/`（63 行） | `internal/user/adapter/mysql.go` | |

> 现有 2,076 行只覆盖 9% 的查询，**剩下 91% 散在 api/service 里**。真正的 repository 要从这 91% 里收回来。

#### `backend/models/`（48 文件）

| 现有 | 新位置 |
| --- | --- |
| `models/article_model.go` | `internal/article/domain/entity.go`（实体）+ `adapter/mysql.go`（row） |
| 同上 `BeforeDelete` 级联逻辑 | `internal/article/app/delete.go`（**必须上移**） |
| `models/{category,tag,article_tag,favorite,user_top_article,user_article_favor,user_article_view_history}` | `internal/article/domain/` |
| `models/comment_models.go` | `internal/comment/domain/` |
| `models/{user,user_session,stat_user}_model.go` | `internal/user/domain/` |
| `models/{chat,user_follow}_model.go` | `internal/{chat,social}/domain/` |
| `models/{image,image_ref}_model.go` | `internal/media/domain/` |
| `models/{global_notif,user_global_msg,message}_model.go` | `internal/notification/domain/` |
| `models/{banner,runtime_site_config}_model.go` | `internal/site/domain/` |
| `models/guest_article_view_record.go` | `internal/article/domain/` |
| `models/enter.go` | **拆分**：`IDRequest`/`OptionsResponse` → kernel 或各 adapter/dto；`BeforeCreate` 雪花钩子 → `platform/mysql/hooks.go` |
| `models/ctype/id.go` | `internal/kernel/id.go`（原样迁移，设计正确：uint64 → JSON string，规避前端精度丢失） |
| `models/enum/*` | 各模块 `domain/value.go` |
| `models/es_settings/*.json` | `platform/es/mapping/` |

#### `backend/core/`（18）+ `platform/`（4）+ `app/`（5）

| 现有 | 新位置 |
| --- | --- |
| `core/init_{db,redis,es,clickhouse,snowflake,ipdb,logrus}.go` | `platform/*/` 对应子包 |
| `core/init_cfg.go` | `platform/config/loader.go` |
| `core/init_mysql_es.go`、`init_image_ref_river.go` | `platform/cdc/canal.go` |
| `core/init_kafka.go` | **删除** |
| `core/init_runtime_site.go` | `platform/config/runtime.go` |
| `platform/dbx/init.go`（一行转发） | `platform/mysql/db.go`（**真实化：加接口 + fake**） |
| `platform/cachex/` | `platform/redis/client.go` |
| `platform/searchx/` | `platform/es/client.go` |
| `app/bootstrap.go` 的 `Infra` | `cmd/server/main.go` 的局部变量 |
| `app/bootstrap.go` 的 `Bootstrap`（**0 调用**） | **删除** |
| `app/lifecycle.go` 的 `OnStop`（**0 调用**） | 修好并接入 `main`（优雅停机） |
| `app/wiring_http.go` | `cmd/server/main.go` + 各模块 `adapter/wire.go` |
| `app/wiring_worker.go` | `cmd/server/main.go`（worker 角色分支） |
| `app/config_normalize.go` | `platform/config/normalize.go` |

#### 其他

| 现有 | 新位置 |
| --- | --- |
| `api.Deps` + `apideps.Deps`（32 个全部） | **删除**，改为能力接口 |
| `middleware/`（12） | `platform/http/middleware/` |
| `router/`（20） | `platform/http/router/`（每模块一个注册函数） |
| `common/res/` | `platform/http/res/` |
| `common/list_query.go` | `kernel/page.go` + 各 adapter |
| `utils/envyaml` | `platform/config/envyaml/` |
| `utils/{jwts,pwd,hash}` | `platform/auth/` |
| `utils/markdown` | `platform/markdown/` |
| `utils/{ipmeta,requestmeta,user_info,info_check}` | `platform/` 对应子包 |
| `utils/logsafe/` | **删除**（反射式序列化废弃；脱敏/截断并入 `platform/log/redact.go`） |
| `core/init_logrus.go`（CommonFieldsHook / FileDateHook） | **删除**，由 `platform/log` 的 Emitter/Writer 取代 |
| `middleware/log_middleware.go` | `platform/http/middleware/access_log.go`（只组装类型化 AccessEvent） |
| `utils/{maps,io_util}` | `kernel/` 或删除 |
| `flags/`（5） | `cmd/cli/` |
| `test/`（19） | 就近搬回被测包；`test/testutil` → `testkit/` |
| `test/architecture/`（2 文件 260 行） | **删除**，由 `tools/archcheck` 取代 |
| `buildinfo/version.go` | 保留，改 `-ldflags -X` 注入 commit + buildtime |

### 3.3 前端映射（两个 app 不合并、不共享代码包）

> 它们唯一的"共享"是各自从后端契约生成类型。

| `frontend`（原 `apps/web`）现有 | 新位置 / 结果 |
| --- | --- |
| `server/routes/api/[...path].ts` | 保留为唯一代理入口 |
| `server/routes/_backend/[...path].ts` | **删除**（合并到单一上游） |
| `server/routes/_origin/[...path].ts` | **删除**（资源由后端/CDN 直出） |
| `server/routes/api/chat/ws.ts`（queue 无上限） | 修 backpressure，或统一走 nginx 删掉该层 |
| `server/utils/upstreamProxy.ts` | `app/lib/http/upstream.ts`（本 app 内，不跨 app） |
| `nuxt.config.ts` 手写 dotenv 解析（45 行） | 保留本 app 一份（不再跨 app 引用） |
| `app/pages/studio/write.vue`（**2,043 行**，占 web app 13.7%） | **已完成（M6）**：`pages/studio/write.vue` **361 行** + `components/studio/{StudioWriteHeader,Toc,Preview,PublishDialog}.vue` + `composables/useStudio*.ts` + `constants/studioToolbar.ts` + `assets/css/studio-write.css`。验收口径：**单文件 ≤800 行**（原写的 `<200` 已作废） |
| `app/types/api.ts`（736 行 / 80 个手写 interface） | **已完成（契约可用性改造）**：压到 **12 个**手写 interface，其余 50 个从契约派生。派生工具见 `app/types/contract.ts`：`RequiredSchema<K>` / `Nullable<T,K>` / `OptionalFields<T,K>`（详见 [contracts](./contracts.md)） |
| `app/services/http/*` | `app/lib/http/` |
| `app/stores/{auth,chat,message}.ts`（循环依赖） | 改事件驱动，`auth` 只发事件不直接调用其他 store |

| `frontend_admin`（原 `apps/admin`）现有 | 新位置 |
| --- | --- |
| `services/http/client.ts`（72 行，与 web 103 行重复） | `app/lib/http/client.ts`（本 app 内，接受重复） |
| `services/image.ts`（292 行，与 web 290 行重复） | `app/lib/http/image.ts` |
| `nuxt.config.ts` dotenv 解析 | 保留本 app 一份，解除对 web 的隐性依赖 |
| `types/`（269 行手写） | 改为 import `app/types/openapi.d.ts` |

**类型自动生成流水线（完全自动，无手写契约文件）**：

```bash
cd backend        && make openapi        # swag init → backend/docs/swagger.json
cd frontend       && pnpm openapi:types  # → app/types/openapi.d.ts
cd frontend_admin && pnpm openapi:types
```

- 后端：handler 加 `// @Summary` / `// @Param` / `// @Success` 注解，`make openapi` 生成。**单一来源是后端代码**。
- 前端：各自生成 `openapi.d.ts`，**禁止手改**；手写类型只允许纯前端状态（UI props、本地枚举）。
- 完整流水线的坑与验证方法见 [contracts](./contracts.md)。

**单一上游**：两个前端的 Nuxt server 只保留 `api/[...path].ts` 一个代理入口（→ `$API_BASE_URL`）；**反向代理的唯一 upstream 定义在 `deploy/nginx`**（基础设施层，不在 app 代码里）；`_origin` 不走 Nuxt 代理，由后端或 CDN 直出。

### 3.4 命名规范

| 对象 | 规范 | 反例（现状） |
| --- | --- | --- |
| 模块目录 | 单数业务名：`article` 而非 `articles` | `article_api`、`user_api` |
| **禁止 `enter.go`** | 用 `service.go` / `handler.go` / `port.go` | **65 个 `enter.go` / 1,327 行** |
| **禁止 `Deps` 结构体** | 构造函数参数列表 | **32 个** |
| 端口接口 | `Reader` / `Writer` / `Gate` / `Counter`（能力名） | — |
| DTO | 后缀 `Req`/`Resp`/`Query`，放 `adapter/dto.go` | `article_types.go` 混杂 |
| 错误 | `domain.ErrXxx` 哨兵 + `errors.Is`；中文文案集中在 `internal/apperr` | 中文 `errors.New` |
| 单文件上限 | **800 行**（archcheck 强制） | `article_sync.go` 1,547 |
| 单函数上限 | 80 行 | — |

---

## 四、已确认的架构决策（用户拍板，不再作为待确认项）

1. **彻底拆解 monorepo，拆成四个解耦目录**。根目录只剩 `backend/`（Go）、`frontend/`（Nuxt 读者站）、`frontend_admin/`（Nuxt 管理后台）、`deploy/`（Docker / compose / nginx）。**不再有 `pnpm-workspace.yaml` / `turbo.json` / `packages/`**。三部分各自独立、`go.mod` / `package.json` 自洽，唯一的耦合点是后端自动生成的 openapi 契约。
2. **`Deps` 结构体直接废弃，不留过渡态**。32 个 `Deps` 全部删除，handler 只持有 3-5 个方法级能力接口，拿不到任何数据库连接。代码量会增加，但依赖关系第一次可读、可测。
3. **`prototype/` 删除**（84 文件 / 7,300 行 / 2.1MB dist，零 API 调用，硬编码 `demo-password`），其唯一消费者 `plugin/`（711 行）一并删除。
4. **`scripts/` 删除**。`arch_guard.ps1` 等 PowerShell 守卫全部移除，改为 `backend/tools/archcheck`（Go AST，跨平台、进 CI）。
5. **openapi 改为"像 FastAPI 那样自动维护"**。已删除手写 14,462 行文档。后端用 `swag` 注解 handler，构建时生成 `swagger.json`（构建产物，非手写）；两个前端各自用 `openapi-typescript` 从这份契约生成类型。**契约单一来源是后端代码，"代码即契约"**。
6. **前后端只保留单一上游**。两个前端各自只代理一个后端（`/api` → `API_BASE_URL` 环境变量），`_backend` / `_origin` 别名删除；反向代理的**唯一 upstream 定义在 `deploy/nginx`**（基础设施，不在 app 代码里）。
7. **admin 与 web 不合并为一个 app**。两个 Nuxt app 保持独立；它们之间唯一的"共享"是各自从后端 openapi 生成的类型，不存在共享代码包。
8. **重建 git 仓库**。删除旧 `.git`，重新 `git init`，首提交即目标形态，避免新旧仓库交叉。
9. **测试体系完全重构**。改为测试金字塔：domain 纯单测穷举规则、app 用有状态 fake 测编排、adapter 用真 MySQL(testcontainers) 验 SQL、HTTP 契约测、E2E 只保主链路。配套 `testkit`（clock/id/http/fake/fixture/golden），全部测试可并行、可复现。详见 [governance](./governance.md) §三。
10. **日志模块彻底治理**。禁止把结构体塞进日志（字段封闭类型、禁 `any`）；改为**有界异步管道**（双限额队列 + 单写协程批量 + ClickHouse 直写 + WAL 兜底），请求路径不序列化/不落盘；去掉每条日志调雪花 ID 与全量调用栈。详见 [governance](./governance.md) §四。

> **可测性是架构的属性，不是测试的属性**：只有当 handler 拿不到 `*gorm.DB`、domain 不依赖外部、时间/ID 可注入时，规则才能被穷举测试。这是"测试重构"必须与"架构重构"同步推进的原因。

---

## 五、不做什么

- **不拆微服务**。单体后端 + 两个前端 app 的形态是合理的。
- **不换 Go 框架**。Gin + GORM 没问题，问题在用法。
- **不重写前端**。Nuxt 4 选型正确，问题是页面过大与类型手写（只拆组件、各自生成类型）。
- **不保留 monorepo / workspace / `packages/` 共享包**。三部分解耦，唯一共享是后端自动生成的 openapi 契约。
- **不追求覆盖率数字**。重点是让已有的 2,357 处断言**真的执行**。
- **不为了"看起来规范"再加一层抽象**。这次的目标恰恰相反：**删掉表演性的抽象**。
- **不上 wire / dig**。手写 150 行装配更可读。
- **不引入完整 DDD**（工厂/规格/领域服务全上）。只用实体 + 值对象 + 端口。

---

## 六、v2 硬约束总表

> 这是 v1 结束时**仍然生效**的全部规则，从 02/03/04/05 四份文档的约束清单去重合并而来。
> 每条都标注了**由什么守卫**：`archcheck Rn` 表示改了会红；「人工」表示编译器与守卫都抓不到，只能靠 review。

### A. 代码形状

| # | 约束 | 守卫 |
| --- | --- | --- |
| A1 | **零列变更 + 允许新增表**：既有表的列名 / 类型 / 列顺序 / 删列一律禁止（索引、外键、CH 的 `TTL`/`CODEC` 允许）；新增表走 `CREATE TABLE IF NOT EXISTS` 登记，经 ADR 立项的登记例外当前只有通用取景表 `picture_crop_models`（[ADR-0020](../adr/0020-zero-ddl-exception-and-generic-framing-table.md)，例外只到新建表）。理由是 **CDC 按列名解析**：ES 文章投影与 image-ref 两条链路都从 binlog 行快照按列名取值。放宽列变更的前提是有列名契约测试（`schema.ColumnsDiff` / `AssertColumns`） | 人工 + 契约测 |
| A2 | **ID 语义字段必须是 `ctype.ID`**，禁止裸 `uint64`/`int64`（含指针与切片）；反序列化须同时接受数字与字符串 | `archcheck R12`，基线 0 |
| A3 | **ID 比较必须显式走 `Uint64()`**——字符串语义下 `"10" < "9"` 成立 | 人工 |
| A4 | **禁止 `type Deps struct`**；`*gorm.DB` 只能出现在 `*/adapter/` 与 `platform/mysql/` | `R3` / `R1` / `R2` |
| A5 | **依赖单向 `adapter → app → domain`**；模块间零 import；`internal/kernel` 只放 ID/分页/错误 | `R5` / `R11` |
| A6 | **事务走 UnitOfWork**（`WithinTx(ctx, fn(ctx, Repos) error)`），domain/app 不得出现 `*gorm.DB` | 人工 |
| A7 | **单文件 ≤ 800 行**（Go 与 Vue 同口径），前端由两端 eslint `max-lines`(error) 兜住；判断标准看职责数而不只看行数 | `R6` / eslint |
| A8 | **命名规范**：禁 `enter.go`、禁 `Deps`、DTO 后缀 `Req`/`Resp`/`Query`、错误用 `ErrXxx` 哨兵 + `errors.Is` | `R8` / 人工 |
| A9 | **row 结构（GORM 标签、列映射、`TableName`）由使用者自持**，不进 domain、不跨模块共享；雪花 `BeforeCreate` 钩子只在 `platform/mysql/hooks.go` | `R10` |
| A10 | **禁裸 `go`**（一律 `safego.Go`）；**禁跨模块 import adapter**；**禁共享 models** | `R7` / `R11` / `R10` |
| A11 | **跨模块协作通则**：读=窄 port 同步直调；写副作用=落 outbox；写结果本身=同步同事务。新增跨模块同步写必须在 ADR 里说明"为什么不走事件" | 人工 |
| A12 | **domain 层零外部依赖**（仅 stdlib + `internal/apperr`）；中文文案不进调用点 | `R3` / `R8` |

### B. 契约与类型

| # | 约束 | 守卫 |
| --- | --- | --- |
| B1 | **`make openapi` 必须用 swag v2 且带 `--v3.1`**，否则静默退回 Swagger 2.0 | CI job `contract` |
| B2 | **注解里禁止写 `map[string]any`**（会静默跳过整份契约写入但 exit code 为 0），必须用具名类型 | 人工 |
| B3 | **验证生成成功不能只看 exit code**：必须 grep 新内容或看文件时间 | 人工 |
| B4 | **`@Success` 必须引用真实 Go 类型**，禁止手写 `object{...}`；`ListData` / `HasMoreListData` / `EmptyData` / `SiteConfigData` 要按 handler **实际调用的 helper** 选 | 人工 |
| B5 | **跨网络传输的类型必须来自生成产物**，禁止从 `openapi.d.ts` 之外 import API 类型 | CI `contract-types` |
| B6 | **可空性三态不得混用工具**：`T`→`RequiredSchema`；`*T`→`Nullable`；`*T`+`omitempty`→`OptionalFields` | 人工 |
| B7 | **`api.ts` 里每个手写类型都要能回答"为什么契约里没有它"** | 人工 |
| B8 | 批量改注解时 **pattern 必须带行首 `//`**；引用跨包类型时那个包必须被文件 import（必要时 `_ "path"`） | 人工 |
| B9 | 产品决策类缺口（后端不返回的字段）**写显式注释，不要用 `RequiredSchema` 把缺口藏起来** | 人工 |

### C. 日志

| # | 约束 | 守卫 |
| --- | --- | --- |
| C1 | **日志字段封闭，禁 `any`、禁传结构体**；`Extra` 只能是 `map[string]string` | `R9 log-no-any` |
| C2 | **`emit.go` 请求路径内不得出现 `json.Marshal` / `os.OpenFile` / 调 `NextSnowflakeID`** | 人工 |
| C3 | **审计字段策略**：`request_body`/`response_body` 仅在 `success=0` 时填充 + `CODEC(ZSTD(9))`；header 不入库；body 先限量读（1MB）再脱敏再截断（≤16KB） | 人工 |
| C4 | **可丢 / 不可丢**：runtime/access 可丢（计数+告警）；audit/login/cdc/replay 不可丢，落 WAL | 人工 |

### D. 门禁与测试

| # | 约束 | 守卫 |
| --- | --- | --- |
| D1 | **棘轮数字只有一处定义**（`scripts/arch-baseline.env`），三处消费；`--skip-tests=false` 那第二遍不能省 | `verify-gates.sh` |
| D2 | **规则自检不可绕过**：每条规则必须能在人造违规上报错；新规则上线当天全绿 = 规则失效 | `rules_test.go` |
| D3 | **门禁不许撒谎**：`run()` 失败须 `return 1`，每个子 shell 末尾须 `exit "$FAILED"`；跳过的步骤必须打印警告 | 人工 |
| D4 | **验证口径必须与门禁一致**：`go vet ./...` 不编译 `//go:build integration` 文件，`go build` 不编译 `_test.go`；收集全量错误用 `go test -tags integration -run='^$' -gcflags=-e ./...` | 人工 |
| D5 | **契约双闸门**：`make openapi` 后 `swagger.json` 无差异**且**两端 `openapi.d.ts` 是从它重新生成的；判据用"重新生成到临时文件再 diff" | CI job 4 + 7 |
| D6 | **测试不许假 Safe**：契约测试跑真 MySQL（testcontainers），SQLite 不冒充 MySQL | 人工 |
| D7 | **golden 口径**：写死种子、逐字节比对、不做归一化；只做 5 个只读模块（comment/search/site/notification/analytics） | 人工 |
| D8 | **覆盖率门槛**：后端 `domain ≥90%` / `app ≥85%` / `platform/log ≥90%`、adapter 每方法至少 1 条、**整体不设**；前端读者站 60/50/66/60、admin 只卡 `app/utils/**` 的 95/90/95/95；`passWithNoTests` 必须是 `false` | CI |
| D9 | **测试判据要能区分"没接上"和"值恰好是 0/兜底"**：夹具缺数据时断言非零，枚举断言具体数字 | 人工 |
| D10 | **flaky 测试隔离到 quarantine，限时修复，不允许长期 retry** | 人工 |

### E. 提交与协作

| # | 约束 |
| --- | --- |
| E1 | 每个 PR 必须过 CI，**不允许 `--no-verify`** |
| E2 | 新代码只能写新结构，禁止往 `api/` / `service/` / `models/` 加文件（这些目录已不存在） |
| E3 | 规则先 `warn` 后 `error`，但必须设定转 `error` 的截止点 |
| E4 | **不保留 pipeline 开关**（如 `BLOGX_<MODULE>_NEW_PIPELINE`）：一模块一 commit，出问题整模块回滚一个 commit |
| E5 | 提交格式：`<任务ID>: <一句话>` + 可选"背景/取舍" + `验收：<跑过的命令/结果>`（详见 [05](../archive/v1/05-baseline-and-process.md) §五） |

---

## 七、术语表

| 术语 | 含义 |
| --- | --- |
| **合成根（Composition Root）** | 唯一允许 `new` 具体实现、组装依赖的地方。本项目应只有 `cmd/server/main.go` |
| **端口（Port）** | 由使用方定义的窄接口，如 `ArticleReader`、`ArticleWriter`、`ArticleGate` |
| **能力接口** | 目标模式：`interface { FindByID(ctx, id) (*Article, error) }`，只暴露用得到的方法 |
| **Deps 装箱** | 现状模式：把 DB/Redis/ES/Logger 塞进结构体层层传递，用的时候自己取 |
| **垂直模块** | `domain` / `app` / `adapter` 三段自包含目录，替代 api/service/repository 水平切分 |
| **kernel** | 跨模块值对象包（ID、分页、时间），可被所有模块 import，但不得含业务规则 |
| **可执行规则** | 能用 `go test` 或 CI 判定通过/失败的规则，而非文档约定 |
| **archcheck** | Go AST 实现的架构检查工具（`backend/tools/archcheck`），替代 PowerShell 正则守卫 |
| **有界异步管道** | 日志写入模型：双限额队列 + 单写协程批量 + ClickHouse 直写 + WAL 兜底，请求路径不阻塞 |
| **死信重投** | CDC 失败的闭环处理（`platform/cdc/dlq`），见 [ADR-008](../adr/0008-dead-letter-replay.md) |

---

## 【归档备注】仅具历史价值、v2 不需要再看

1. **逐条的行号级证据清单**（如 `api/article_api/article_detail.go:85-87`）。旧路径在 R0 目录重排后已不存在，v2 按行号去找毫无意义；已压缩为 §1.7 的分类汇总。
2. **A1–A5 / B1–B10 / C1–C10 的问题编号与"归属阶段 S0-S5"列**。编号只服务于当时的迁移排期。
3. **"五个 P0 / 三个看起来做了其实没做"中的修辞性段落**。它们的作用是当年说服团队接受重构，结论已固化在 §四 的 10 条决策里。
4. **重复代码的逐对举例**（191 对 / 563 对的样本列表、`fans_list.go` vs `follow_list.go` 0.97 等）。具体文件已随迁移消失；保留"重复曾导致审计埋点只有 15% 覆盖"这个**机制性教训**就够了。
5. **迁移顺序 R0 / S0–S5 的排期论证**。属于计划，不是约束；过程见 [05](../archive/v1/05-baseline-and-process.md)。
6. **架构守卫的 PowerShell 实现细节**（`Assert-NoMatch` 用 `Select-String -Quiet`、`Get-GoFiles` 排除 `_test.go`）。脚本已删除，这些脆弱性描述只解释了"为什么改用 AST"。
