# ADR-0029：配置段注册表与九段具名化

- 状态：已交付
- 日期：2026-11-06
- 来源：parent spec [#272](https://github.com/Jenlybein/NewblogX/issues/272) 契约阶段 · 落地票 [#283](https://github.com/Jenlybein/NewblogX/issues/283)（配置·段注册表与九段具名契约）。本条与 [ADR-0028](./0028-named-outline-trees-as-the-only-shape.md) 各自独立、互不引用为替代：0028 只记合集/项目目录树成为唯一对外形状，本条只记运行时配置段的注册表与具名契约。

## 背景与问题

运行时配置一直是「一段一行 JSON」（`runtime_config_models(name, payload)`，零 DDL）。#139 之后库里实际有**九段**：`site / ai / mail / storage / oauth / log / upload / deco / image_gc`。但写路径的四件事（默认值补种、全量解码、缓存应用、按段写入校验）曾各写一份 `switch name`；`applySection` 遇到没登记的段只 `Warnf("未知的配置段，缓存未更新")` 就返回——**库里写进去了、进程缓存却没更新**，且没有任何失败信号。新增第十段时漏登记一个分支，编译器、契约、路由门禁都不会红。

对外形状同时停在「八段」口径：`GET|PUT /site/{name}` 与 `GET /site/admin/{name}` 是参数化路由，响应写成 `kernel.SiteConfigData = map[string]any`、请求体写成 `object`。于是契约（开放接口契约）里没有字段级 schema 可派生：两个前端各手抄一份 `SiteRuntimeConfig` / `SiteDecoConfig` / `AIRuntimeConfig` / `MailRuntimeConfig` 等视图模型；#39 前端多写了后端没有的 `site_info.subtitle` 又漏了 `beian`，#56 把登录开关键写成 `qq/email/pwd` 而后端发的是 `qq_login/username_pwd_login/email_login`。为了拦这类漂移，`frontend/test/gates/site-runtime-config-parity.test.ts` 只能逐字段解析 Go `json` tag 与 TS 键的**源码文本**——第三份字段清单、且是「读源码字符串」而不是「验接口外部行为」的测试接缝。第九段 `image_gc` 已在库里有默认值与读路径，却没有 HTTP 出入口，因为「八段」口径没有它。

`docs/architecture/configuration.md` §八 当初把「拆成八组路由 + 八个 DTO」明确列为被否决方案，理由是「路由表变长、审计要多记一层」；§六、§五 的标题与段落也全部按「八个配置段 / 八个独立路由 / 后台新增 Tab 8」写。

## 决策

1. **九段收成一份段注册表**：`backend/internal/platform/config/registry.go` 的 `sectionRegistry` 是唯一事实，每项登记 `Name / Defaults / Decode / Apply`。`decodeAll`、`seedMissing`、`validateSection`、`applySection` 全部从 `RegisteredSections()` / `LookupSection()` 派生，不再各写一份 switch。`validateSection` 在**写库事务之前**查登记项与类型：未登记或类型不匹配直接返回错误，不留「库里写进去了、缓存没更新」的半状态；`applySection` 未登记则 panic（入口已保证，属于不可达防线）。单段 JSON 解析失败仍按旧口径「warn + 回退该段默认值」——「配置坏了」与「漏登记」严重性不同。登记覆盖面与失败语义由 `registry_test.go` 从 `UpdateXxx` 的**外部可见返回值与缓存状态**钉住，不读源码里有没有 `Warnf` / `switch` / `case`。
2. **九段全部具名进契约，自由键值块退场**：参数化 `GET /site/{name}`、`GET /site/admin/{name}`、`PUT /site/{name}`、`SiteInfoRequest` 与 `kernel.SiteConfigData` 一并删除。每段各有具名 `GET /site/admin/<段名>` 与 `PUT /site/<段名>`；`site` / `deco` 另有公开具名 `GET`（`deco` 无敏感字段、与管理读同形；公开 `site` 仍触发一次站点访问量埋点）。每段一对 `SiteSection<Name>Response` / `SiteSection<Name>Request`（`dto_runtime_sections.go`），底层是 `conf` 包的段结构、但用**新类型**而不是真别名，让 swag 生成稳定的具名 schema；九套响应各有 `kernel.Envelope[...]` 包装。绑定与审计由 `handler_runtime_sections.go` 一份泛型 `updateNamedSection` + 各 handler 写死的 `TargetID` 给出，审计口径（`runtime_config_update` + `TargetID=段名`）不变；路由表由 `site_router.go` 里一份九项循环注册。
3. **两端类型从契约派生，手写视图模型与源码文本断言退场**：`frontend/app/types/api.ts` 与 `frontend_admin/app/types/api.ts` 的 `SiteRuntimeConfig` / `SiteDecoConfig` / `SiteLoginRuntimeConfig` / `AIRuntimeConfig` / 其余各段视图模型改成 `RequiredSchema<"internal_site_adapter_http.SiteSection...">`（读者站 `SiteDecoConfig` 只在契约形状上叠加「数值键可为 null」的归一化语义，字段清单仍来自 `conf.Charm`）。`site-runtime-config-parity.test.ts` 里 `readFileSync` + Go `json` tag / TS 键的逐字段解析整体删除，换成 `expectTypeOf` 的**编译期类型对账**：消费的是两端各自从 `swagger.json` 生成的 `openapi.d.ts`，不读源码文本、不维护第三份字段清单，也不新增源码文本断言。
4. **契约三份产物同批重生成，基线按实测抬高**：`backend/docs/swagger.json` 与两个 app 的 `app/types/openapi.d.ts` 由 `cd backend && make openapi` → 两端 `pnpm openapi:types` 同批产出，不手改、不跨 app 共享。实测条目数从 119/159/288 上升到 **135/176/326**（净增 16 path / 17 operation / 38 schema：18 个具名 path 换掉 2 个参数化 path、20 个具名 operation 换掉 3 个、新增 40 个 schema 同时删掉 `SiteConfigData` 及其 `Envelope` 2 个）。`scripts/contract-structures.json` 的 `minimumEntries` 按实测登记为 135/176/326，`addedIn` 追加 `ticket: "283"` 的 18 个 path / 20 个 method / 27 个 schema；#273 / #274 的 19 项仍在契约中故全部保留。**判据四文件零改动**：`scripts/check-contract-freshness.mjs`、`scripts/test-contract-freshness.mjs`（8/8 期望）、`scripts/verify-gates.sh`（5/14 注入）、`scripts/tool-versions.env`（唯一版本来源）。基线抬高不是为绕过 `ENTRY_COUNT_DROPPED` 而改判据：「只增不减」逻辑一字未改，变的只是独立事实源里登记的实测数字。
5. **零 DDL 不变**：不改表结构、不新增表、不 ALTER；`runtime_config_models` 仍是 `(name, payload)` 一段一行，运行时没有 AutoMigrate。

## 理由与已否决方案

**当初「八组路由 + 八个 DTO」为何被推翻**：§八 记的两条代价在当时的实现下成立——拆开意味着 8 份 `var body conf.Xxx` + `ShouldBindJSON`、8 行额外路由、审计要多一层 name 分派。现在成本结构变了：九段只有**一份**注册表作事实源，绑定只靠一个泛型 `updateNamedSection[T]`，路由是一份九项循环，`TargetID` 在每个 handler 里写死（没有「多一层」），契约里的 18 个形状由 swag 从同一批注解生成。于是「路由表变长」不再是维护负担，而「不拆」的代价却一直在真实发生：契约派不出字段级类型 → 两端手抄视图模型 → #39 / #56 的字段漂移 → 只能用读源码文本的门禁自证，而那道门禁恰好拦不住「后端改了 json tag、前端改了类型、两边一起改错」。

**接缝口径**：已确认的测试接缝 = 后端 HTTP 接口 + 契约（开放接口契约）。好测试只验接口外部行为：登记覆盖面从 `UpdateXxx` 返回值 / 缓存状态验，九段具名化从已生成的 `swagger.json` 验，类型同源从生成的 `openapi.d.ts` 在编译期验。不把「源码里有没有某个 case / 某行注释」当判据。

**否决过的其它选项**：

- 保留参数化路由、只补第九段 `image_gc` 的 case：治不了契约没有形状这个根因，手写模型与源码文本断言都得留着。
- 保留 `map[string]any`、给每段补一份「字段清单 JSON」当权威：等于新增第三份事实源，正是本条要退场的东西。
- 兼容层（具名端点与 `/site/{name}` 并存）：旧路径一旦还在契约里就仍有自由键值块，两端会继续按旧形状派生，破坏性收缩不彻底。
- 为求绿下调基线或改动判据：把「只增不减」判据本身改松，等于让门禁撒谎；本批按实测**抬高**基线，判据零改动。

## 代价 / 不做什么

- 契约是破坏性变更：旧参数化端点与自由键值块整体下线，仍按 `/site/{name}` 调用的客户端会 404；两个前端的派生类型同批重生成。
- 条目数基线随批变动（119/159/288 → 135/176/326），「只增不减」的判定逻辑继续生效；`addedIn` 只登记事实，不读源码文本。
- 不做兼容层、不做按开关二选一的分支、不新增第二个测试接缝、不新增源码文本断言。既有契约类型双闸门（两端 `openapi.d.ts` 重新生成后逐字比对）、`check-contract-freshness.mjs` 的 8/8 期望、契约↔路由双向对账（`TestContractMatchesRoutes`）、admin 鉴权行为探针与 `TestAdminOnlyListCoversContract` 继续作为守卫。
- 不做 #284（生效值模块 / 往返测试 / 敏感占位符 / 每天 99 条 / 展示开关四处收口）、不动 #285–#287、不改渲染边界、**零 DDL**。
- 不改 [ADR-0028](./0028-named-outline-trees-as-the-only-shape.md) 的结论：它记的是目录树收缩与本批之前的 119/159/288 登记；两条 ADR 各自独立，本条不标「取代」，也不回头改写它。

## 当前生效约束

1. 运行时配置恰好九段：`site / ai / mail / storage / oauth / log / upload / deco / image_gc`。`sectionRegistry` 是唯一事实源；补种、全量解码、缓存应用、写入校验都从它派生。**漏登记必须在写库前失败**，不得回到「警告后缓存不更新」；单段 JSON 解析失败才降级为 warn + 该段默认值。
2. 对外只留九段具名端点：每段 `GET /site/admin/<段名>` 与 `PUT /site/<段名>`，`site` / `deco` 另各有公开 `GET`；参数化 `/site/{name}`、`/site/admin/{name}`、`SiteConfigData` 自由键值块、`SiteInfoRequest` 不得重新登记。公开读仍只有 `site` / `deco` 两段，读公开 `site` 触发一次站点访问量埋点。
3. 每段一对具名响应/请求形状，字段只有 `conf` 包一份定义（新类型而非真别名）；两端类型一律从各自生成的 `openapi.d.ts` 派生，不得手写配置视图模型、不得维护第三份字段清单、不得新增解析 Go tag / TS 键的源码文本断言。
4. 三份契约产物（`backend/docs/swagger.json` + 两个 app 的 `app/types/openapi.d.ts`）必须同批由生成器产出；`minimumEntries` 以 `scripts/contract-structures.json` 为独立事实源（当前 135/176/326），判据四文件不得为绕开条目变动而改；`addedIn` 里 273 / 274 / 283 三批结构必须在重新生成的产物中在场。
5. 审计口径不变（`runtime_config_update` + `TargetID=段名`），`TargetID` 由具名 handler 写死；右栏卡片是独立运营位，不属于九段注册表。
6. 配置分层文档、根 `GLOSSARY.md`「运行时配置」词条与代码注释只写九段现状，否决理由链到本 ADR；不再出现「八个配置段 / 八个独立路由 / 八组路由 + 八个 DTO / 后台新增 Tab 8 / 第 8 段」这类口径。
7. 本条与 ADR-0028 各自独立、互不引用为替代；零 DDL 口径不变：`-db` 只做 `CREATE TABLE IF NOT EXISTS`，不 DROP / ALTER。

## 相关代码与文档

- 概念：[GLOSSARY · 运行时配置](../../GLOSSARY.md) · [GLOSSARY · 契约（开放接口契约）](../../GLOSSARY.md) · [配置分层](../architecture/configuration.md) · [契约](../architecture/contracts.md)
- 上游决策：[ADR-0003](./0003-generated-contract-single-source.md)（契约以代码生成为单一来源）· [ADR-0016](./0016-deco-segment-and-cursor-charm.md)（第 8 段 deco 的当年口径，历史判断保留）· [ADR-0028](./0028-named-outline-trees-as-the-only-shape.md)（各自独立，非替代）· [架构总纲 §六 A1](../architecture/overview.md)（零 DDL）
- 后端注册表：`backend/internal/platform/config/registry.go`、`registry_test.go`、`runtime.go`、`runtime_section_read.go`
- 九段具名出入口：`backend/internal/site/adapter/http/{dto_runtime_sections.go,handler_runtime_sections.go,handler.go}`、`internal/site/app/service.go`、`internal/site/domain/port.go`、`internal/site/apptest/env.go`、`app/site_wiring.go`、`internal/platform/http/router/{site_router.go,admin_gate_scan_test.go,admin_gate_test.go,runtime_sections_contract_test.go,contract_routes_test.go}`
- 前端与门禁：`frontend/app/types/api.ts`、`frontend/test/gates/site-runtime-config-parity.test.ts`、`frontend_admin/app/types/api.ts`、`frontend_admin/app/services/admin.ts`、两个 app 的 `app/types/openapi.d.ts`
- 契约与守卫：`backend/docs/swagger.json`、`scripts/contract-structures.json`、`scripts/check-contract-freshness.mjs`、`scripts/test-contract-freshness.mjs`、`scripts/verify-gates.sh`、`scripts/tool-versions.env`
