# blogx 项目长期记忆

## 文档结构（2026-10-29 重组）
- 入口：`docs/README.md`（文档地图）。架构与治理在 `docs/architecture/`：
  `overview.md`（**§六 是硬约束总表**）/ `governance.md` / `contracts.md` /
  `configuration.md` / `ai-integration.md`。
- 决策：`docs/adr/0001-0019`（0001–0009 从 v1 ADR 合集拆出，0010–0019 为 v2 前端视觉 / 呈现层 / 装饰层 / 渲染与性能策略）；归档原文在 `docs/archive/v1/`。
- 笔记：`docs/notes/`（`memory/` 是历史记忆）；待办：`docs/TODO.md`。
- 词汇表：根目录 `GLOSSARY.md`；agent 技能配置：`docs/agents/`。
- 历史 v1 文档（01-12 / ADR-00x / baseline）已删除，需要时 `git checkout` 旧提交。

## 仓库形态
- **非 monorepo**：`backend/`（Go，module `myblogx`）、`frontend/`（Nuxt 读者站）、
  `frontend_admin/`（Nuxt 后台）、`deploy/`、`docs/`。无共享代码包，唯一耦合是后端生成的 openapi 契约。
- git 重建于 2026-10-24；旧历史在 `../blogx-old-repo.bundle`。`core.autocrlf=false`。
- `git status` 会出假 M，**以 `git diff HEAD --name-only` 为准**。

## 工程技能配置（2026-10-29）
- **Issue tracker = GitHub Issues**（`Jenlybein/NewblogX`）；`gh` 已安装（GitHub.cli 2.103.3，`C:\Program Files\GitHub CLI\gh.exe`），凭据走 git credential helper。
- triage 标签已建：`needs-triage` / `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`。
- 入口：`AGENTS.md` 的 `## Agent skills` → `docs/agents/{issue-tracker,domain,triage-labels}.md`；领域词汇表 `GLOSSARY.md`。
- 原 `.workbuddy/memory/` 已迁到 `docs/notes/memory/`，`.workbuddy/` 目录已删除（`.gitignore` / `.dockerignore` 的忽略项保留，防工具重建）。

## 不可违背的约定
1. **零 DDL 是硬约束**（ES 投影靠 CDC 按列名解析）。新增表可以，改/删既有表不行。
2. **ID 语义字段必须 `ctype.ID`**，禁止裸整型。守卫 R12 兜底，基线 0。
3. **不要拿 ID 比大小**（`"10" < "9"` 成立，用 `.Uint64()`）；**不要 `ID(123)` 构造**（用 `NewID`，
   只有 `go vet` 的 stringintconv 能抓）。
4. 一个业务拆出的多个文件收进子包；子包可依赖父包，反之不行。
5. 单文件 ≤800 行，由两端 eslint `max-lines` 强制（`article/[id].vue` 791 行是下一个会红的）。
6. 后台 handler 不得直接查库；HTTP 层零 SQL。
7. **线上重建镜像必须串行**：先 `docker compose down`（全部容器）→ 然后**一条一条**
   `docker compose build <svc>`（严禁并行、严禁 `up -d --build`）→ 全部构建完成后再
   `docker compose up -d` 一次拉起。原因：生产机只有 **2C4G（实测 3.6Gi 可用）**，
   并行构建会直接把机器压到重启——2026-10-31 用 `up -d --build` 上线视觉改造时就是这么
   把服务器打重启的（容器靠重启策略回来，但跑的是旧镜像）。
8. **提交前不要用 `git add -A`**：工作区可能有用户自己的未提交改动（例如 README 的七牛云
   章节）。2026-10-31 就发生过 `add -A` 把用户的文档改动顺手塞进一个 CI 修复提交里，
   事后只能靠一条独立文档提交把归属补回来。**逐文件 `git add <path>`**。

## 配置分层（2026-10-29 定稿，见 `docs/architecture/configuration.md`）
- **判据**：改它会不会影响"进程怎么跑"？会 → 环境变量；不会 → 后台。
- **业务配置在数据库**：表 `runtime_config_models(name, payload)`，一段一行 JSON，
  七段 site/ai/mail/storage/oauth/log/upload。后台「站点」页七个 Tab 改的就是这里。
- **52 个 `BLOGX_*` 已删除**（邮件/QQ/七牛/站点登录/上传/日志/AI）。留给环境变量的只有
  基础设施与进程级：DB/Redis/ES/ClickHouse/CDC/JWT/监听地址/日志服务名与目录。
- `settings.yaml` 只剩进程级；**业务默认值只有一个来源** `site_default_settings.yaml`（字面量，
  无 `${VAR}`）。`conf.Config` 已删 `Upload/Site/Email/QQ/QiNiu/AI` 六个字段与 `conf_email.go`。
