# 2026-11-05 拆树·收缩与出处链（#277）发布说明与部署要点

- 批次：spec #272 第一批拆树的 **contract** 阶段（#277；expand #274 / 迁移 #275 #276 之后）。
- 范围：5 个旧共享端点与旧共用树表 `content_outline_node_models` 从登记、路由、handler、DTO、swag 注解、契约产物与三份 route guard 清单一并下线；两张具名树表与两个具名端点成为唯一对外形状。列表卡片归属树读模型收口一处（PUT 具名树 → 两处列表卡片篇数 / 封面同步）。
- 出处链：[ADR-0028](./../adr/0028-named-outline-trees-as-the-only-shape.md) 取代 [ADR-0023](./../adr/0023-outline-tree-for-collections-and-projects.md)（0023 只改状态行，正文一字不动）；`GLOSSARY.md` 新增「合集目录树」「项目目录树」，旧正名「目录树」只以退役写法留在两个新词条的 `_Avoid_` 里；「契约」词条不动。`docs/adr/README.md` 与 `docs/README.md` 索引同步到 28 条。
- 零 DDL：代码里只有 `CREATE TABLE IF NOT EXISTS`，没有 ALTER / DROP / 运行时 AutoMigrate；`-db` 不会 DROP 旧表，旧共用表的物理下线是**部署时手工 DROP**。

## 部署要点

1. 备份数据库。
2. 停服后按 `README.md` §11.4 的顺序**串行**重建镜像（内存受限机器不要并行构建）。
3. **手动跑一次建表**：`deploy/docker/api/entrypoint.sh` 只认 `runtime_config_models` 是否存在，升级时新表不会被自举。`-db` 只执行 `CREATE TABLE IF NOT EXISTS`，可重复跑，不会替你 DROP 任何表；当前两张具名树表由它幂等建出。

   ```bash
   docker compose run --rm blogx_server /app/server \
     -f config/settings.yaml -db
   ```

4. **部署时手工 DROP 旧共用树表**（代码与 `-db` 都不做这一步）：

   ```sql
   DROP TABLE IF EXISTS content_outline_node_models;
   ```

   这一步只删已从 `schema.sql`、testkit 与全部读面移除的旧表；两张具名树表不在其中。
5. `docker compose up -d --force-recreate blogx_web blogx_admin blogx_server`，然后 `docker compose restart nginx`（否则 502）。
6. 健康检查 + 冒烟：两个具名端点各读回一棵树，合集 / 项目两处列表卡片的篇数与封面与具名树一致；两个读者详情页、阅读页左栏照旧可用。

### `-db` 会 InitES 的坑

`-db` 启动时会 `InitES`，ES 不可达会直接 `fatal` 退出（见 `README.md` §11.4.4 与 [`2026-11-04-outline-tree-release-notes.md`](./2026-11-04-outline-tree-release-notes.md) 末尾）。所以跑第 3 步前必须先把 `es`（以及依赖它的 `mysql-slave`、`clickhouse`）起起来并等 `healthy`，否则建表命令会以 ES 连接失败告终，看起来像「建表脚本坏了」。

## 契约与类型

- 重新生成三份产物同批提交：`cd backend && make openapi` → `backend/docs/swagger.json`；两端各自 `pnpm openapi:types` → `frontend/app/types/openapi.d.ts` 与 `frontend_admin/app/types/openapi.d.ts`。不手改、不跨 app 共享。
- `scripts/contract-structures.json` 的 `minimumEntries` 重新登记为 `paths=119 / operations=159 / schemas=288`（原 124/164/292）。**下降来自 #274 expand 期票内过渡被 #277 收缩，不是生成器退化**：判据本体、`test-contract-freshness.mjs` 的 8 个期望、`verify-gates.sh` 5/14 注入与 `scripts/tool-versions.env` 这个唯一版本来源一字未改；#273 / #274 两批仍在契约中的新增结构继续留在 `addedIn`，被删的旧共享 schema 不在清单里。
- 保留并复用 `OutlineNodeRequest` / `OutlineNodeResponse` / `OutlineArticleRefResponse`（两个具名形状的叶子 DTO），`OutlineTreeResponse` 仍由两个详情响应使用；删掉的是 `OutlineSaveRequest`、`ArticleOutlineContextResponse` 与旧端点外壳。

## 代码下线清单（复核用）

- 后端：`adapter/http/{router.go,handler_outline.go,dto_outline.go}`、`app/{outline.go,article_context.go,named_outline.go,port.go,collection.go,outline_reader.go}`、`adapter/mysql/{outline_repo.go,rows.go,card_stats.go,uow.go,collection_repo.go,project_repo.go}`、`apptest/{repos.go,repo_outline.go}`、`domain/outline/outline.go`（去掉 `ListByArticle`）。
- 表集合与守卫：`internal/platform/mysql/ddl/schema.sql`、`internal/testkit/schema/{schema.go,outline_seed.go}`（后者整文件删除）、`internal/platform/mysql/ddl/ddl_integration_test.go`、`cmd/cli/flag_db_integration_test.go`（期望数仍从 `len(schema.Models(schema.All))` 推导）；`schema.go` 的 `All` 注释同步为 41 张表。
- 前端：`frontend/app/services/outline.ts` 去掉无来源回退（零请求 + 空读模型）、`frontend/app/utils/outlineContext.ts` 改从具名契约派生、`frontend_admin/app/types/api.ts` 删掉旧 `OutlineSavePayload`；两端服务与单测同步。

## 验收记录（本机，2026-11-05）

- 真 MySQL 8.0 容器：`TestNamedOutlineTreesDriveBothListCards`（PUT 两个具名树 → 读者 / 后台两处列表卡片篇数与封面同步）、两张具名表互不串数据、旧共享端点已下线；`ddl.Apply` 连跑两次验幂等 + `information_schema` 逐表列类型与 `article_id` 可空性。
- 库表集合双向无差：`TestTableNamesAgreeWithSchemaSQL`、`TestColumnNamesAgreeWithSchemaSQL`、`TestGeneratedRowsMatchSchemaSQL`（两个独立库）、`TestFlagDB`。
- 契约双闸门：`node scripts/check-contract-freshness.mjs` 判 `[reason] OK`（119/159/288 不低于重新登记的基线，19 项新增结构在场）；`node scripts/test-contract-freshness.mjs` 8/8；两端 `openapi.d.ts` 重新生成到临时文件再 `diff -q`。契约↔路由表双向对账与 14 道 canary 见本票提交说明的门禁记录。

## 已知限制

1. 本文只写手动 `-db` 与部署时手工 DROP 旧表的步骤，不含生产部署执行；部署结果由部署人按上面的命令回填。
2. 阅读页左栏在 `?from=` 缺失 / 不合法时不发请求、按「没有可展示的树」处理；共用呈现组件 / 版式仍是一份，两个入口、两张主表、两套规则不变，没有兼容层或开关分支。
3. 条目数基线按本批收缩后的实际产物重新登记；「只增不减」的判据继续生效，只是基线本身随批下调。
