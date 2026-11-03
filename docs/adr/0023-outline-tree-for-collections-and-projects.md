# ADR-0023：合集/项目的目录树（文件夹节点 + 文章节点，取代扁平收录表）

- 状态：已交付
- 日期：2026-11-03
- 来源：2026-11-03 `/grill-with-docs` 会话；逐条决定与事实证据见 [2026-11-03 目录树 / 阅读页 / 裁剪窗 决定清单](../notes/2026-11-03-outline-tree-and-reading-page-decisions.md)

## 背景与问题

合集（`collection_models` + `collection_article_models`）与项目（`project_models` + `project_article_models`）在读者侧都只有**一条扁平的顺序列表**：详情页按 `sort_order` 把收录文章排成一行，没有任何层次。产品要的是文档站 / Obsidian 文件树那种结构——"项目 → 章节文件夹 → 文章页"——并且要求**文章阅读页左侧也能看到同一棵树**。三个问题一起暴露：

1. 现有模型只能表达"第几篇"，表达不了"这一篇属于哪一章"；往收录关系的中间表上加层次字段就得 ALTER 既有表，直接违反零 DDL 硬约束（`docs/architecture/overview.md` §六 A1）。
2. 合集与项目的实现是**四层逐行克隆**（`domain` / `app` / `adapter/mysql` / `adapter/http` 各一份），[#207](https://github.com/Jenlybein/NewblogX/issues/207) 已把它列为待收的结构债；再给两者各加一套树，克隆面会翻倍。
3. 阅读页此前没有"内容上下文"的概念，既不知道自己在哪棵树里，也无法在章节间前后翻阅。

## 决策

1. **新增一张与 owner 无关的树表** `content_outline_node_models`：`owner_kind`（合集 / 项目）+ `owner_id` + `parent_id` + `kind`（文件夹 / 文章）+ `title` + `article_id` + `sort_order` + 软删。**一张表同时服务合集与项目**，树的读写、嵌套、校验、上一章/下一章只写一份。既有表一律不动（零 DDL）。
2. **两种节点**：文件夹节点只分组、不可阅读、不可点击（无封面、无摘要、无正文）；文章节点指向一篇文章。**任意深度**嵌套；树里只允许"含文章或子文件夹"的文件夹，纯资源目录（如素材目录）不建模。
3. **"属于哪棵树"是节点的属性**：一篇文章可以同时出现在多棵树里（沿用"一篇文章可进多个合集"的既有语义），同一棵树内最多一个节点（唯一约束）。
4. **读者侧详情页就是目录页**：`GET /collections/{id}` / `GET /projects/{id}` 从扁平 `articles[]` 改为**嵌套树**；点文章节点跳到 `/article/{id}?from=<kind>:<id>`；另有一个文章上下文接口回"当前树、当前节点、上一篇/下一篇"。
5. **阅读页左侧挂同一棵树**：`from` 缺失时回退到"第一棵归属树"，都没有就不显示左栏；右栏的标题大纲（`ArticleTocAnchor`）继续独立存在；`<1024px` 收进「目录」抽屉。
6. **树只在客户端取数（骨架占位）**：`/collections/**`、`/projects/**` 保持 SPA；`/article/**` 的 SSR 边界不动（[ADR-0019](./0019-reader-rendering-and-cache-strategy.md)）。
7. **旧扁平中间表冻结退场**：`collection_article_models` / `project_article_models` 不再读写；**不做数据迁移**（本批上线时生产两张表均为 0 行），由管理员用新的**树编辑器**重新编排。树编辑器整体取代 `collection-arrange` / `project-arrange` 与列表页的「收录管理」弹窗，采用按钮式交互（上移 / 下移 / 缩进 / 移出 / 新建 / 重命名 / 批量收录）。
8. **失效行口径沿用**：文章后来不再公开时节点保留，读者侧靠 JOIN 过滤、后台标「已失效」并可一键清理（与热门置顶、旧收录同一口径）。

## 理由与已否决方案

- **为什么不给中间表加 `parent_id`**：加列就是 ALTER，零 DDL 硬约束下没有迁移通道；而且"层次"落在一张按 `(collection_id, article_id)` 唯一的表上，表达不了"同名文件夹""空文件夹""把文件夹整体移动"。
- **为什么不是每实体一张树表**：两张结构镜像的树表就是 #207 病灶的放大版。一张表 + `owner_kind` 让嵌套、顺序、失效行、上一章/下一章只有一份实现；合集与项目的差异留在各自的主表与文案层。
- **否决「详情页内嵌正文（文档站式）」**：章节会因此失去 `/article/**` 的 SSR 与分享链接（[ADR-0019](./0019-reader-rendering-and-cache-strategy.md) 的白名单只保留 `/` 与 `/article/**`），SEO 与分享卡片一起丢。
- **否决「保留旧中间表做收录真相、新表只存分组」**：顺序与成员会出现两个真相，失效行清理与排序提交会在两处打架。
- **否决「迁移旧收录」**：本次上线时生产两张中间表都是空的，迁移只换来一个一次性脚本、一次数据回填验证与一个永久存在的新旧对照口径。
- **否决「从标题前缀自动推导层次」**：`00-` / `01-` 是写作习惯不是契约，命名一旦不统一整棵树就散架；也无法表达空文件夹。
- **否决「拖拽排序」**：HTML5 DnD 在移动端与自动化测试里都脆，仓库现有排列页的按钮式交互已经能表达全部语义。

## 代价 / 不做什么

- **契约破坏性变更**：`collections` / `projects` 的 detail 响应从扁平列表变嵌套树，两端前端必须同批发版；旧客户端读到的是空树。
- **编排入口合并**：两个排列页被删除，管理员要重新学一个页面；本批生产为空，重建成本为 0，但旁路环境需要手工重排。
- **不做什么**：不做文件夹的可读页（不做"章节概览"）；不做拖拽排序；不做自动编号；不做树内搜索；不做"多树合并视图"（一篇文章同时属于多棵树时，阅读页只显示来路指定的那一棵）；不做旧收录的数据迁移；不 DROP 任何既有表。

## 当前生效约束

1. 层次只能存在于 `content_outline_node_models`；**不得**在文章表、合集表、项目表或任何既有表上加层次字段，也**不得**另建第二张树表（新增表仍走 `schema.sql` + testkit 三处登记）。
2. 文件夹节点不可阅读、不可点击，不带封面 / 摘要 / 正文；树里只允许"含文章或子文件夹"的文件夹。
3. 同一棵树内一篇文章最多一个节点；跨树重复允许。
4. 读者侧详情页只渲染树（不渲染扁平列表）；文章节点链接必须带 `?from=<kind>:<id>` 来路；阅读页左栏只在"有归属树"时出现，右栏标题大纲始终保留。
5. 树在读者侧**只从客户端取数**；`/collections/**`、`/projects/**` 保持 SPA，`/article/**` 的 SSR 白名单不得因本决策改动（要改先改 ADR-0019）。
6. 旧中间表 `collection_article_models` / `project_article_models` 已冻结：代码里不得新增任何读写；它们既不迁移也不 DROP。
7. 后台的收录与排序**只经树编辑器**一个入口；不得再造第二套顺序真相（旧的排列页已删除）。

## 相关代码与文档

- 概念：[GLOSSARY · 目录树](../../GLOSSARY.md) · [GLOSSARY · 合集](../../GLOSSARY.md) · [GLOSSARY · 项目](../../GLOSSARY.md)
- 上游决策：[ADR-0019](./0019-reader-rendering-and-cache-strategy.md)（渲染边界与缓存）· [ADR-0017](./0017-publish-display-switches-and-hot-top-operator-slot.md)（失效行与运营位口径）· [ADR-0009](./0009-es-sync-article-projection-only.md)（合集/项目不进 ES）
- 代码落点（按模块称名）：后端 `article` 模块内的合集 / 项目域与树读取（`GET /collections/{id}`、`GET /projects/{id}`、文章上下文接口）、`content_outline_node_models` 建表登记（`schema.sql` + testkit 三处）；读者站 `frontend/app/pages/collections/[id].vue`、`pages/projects/[id].vue`、阅读页 `pages/article/[id].vue` 的左栏与「目录」抽屉；后台 `articles/collections.vue`、`articles/projects.vue` 与新的树编辑器（取代 `collection-arrange.vue` / `project-arrange.vue`）。
- 部署：[README §11.4](../../README.md)（新表必须手动跑一次 `-db`，entrypoint 只认 `runtime_config_models`）
- 会话记录：[2026-11-03 目录树 / 阅读页 / 裁剪窗 决定清单](../notes/2026-11-03-outline-tree-and-reading-page-decisions.md)

## 后续修订

- 2026-11-04（code-review 复核）：**空文件夹允许保留**。决策 2 与「当前生效约束」里「树里只允许含文章或子文件夹的文件夹」这一限定，与用户故事 33「空文件夹也能建（先搭章节骨架再挂文章）」相互矛盾；以用户故事为准——后台可以先建章节骨架，读者侧也保留空文件夹。`domain/outline.Build` 的实现本就这样，不需要改代码；本节取代原约束中那处限定。

</content>
