# 2026-11-03 目录树 / 阅读页 / 裁剪窗 决定清单

本文件是 `/grill-with-docs` 会话的产出：把这一轮全部决定逐条落纸，供开工前评审。
**代码尚未改动**；本批按产品决定合并为**一个 PR**。

约定：`✓已定` = 本会话定案；`⚠假设` = 我按此执行、若不符请指出；`✗风险` = 需要你知情的代价。

---

## 0. 事实底座（已在生产机上只读核实）

1. 部署版本 = 仓库最新提交 `cd25c40`（`blogx-server` / `blogx-web` / `blogx-admin` 三镜像 2026-11-03 17:15–17:19 构建；容器内 bundle 能查到"正在定形"文案）。
2. **`gentlybeing.cn` 与 `image.gentlybeing.cn` 当前公网 DNS 均为 `NXDOMAIN`**：223.5.5.5 / 119.29.29.29 / 8.8.8.8 / 1.1.1.1 与 Cloudflare DoH 一致；权威 SOA 为 `f1g1ns1.dnspod.net` + **`nxdomainadmin.dnspod.com`**（DNSPod 对"解析被暂停"的专用应答）。宿主机自己解析不到；本机可访问是因为本地 DNS 返回 `198.18.1.98`（代理 fake-IP 网段）。
3. **579 的链路**：`runtime_config_models.storage.callback_url = https://gentlybeing.cn/api/images/qiniu/callback`；七牛上传回调失败 → 浏览器拿到 HTTP 579（[README §16.7](../../README.md) 同口径）。同时前端 `frontend/app/services/image.ts` 上传成功后**只轮询任务状态，从不调 `POST /images/upload-tasks/complete`**（全仓零调用点），确认链路 100% 依赖那条已断的回调。
4. **私信 WS 的链路**：nginx access log 里 `GET /api/chat/ws?ticket=…` 恒返 **403 / 10 字节**（gorilla `Upgrade` 校验 `Origin` 失败时默认写的 `Forbidden\n`）。`backend/internal/platform/http/middleware/origin_allowlist.go` 的默认白名单只有 `https://blog.gentlybeing.cn`、`https://blogx.gentlybeing.cn` 两个旧子域，**没有生产域 `https://gentlybeing.cn`**；生产 `.env` 也没有 `BLOGX_CORS_ALLOWED_ORIGINS`。票据接口 200、Redis 票据实现正常、nginx 的 `/api/chat/ws` Upgrade 头已配好。
5. **裁剪其实在跑**：`frontend/app/utils/imageShaping.ts` 的 `shapeImage` 每次都会居中裁 + 降采样（无"合规就跳过"的快速路径），`useStudioArticle.ts`（封面 16:9 / 1600×900）、`settings.vue`（头像 1:1 / 256 PNG）、后台 `projects.vue` / `collections.vue` / `media/index.vue` 都在上传前调用。用户看到的"没有裁剪"是上传在裁剪之后那一步 579 中断后的表象。
6. **生产库里合集 0 条、项目 0 条、两张收录中间表 0 行**——所以"不迁移"没有数据损失；`#189` 的项目功能当天刚上线。

---

## A. 目录树（内容模型）✓已定

| # | 决定 | 备注 |
| --- | --- | --- |
| A1 | **合集与项目都上树，共用一套树模块** | 新增与 owner 无关的树表，`owner_kind` 区分；两个实体、两个入口、两套数据都保留。接受"两者同构、差异只剩名字与入口"这个事实 |
| A2 | **任意深度文件夹 + 文章叶子** | 文件夹节点只分组、不可阅读、不可点击；不建模 `assets` 这类纯资源目录（树里只允许"含文章或子文件夹"的文件夹） |
| A3 | **一篇文章可同时属于多棵树** | "属于哪棵树"是节点属性；同一棵树内一篇文章最多一个节点 |
| A4 | **不迁移旧收录** | `collection_article_models` / `project_article_models` 冻结退场（不再读写，不 DROP，零 DDL 不允许）；生产为 0 条，部署后由管理员用树编辑器重新编排 |
| A5 | **失效行口径沿用** | 文章不再公开时节点保留、读者侧 JOIN 过滤、后台标「已失效」并可清理 |
| A6 | **不自动编号** | 像 `00-项目综述` 这样的序号写在标题里，由管理员自己维护 |
| A7 | **树的一行 = 标题 + 折叠箭头 + 缩进引导线** | 删掉现有列表的"第 N 篇"；当前文章高亮；展开/折叠状态记在 `localStorage` |
| A8 | **详情页 = 目录页** | `/collections/{id}`、`/projects/{id}` 只渲染树 + 简介，不再渲染扁平列表；点文章节点跳 `/article/{id}?from=<kind>:<id>` |
| A9 | **阅读页左侧挂同一棵树** | 当前节点高亮 + 上一篇/下一篇（按树内 DFS 顺序）；`from` 缺失时回退到"第一棵归属树"，都没有就不显示左栏（右栏标题大纲始终保留） |
| A10 | **窄屏 `<1024px` 树收进「目录」抽屉** | 与现有标题大纲同一套降级思路（现在 `<1024px` 本来就没有侧栏） |
| A11 | **树只在客户端取数，骨架先占位** | `/collections/**`、`/projects/**` 保持 SPA；`/article/**` 的 SSR 边界不动（ADR-0019） |

