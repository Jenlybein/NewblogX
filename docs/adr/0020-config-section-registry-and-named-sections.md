# ADR-0020：配置段注册表与十段（含 `sms`）具名化

- 状态：已交付
- 日期：2026-11-06
- 来源：parent spec [#272](https://github.com/Jenlybein/NewblogX/issues/272) 契约阶段 · 落地票 [#283](https://github.com/Jenlybein/NewblogX/issues/283)（配置·段注册表与具名契约）· 手机号 + 短信验证码注册 / 登录（阿里云号码认证服务 PNVS 的短信认证）对应 `sms` 段。本条与 [ADR-0019](./0019-named-outline-trees-as-the-only-shape.md) 各自独立、互不取代：该条只记合集 / 项目目录树成为唯一对外形状，本条只记运行时配置段的注册表与具名契约。

## 背景与问题

运行时配置一直是「一段一行 JSON」（`runtime_config_models(name, payload)`，零 DDL）。写路径的四件事（默认值补种、全量解码、缓存应用、按段写入校验）曾各写一份 `switch name`；`applySection` 遇到没登记的段只 `Warnf("未知的配置段，缓存未更新")` 就返回——**库里写进去了、进程缓存却没更新**，且没有任何失败信号。新增一段时漏登记一个分支，编译器、契约、路由门禁都不会红。

对外形状同时停在参数化口径：`GET|PUT /site/{name}` 与 `GET /site/admin/{name}` 响应写成 `kernel.SiteConfigData = map[string]any`、请求体写成 `object`。于是契约（开放接口契约）里没有字段级 schema 可派生：两个前端各手抄一份 `SiteRuntimeConfig` / `SiteDecoConfig` / `AIRuntimeConfig` / `MailRuntimeConfig` 等视图模型，曾出现前端多写后端没有的 `site_info.subtitle` 又漏了 `beian`、把登录开关键写成 `qq/email/pwd` 而后端发的是 `qq_login/username_pwd_login/email_login` 这类漂移；为了拦这类漂移只能逐字段解析 Go `json` tag 与 TS 键的**源码文本**——第三份字段清单、且是「读源码字符串」而不是「验接口外部行为」的测试接缝。

`docs/architecture/configuration.md` §八 当初把「拆成八组路由 + 八个 DTO」明确列为被否决方案，理由是「路由表变长、审计要多记一层」；段标题与文档也全部按「八个配置段 / 八个独立路由 / 后台新增 Tab 8」写。

手机号 + 短信验证码注册 / 登录要带 `access_key_secret`：它是机密，必须经 `sealForStore` 加密落库；而 `site` 段**有匿名可读的公开端点**（`PublicSite()` 直接返回整个 `conf.Site`），把密钥塞进 `site` 等于通过 `GET /site/site` 发给所有访客；塞进 `mail` 段也不行，那段语义是腾讯云 SES，两套密钥共用一段会让「改邮件配置顺手清掉短信密钥」变成可能，后台两个编辑页也会互相覆盖。因此 `sms` 是独立的一段，而不是扩写 `site` 或 `mail`。

## 决策

1. **十段（含 `sms`）收成一份段注册表**：`backend/internal/platform/config/registry.go` 的 `sectionRegistry` 是唯一事实，每项登记 `Name / Defaults / Decode / Apply`；段清单恰好十段：`site / ai / mail / storage / oauth / sms / log / upload / deco / image_gc`。`decodeAll`、`seedMissing`、`validateSection`、`applySection` 全部从 `RegisteredSections()` / `LookupSection()` 派生，不再各写一份 switch。`validateSection` 在**写库事务之前**查登记项与类型：未登记或类型不匹配直接返回错误，不留「库里写进去了、缓存没更新」的半状态；`applySection` 未登记则 panic（入口已保证，属于不可达防线）。单段 JSON 解析失败仍按旧口径「warn + 回退该段默认值」——「配置坏了」与「漏登记」严重性不同。`sms` 段的 `access_key_secret` 必须经 `sealForStore` 加密落库；登记覆盖面与失败语义由 `registry_test.go` 从 `UpdateXxx` 的**外部可见返回值与缓存状态**钉住，不读源码里有没有 `Warnf` / `switch` / `case`。
2. **十段（含 `sms`）全部具名进契约，自由键值块退场**：参数化 `GET /site/{name}`、`GET /site/admin/{name}`、`PUT /site/{name}`、`SiteInfoRequest` 与 `kernel.SiteConfigData` 一并删除。每段各有具名 `GET /site/admin/<段名>` 与 `PUT /site/<段名>`；`site` / `deco` 另有公开具名 `GET`（`deco` 无敏感字段、与管理读同形；公开 `site` 仍触发一次站点访问量埋点）。每段一对 `SiteSection<Name>Response` / `SiteSection<Name>Request`（`dto_runtime_sections.go`），底层是 `conf` 包的段结构、但用**新类型**而不是真别名，让 swag 生成稳定的具名 schema；十套响应各有 `kernel.Envelope[...]` 包装。绑定与审计由 `handler_runtime_sections.go` 一份泛型 `updateNamedSection` + 各 handler 写死的 `TargetID` 给出，审计口径（`runtime_config_update` + `TargetID=段名`）不变；路由表由 `site_router.go` 里一份十项循环注册。
3. **`sms` 段的边界**：`sms` 段只放发送凭证（`access_key_id` / `access_key_secret` / `endpoint` / `sign_name` / `template_code` / `timeout_sec`）；**所有登录策略与限流留在 `site.login`**，与邮件那几项并排（`phone_login` / `sms_code_timeout` / `sms_send_window_second` / `sms_send_per_phone_max` / `sms_send_per_ip_max`）——同一个概念只有一处。`email_register` 与 `email_login` 分开：此前两者共用一个开关，一关邮箱登录就再也发不出「绑定邮箱」的验证码，而绑定邮箱是手机号注册用户之后补邮箱的唯一入口。
4. **两端类型从契约派生，手写视图模型与源码文本断言退场**：`frontend/app/types/api.ts` 与 `frontend_admin/app/types/api.ts` 的 `SiteRuntimeConfig` / `SiteDecoConfig` / `SiteLoginRuntimeConfig` / `AIRuntimeConfig` / 其余各段视图模型改成 `RequiredSchema<"internal_site_adapter_http.SiteSection...">`（读者站 `SiteDecoConfig` 只在契约形状上叠加「数值键可为 null」的归一化语义，字段清单仍来自 `conf.Charm`）。`site-runtime-config-parity.test.ts` 里 `readFileSync` + Go `json` tag / TS 键的逐字段解析整体删除，换成 `expectTypeOf` 的**编译期类型对账**：消费的是两端各自从 `swagger.json` 生成的 `openapi.d.ts`，不读源码文本、不维护第三份字段清单，也不新增源码文本断言。
5. **契约三份产物同批重生成**：`backend/docs/swagger.json` 与两个 app 的 `app/types/openapi.d.ts` 由 `cd backend && make openapi` → 两端 `pnpm openapi:types` 同批产出，不手改、不跨 app 共享。`scripts/contract-structures.json` 的 `minimumEntries` 以实测登记，`addedIn` 记录各批新增结构；判据四文件（`scripts/check-contract-freshness.mjs`、`scripts/test-contract-freshness.mjs`、`scripts/verify-gates.sh`、`scripts/tool-versions.env`）不得为绕开条目变动而改——「只增不减」逻辑不变，变的是独立事实源里登记的实测数字。
6. **零 DDL 口径**：`-db` 只做加法（建表 + 按 `columnAdditions` 补列），不改名、不改类型、不删列，也不 DROP 任何表；`runtime_config_models` 仍是 `(name, payload)` 一段一行，运行时没有 AutoMigrate。零 DDL 口径见 `docs/architecture/overview.md` §六 A1。

## 理由与已否决方案

**当初「八组路由 + 八个 DTO」为何被推翻**：`docs/architecture/configuration.md` §八 记的两条代价在当时的实现下成立——拆开意味着 8 份 `var body conf.Xxx` + `ShouldBindJSON`、8 行额外路由、审计要多一层 name 分派。现在成本结构变了：十段（含 `sms`）只有**一份**注册表作事实源，绑定只靠一个泛型 `updateNamedSection[T]`，路由是一份十项循环，`TargetID` 在每个 handler 里写死（没有「多一层」），契约里的具名形状由 swag 从同一批注解生成。于是「路由表变长」不再是维护负担，而「不拆」的代价却一直在真实发生：契约派不出字段级类型 → 两端手抄视图模型 → 字段漂移 → 只能用读源码文本的门禁自证，而那道门禁恰好拦不住「后端改了 json tag、前端改了类型、两边一起改错」。

**为什么 `sms` 必须是独立段**：`sms` 含 `access_key_secret`，机密必须 `sealForStore` 加密落库；`site` 有匿名可读公开端点（`PublicSite()` 返回整个 `conf.Site`），放进去等于把密钥发给所有访客；`mail` 语义是腾讯云 SES，两套密钥共用一段会让后台两个编辑页互相覆盖。`sms` 只是注册表里多的一项，段注册表、具名端点、破坏性收缩三件事机制原样复用，不是新机制；登录策略与限流仍归 `site.login` 一处。

**接缝口径**：已确认的测试接缝 = 后端 HTTP 接口 + 契约（开放接口契约）。好测试只验接口外部行为：登记覆盖面从 `UpdateXxx` 返回值 / 缓存状态验，十段具名化从已生成的 `swagger.json` 验，类型同源从生成的 `openapi.d.ts` 在编译期验。不把「源码里有没有某个 case / 某行注释」当判据。

被否决的方案：

- **保留参数化路由、只补缺段的 case**：治不了契约没有形状这个根因，手写模型与源码文本断言都得留着。**否决**。
- **保留 `map[string]any`、给每段补一份「字段清单 JSON」当权威**：等于新增第三份事实源，正是本条要退场的东西。**否决**。
- **兼容层（具名端点与 `/site/{name}` 并存）**：旧路径一旦还在契约里就仍有自由键值块，两端会继续按旧形状派生，破坏性收缩不彻底。**否决**。
- **把短信凭证塞进 `site` 或 `mail`**：前者经公开端点泄露密钥，后者让两套密钥与两个编辑页互相覆盖。**否决**。
- **把登录策略 / 限流也放进 `sms` 段**：同一概念出现两处，两个段互相打脸；`email_register` 与 `email_login` 共用一个开关会堵死手机号用户补邮箱的唯一入口。**否决**。
- **为求绿下调基线或改动判据**：把「只增不减」判据本身改松，等于让门禁撒谎；按实测登记，判据逻辑不改。**否决**。

## 代价 / 不做什么

- 契约是破坏性变更：旧参数化端点与自由键值块整体下线，仍按 `/site/{name}` 调用的客户端会 404；两个前端的派生类型同批重生成。
- `minimumEntries` 以 `scripts/contract-structures.json` 的实测为独立事实源，「只增不减」判定逻辑生效；`addedIn` 只登记事实，不读源码文本。
- **`sms` 段伴随 `user_models.phone` 补列通道**：出处见 `docs/architecture/overview.md` §六 A1。
- 不做兼容层、不做按开关二选一的分支、不新增第二个测试接缝、不新增源码文本断言。既有契约类型双闸门（两端 `openapi.d.ts` 重新生成后逐字比对）、`check-contract-freshness.mjs` 的期望、契约↔路由双向对账（`TestContractMatchesRoutes`）、admin 鉴权行为探针与 `TestAdminOnlyListCoversContract` 继续作为守卫。
- 不做生效值模块 / 往返测试 / 敏感占位符 / 每天 99 条 / 展示开关四处收口，不改渲染边界，**零 DDL**。
- 不改 [ADR-0019](./0019-named-outline-trees-as-the-only-shape.md) 的结论：它记的是目录树形状，两条 ADR 各自独立，本条不标「取代」，也不回头改写它。

## 当前生效约束

1. 运行时配置恰好十段（含 `sms`）：`site / ai / mail / storage / oauth / sms / log / upload / deco / image_gc`。`sectionRegistry` 是唯一事实源；补种、全量解码、缓存应用、写入校验都从它派生。**漏登记必须在写库前失败**，不得回到「警告后缓存不更新」；单段 JSON 解析失败才降级为 warn + 该段默认值。
2. `sms` 段只放发送凭证（`access_key_id` / `access_key_secret` / `endpoint` / `sign_name` / `template_code` / `timeout_sec`），`access_key_secret` 必须经 `sealForStore` 加密落库；所有登录策略与限流留在 `site.login`，`email_register` 与 `email_login` 分开。
3. 对外只留十段具名端点：每段 `GET /site/admin/<段名>` 与 `PUT /site/<段名>`，`site` / `deco` 另各有公开 `GET`；参数化 `/site/{name}`、`/site/admin/{name}`、`SiteConfigData` 自由键值块、`SiteInfoRequest` 不得重新登记。公开读仍只有 `site` / `deco` 两段，读公开 `site` 触发一次站点访问量埋点；`site` 的公开端点不得承载任何密钥。
4. 每段一对具名响应/请求形状，字段只有 `conf` 包一份定义（新类型而非真别名）；两端类型一律从各自生成的 `openapi.d.ts` 派生，不得手写配置视图模型、不得维护第三份字段清单、不得新增解析 Go tag / TS 键的源码文本断言。
5. 三份契约产物（`backend/docs/swagger.json` + 两个 app 的 `app/types/openapi.d.ts`）必须同批由生成器产出；`minimumEntries` 以 `scripts/contract-structures.json` 的实测为独立事实源，`addedIn` 记录的各批结构（含手机号登录那批的结构）必须在产物中在场；判据四文件不得为绕开条目变动而改，`tool-versions.env` 是唯一版本来源。
6. 审计口径不变（`runtime_config_update` + `TargetID=段名`），`TargetID` 由具名 handler 写死；右栏卡片是独立运营位，不属于十段注册表。
7. 配置分层文档、根 `GLOSSARY.md`「运行时配置」词条与代码注释只写十段（含 `sms`）现状，否决理由链到本 ADR；不得出现「八个配置段 / 八个独立路由 / 八组路由 + 八个 DTO / 后台新增 Tab 8 / 第 8 段」这类旧口径。
8. 零 DDL：`-db` 只做加法（建表 + 按 `columnAdditions` 补列），不改名、不改类型、不删列，也不 DROP 任何表；`user_models.phone` 补列通道见 `docs/architecture/overview.md` §六 A1，零 DDL 口径见 `docs/architecture/overview.md` §六 A1。
9. 本条与 [ADR-0019](./0019-named-outline-trees-as-the-only-shape.md) 各自独立、互不引用为替代。

## 相关代码与文档

- 概念：[GLOSSARY · 运行时配置](../../GLOSSARY.md) · [GLOSSARY · 契约（开放接口契约）](../../GLOSSARY.md) · [配置分层](../architecture/configuration.md) · [契约](../architecture/contracts.md) · [架构总纲 §六 A1](../architecture/overview.md)（零 DDL 与补列通道）
- 上游决策：[ADR-0003](./0003-generated-contract-single-source.md)（契约以代码生成为单一来源）· [ADR-0014](./0014-deco-segment-and-cursor-charm.md)（deco 段当年口径，历史判断保留）· [ADR-0019](./0019-named-outline-trees-as-the-only-shape.md)（各自独立，非替代）
- 后端注册表：`backend/internal/platform/config/registry.go`（`SectionSMS` 等十段登记）、`registry_test.go`、`runtime.go`（`sealForStore`）、`secrets.go`、`runtime_section_read.go`
- 十段具名出入口：`backend/internal/site/adapter/http/{dto_runtime_sections.go,handler_runtime_sections.go,handler.go}`（`PublicSiteSection` / `PublicSite()` 返回整个 `conf.Site`）、`backend/internal/site/app/service.go`、`backend/internal/site/domain/port.go`、`backend/internal/site/apptest/env.go`、`backend/app/site_wiring.go`、`backend/internal/platform/http/router/{site_router.go,admin_gate_scan_test.go,admin_gate_test.go,runtime_sections_contract_test.go,contract_routes_test.go}`
- 段形状与消费方：`backend/conf/conf_sms.go`（`access_key_secret` / `endpoint` / `sign_name` / `template_code` / `timeout_sec`）、`backend/conf/site/enter.go`（`email_register` / `email_login` / `phone_login`）、[`platform/sms`](../../backend/internal/platform/sms/sms.go)（阿里云 PNVS 适配器）、`backend/app/user_wiring.go` 的 `smsCodeBridge`、`backend/internal/platform/redis/redis_user`（发码限流）
- 前端与门禁：`frontend/app/types/api.ts`、`frontend/test/gates/site-runtime-config-parity.test.ts`、`frontend_admin/app/types/api.ts`、`frontend_admin/app/services/admin.ts`、两个 app 的 `app/types/openapi.d.ts`
- 契约与守卫：`backend/docs/swagger.json`、`scripts/contract-structures.json`、`scripts/check-contract-freshness.mjs`、`scripts/test-contract-freshness.mjs`、`scripts/verify-gates.sh`、`scripts/tool-versions.env`
