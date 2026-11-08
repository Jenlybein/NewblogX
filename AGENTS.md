# blogx

个人博客项目：Go 后端（`backend/`，module `myblogx`）+ 两个 Nuxt 前端（`frontend/` 读者站、`frontend_admin/` 管理后台）+ `deploy/` 基础设施。前后端唯一耦合是后端生成的 OpenAPI 契约。

- 运行 / 部署 / 运维：看 [`README.md`](./README.md)
- 文档入口：看 [`docs/README.md`](./docs/README.md)
- 领域词汇：看 [`GLOSSARY.md`](./GLOSSARY.md)

## 分支与 CI

- **`main` 只走 PR，禁止直接 push。** 分支保护要求 13 项状态检查，但管理员权限能 bypass ——
  "能推上去"不等于"合规"，不要用 bypass 当捷径。流程一律是：开分支 → push → `gh pr create`
  → 等检查全绿 → squash merge → 删分支。
- **push 之后必须核对 CI 结果，不能只看本地绿灯。** 本地通过和 CI 通过是两件事：
  CI 跑的是 `scripts/gates/run.mjs --group <group>` 全组 + `archcheck --skip-tests=false` 那遍
  + `test-contract-freshness.mjs` 的 8 条行为用例 + 两端 lint/typecheck/`openapi:types`。
  只挑自己熟悉的那几条跑，就会漏。核对命令：

  ```bash
  gh pr checks <pr> # PR 的检查
  gh run list --branch main --limit 3   # 直接推过的话，看 main 上那次
  gh run view <run-id> --log-failed     # 红了先看失败 job 的日志再动手
  ```

  历史上 `#302` 一批连续三次红在 `contract up-to-date` / `archcheck` / `frontend lint`，
  都是"本地口径比 CI 少一步"造成的：漏抬高 `scripts/contract-structures.json` 的
  `minimumEntries`、测试文件里用了裸 `uint64`、手改生成物多留一个换行。

## 部署

- **顺序固定：先 `compose down`，再逐个容器串行重构建，最后 `up -d`。**
  一次性 `up -d --build` 会并行构建三个镜像；生产机内存小（3.6G），两个 Nuxt 构建同时跑
  会把机器压进换页抖动，连 sshd 都起不来（`#302` 就此让线上停了近 40 分钟）。
  串行构建 + `NODE_OPTIONS=--max-old-space-size=1024` 才稳。
  先 `down` 的意义是：不留旧容器，`up` 时 nginx 重新解析 upstream，避免它握着旧容器 IP
  导致 502。

  ```bash
  cd deploy/compose/production
  docker compose down                       # 不要加 -v：数据在命名卷里
  docker compose build blogx_server
  NODE_OPTIONS=--max-old-space-size=1024 docker compose build blogx_web
  NODE_OPTIONS=--max-old-space-size=1024 docker compose build blogx_admin
  docker compose up -d                      # 先起服务（含 mysql / redis）
  docker compose ps                         # 等全部 healthy
  # 有新表 / 改了 schema.sql 才需要，语句都是 CREATE TABLE IF NOT EXISTS，可重复跑。
  # 必须在**新镜像构建之后**：schema.sql 是 go:embed 进二进制的，旧镜像里没有新表。
  # 用 exec 而不是 `run --rm --no-deps`：这条命令会先初始化整条基础设施（Redis 也在内），
  # 栈停着的时候 run 会以 `lookup redis on 127.0.0.11:53: server misbehaving` 退出
  # （2026-11-08 踩过，且因为脚本 set -e 而卡在 up 之前，站点一直不起来）。
  docker compose exec blogx_server /app/server -f config/settings.yaml -db
  ```

- **动手前先看机器余量**：`free -h` / `df -h`。并把当前镜像打回滚标签
  （`docker tag blogx-server:production blogx-server:<日期>-pre<票号>`），库先备份到 `backups/`。
- **部署完要验收**，不是看容器 healthy 就结束：站点 200、`/robots.txt`、`/api/sitemap.xml`、
  新端点的鉴权（匿名应返回 `code:1003` 而不是 404），以及 `docker logs` 里没有启动错误。

## Agent skills

### Issue tracker

Issues 在 GitHub Issues（`Jenlybein/NewblogX`），用 `gh` CLI 读写。见 `docs/agents/issue-tracker.md`。

### Triage labels

五个 triage 角色与标签字符串同名（`needs-triage` / `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`）。见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文（single-context）：根目录 `GLOSSARY.md` + `docs/adr/`。见 `docs/agents/domain.md`。
