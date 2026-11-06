# ADR-0028：合集目录树与项目目录树成为唯一对外形状

- 状态：已交付
- 日期：2026-11-05
- 来源：parent spec [#272](https://github.com/Jenlybein/NewblogX/issues/272) 契约阶段 · 落地票 [#277](https://github.com/Jenlybein/NewblogX/issues/277)（拆树·收缩与出处链）· 取代 [ADR-0023](./0023-outline-tree-for-collections-and-projects.md)

## 背景与问题

#274 expand 期为了让 #275 / #276 逐张保持全量门禁绿，票内暂留了 5 个旧共享端点（`GET|PUT /collections|/projects/{id}/outline*` 与 `GET /articles/{id}/outline-context`）与旧共用树表 `content_outline_node_models`，同时新增两张按实体命名的树表与各自具名端点。那只是**同一批票内部的过渡**，不是长期兼容层。两套端点、三张表并存时，读面出现了分歧：#276 把项目列表卡片改成读项目具名树，#275 的合集列表卡片仍读旧共用表——同一份「PUT 具名树」会让两个入口的篇数 / 封面一真一假。契约基线也停在 expand 期的峰值（`paths=124 / operations=164 / schemas=292`），低于它就会报 `ENTRY_COUNT_DROPPED`；而收缩必然让条目数下降。

## 决策

1. **对外只留两个具名形状**：`GET|PUT /collections/{id}/outline-tree` 与 `GET|PUT /projects/{id}/outline-tree`。5 个旧共享端点从路由、handler、DTO、swag 注解、契约产物与三份 route guard 清单一并删除；`content_outline_node_models` 从 `schema.sql`、testkit `schema.All` / `Outline` 组、行结构、仓储与真库夹具中全部删除。`schema.sql` 与 `schema.Models(schema.All)` 双向无差集仍是唯一权威，`All` 注释同步为 41 张表。
2. **归属树读模型收口为一处**：列表卡片调用方只传「哪个 owner」，实现内部按归属走 `collection_outline_node_models` / `project_outline_node_models`；旧共用表不再有任何读取点。读者详情、阅读页左栏与后台编辑器同样只经具名树仓储，事务入口一次给出归属（`NamedOutlineRepository`）。
3. **阅读页左栏不再有回退分支**：归属只由 `?from=<kind>:<id>` 给出；来源缺失或无法解析时按「没有可展示的树」返回空读模型、不打任何请求。共用呈现组件 / 版式仍只有一份，两个入口、两张主表、两套规则不变，不加兼容层、不加开关分支、不新增第二测试接缝。
4. **契约基线同批重新登记**：`scripts/contract-structures.json` 的 `minimumEntries` 从 124/164/292 改为 119/159/288。**下降来自 #274 expand 期票内过渡被 #277 收缩，不是生成器退化**；判据本体、`test-contract-freshness.mjs` 的 8 个期望、`verify-gates.sh` 5/14 注入与 `scripts/tool-versions.env` 这个唯一版本来源一字未改，#273 / #274 两批仍在契约中的新增结构继续留在 `addedIn`。三份产物（`backend/docs/swagger.json` 与两个 app 的 `app/types/openapi.d.ts`）同批由 `make openapi` + 各 app `pnpm openapi:types` 重新生成，不手改、不跨 app 共享。
5. **零 DDL 不变**：代码里只有 `CREATE TABLE IF NOT EXISTS`，没有 DROP / ALTER / 运行时 AutoMigrate；`-db` 不会 DROP 旧表，旧表的物理下线是**部署时手工 DROP**。

## 理由与已否决方案

本批迁移成本最低。

## 代价 / 不做什么

- 契约是破坏性收缩：旧共享端点与旧共用形状不再出现在契约里，两个前端的派生类型同批重生成；任何仍按旧路径调用的客户端会 404。
- 条目数基线按本批收缩后的实际产物重新登记；「只增不减」的判据逻辑继续生效，只是基线本身随批下调。
- 不做兼容层、不做按开关二选一的分支、不新增源码文本断言；既有 `schema.Models(schema.All)` ↔ `schema.sql` 双向无差集、`columns_static_test.go`、`schema_sql_integration_test.go`、`TestFlagDB` 与契约↔路由双向对账继续作为守卫。
- 不在代码里 DROP / ALTER 任何表，不新增 DDL；运行时建表仍走 `-db`（它会顺带跑 `InitES`，ES 未起时会 fatal，部署顺序见发布说明）。

## 当前生效约束

1. 层次只能存在于 `collection_outline_node_models` / `project_outline_node_models` 两张具名树表；不得在文章表、合集表、项目表或任何既有表上加层次字段，`content_outline_node_models` 不得回到 `schema.sql`、testkit 或任何读面。
2. 对外树端点只有 `GET|PUT /collections/{id}/outline-tree` 与 `GET|PUT /projects/{id}/outline-tree`；`GET /articles/{id}/outline-context` 与 `GET|PUT /collections|/projects/{id}/outline[/admin]` 不得重新登记。
3. 归属树读模型只有一处：调用方给出 owner，实现内部按归属选表；列表卡片篇数 / 封面必须与同一归属的具名树同步（真 MySQL 行为用例钉住「PUT 具名树 → 两处列表卡片同步」）。
4. 阅读页左栏只在 `?from=<kind>:<id>` 指向某棵具名树时出现；来源缺失 / 不合法时不发请求、不猜默认归属，右栏标题大纲始终保留。
5. 读者共用组件与版式仍是一份，两个入口、两张主表、两套规则不变；不得引入兼容层、开关分支或第二个测试接缝。
6. 零 DDL 口径不变：`-db` 只做 `CREATE TABLE IF NOT EXISTS`，不 DROP；旧共用表由部署时手工 DROP。契约基线以 `scripts/contract-structures.json` 为独立事实源，判据实现不得为绕开下降而改。
7. `OutlineNodeRequest` / `OutlineNodeResponse` / `OutlineArticleRefResponse` 是两个具名形状复用的叶子 DTO，不因「名字带 Outline」删除；`OutlineTreeResponse` 仍由两个详情响应使用。

## 相关代码与文档

- 概念：[GLOSSARY · 合集目录树](../../GLOSSARY.md) · [GLOSSARY · 项目目录树](../../GLOSSARY.md) · [GLOSSARY · 合集](../../GLOSSARY.md) · [GLOSSARY · 项目](../../GLOSSARY.md)
- 上游决策：[ADR-0023](./0023-outline-tree-for-collections-and-projects.md)（已取代）· [ADR-0003](./0003-generated-contract-single-source.md)（契约以代码生成为单一来源）· [ADR-0019](./0019-reader-rendering-and-cache-strategy.md)（渲染边界）· [架构总纲 §六 A1](../architecture/overview.md)（零 DDL）· [契约](../architecture/contracts.md)
- 后端：`backend/internal/article/app/named_outline.go`、`app/outline.go`、`app/outline_reader.go`、`app/collection.go`、`app/project.go`、`adapter/mysql/card_stats.go`、`adapter/mysql/collection_repo.go`、`adapter/mysql/project_repo.go`、`adapter/http/router.go`、`adapter/http/handler_named_outline.go`、`adapter/http/dto_outline.go`、`internal/platform/mysql/ddl/schema.sql`、`internal/testkit/schema/schema.go`
- 契约与守卫：`backend/docs/swagger.json`、`scripts/contract-structures.json`、`scripts/check-contract-freshness.mjs`、`backend/internal/platform/http/router/{retired_flat_include_test.go,retired_shared_outline_test.go,admin_gate_scan_test.go,contract_routes_test.go}`
- 前端：`frontend/app/services/outline.ts`、`frontend/app/utils/outlineContext.ts`、`frontend_admin/app/services/admin.ts`、两个 app 的 `app/types/openapi.d.ts`
- 部署与发布：[发布说明](../notes/2026-11-05-outline-tree-contract-release-notes.md) · [README §11.3](../../README.md) · [README §11.4.4](../../README.md)

## 后续修订（2026-11-06，#286 参数门禁生成式）

本条只追加后续修订，0028 正文一字未改。#286 把 `deco.charm` 的 12 个数值参数收进 `config/deco-charm-parameters.json` 一张仓库级参数表；构建期生成后端、默认种子、读者站、后台与契约（开放接口契约）五处消费物。`conf.Charm` 的字段事实以 min/max/default 进入契约，三份产物同批重新生成；契约基线按实测仍为 **135/176/326**（本批只在既有 schema 上补字段事实，没有新增路径 / schema），`addedIn` 追加 `ticket: "286"` 并在 `comment` 保留 #277（124/164/292→119/159/288）与 #283（→135/176/326）的历史理由；判据**实现**三份 `check-contract-freshness.mjs` / `test-contract-freshness.mjs` / `tool-versions.env` 相对 `388a21e` 零 diff，不复制工具版本字面量；`verify-gates.sh` 是 canary 编排/消费方，按票 #286 AC 3 要求 14/14 整节演练退场，**仅**文件头注释与第 14 节（7 个 hunk）改动，#273 的 5/14、版本安装与判定逻辑、其余 13 道 canary 一字未动，判定未被绕过。ADR-0003 正文不动：契约仍以生成产物为单一来源，本批只是把参数事实并入同一条生成链。旧范围门禁、629 行门禁测试、锚点清单、夹具消费检查与 14/14 整节演练退场；其余 13 道 canary 保留。共享归一化夹具（122 条）与三套消费测试保留，挂件物理链条与指针端偏移行为不变。本批零 DDL。