- **加字段零迁移**：读时深度合并（null 不覆盖 / 对象递归 / 数组整体替换），见
  `platform/config/merge.go`。消费点一律 `func() T` 现读，**不要写启动时快照**。
- 密钥明文存库（用户的选择）；遮蔽只在出边界做，`domain.SensitivePlaceholder = "******"`，
  传占位符=不改、传空串=真清除。
- **`log.app/dir/wal_dir/stdout_format` 提交了也会被覆盖回当前值**（进程级），UI 上是只读。

## AI 接入（2026-10-29 定稿，见 `docs/architecture/ai-integration.md`）
- **调用链唯一出口**：业务 6 个子包 → `internal/ai/llm.go`（`Chat`/`ChatStream`）→
  `platform/llm/chat.go` → `factory.go` → eino-ext 组件。换框架/换供应商都只动这两层。
- **模型配置在 `ai_provider_models` 表**：`name/vendor/model/enabled` 列化，其余字段进
  `config` JSON（各家字段不同 + 无 ALTER 通道）。`enabled` 全局唯一靠服务层事务。
- **8 个供应商**：openai / claude / gemini / ark / deepseek / qwen / qianfan / openrouter。
  千帆**不用专用组件**（AK/SK 在进程级单例里，多配置会互相覆盖），走 OpenAI 兼容。
  gemini 要两步构造（先 `genai.NewClient`）；claude 的 `MaxTokens` 必填。
- **两层开关**：站点级 `ai.enable`（runtime_config 的 ai 段）+ 条目 `enabled`（表）。
- **`conf.AI.Active`** 是派生字段（`yaml:"-" json:"-"`），装配层从 ProviderService
  的内存快照填进去；子包仍只收 `conf.AI` 一个参数。
- **BaseURL 归一化**：历史配置是完整 endpoint，eino 要 base，见 `NormalizeBaseURL`。
- **部署多两步**：`docker exec blogx-server /app/server -f /app/config/settings.yaml -db`
  建新表，然后 `docker restart blogx-server`（ProviderService 只在启动时 Load）。
  entrypoint 只看 `runtime_config_models` 在不在，**不会**建新表。

## 线上环境（2026-10-29 确认）
- **轻量实例 `lhins-dalwc1lc`，地域 `ap-seoul`（首尔二区）**，公网 `43.155.228.146`，
  `gentlybeing.cn` 就解析到它。**别只扫广州/北京/上海/香港——漏了首尔会误判"博客不在我的实例上"**。
- 部署目录 `/opt/blogx`（git clone，分支 main，owner `ubuntu`）；TAT 以 root 跑，
  首次要 `git config --global --add safe.directory /opt/blogx`。
- compose：项目名 `production`，9 容器，文件 `deploy/compose/production/docker-compose.yml`；
  `.env` 在 compose 文件同目录（自动加载）。
- 后端 `-role=all`（同进程跑 WireWorker + WireHTTP）。**只起 `-role=api` 就没有 CDC 消费者**，
  新文章永远进不了 ES 且不报错。
- **部署流程（顺序不可改，见「不可违背的约定」第 7 条）**：`git pull --ff-only` →
  `docker compose down` → 逐条 `docker compose build blogx_web`（完了再 `build blogx_admin`；
  后端没改就不建 `blogx_server`）→ `docker compose up -d` →
  **`./scripts/verify-deploy.sh`**（必须过：`healthy:true` 且 `cdc_river` 为 `running`）。
  **不要 `up -d --build`**：它会并行构建，2C4G 上会把机器压重启（2026-10-31 已发生过一次）。
  单个 Nuxt 构建约 3–5 分钟；用 `nohup ... > /tmp/build-x.log 2>&1 &` 再轮询日志，
  别把长构建挂在 SSH 通道上（会超时丢连接、看不到进度）。
- **服务名是下划线、容器名是连字符**：`blogx_server`/`blogx_web`/`blogx_admin` 是 compose 服务名，
  `blogx-server`/`blogx-admin` 是容器名。`build blogx-server` → `no such service`；
  `exec blogx_server` → `No such container`。
- **远端 `execute_command` 有内容过滤**：`DROP TABLE ...`、以及 SQL 里出现 `secret` 字样都会
  直接返回 `AccessDeny`。要在库上做破坏性 DDL，只能让用户手动执行或换通道（先 `mysqldump`
  到 `/root/db-backup/` 再动）。
- 首启自举：`deploy/docker/api/entrypoint.sh` 先 `SHOW TABLES LIKE 'runtime_config_models'`，
  缺失才跑 `-db` + `-es -s ensure`。所以**新增表也会被自动建**，但只要这张表在，其它建表逻辑就不跑。
