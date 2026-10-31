# 配置分层与后台化（2026-10-29 定稿）

> **这份文档是什么**：把"站点自己的配置"从环境变量搬到数据库 + 后台页面这件事的
> 设计说明——分几层、为什么这么分、存储为什么是这个形状、代价是什么。写法同 v1：
> 只留能支撑决策的事实与数字。
>
> **什么时候翻它**：
> - 要加一个新的"后台可改配置项" → §二 判据 + §三 合并规则
> - 改完配置发现"没生效" → §四 的闭包约定
> - 想知道某个字段为什么改了不算数 → §七 第 3 条
> - 想再加一个配置 Tab / 配置段 → §六
>
> **历史决策**：v1 时期的背景与硬约束见 [overview](./overview.md)；决策清单见 [ADR 索引](../adr/README.md)。

---

## 一、问题不是"配置难找"，是"开关和密钥不配套"

模板开箱状态里有三个坑，共同形状是**开关开着、凭据没配**：

| 开关 | 缺什么 | 用户看到什么 |
| --- | --- | --- |
| `login.email_login` 默认 true | 没配 SMTP | 点「获取验证码」直接报错 |
| `ai.enable` 默认 true | 占位 token | 每次对话被上游拒 |
| `qiniu` / `qq` 默认 false | —— | 反而没事，因为开关是关的 |

前两个不是代码缺陷，是"配置缺一半"。而配置散在 `.env` 里的另一个代价是：改一行就得
重启整个后端（compose 的 env 是启动时注入的），所以没人愿意改，坑就一直躺着。

**结论**：把这类配置整体搬进数据库，由管理员在后台改，改完即时生效；`.env` 只留
"进程怎么跑"。

---

## 二、三层模型与判据

| 层 | 载体 | 谁改 | 生效时机 | 例子 |
| --- | --- | --- | --- | --- |
| 进程级 | `settings.yaml` + 环境变量 | 部署者 | 启动时 / 重启 | 监听地址、DB 连接、JWT 密钥、日志目录、CDC 位点目录 |
| 业务默认值 | `site_default_settings.yaml` | 仓库 | 库里没有该段时灌入 | 站点标题、上传白名单、登录策略初值 |
| 运行时配置 | `runtime_config_models` 表 | 管理员（后台） | 下一次请求 | 邮件密钥、AI 密钥、七牛密钥、日志级别、审核开关 |

**判据一句话**：改它会不会影响"**进程怎么跑**"？

- 会（监听哪个端口、连哪个库、日志写哪个目录、CDC 从哪起）→ 环境变量。
  这类值改了必须重启，而且改错会让进程起不来 —— 不该由网页决定。
- 不会（邮件用哪个模板、AI 用哪个模型、要不要审核评论）→ 后台。
  这类值改错只是功能不生效，能立刻改回来，正好适合放在网页上。

**为什么不干脆全搬到后台**：`settings.yaml` 里剩下的是"进程启动前就必须知道"的东西——
没有数据库地址就连不上装配置的那张表，没有 JWT 密钥就谁都不认。这是自举问题，不是取舍。

---

## 三、存储形态：一段一行 JSON

