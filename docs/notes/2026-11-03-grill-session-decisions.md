# 2026-11-03 grill 会话决定清单

本文件是 `/grill-with-docs` 会话的产出：把 40+ 轮追问后的全部决定逐条落纸，供开工前评审。
**代码尚未改动**；本批计划合并为**一个 PR**、走完整部署序列。

约定：`✓已定` 表示已在本会话定案；`⚠假设` 表示我按此执行、若不符请指出；`✗风险` 表示需要你知情的代价。

---

## 0. 范围与流程

1. 本批共 **7 件事**：顶栏重排、项目新实体、备案号修复+站点设置瘦身、删除文章不同步、封面取景体系全量退场、头像定形、浏览历史重做、私信重构。
2. **一个 PR**（Q16 / Q26 / Q36 三轮一致维持）。
3. **完整部署序列**（Q17 / Q41）：备份 DB → `down` → 串行重建三镜像 → 手动 `-db` 建新表 → 物理下线旧表 → 重建 ES 索引 → `up -d --force-recreate` → 重启 nginx → `verify-deploy` + 冒烟。
4. 文档与代码**同 PR 提交**（Q32）：ADR / GLOSSARY / README / 本清单。
5. 视觉类改动沿用「先截图核对、效果对了再提交」的既有习惯；当前模型读不了图，只做几何量测 + 截图存盘，由你目视确认。

---

## A. 顶栏与导航

6. ✓已定 新增「项目」为导航第五项：`首页 / 搜索 / 合集 / 项目`（`frontend/app/utils/chrome.ts:22-30`）。
7. ✓已定「合集」保持第四项，文案、路由 `/collections`、数据、后台页面**全部不动**。
8. ✓已定「项目」与「合集」是**两套独立数据、两个入口**（不是改名、不是同一份数据的两个视图）。
9. ✓已定 新入口内部（列表页 / 详情页 / 卡片 / 空态 / SEO）文案一律写「项目」，不混用「合集」字样。
10. ✓已定「创作」从 `nav.site-nav` 移出，进入 `.site-header__actions`（`AppHeader.vue:174-237`）。
11. ✓已定 右栏顺序：**亮暗切换 → 创作 → 账户（个人信息） → 手机菜单**（Q4：菜单仍在最后）。
12. ✓已定 创作按钮**保留 `entryVisible` 门控**（站点关闭发布时不显示，ADR-0017 决策 2）与 `openWriteEntry()` 弹窗行为（未登录弹登录框、已登录新标签打开 `/studio/write`）。
13. ✓已定 创作按钮样式并入 `.site-action` 家族，取**纯文字档**（同 `.site-action--text`，`AppHeader.vue:421-451`）。
14. ✓已定 手机端（`<640px`）断点降级为**图标态** + `aria-label="创作"` / `title="创作"`；`≥640px` 显示「创作」文字。
15. ✓已定 actions 区在手机端常显，创作因此常显；折叠导航面板里**不再**出现创作。
16. ✓已定 创作移出后，`nav` 内不再有 `<button>`，模板里 `v-if item.key !== 'write'` / `v-else` 分支可以收敛成纯 `NuxtLink`。
17. ✓已定 `HeaderNavKey` 新增 `project`，需三处同改：`chrome.ts:13`（联合类型）、`chrome.ts:22-30`（表）、`navIcon.ts:22-27`（`Record<HeaderNavKey, …>` 图标映射）。
18. ✓已定 需要改的既有测试：`chrome.test.ts:20-27`（四项顺序断言）、`navIcon.test.ts:14-19`（四个 key）、`AppHeader.test.ts:151-152`（nav 内 a=3 / button=1）、`:273-278`（点 nav button 触发 `openWriteEntry`）、`:280-289`（关闭发布时 nav 内 button=0）。
19. ⚠假设 项目入口图标选用与合集（`IconStack2`）明显不同的图形，避免读者混淆；具体图形在实现时定。

---

## B. 项目（新内容实体）