- 2026-10-29 上线后遗留：旧表 `runtime_site_config_models` 仍在（DROP 被拦），
  备份 `/root/db-backup/runtime_site_config_models-20261029.sql`。已无代码读它。
- 健康检查：`/health` = 存活（进程活着就 200）；`/health/ready` = 就绪（组件 failed → 503）。
  **compose healthcheck 必须留在 `/health`**：nginx 是 `depends_on service_healthy`，
  改成就绪会让 CDC 故障连带 nginx 起不来、全站不可用。`/health/ready` 只在内网可达（刻意）。

## 环境
- Go **1.27.3**：`C:\Users\Jen\scoop\apps\go\1.27.3\bin`；gcc：scoop mingw。
- sqlite 测试需 `CGO_ENABLED=1`（不开则后端全红）。
- 前端必须 `corepack pnpm`（10.33.0），系统 12.x 在 Windows 上不建链接。
- `nuxi typecheck` 需 `NODE_OPTIONS=--max-old-space-size=8192`。
- 门禁：`bash scripts/check.sh`（本地必须跑这个，别只跑 go build/vet）。
- `GOPROXY=https://goproxy.cn,direct`（全局设置，proxy.golang.org 常 EOF）。

## 门禁的坑
- **`go vet ./...` 不编译 `//go:build integration` 的文件**，`go build` 不编译 `_test.go`。
  验证口径必须与门禁一致，收集全量错用
  `go test -tags integration -run='^$' -gcflags=-e ./...`（vet 每包只报前几条，收敛极慢）。
- 子 shell + 全局 `FAILED` 标志会吃掉退出码 → 门禁假绿。已在 `check.sh` 修过。
- **vue-tsc：可空 ref + `v-model` 会丢 `v-if` 的窄化**。`ref<T | null>(null)` 配
  `v-if="config"` 时只有**每个作用域里第一条** `v-model` 不报错，其余全报 `TS18047`
  （实测 7 个文件 35 条）。正解是非空占位初值 + `loaded` 门控：`ref({} as T)` + `v-if="loaded"`。
  排查这类错误用 `pnpm exec vue-tsc --noEmit -p .nuxt/tsconfig.app.json`，6 秒出结果，
  比 `nuxi typecheck` 快十倍。
- **`openapi:types` 容易漏**：改了 Go 注解 → 重新生成 `swagger.json` 之后，还要在两个前端各跑
  一次 `corepack pnpm openapi:types`，否则门禁的"契约类型未过期"会红（`/setup/*` 就是这么漏的）。
- `frontend: nuxt prepare` 在 Windows 偶发 `EPERM: .nuxt/eslint.config.mjs`，不是代码问题，单独重跑即好。

## 契约流水线（2026-10-26 定稿）
```
swag v2 --v3.1（直出 OpenAPI 3.1）→ openapi-typescript → app/types/openapi.d.ts
```
- `backend/Makefile` 的 `openapi` target 是唯一生成入口；`docs/` 只有 `swagger.json`。
- **swag v2 三个坑**：①必须带 `--v3.1`，否则默认仍出 Swagger 2.0；②带枚举的 domain 类型 +
  tag 里 `enums` 收窄时输出 `$ref`+`enum` 并存，`openapi-typescript` 只认 `$ref` 会放松枚举
  → 用 `swaggertype:"integer/string"` 强制内联；③**注解里不能写 `map[string]any`**
  （报 `cannot find type definition` 后**静默跳过整份契约写入，exit code 仍是 0**）→ 用具名类型。
  **验证生成成功要 grep 新内容或看文件时间，不能只看 exit code。**
- swag v2 只有 rc6 无正式版；v1 二进制备份在 `~/go/bin/swag-v1.16.4.exe`。
- 注解引用跨包类型时**该包必须被文件真正 import**（注释不参与编译），代码不用就写 `_ "path"`。
- 批量替换注解的 pattern **必须带行首 `//`**，否则变成 `// //` 被注释掉、swag 静默丢弃。

## 契约可用性（2026-10-26 已交付，见 `docs/architecture/contracts.md`）
- 根因已消除：**119 个手写 `object{...}` 响应注解 → 0**，全部 `kernel.Envelope[真实DTO]`。
  契约 schema 107 → 207，123 接口响应全部解析成功，无类型 list 7 → 0。
- 后端泛型响应类型在 `backend/internal/kernel/envelope.go`：
  `Envelope[T]` / `ListData[T]` / `HasMoreListData[T]` / `EmptyData` / `SiteConfigData`。
  **注解里泛型可正常用**（支持自定义类型实参）。
