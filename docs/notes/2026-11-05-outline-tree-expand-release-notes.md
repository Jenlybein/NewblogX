# 2026-11-05 拆树·扩展（#274）发布说明与部署要点

- 批次：spec #272 第一批拆树的 **expand** 阶段（#274）。
- 范围：新增两张按实体命名的独立树表、两个具名形状（`CollectionOutlineTreeResponse` / `ProjectOutlineTreeResponse`）与两个具名端点（`GET|PUT /collections/{id}/outline-tree`、`GET|PUT /projects/{id}/outline-tree`）；契约同批重生成，两端 `openapi.d.ts` 各自派生类型。
- 旧共享端点（`GET|PUT /collections|/projects/{id}/outline*` 与 `GET /articles/{id}/outline-context`）票内**暂留**、旧共用树表 `content_outline_node_models` 逐字保留。新旧端点并存只是**本票内部过渡**，让 #275 / #276 能逐张保持全量门禁绿，**不是长期兼容层**，由 #277 收缩回单一形状。
- 零 DDL：只新建表。既没有给任何既有表加列 / 改列 / 删列，也没有 ALTER、DROP、运行时 AutoMigrate。

## 新增表逐张指名立项

按「新建表需要 ADR 立项」的口径（`docs/adr/0022` 决策 1；`README.md` §11.3 / §十三第 1 条），本批两张新表逐张指名如下；它们只走 `backend/internal/platform/mysql/ddl/schema.sql` 的 `CREATE TABLE IF NOT EXISTS`，并在 testkit 三处登记（`named_outline_seed.go` 的行结构 / `schema.go` 的 `Outline` 组与写死 `All` 清单）与生产侧 `backend/internal/article/adapter/mysql/rows.go` 的两份私有 row 同步在案。两张表去掉归属列 `owner_kind`，唯一键与索引收成 `(owner_id, article_id)`，`article_id` 可空（文件夹节点为 NULL）。

1. **`collection_outline_node_models`（合集目录树）**：`UNIQUE KEY uk_collection_outline_owner_article (owner_id, article_id)`、`KEY idx_collection_outline_owner (owner_id, article_id)`。对应具名端点 `/collections/{id}/outline-tree` 与具名形状 `CollectionOutlineTreeResponse`。
2. **`project_outline_node_models`（项目目录树）**：`UNIQUE KEY uk_project_outline_owner_article (owner_id, article_id)`、`KEY idx_project_outline_owner (owner_id, article_id)`。对应具名端点 `/projects/{id}/outline-tree` 与具名形状 `ProjectOutlineTreeResponse`。

> 两张表是**本批迁移成本最低**的拆法：合集与项目各一张表，读写路径按端点身份选表，一篇文章可同时进两棵树。出处链的完整取代动作（新 ADR 0028 取代 0023、旧表从登记中移除）归 #277；本票不重写 0023 正文、不下线旧表。

## 部署要点

1. 备份数据库。
2. 停服后按 `README.md` §11.4 的顺序**串行**重建镜像（内存受限机器不要并行构建）。
3. **手动跑一次建表**：`deploy/docker/api/entrypoint.sh` 只认 `runtime_config_models` 是否存在，升级时新表不会被自举，缺了这一步用到新表的两个具名端点会报「表不存在」而其它功能照常。

   ```bash
   docker compose run --rm blogx_server /app/server \
     -f config/settings.yaml -db
   ```

   `-db` 只执行 `CREATE TABLE IF NOT EXISTS`，可重复跑、不会 ALTER 既有表、也不会替你 DROP 任何表。产出上面**两张**新表；旧共用树表继续存在。
4. `docker compose up -d --force-recreate blogx_web blogx_admin blogx_server`，然后 `docker compose restart nginx`（否则 502）。
5. 健康检查 + 冒烟：两个具名端点各读回一棵空树；旧共享端点与两个读者详情页、阅读页左栏照旧可用。

### `-db` 会 InitES 的坑

`-db` 启动时会 `InitES`，ES 不可达会直接 `fatal` 退出（见 `README.md` §11.4.4 与 [`2026-11-04-outline-tree-release-notes.md`](./2026-11-04-outline-tree-release-notes.md) 末尾）。所以跑第 3 步前必须先把 `es`（以及依赖它的 `mysql-slave`、`clickhouse`）起起来并等 `healthy`，否则建表命令会以 ES 连接失败告终，看起来像「建表脚本坏了」。

## 契约与类型

- 重新生成经过 #273 修好的流水线：`cd backend && make openapi` → `backend/docs/swagger.json`；两端各自 `pnpm openapi:types` → `frontend/app/types/openapi.d.ts` 与 `frontend_admin/app/types/openapi.d.ts`。三份产物同批提交。
- `scripts/contract-structures.json` 追加 `ticket: "274"` 条目，登记本次新增的两个 path（各 `get` / `put`）与 6 个具名 schema；`minimumEntries` 基线抬到 `paths=124 / operations=164 / schemas=292`。
- `frontend/app/utils/outlineContext.ts` 与 `frontend_admin/app/types/api.ts` 的字面 schema 名派生点同批对齐；消费端页面切到新端点归 #275 / #276。

## 验收记录（本机，2026-11-05）

- 真 MySQL 8.0 容器上跑过的契约用例：两张新表各自的整树提交、失效文章过滤、封面回退、上一章 / 下一章、`?from=<kind>:<id>` 来路、两表互不串数据、旧共享端点仍可用；`ddl.Apply` 连跑两次验幂等 + `information_schema` 逐表列类型与 `article_id` 可空性。
- 库表集合双向无差：`TestTableNamesAgreeWithSchemaSQL`、`TestColumnNamesAgreeWithSchemaSQL`、`TestGeneratedRowsMatchSchemaSQL`（两个独立库）、`TestFlagDB`。
- 契约双闸门：`node scripts/check-contract-freshness.mjs` 判 `OK`；两端 `openapi.d.ts` 重新生成到临时文件再 `diff -q`。契约↔路由表双向对账与 14 道 canary 见本票提交说明的门禁记录。

## 已知限制

1. **expand 期的存取关系**：两个具名端点只读写自己那张新表；旧共享端点仍读写旧共用表 `content_outline_node_models`。两套数据在本票内互不复制、互不回退——这是票内过渡，不是兼容层；消费端迁移由 #275（合集）/ #276（项目）各自完成，旧端点与旧表由 #277 收缩。
2. 本票不改后台树编辑器、读者详情页 / 阅读页左栏、管理员可见性统一：那是 #275 / #276 的范围。读者侧呈现层仍只留一份。
3. 本文只写手动建表步骤，不含生产部署执行；部署结果由部署人按上面的命令回填。
