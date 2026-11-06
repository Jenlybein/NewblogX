# 2026-11-04 目录树批次发布说明与冒烟清单

- 批次：#209 合批（#210–#221 交付，code-review 后补 #223 / #224；收尾清理 #225）
- 范围：合集/项目的目录树、阅读页左栏与「目录」抽屉、后台树编辑器、裁剪窗、后端上传用途与比例校验、封面兜底、正文渲染主题、创作页预览主题退场、文章页编辑入口、私信 WS Origin 白名单；#225 另删旧扁平收录接口与两张旧中间表的代码登记、清 `staleItems` 死代码、裁剪窗纳入同源门禁、折叠状态随路由参数重建
- 契约：合集/项目详情从扁平 `articles[]` 改为嵌套 `outline`（破坏性变更），两端同批发版；#225 又删掉 13 条旧扁平收录/排序 operation（保留 `PATCH /projects/order`）
- 数据库：新增 `content_outline_node_models`；零 ALTER；两张旧扁平收录中间表在 #225 里从 `schema.sql` 与 testkit 登记中删除，生产库的两张表由部署时**手动 DROP**（代码里不做任何 DDL）

## 部署要点

1. 备份数据库。
2. 停服后**串行**重建三个镜像（内存受限机器不要并行构建）。
3. 用新镜像手动跑一次建表（entrypoint 只认 `runtime_config_models`，新表不会被自举）：

   ```bash
   docker compose run --rm blogx_server /app/server \
     -f config/settings.yaml -db
   ```

   产出 `content_outline_node_models`（合集与项目共用的目录树表）。
4. `docker compose up -d --force-recreate blogx_web blogx_admin blogx_server`，然后 `docker compose restart nginx`（否则 502）。
5. 健康检查 + 下面的冒烟清单。

旧扁平收录中间表 `collection_article_models` / `project_article_models` 自 #218 起不再被读写；#225 已把它们的建表登记与仓储代码全部删掉（`schema.sql` 与 testkit 都不再登记，运行时 `-db` 只做 `CREATE TABLE IF NOT EXISTS`、不会重建它们）。生产库的这两张表由部署时手动 DROP（上线时均为 0 行），部署后由管理员在树编辑器里重新编排。完整部署小节见 [README §11.4.4](../../README.md#1144-本批209-合批目录树的一次性部署序列)。

## 冒烟清单

逐项在真实环境（后台 + 读者站，外网访问）验证：

| # | 项目 | 通过标准 |
| --- | --- | --- |
| 1 | 目录树 | 后台在合集/项目里新建文件夹、批量收录文章、整树保存成功；刷新读者站详情，看到同构的嵌套树（含空文件夹）；文章节点点进去带 `?from=<kind>:<id>` |
| 2 | 阅读页左栏与「目录」抽屉 | 从详情点入的文章页左栏出现同一棵树，当前节点高亮，上一篇/下一篇可跳；窄屏 <1024px 收进「目录」抽屉；文章不属于任何树时不出现左栏，右栏标题大纲仍在 |
| 3 | 编辑入口 | 作者本人打开自己的文章页，元信息行出现「编辑」，点击进入创作页编辑态；他人/匿名看不到 |
| 4 | 封面兜底 | 无封面卡片是与父容器同底色的 16:9 空白，**没有 "NO COVER" 文案**；封面 URL 失效同样退化为空白；文章详情无封面时不渲染图区 |
| 5 | 裁剪窗 | 与目标比例相差 >1% 的封面/头像弹出裁剪窗（固定比例框 + 拖动 + 1–3× 缩放），确认后上传；取消则不上传、原封面保留；已合规的图直接自动居中裁、不弹窗 |
| 6 | 私信页「实时已连接」 | 读者站与后台私信页显示「实时已连接」；在线时收到新消息提示 |
| 7 | 上传比例校验 | 封面按 16:9、头像按 1:1 校验；明显不合规的图被后端判 failed 并返回明确错误码。（DNS 已恢复，本项**必做**，不再跳过） |
| 8 | 列表卡片与右栏篇数 | 只在树里编排（旧中间表为空）后，首页/列表里的合集与项目卡片、首页右栏「合集」卡显示树里**公开文章**的篇数；封面按 DFS 首篇公开文章回退，首篇无封面时卡片为空白兜底（不借用后一篇） |