20. ✓已定 **新建两张表**：`project_models` + `project_article_models`，镜像合集的两表结构（零 DDL 允许新建表；既有表加列一律禁止）。
21. ✓已定 项目**不进 ES**、**不接 CDC 监听**（同合集口径：运营编排不是可检索内容）。
22. ✓已定 读者侧路由 `/projects`（列表）+ `/projects/{id}`（详情）。
23. ✓已定 后台在「文章管理」父项下新增「项目管理」「项目编排」两页（照合集管理/合集编排的落点，`frontend_admin/app/utils/menu.ts:76-87` 附近）。
24. ✓已定 部署时必须手动跑一次 `-db` 建新表（`README.md:955`：新增表不会被 entrypoint 自动自举）。
25. ✓已定 门禁同步：`legacy-colors.test.ts:139` 的页面数硬断言、`rendering-boundary.test.ts` 的新路由必须归入 `CLIENT_ONLY_PATTERNS`（`frontend/shared/readerRenderingBoundary.ts:32`）。
26. ✓已定 列表/详情复用合集的读者侧实现：`utils/collection.ts` 的视图映射、`ArticleCover.vue`、`ProfileEmptyState.vue`、`services/collection.ts` 的请求形态——**数据来源独立**。
27. ✓已完成 GLOSSARY 新增「项目（project）」词条（与「合集」并列、两套数据）。
28. ✓已定 三义边界写进词条：内容语境「项目」专指该实体；`site.project` 一律写「站点项目配置」；文档里的「本项目」改说「本仓库」。
29. ⚠假设 项目表字段与合集对齐（标题 / 描述 / 封面 / 展示开关 / 排序 / 创建者），不额外引入新字段。
30. ⚠假设 项目页版式复用合集页（同网格、同卡片、同空态），只换文案与数据源。

---

## C. 备案号修复 + 站点设置瘦身

31. ✓已定 **根因**：`AppFooter.vue:50` 渲染硬编码 `FOOTER_ICP`（`chrome.ts:66`），后台写的是 `site_info.beian`，两者无连线——与缓存无关。
32. ✓已定 页脚改为读 `siteStore.runtimeConfig?.site_info?.beian`。
33. ✓已定 **空值隐藏该 span**，不显示假备案号。
34. ✓已定 不改布局取数模型（`layouts/default.vue:16` 的 `void ensurePublicBootstrap()` 保持 fire-and-forget）；接受 SSR HTML 里为空、hydration 后补上，且最多 30s 匿名 SSR 缓存滞后（`server/utils/ssrAnonymousCache.ts:25`）。
35. ✓已定 删除后端 **`site_info.title`** 与 **`site_info.logo`** 两个字段（连带 `backend/conf/site/enter.go:8-12`、`backend/config/site_default_settings.yaml:16,18`）。
36. ✓已定 **保留 `site_info.beian`**（Q21）。
37. ✓已定 **保留整个 `project` 段**（`title` / `icon` / `web_path`），但后台不再允许自定义：后台「基础信息」只留「备案号」一项（`frontend_admin/app/components/settings/SettingsSite.vue:70-72`）。
38. ✓已定 Q43：连带从 `SEOResponse` 删掉 `site_title` 与 `logo` 两个字段（它们唯一来源就是 `SiteInfo.Title/Logo`，`backend/internal/site/app/service.go:85-95`）。
39. ✓已定 首页 SEO 兜底退回 `BRAND`（`brandFullName()` / `brandDescription()`，`pages/index.vue:23-27`）；favicon 仍走 `project.icon`（`layouts/default.vue:22-28`）。
40. ✓已定 删除死代码 `brandProjectTitle()`（`frontend/app/utils/brand.ts:63-65`，全站零消费）及 `brand.test.ts:52-57` 对应用例。
41. ✓已定 需同步改的门禁：`frontend/test/gates/site-runtime-config-parity.test.ts` 的 `:69-70`（硬断言两端含 `title`）、`:73-74`（逐字段全等）、`:147-164`（canary 依赖 `title?: string;` / `logo?: string;` 原文）。
42. ✓已定 需同步改的后端测试：`internal/site/app/service_test.go`、`internal/platform/config/runtime_site_test.go`、`internal/platform/config/runtime_test.go`。
43. ✓已定 需同步改的手写投影类型：`frontend/app/types/api.ts:24-32`、`frontend_admin/app/types/api.ts:227-241`。
44. ✓已定 需同步改的 README：`:261` 表格行、`:266`「新手最少要做的」、`:1574`。
45. ✓已定 重生成契约：`backend/Makefile:22-24` 的 `make openapi` + 两端 `pnpm openapi:types`；CI 会校验三者一致（`.github/workflows/ci.yml:169-180,343-355`）。
46. ✓已定 `golden` 零命中（已核实，省事）。