- 前端 `contract.ts` 三层工具（**在前端用契约的正解**）：
  `RequiredSchema<K>`（= `DeepRequired<Schema<K>>`，递归去 `?`/`undefined`，用于响应）、
  `Nullable<T,K>`（显式声明可 null）、`OptionalFields<T,K>`（后端可能不返回）。
  请求体用裸 `Schema<K>`（指针字段全是可选）。
- **可空性三态必须分清**：`T` → `T`；`*T` → `T | null`（`Nullable`）；
  **`*T` + `omitempty` → 字段整个缺席，用 `OptionalFields`**。
  漏了第三态会导致任何构造对象字面量的地方（兜底对象）编译不过。
- **TS 的 `A & B` 是交集、只会收窄**。想放宽枚举必须 `Omit<T, K> & {...}`。
- **两端都已接契约**：读者站 `api.ts` 265 行（50 派生/12 手写），
  admin `api.ts` 172 行（21 派生/7 手写）；起点合计 1020 行，现 437 行。
  admin 从未有过 guard，本轮才补齐 `contract.ts` 三层工具。
  契约里的枚举生成**字面量联合**，迁移不会放宽 `stage: "manual"` 这类窄化。
- **required 不是目标**：响应侧 289 字段只有 2 个有 `binding`（68 处 binding 全在请求 DTO），
  Go 响应 struct 没有"一定返回值"的表达载体。**不要**为了 required 去手写注解。
- **api.ts 不会归零**：剩 19 个手写 = 纯 UI 形状 7 + SSE 帧 3 + 前端枚举 2 +
  多接口组装视图模型 4 + 契约与后端不一致待决策 3 + 无固定 schema 2。
  判据是"每个类型都要能回答为什么契约里没有它"。
- **`swag` 不表达 null**（`nullable`/`type:[x,null]` 全仓 0 处）→ 只能前端声明。
- **死代码清理已完成**（18 类型 + 2 函数）。做法：从**函数入口**反向确认 + 可达性分析
  （从"外部有引用的类型"求内部闭包），不要单删类型（会通过类型链连累不相关页面）。

## 产品缺陷清单（2026-10-26 全部处理完毕）
同类病根：**前端照错的类型写 → 运行时 undefined → 不报错只失效**，编译器一字不提示。
1. 邮箱验证码登录/注册/绑定（读 `payload.email_id`，后端返 `{id}`）—— 已修
2. 「创建并收藏」（读 `created.id`，PUT 返 `EmptyData`）—— 已修
3. 主页「点赞/收藏/评论」恒为 0 —— **撤掉 UI**：`user_stat` 只有 5 列且零 DDL 禁加列
4. admin 用户列表「角色」恒空 —— 已补（`domain.User` 本来有 Role/Status，没带出边界）
5. admin「恢复已隐藏文章」走不到 —— **此判断本来是错的**：`type=5` 走 `adminQuery`
   本就不带可见性过滤，真因是契约枚举只印了 `"visible"` → 已把搜索枚举别名到 kernel/enum
6. 文章页作者卡三统计恒为 0 —— 已修（article 的 userPort 顺带读 `user_stat_models`）
7. **admin 角色数值前后端相反（提权方向）**：后端 `RoleAdmin=1/RoleUser=2`，前端 `1=普通用户`
   → 选「普通用户」实际把人设成管理员。已收口到 `frontend_admin/app/utils/roles.ts` + 单测钉数字

**教训**：①别用契约反推运行时行为（契约本身可能就是错的）；②补上一个字段常会立刻暴露
它掩盖的另一个 bug；③测试夹具缺数据时，只有断言"非零"才能抓到"字段没接上"。

## 门禁与 CI（2026-10-26 补齐）
- CI 有 **8 个 job**，含新增的 `contract`（swag v2 → `make openapi` → `git diff --exit-code`），
  防止"注解改了忘生成、契约静默过期、门禁全绿但契约是错的"。
- CI 的 `frontend-test` 与 `scripts/check.sh` 都跑 `test:coverage`（两端）。
  读者站覆盖 `app/` 全量（60/50/66/60）；**admin 只覆盖 `app/utils/**`（95/90/95/95）**,
  含义是"已测过的不许退化"，不是粉饰整站。
- admin 有 23 个用例（roles/articleLabels/format），`passWithNoTests` 已改 `false`。
- `deadcode`（`~/go/bin/deadcode.exe`）**适合定期审查不做硬门禁**：88 条里多数是 apptest 辅助函数。
  它查出过 `core.InitIPDB` 零调用（IP 归属地库从未加载）这种编译/vet/test 全发现不了的缺陷。