## 已知限制

1. **上传 579（运行侧）：前置条件已解除，闭环待部署后复验**：`gentlybeing.cn` 与 `image.gentlybeing.cn` 的 DNSPod 解析已于 2026-11-04 恢复（见下节），回调地址不再因 NXDOMAIN 打不到业务服务器。但「裁剪 → 上传 → 七牛回调 → 任务 completed」这条真实闭环**尚未**在线上跑过——本分支不改上传链路，也不代跑线上复验。
2. **`/article/**` 直链 SSR 首屏没有左栏**：树的上下文只在客户端取数（先骨架占位），首屏 HTML 里没有左栏；渲染边界维持 ADR-0019，不改白名单。
3. **`from` 缺失时的回退顺序**：回退到"第一棵归属树"，顺序钉为 `(owner_kind, owner_id, id)` 升序，保证同一份数据每次落到同一棵树；多树合并视图不在本批范围。
4. **旧扁平收录/排序 HTTP 路由已删除（#225）**：`collections/projects` 的收录、批量收录、项目内排序、失效清理等旧接口已连同两张旧中间表的代码登记一起下线（保留 `PATCH /projects/order`）；契约与两端 `openapi.d.ts` 已同步。生产库的两张旧表由部署时手动 DROP，代码里不做 DDL。
5. **上传用途可选**：未声明用途的上传（例如私信发图）不做比例校验；只有封面 / 头像任务记录目标比例并校验。

## 交付后补齐的两处读面修正（#223 / #224）

code-review 两轴在本批上又查出两处「迁移没做到底」，已在同一 PR 内修完：

1. **列表卡片与右栏投影仍读冻结旧表**（#223）：合集/项目卡片的篇数与封面回退、首页右栏「合集」卡，原本还在读 `collection_article_models` / `project_article_models`。迁移后只在树里编排时，卡片会显示 0 篇、无回退封面。现在四处读面（读者卡片、后台卡片、右栏、详情）统一走 `outline.ReaderView` 一份遍历；站点模块不再有合集 SQL，改由端口注入。
2. **封面回退口径校准**（#223）：由「第一个带封面的公开文章」改为「DFS 第一个公开文章节点的封面」。首篇没有封面就是空（前端按空白兜底），**不再借用后一篇**——与 ADR-0023 决策 4 一致，观感与旧版不同属预期。
3. **后台树对软删文章**（#223）：保留文章标题并把失效原因区分为 `deleted`（文章软删）与 `missing`（节点指向的文章不存在），与旧扁平 LEFT JOIN 口径一致。
4. **读者详情不再吞错**（#224）：仓储返回非「文章不存在」的错误时上抛（页面走中性错误态），不再把数据库错误当成"文章已删除"静默隐藏。

## 部署记录（2026-11-04，已完成）

- 代码：`main` @ `97d0a38`（PR #222 squash 合入，Closes #210–#221、#223–#225）
- 镜像：`blogx-server` / `blogx-web` / `blogx-admin` 三个 `:production` **串行**重建（服务器 3G 内存，构建前先 `compose down` 释放内存）
- 建表：`docker compose run --rm blogx_server /app/server -f config/settings.yaml -db` → exit 0，产出 `content_outline_node_models`
- **旧表物理下线**：确认 `collection_article_models` / `project_article_models` 各 0 行后 `DROP TABLE`；库内表集合与 `schema.sql` 完全一致（39 = 39，无多余、无缺失）
- 重启：`up -d --force-recreate blogx_web blogx_admin blogx_server` + `up -d nginx` + `restart nginx`（nginx 会缓存上游 IP）

