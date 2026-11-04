# 部署记录：#250 合批（阅读页计数真相、互动条、评论面板、后台用户 / 表单 / 合集项目）

- 时间：2026-11-04 19:52–20:02（服务器本地时区）
- 目标：腾讯云 `43.155.228.146`，`/opt/blogx`，compose 目录 `deploy/compose/production`
- 代码：`2b11387` → **`8056e37`**（squash 合入的 PR [#259](https://github.com/Jenlybein/NewblogX/pull/259)）
- 方式：**串行**（后端 → 读者站 → 后台），每层建完先验证再进下一层

## 追加：两处验收反馈修复（同日 20:20–20:40）

上线后按用户反馈改了两处，只重建 `blogx_web`（后端与后台无代码变化）：

| 提交 | 内容 | 上线前的问题 |
| --- | --- | --- |
| `497e59a` | **正文列不再单独收窄**：删掉 `.article-main` 的 `max-width:72rem` + `margin-inline:auto`；行宽 token 80ch → 92ch | 上一版把正文列整体收窄并居中，与上方 hero 元信息条、右侧栏脱开，两侧露出页面底色 |
| `7e32482` | **六层标题装置对齐 `themes/graphite-island-spectrum`**：H1 定标线改「三角+方块」、H2 改 6px 粗条 + 向右淡出的色带、H3 加通栏发丝线、H4 改 44px 记号笔横条、H5 改贴纸标签块、H6 改前置 1px 引线 | 六层颜色不同但装置太像（三角形/短下划线/双线/顶线），层级辨识度不够 |

验证（全部为实际命令输出）：

- `blogx-web` healthy；`/`、`/admin/`、文章页均 200。
- 全局入口 CSS `entry.TIvKCog_.css`：含 `--type-measure:92ch`、**不含** `max-width:72rem`；nginx 实发的字节数与镜像内一致（68955 / 68956，差 1 字节是命令替换吃掉的换行）。
- 文章页 SSR：`article-actions-bar--hero` ×1、`去发表评论` ×1、`article-actions-bar--fixed` **×0**。
- 标题装置（注入 shadow root，落在 JS 而非 CSS）：`--md-heading-h4) 44px` ×2、H1 三角 `polygon(...)` ×4、H5 `border: 1px solid var(--md-heading-h5)` ×2、H2 `border-left: 6px solid` ×2、H6 引线 `height: 1px` ×2。
- `blogx_admin` **未重建**：它那份 `tokens.css` 同步到了 92ch（两端逐字节一致由门禁保证），但后台没有任何地方消费 `--type-measure`，观感无变化。

回滚：这次只动了 `blogx_web`，回滚只需 `docker compose up -d blogx_web`（镜像 tag 指回 `2026-11-04-pre250`）。

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

## 追加二：目录归属改造（同日 20:50–21:10）

两处"目录"按验收反馈重做，读者站与后台各重建一次（仍然串行，各带一次 nginx 重启）：

| 提交 | 内容 | 上线前的问题 |
| --- | --- | --- |
| `257330e` | 阅读页目录改**贴左边缘的可收起抽屉**：竖排窄标 + 左侧滑出面板（`min(20rem, 86vw)`）覆盖正文，点遮罩 / Esc / × 关闭，收起时面板 `inert`；页面布局恒定两列，删掉 `.article-layout--with-outline` 那条 13.5rem 第三列；骨架去掉 `with-outline` | 目录占一列，把正文挤窄，两侧还脱节 |
| `d9a43a5` | 后台目录树改用 **Naive UI `NTree`**：树线 + 16px 缩进 + 折叠 + 内置拖拽；行内六个按钮收进「⋯」下拉；新增纯函数 `outlineDropIndex` 做落点换算；移除 `sortablejs` + `@types/sortablejs`；补 `.outline-tree` / `.outline-node*` 样式 | 手写 `<ul>` 只有 20px 缩进、没有任何树线与样式，每行还摊开六个按钮 |

验证（实际命令输出）：

- 读者站：`outline-rail__tab` 在产物里命中 4 处，`article-layout--with-outline` **0 处**；`blogx-web` healthy，文章页 / 首页 200。
- 后台：`outline-node__more` 命中 3 处；`blogx-admin` healthy，`/admin/` 200。
- 「sortable 字样还在」的解释：`DataTable-D0X4UZZF.mjs` 里的 4 处 `Sortable` 是 **naive-ui 自己的表格列排序术语**（`sortable: true` 那套），不是我们的依赖；`sortablejs` 已从 `package.json` 摘除。
- 两端口径未动：tokens.css 仍逐字节一致（parity 门禁跑过）。

回滚：两个提交各自只动一个容器，`docker compose up -d blogx_web` / `blogx_admin`（镜像 tag 指回 `2026-11-04-pre250`）即可分别回退。

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