### 树的数据形态（实现口径，供评审）

- 新表 `content_outline_node_models`：`id` / 时间戳与软删 / `owner_kind`（1=合集 2=项目）/ `owner_id` / `parent_id`（0=根）/ `kind`（1=文件夹 2=文章）/ `title`（文件夹标题；文章节点留空，读文章自身标题）/ `article_id`（仅文章节点）/ `sort_order`。
- 唯一性：`(owner_kind, owner_id, article_id)` 软删内唯一（同树内不重复）；兄弟分组内 `sort_order` 由整树一次性提交维护（沿用现有"整份顺序一次 PATCH"的做法）。
- 迁移：**无**。`-db` 只负责建新表；新表初始为空。

---

## B. 后台 · 接口 · 契约 ✓已定

| # | 决定 | 备注 |
| --- | --- | --- |
| B1 | **一个树编辑器整体取代** `collection-arrange` / `project-arrange` 与列表页的「收录管理」弹窗 | 同一套界面服务合集与项目，只换数据源与文案 |
| B2 | **按钮式编排** | 上移 / 下移 / 缩进 / 移出 + 新建文件夹 / 重命名 / 移除节点 + 批量收录（沿用现有批量收录的关键词搜索与多选形态）；不做拖拽 |
| B3 | **契约破坏性变更** | `GET /collections/{id}`、`GET /projects/{id}` 从扁平 `articles[]` 改为嵌套树；`make openapi` + 两端 `pnpm openapi:types` 必须同批 |
| B4 | **新增文章上下文接口** | 阅读页左栏取数：当前树、当前节点、上一篇/下一篇；入参带 `from`，无则取第一棵归属树 |

---

## C. 阅读页外观 ✓已定

| # | 决定 | 备注 |
| --- | --- | --- |
| C1 | **缺封面 = 父容器底色的纯色块，不再有 NO COVER** | 列表类（首页卡片、合集/项目列表与详情、收藏、历史）保留 16:9 占位、背景 `transparent`、无文字无纹理、`aria-hidden`；文章详情首屏无封面时**不渲染图区**；图片加载失败退化成同一块纯色 |
| C2 | **Obsidian `graphite-island` 只影响 markdown 渲染** | 改 `frontend/app/utils/markdownTheme.ts` 的 shadow-root 主题：正文 16px/1.85；H1 居中 + 横线记号、H2 左侧色带、H3 三角、H4 短下划线、H6 大写字距；代码块用"另一层纸面"的层次；引用块用强调面。**颜色全部落回语义 token**（石墨阶 + 包豪斯黄/珊瑚橙点缀），不引入陶土红 / 橄榄 / 薄荷等第六色相（ADR-0012） |
| C3 | **创作区 markdown 预览主题彻底退场** | 删 13 套选项、`useStudioMarkdownTheme`、`markdown-theme` 依赖、`themeHref` / `themeLabel` 注入通道与"预览样式"指示条 |
| C4 | **阅读页「编辑」按钮** | 位置：hero 元信息行（作者/时间/阅读数旁）；门控：仅作者本人（管理员走后台文章管理）；跳 `/studio/write?article_id=`；`aria-label="编辑这篇文章"` |
| C5 | **封面兜底的语义** | 「暂无封面」不再是可读内容，也不是可视元素；截图验收只看"卡片不塌、栅格不跳" |

---

## D. 裁剪窗与上传 ✓已定