```sql
CREATE TABLE IF NOT EXISTS `runtime_config_models` (
  `id` bigint unsigned NOT NULL,
  `created_at` / `updated_at` / `deleted_at` datetime(3),
  `name` varchar(32) NOT NULL,   -- site / ai / mail / storage / oauth / log / upload
  `payload` longtext NOT NULL,   -- 一段完整 JSON
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_runtime_config_models_name` (`name`)
)
```

### 3.1 为什么不是一段两列（`mail_secret_id` / `mail_region` / …）

**被零 DDL 红线否决的**：`-db` 只执行 `CREATE TABLE IF NOT EXISTS`，全仓没有 ALTER 通道
（ES 投影靠 CDC 按列名解析，改列会让投影和库对不上）。列化意味着**每加一个配置字段
都要一次 DDL 变更**，而项目没有这个能力。JSON 段则是加字段零迁移。

### 3.2 为什么不是一字段一行（EAV）

读一次完整配置要扫 N 行、每行都要做类型转换、写入要开事务保证原子性。而配置读取发生在
**每个请求**上（见 §四），把一次主键查询换成 N 行扫描是纯亏。

### 3.3 深度合并规则（"加字段零迁移"的实现）

新代码读旧 JSON 时，缺的字段要能自动补上默认值。合并规则三条：

| 情况 | 行为 | 为什么 |
| --- | --- | --- |
| 值为 `null` | **不覆盖**默认值 | 前端把未填项传成 null 时不该清掉基线 |
| 两边都是对象 | 递归合并 | 只加了一个子字段时不必重写整段 |
| 数组 | **整体替换** | 上传白名单这类"删一项"的语义，合并会变成只增不减 |

代码在 `backend/internal/platform/config/merge.go`，单测覆盖了这三条。

### 3.4 明文存储（已知代价）

`payload` 里是明文密钥。选它的理由：**慢查询日志、备份、`docker inspect` 里本来就有
明文**——`.env` 方案并没有更安全，只是把明文换了个地方。遮蔽只在**出边界的路上**做
（见 §四），不指望存储层。

---

## 四、Provider 闭包：热更新只有这一种写法

所有消费点拿到的不是配置**值**，而是取值**函数**：

```go
// 错的：启动时快照，改完要重启
type Service struct{ cfg conf.Mail }
// 对的：每次用时现读
type Service struct{ cfg func() conf.Mail }
```

现在一共 **11 个文件、25 处读取点**，全部是现读，没有任何启动时快照。最典型的四个：

| 消费点 | 为什么必须是闭包 |
| --- | --- |
| `emailCodeBridge`（发验证码） | 管理员刚填完 SES 密钥就点「获取验证码」，不能要求他重启服务 |
| `qiniuStorage`（传图） | 关掉七牛后，上传接口要立刻改口说"站点未启用图片上传" |
| `middleware.Runtime.logConfig()` | 日志级别改完要作用在下一次请求，否则"调成 debug 看日志"永远看不到 |
| `cron_wiring` 的清理任务 | 保留天数在任务**执行时**读，而不是排期时读 |

### 敏感字段的三态（这里最容易写错）

| 前端传什么 | 后端当作 | 结果 |
| --- | --- | --- |
| `"******"`（`domain.SensitivePlaceholder`） | "没改" | 沿用库里的旧值 |
| `""` | "清空" | 真的写成空串 |
| 任意其它字符串 | "改成这个" | 覆盖 |

漏掉第一态的直接后果：管理员改一下「发件人显示名」，顺手把密钥字段原样提交，
密钥就被写成 `******`，下一次发信全挂。所以遮蔽与回滚都在 `app` 层成对出现
（`AdminMail` / `UpdateMail`、`AdminAI` / `UpdateAI`、`AdminStorage` / `UpdateStorage`、
`AdminOAuth` / `UpdateOAuth`）。

---

## 五、八个配置段

| 段 | 类型 | 真正生效的地方 | 生效时机 |
| --- | --- | --- | --- |
| `site` | `conf.Site` | 站点信息、SEO、登录开关与限流、审核开关 | 下一次请求 |
| `ai` | `conf.AI` | AI 对话与文章评分 | 下一次调用 |
| `mail` | `conf.Mail` | 腾讯云 SES `SendEmail`（模板发信） | 下一次发信 |
| `storage` | `conf.QiNiu` | 上传 token、URL 反解、image-ref 同步 | 下一次上传 |
| `oauth` | `conf.QQ` | QQ 登录回调 | 下一次登录 |
| `log` | `conf.Logrus` | 日志级别、请求日志、查询限额、清理窗口 | 级别立即，其余下一次请求 |
| `upload` | `conf.Upload` | 上传格式白名单 | 下一次上传 |
| `deco` | `conf.Deco` | 光标挂件的开关、站标尺寸、挂绳几何与物理、描边与静止轻晃 | 下一次页面请求 |

`deco`（装饰）不承载任何业务语义：它只影响"页面看起来怎样"。开这个独立段而不是把
挂件参数塞进 `site`，是因为装饰会越来越多（背景、边框），而每开一段都要走一遍段注册的
八个点——第 8 段把这笔成本一次付清。本票只有挂件子块 `deco.charm`：总开关 `enable`，
以及八个数值参数 —— 站标尺寸 `logo_size`（px）、挂绳长度 `rope_length`（px）、
节点数 `segments`（节，计数）、描边起始宽度 `line_width`（px，收尾固定为起始的 0.6 倍）、
重力 `gravity`（px/s²）、每步阻尼 `damping`（无量纲）、静止摆动频率 `idle_sway_hz`（Hz）、
静止摆动幅度 `idle_sway_amp`（px/s²）。`segments` 是计数，钳位时会先取整再夹到边界；
`line_width` 钳位后保留一位小数。`idle_sway_amp` 与 `idle_sway_hz` 把 0 当作合法值：
幅度为 0 或系统开启「减少动态」时读者站走"静止即停帧"的零帧车道，只有幅度大于 0 才进入
约 20fps 的怠速车道。弧垂没有单独参数，它来自真实松弛（绳长 `rope_length` 减去指针到
站标的实际距离），调大 `gravity` 不会加深弧垂。

这一段在**写入路径上钳位**：越界钳到边界、缺省补默认值、NaN/非法值回落默认，
**不报错**（`conf.Deco.Normalize()`，在 `app` 层的 `UpdateDeco` 里调用）。理由：管理员改一半
被 400 打回只会让他猜哪里错了，而钳位保证库里永远存不下发散值。读者站与后台各自再做一次
同样的钳位，作为最后一道防线。

邮件从 SMTP 换成腾讯云 SES 的原因不是偏好：SES 官方 SDK 的 `SendEmail` **只支持模板发信**，
模板已在控制台审核，代码里不可能被注入任意正文 —— 而验证码本来就只有一个模板。
四个场景（注册 / 重置密码 / 绑定邮箱 / 邮箱登录）共用同一模板，变量是
`action` / `code` / `timeout`。

---

## 六、后台页面：八个 Tab，一个例外

`frontend_admin/app/pages/site/index.vue` 是 `NTabs` 容器，八个子组件各读写自己那一段：
`SettingsSite` / `SettingsAi` / `SettingsMail` / `SettingsStorage` / `SettingsOAuth` /
`SettingsLog` / `SettingsUpload` / `SettingsDeco`。

**唯一的例外**：「登录策略」它不是一段，而是 `site.login` 子对象。所以它和站点基础信息
放在**同一个 Tab、同一次保存**里 —— 两个组件各自读写整段 `site` 会互相把对方刚保存的值
覆盖回去（A 存标题时带着旧的登录开关，B 存开关时带着旧的标题）。

请求/响应形状是这样（这是本方案的固有代价，见 §七）：

| 方向 | 形状 | 契约里是什么 |
| --- | --- | --- |
| 读 | `GET /site/admin/{name}` | `map[string]any` —— 没有字段级 schema |
| 写 | `PUT /site/{name}`，body 是 object | `object` —— 绑定时才按 name 决定类型 |

前端的类型因此是**手写视图模型**（`app/types/api.ts`），不在契约棘轮的保护范围内。
这是有意的取舍：八段共用一条路由，换来的是路由表短、审计日志统一（`runtime_config_update`
+ `TargetID=段名`）；代价是这段类型没人替你检查，改字段要两边一起改。

### 6.1 一个真实的类型陷阱（实测数据）

七个组件的初版都写成 `const config = ref<T | null>(null)` + 模板 `v-if="config"`。
`nuxi typecheck` 报 **35 条 `TS18047: 'config.value' is possibly 'null'`**，集中在
`v-model` 上，且**只有每个作用域里的第一条 `v-model` 不报**。

原因是 vue-tsc 生成的双向写代码（`config.value.x = $event`）会让 TS 在第一次属性赋值后
丢弃 `config.value` 的窄化。改成"非空初值 + `loaded` 门控"后归零：

```ts
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
const config = ref({} as MailRuntimeConfig)
const loaded = ref(false)
```

同一次还修掉了另一个小问题：加载失败时原来一直显示"配置加载中"（因为 `v-if` 恒假），
现在按 `loading` 区分"加载中 / 加载失败，刷新页面重试"。

---

## 七、首次初始化闸门

**库里没有管理员时**，`/setup` 才开放。判断只 `SELECT ... WHERE role = RoleAdmin`：

- **不看 `status`**：被禁用/待审核的管理员也算"已经初始化过"。否则一个停用的管理员账号
  会让闸门重新开放，等于任何人可以再建一个管理员。
- **创建时强制覆盖** `Role = RoleAdmin`、`Status = StatusActive`：请求体里的 role 一律忽略，
  否则"第一个管理员"可以被指定成普通用户，闸门下次仍然开门。
- 复用 `Create` 的校验（用户名唯一、密码强度、邮箱查重），只跳过权限检查 —— 这条路径
  本来就没有登录态。
- 路由挂在 `SetupRouter`，**没有鉴权中间件**（还没有账号可登）；安全性由服务层闸门保证，
  `SetupAlreadyDone` 错误码在第二次调用时返回。

不做 CLI 建管理员的原因：容器里没有交互式终端，`docker exec` 建号要求部署者记住一条
与项目无关的命令；而"第一次打开后台 → 设置密码"是任何 CMS 的默认心智。

---

## 八、固有三处代价（不粉饰）

1. **契约管不到这段**。`/site/admin/{name}` 的响应是 `map[string]any`，请求体是 `object`。
   字段级类型只存在于前端手写的 `api.ts` 里，两端靠人同步。想根治要把八段拆成八组
   路由 + 八个 DTO，代价是路由表变长、审计要多记一层。
2. **密钥明文入库**（§3.4）。它不会比 `.env` 更安全，只换来"改完即生效"。
3. **有些字段提交了也不生效**。`log.app` / `log.dir` / `log.wal_dir` / `log.stdout_format`
   是进程级配置，`UpdateLog` 会把它们**覆盖回当前值**。UI 上必须做成只读并写明原因，
   否则会被当成 bug。为什么不让改：改了 `dir` 而清理任务仍按旧目录算窗口，等于日志
   无人清理；改了路径在容器里不存在，写入直接失败——**连记录这次失败的日志也一起没了**。

---

## 九、这一轮的实际改动量

| 项 | 数量 |
| --- | --- |
| 删除的 `BLOGX_*` 环境变量 | 52（compose / 两个 env 模板 / `.envrc.example` 同步） |
| `settings.yaml` 删掉的业务段 | 4（`upload` / `email` / `qq` / `qiniu`） |
| `conf.Config` 删掉的字段 | 6（`Upload`/`Site`/`Email`/`QQ`/`QiNiu`/`AI`） |
| 新增/改写的配置段类型 | `conf.Mail` 新建；`conf.Logrus` 补 11 个 json tag |
| 后台新增 Tab | 7（一个页面 + 七个组件） |
| 新增接口 | `GET /site/admin/{name}`、`PUT /site/{name}`、`GET /setup/status`、`POST /setup/admin` |
| 修复的既有缺陷 | 3（软删除行撞 `name` 唯一索引 / 重启后站点配置被重置 / 默认值回退成全零） |

`settings.yaml` 现在只剩进程级配置。业务默认值只有一个来源
（`site_default_settings.yaml`），不再有两份默认值互相打架。
