# 部署记录：#250 合批（阅读页计数真相、互动条、评论面板、后台用户 / 表单 / 合集项目）

- 时间：2026-11-04 19:52–20:02（服务器本地时区）
- 目标：腾讯云 `43.155.228.146`，`/opt/blogx`，compose 目录 `deploy/compose/production`
- 代码：`2b11387` → **`8056e37`**（squash 合入的 PR [#259](https://github.com/Jenlybein/NewblogX/pull/259)）
- 方式：**串行**（后端 → 读者站 → 后台），每层建完先验证再进下一层

## 一次性部署序列（本次实际执行）

```bash
# 0) 回滚锚点：先给当前三个镜像打日期 tag（11.5 的前提）
docker tag blogx-server:production blogx-server:2026-11-04-pre250
docker tag blogx-web:production    blogx-web:2026-11-04-pre250
docker tag blogx-admin:production  blogx-admin:2026-11-04-pre250

# 1) 后端
cd /opt/blogx && git merge --ff-only origin/main          # -> 8056e37
cd deploy/compose/production && set -a && . ./.env && set +a
docker compose up -d --build blogx_server
# 2) 读者站
docker compose up -d --build blogx_web
# 3) 后台
docker compose up -d --build blogx_admin
# 4) 后端重建换了容器 IP，nginx 必须重启（否则 upstream 缓存导致 502）
docker compose restart nginx
```

## 验证（逐层，都是实际命令输出）

| 层 | 判据 | 结果 |
| --- | --- | --- |
| 后端 | `docker compose ps` 中 `blogx-server` | Up，`healthy`；最近 200 行日志 `level":"error"` 计数 **0** |
| 读者站 | 容器健康 + 产物标记 | `blogx-web` healthy；`.output` 里 `去发表评论`、`comment-sheet__panel` 命中，**`article-actions-bar--fixed` 已无命中** |
| 后台 | 容器健康 + 产物标记 | `blogx-admin` healthy；`collection-overlay__pair`、`admin-field--number` 命中；构建产物里**没有 outline 页 chunk** |
| 入口 | `/`、`/admin/`、`/messages`、`/history` | 全部 **200** |
| 静态资源 | nginx 实发的 chunk 尺寸 == 镜像内尺寸 | admin `Y5uPB653.js` 61957 == 61957 且含 `collection-overlay__pair`；reader `wGuoyD0I.js` 59755 且含 `comment-sheet__panel` |
| 文章页 SSR | 真实文章 `375326834096607232` 的 HTML | `article-actions-bar--hero` ×1、`去发表评论` ×1、**`article-actions-bar--fixed` ×0**；`comment-sheet` ×0（面板 `v-if` 收起态，符合预期） |

## 本批没有的一次性动作

- **无 DDL**：计数账本只改并账口径（`digg_count` / `favor_count` 由 `COUNT(关系行)` 覆盖），没有新表、没有列变更，不需要手动建表或重建 ES。
- **无索引回填**：契约形状未变（`make openapi` 无需重跑，CI 的 `contract up-to-date` 也验过）。
- **并账自动收敛**：30 分钟 cron 会跑第一次覆盖式并账，把历史漂移一次抹平；不需要手工执行 `-t reconcile`。

## 回滚

```bash
cd /opt/blogx/deploy/compose/production
# 把三行 IMAGE 改回 2026-11-04-pre250（或直接指定 tag），然后：
docker compose up -d            # 注意：不要带 --build，否则等于又发布一次
```

回滚只影响镜像，不影响数据：并账已把列值收敛到真实关系行数，旧镜像的增量语义依旧自洽。

## 仍需人工确认（代码与产物层已验，观感层验不了）

1. 读者站底部是否真的只剩一份互动条（在 1280px 与 <1024px 各看一次）。
2. 评论「去发表评论」面板：滑出位置、宽度、点外部 / Esc / × 关闭、手机上是否挡正文。
3. 后台：合集 / 项目卡片 hover 的编辑删除、弹窗 + 抽屉是否成组居中、设置表单各字段宽度。
4. 点赞后刷新，数字与按钮是否一起变化（本批 #251 的原始症状）。