部署后**已验**（在生产服务器上实跑）：

| 检查 | 结果 |
| --- | --- |
| `GET https://127.0.0.1/health` | 200 |
| `GET /`、`/collections`、`/projects` | 200 |
| `GET /admin/`（后台站） | 200 |
| `GET /api/collections` | 200（`{"code":0,"count":0,"list":[]}`） |
| `GET /api/articles/1/outline-context` | 200（新接口在位） |
| `POST /api/collections/{id}/articles`（旧收录） | **404**（已下线） |
| `DELETE /api/collections/{id}/admin/stale`（旧清理） | **404**（已下线） |
| `blogx-server` 启动后 10 分钟日志 | 无 `error` / `fatal` |
| `nginx` 最近日志 | 无上游 `502` / `503` |
| 容器状态 | 9 个容器全部 `healthy` |

**仍未验证**（都需要真实账号 + 浏览器，未代跑）：后台树编辑器的真实增删改与整树保存、阅读页三栏版式与 `<1024px` 目录抽屉、上传闭环（裁剪 → 上传 → 七牛回调 → 任务 completed）、绕过前端的比例校验用例、私信实时互发。未登录访问 `/api/chat/ws` 与 `POST /api/images/upload-tasks` 均返回业务码「请先登录」（HTTP 200 + 包体业务码是本项目约定），说明请求已穿过 Origin 检查到达鉴权层——但这不是 WS 建连成功的证据。

部署过程踩到的一个坑（已写进 README §11.4.4）：`-db` 启动时会 `InitES` 并在 ES 不可达时 `fatal` 退出，所以跑建表前必须先把 `es`（以及 `mysql-slave`、`clickhouse`）起起来并等 healthy。

## DNS 已恢复（2026-11-04）

DNSPod 解析已重新启用。2026-11-04 在生产服务器 `43.155.228.146` 上做的**只读检查**（部署前的当前版本，非本分支）结果：

| 检查 | 结果 |
| --- | --- |
| `getent hosts gentlybeing.cn` | → `43.155.228.146`（本机），不再是 `NXDOMAIN` |
| `getent hosts image.gentlybeing.cn` | → 七牛 CDN 链路（CNAME `opencdnqiniustaticv6.jomodns.com` / `image-gentlybeing-cn-idvs7kh.qiniudns.com` 等） |
| `GET https://127.0.0.1/health` | 200 |
| `GET https://gentlybeing.cn/` | 200 |
| `POST https://gentlybeing.cn/api/images/qiniu/callback` | 200（**端点可达**；未带合法签名，不构成业务成功） |

**尚未执行**（需要部署本分支后用真实账号跑）：
- 真实上传闭环：裁剪 → 确认 → 上传 → 七牛回调 → 任务 `completed`；
- 绕过前端的后端比例校验（封面 16:9 / 头像 1:1 的 failed 用例）；
- 私信实时互发。

上线后按下一节 5 步逐项执行，并把结果回填到本文档。

## DNS 恢复后的复验步骤（部署后必做）

1. 在宿主机与公网解析确认 `gentlybeing.cn`、`image.gentlybeing.cn` 不再是 `NXDOMAIN`（`dig +short` / `nslookup`），且 `runtime_config_models.storage.callback_url` 指向的回调地址可达。
2. 清理/观察此前停留在 pending 的上传任务；重新上传一张封面（不合规比例）：应弹裁剪窗 → 确认 → 上传成功 → 七牛回调到达 → 任务 completed。
3. 复验后端比例校验：封面 16:9、头像 1:1；不合规的图（绕过前端）应被判 failed 并返回明确错误码，合规图通过。
4. 复验私信实时：私信页显示「实时已连接」，`/api/chat/ws` 不再返回 403；在线互发一轮消息有提示。
5. 复验裁剪窗取消路径：取消不上传、原封面保留；确认路径产物是烤进文件的新图，线上构图与预览一致。