---

## D. 删除文章不同步（用户报的 bug）

47. ✓已确证 **根因链**：`DELETE /api/articles` 只产生 `article_models.deleted_at` 的 **UPDATE**（软删，非 channel DELETE）；river 对 UPDATE 只做「变更列 → ES 局部更新」，而 `deleted_at` **不在可投影字段表**（`articlesync/article_sync.go:147-225` 无该 case）→ `data` 为空 → `continue` → **零 ES 写入**；只有 `canal.DeleteAction` 才走 `DeleteESDocs`（`article_search_projection.go:37-45`）。
48. ✓已确证 **读者站只读 ES**（`useHomePageSources.ts:82-96` → `GET /api/search/articles` → `search.go:446-451` 纯 ES 不回 MySQL），且 ES 查询只按 `publish_status=3` / `visibility_status=visible` 过滤（`search.go:125-137`），**无软删兜底**，mapping 也无该字段。
49. ✓已确证 `article.deleted` 事件确实进了 outbox（`app/delete.go:101`），但 `backend/app/outbox_wiring.go:32-43` **没有注册订阅者**（全仓 `relay.On` 无该事件）——而 GLOSSARY 明写「跨模块写副作用必须走 outbox」。
50. ✓已定 Q37 **三层全做**：
    - ① 注册 `article.deleted` 订阅者 → 调 `DeleteESDocs`（正规路径，重试/死信由中继负责）；
    - ② river 的 UPDATE 分支补 `deleted_at` 判断 → 发 ES delete（覆盖直接改库的旁路）；
    - ③ ES mapping 加软删标记字段 + 查询侧兜底过滤（防位点落后/死信时露出幽灵文章）。
51. ✓已定 因 ③ 要改 `articlesync/mapping/article_mapping.json`，需**重建索引**并全量回填（用既有 `-es -s article-sync`，ADR-0009 约束）。
52. ✓已定 后台「文章已删除」是前端硬编码（`frontend_admin/app/pages/articles/index.vue:224`），后端只回 `OkWithMsg("删除成功")`（`handler.go:334`）→ 两者口径统一。
53. ✓已定 补测试（现状零覆盖）：新增「删除后 ES 文档消失」的集成断言；补 river 软删 UPDATE 的投影用例（现在 `UpdateAction` 在该包测试里零命中）。
54. ✓已定 顺带修：删除时真删 `picture_crop_models` 会触发 `SyncESDocs` 回读软删行失败并写 DLQ；crop 退场后该路径消失，退场结束时确认无残留。
55. ✓已定 部署后检查 `cdc_dead_letter` 是否有历史堆积（生产上可能已有）。
56. ✓已定 文档：ADR-0009 追加后续修订（软删与 ES 的不一致），或新 ADR；`docs/TODO.md:89` 相关条目顺带更新。

---

## E. 封面取景体系全量退场

57. ✓已定 **这轮就做、全量退场**（Q29 / Q30；覆盖 Q25 的"之后再看"）。
58. ✓已定 拆掉的内容：CDC 监听（`backend/config/settings.yaml:208` + `river/article_crop_projection.go`）、ES 投影（`articlesync/article_sync.go:273-275`）、`_source` 白名单里的 `crop`（`search.go:395-401`）。
59. ✓已定 契约删除 `crop` 字段与错误码 `banner.crop_invalid`——**显式覆盖** ADR-0020「契约与错误码一律不动」的原话。
60. ✓已定 16 份 DSL golden 全量重写。
61. ✓已定 两端取景窗组件拆掉：`StudioCoverFramingView.vue`、`BannerFramingView.vue`、`BannerFramingDialog.vue`。
62. ✓已定 `ArticleCover.vue` / `ArticleFeedItem.vue` 的 `framingStyle(crop)` 改为固定居中（`object-fit: cover`）。
63. ✓已定 **历史数据不迁移**（Q34）：已按「焦点+缩放」存的参数不做回填、不重裁、不保留旧路径；旧封面按默认居中兜底（与"无取景记录时 0.5/0.5"同口径）。
64. ✓已定 物理删表是一次性现场操作，**先备份 DB**，按 `README §11.4.3` 先例执行。
65. ✓已定 封面上传改造：轮播封面 / 文章封面 / 合集封面统一改为**上传前物理定形**（客户端裁到目标比例 + 降采样），后台不再存 crop 数据。
66. ✓已定 图像库 **直接用 cropperjs**（Q42，不再调研）；读者站与后台共用一个实现，不各引一个。
67. ✓已定 ADR 处置（Q39 / Q40，**显式覆盖** `docs/adr/README.md:52`「已交付的 ADR 不改结论」）：**直接重写 ADR-0015 正文**为新方案，ADR-0020 同步标「已取代」/重写，索引表同步。
68. ✓已定 GLOSSARY（Q38）：**删「取景」「焦点」「缩放」三个词条**；保留「头像定形」（它本就独立）；同步修引用取景表 / ADR-0015 / ADR-0020 的其他词条与链接。