| # | 决定 | 备注 |
| --- | --- | --- |
| D1 | **比例不合规才弹裁剪窗** | 合规（与目标比例相差 ≤1%）直接自动居中裁；不合规才弹窗由作者选区域 |
| D2 | **裁剪窗交互** | 固定比例框 + 拖动 + 1–3× 缩放；不做旋转/镜像/滤镜；确认把结果**烤进新文件**，不存任何裁剪参数；取消 = 中止本次上传、保留原封面 |
| D3 | **实现方式：手写 canvas，复用 #204 删掉的取景窗结构** | 从 git 历史（`cd25c40^`）取 `StudioCoverFramingDialog.vue` / `StudioCoverFramingView.vue` / `BannerFramingDialog.vue` / `BannerFramingView.vue` 的结构改写；**不引入 cropperjs**（ADR-0015 已撤回该选型） |
| D4 | **覆盖所有封面/头像入口** | 读者站文章封面 + 头像；后台轮播 + 合集 + 项目封面；两端同一份实现（沿用 `imageShaping.ts` 逐字节同源门禁） |
| D5 | **后端校验比例** | 创建上传任务时带 `purpose`（cover / avatar），任务里记目标比例；确认（七牛回调与前端 `complete` 共用同一路径）按 ±1% 校验，不合规把任务判 failed 并给明确错误码（需重生成契约） |

---

## E. 三个 bug ✓已定

| # | 决定 | 备注 |
| --- | --- | --- |
| E1 | **579 不改上传链路代码** | 等 DNSPod 恢复解析（`gentlybeing.cn` + `image.gentlybeing.cn`）。✗风险：DNS 恢复前，封面/头像上传仍会以 `HTTP 579` 结束、任务停在 pending；本批交付后必须在 PR 与 README 写明这条已知限制 |
| E2 | **私信实时连接：Origin 白名单从环境派生** | 把 `BLOGX_WEB_SITE_URL`、`BLOGX_ADMIN_SITE_URL`（去掉 path 后）并入白名单，`BLOGX_CORS_ALLOWED_ORIGINS` 继续可覆盖；补单测钉住"生产域必须在名单里"。⚠注意：`normalizeOrigin` 拒绝带 path 的 URL，`BLOGX_ADMIN_SITE_URL=https://gentlybeing.cn/admin` 必须先剥 path |
| E3 | **裁剪"没触发"的处置** | 用 D1–D4 的裁剪窗让裁剪过程可见；**不补 complete 兜底**（与 E1 一致，DNSPod 恢复前上传本来就会失败） |

---

## F. 文档与部署 ✓已定

| # | 决定 | 备注 |
| --- | --- | --- |
| F1 | **新开 `ADR-0023`**（合集/项目的目录树）并登记索引；[ADR-0015](./../adr/0015-banner-framing-as-parameters.md) 追加「后续修订」（不合规时改为作者自选区域，产物仍是烤进文件、不存参数） |
| F2 | **GLOSSARY 新增「目录树（outline）」词条**；更新「项目」「头像定形」两条的位置与口径 |
| F3 | **README 部署段补"新表要手动 `-db`"** | 与 `#189` 的 `project_models` 同类；entrypoint 不自举新表 |
| F4 | **一个 PR** | 后端（域/应用/适配/契约）+ 两端前端 + 文档同批；部署序列：备份 → 串行重建三镜像 → `-db` 建新表 → `up -d --force-recreate` → 重启 nginx → `verify-deploy` + 冒烟（树、裁剪窗、编辑按钮、私信实时已连接） |

---

## G. 风险（请知情）

1. ✗ **PR 体量大**：目录树（新表 + 契约破坏性变更 + 后端模块 + 后台树编辑器 + 读者树 + 阅读页左栏）+ 裁剪窗 + markdown 主题 + 封面兜底 + 编辑按钮 + 主题退场 + WS 白名单；回滚只能整 PR 回滚。
2. ✗ **契约破坏性变更**：`collections` / `projects` detail 形状改变，两端必须同批发版；旧客户端会读到空树。
3. ✗ **DNS 修复待你操作**：DNSPod 恢复解析后才能验证上传全链路（含 D5 的比例校验）。若长时间不恢复，本批的裁剪窗只能验证到"出图 + 进入上传"，最终仍 579。
4. ✗ **旧数据不迁移**：本批生产为空所以无代价；若有旁路环境已录入合集/项目，需要手工重排。

---

## H. 待确认的三处假设

1. 树内"上一篇/下一篇"按**深度优先（DFS）**顺序取相邻文章节点（含跨文件夹）。
2. `from` 参数格式 `project:<id>` / `collection:<id>`（生成链接时由详情页负责带上）。
3. 详情页在树为空时沿用现有中性空态文案（"这个项目还没有文章"），不出现"未发布 / 不可见"这类内部状态。
</content>