---

## F. 头像定形

69. ✓已定 上传前在客户端**居中裁成 1:1** 并降采样到 **256×256**，输出 **PNG**（Q19 / Q20 收敛后 Q22 定的引库路线）。
70. ✓已定 产出是**新文件**，原图不上传、不入库。
71. ✓已定 **引入轻量库**（Q22），Q42 明确**直接用 cropperjs**。
72. ✓已定 现有上传入口在 `frontend/app/pages/studio/settings.vue:246-280`，当前只判 `image/` MIME、原图直传。
73. ✓已定 补单测：现状 `frontend/test/**` 对头像上传**零覆盖**（新逻辑不会撞红任何既有用例）。
74. ✓已完成 GLOSSARY 新增「头像定形（avatar squaring）」词条，并收窄「取景」的 `_Avoid_`。
75. ⚠假设 mock 掉上传任务接口，只测"裁切工具"纯函数与调用顺序，不测真实七牛直传。

---

## G. 浏览历史

76. ✓已定 **页面定位纠正**：浏览历史在 `frontend/app/pages/studio/history.vue`（`/studio/history`），**不在**「个人主页」`users/[id].vue`（那里只有文章/收藏/关注/粉丝四个 tab）。
77. ✓已定 **整体重做**（Q33）。
78. ✓已定 因 crop 退场，**不再**为旧链路补 crop（`social_repo.go:487-505` 的 SQL join、DTO、契约、golden 都不动）。
79. ✓已定（Q45 / Q47 修订，**取代原「长列表项」口径**）历史页改为**短卡片**：**16:9 封面 + 标题在下方 + 一行小字浏览时间**；栅格 **2 / 3 / 4 列**响应式；卡片明显小于合集卡片。
80. ✓已定 顺手清理：`getHistoryArticles` 发了个 `type` 查询参数，后端 DTO 里没有（`dto.go:253-257`），被静默忽略。
81. ✓已定 补测试：现状前端零覆盖；`article-cover-shape.test.ts:63-72` 依赖 `.article-history-card__cover` 选择器与 16:9 断言，**改版式必须同步改该用例**（保留「窄屏只改宽度、不改形状」口径）；`rounding.test.ts:145-159` 的圆角约束继续满足。
82. ✓已定 **不再显示摘要与昵称** → 因此**不给 `/api/articles/history` 补 `abstract` 与作者字段**，契约 / DTO / golden 全部不动；「`nickname` 指向浏览者自己」的问题失去显示意义（字段仍返回、不渲染）。另：删除操作改为卡片**右上角悬浮 ✕ 图标按钮**（hover / focus 显现、手机端常显，`aria-label="移出历史"`）；保留独立**失败态**；封面按退场后新口径居中（`object-fit: cover`）。

---

## H. 私信重构

83. ✓已定 **保留 `internal/chat` 模块路径与既有 7 个接口，重做内部分层**（Q35；不另起新模块、不引入消息队列）。
84. ✓已定 修表名：`backend/internal/chat/adapter/mysql/chat.go:72` 的 `chat_msg_user_states` → `chat_msg_user_state_models`（与 `schema.sql:254` 对齐）。这是「完全没法用」的主因。
85. ✓已定 内部分层：会话聚合 + 读写分离 + 写副作用走既有 **outbox** + **HTTP 落库 / WS 只推通知**双通道（这样 WS 断了也能发）。
86. ✓已定 新增 `POST /chat/messages` 可靠落库接口（现状只有 WS 能发，`handler.go:359`）。
87. ✓已定 陌生人开关接运行时配置 + 后台 UI（现状 `service.go:87-94` + `chat_wiring.go:111-116` 无配置行即 false，前端零暴露）。
88. ✓已定 修 WS 自发送逻辑倒置（`handler.go:379-386`：成功时反而无回执）。
89. ✓已定 把 `schema.sql` vs AutoMigrate 的表名比对改成**默认可跑的门禁**（现状 `schema_sql_integration_test.go` 是 `//go:build integration`，默认不跑——这正是表名 bug 逃逸的原因）。
90. ✓已定 补 stateRow 写入侧集成用例（现有 6 个 chat 集成用例**没有一个**覆盖 `DeleteMessagesForUser` / `UserDeletedAt`）。
91. ✓已定 前端补真实契约测试（现状私信链路全靠 mock，`useChatSocket`、`services/inbox` 均无真实校验）。
92. ✓已定 顺手更新 `docs/TODO.md:90`（该条引用重构前的旧文件 `chat_ws.go`，已过期）。
93. ✓已完成 GLOSSARY 新增「私信」「会话」两个词条。
94. ⚠假设 双通道重构不改变既有接口的路径与语义（只在内部换实现），避免推翻前端与 nginx 配置。

---

## I. 文档与部署

95. ✓已定 本 PR 内的文档改动：ADR-0015 重写、ADR-0020 标已取代、ADR-0009 追加修订、（私信双通道）视情况新开一条 ADR、`docs/adr/README.md` 索引、GLOSSARY（4 新增 + 3 删除 + 若干引用修正）、README（站点配置段 + §11.4 部署说明补"新增表要手动 `-db`"）。
96. ✓已定 部署前置：**先备份 MySQL**。
97. ✓已定 部署序列：`down` → 串行构建三镜像（4 GB 内存限制下不并行）→ 手动 `-db`（建 `project_models` / `project_article_models`）→ 物理下线 `picture_crop_models` → ES 索引重建 + 全量回填 → `up -d --force-recreate` → 重启 nginx → `verify-deploy` → 冒烟。
98. ✓已定 冒烟项：`/health`、`/`、`/admin/`、`/collections`、新增 `/projects` 均 200；私信收发可用；删文章后读者站不再显示；备案号正确显示；头像上传后为正方形。
99. ✓已定 容器镜像 tag 必须与运行中容器一致（沿用既有验收口径）。
100. ✓已定 部署后检查 `cdc_dead_letter` 与 river 健康。

---

## J. 显式覆盖仓库惯例（两处，需你知情）

101. ✓已定 **Q39 / Q40**：直接重写**已交付**的 ADR-0015 正文 —— 覆盖 [docs/adr/README.md:52](docs/adr/README.md)「已交付的 ADR 不改结论，只能追加后续修订或另开新 ADR 并标 `已取代`」。
102. 历史记录：上一轮（G8 首屏）也有一次同类覆盖（Q5「不新开 ADR，只追加 0013 的后续修订」），本清单不重复计入本 PR。

---

## K. 风险（请知情）

103. ✗风险 **PR 体积极大**：涉及前后端契约重生成、DDL 新建与物理删表、CDC 监听变更、ES mapping 变更 + 重建、16 份 golden 重写、两端取景窗重写、私信重构、三个 bug 修复。整体回滚只能整 PR 回滚（Q36 你已明确坚持一个 PR）。
104. ✗风险 **物理删表不可逆**：`picture_crop_models` 下线后无法恢复取景参数；旧封面构图会按默认居中变化（Q34 已接受）。
105. ✗风险 **ES 索引重建期间**搜索可能短暂异常（有全量回填脚本兜底）。
106. ✗风险 **备案号首屏缺失**：SSR HTML 里不含备案号（法定展示信息通常要求首屏可见），本批按 Q10 接受"客户端补上"。
107. ✗风险 **视觉核对靠人**：当前模型读不了图，最终观感需你目视确认。

---

## L. 待你确认的三处假设

- **B.29 项目表字段与合集对齐**（不新增字段）
- **B.30 项目页复用标准卡片组件**（历史页已改为短卡片，不在此列）
- **H.94 私信双通道不改变既有接口路径与语义**

若这三处与你的预期不符，请在开工前指出；否则我按此执行。
