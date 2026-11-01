# 2026-11-02 需求 grill 记录（未拆票 / 未写 spec）

> 本轮是 `/grill-with-docs` 会话的**事实与决策沉淀**，不是计划。
> 拆票与 spec 由用户后续另调技能完成，本文不排期、不估点。
> 状态：进行中（部分探子未回）。

---

## 一、需求清单（用户原始表述，共 11 项）

### 首批 6 项 + 1 个 bug

1. 管理员后台可管理首页右边栏各种卡片，并支持预览效果。
2. 删掉用户访问端多余说明字段：搜索页"关键词搜索标题、摘要与正文…"、「未刷新前，已访问页会直接复用缓存。」
3. 个人主页「最近登录」重复记录多，希望同一设备只显示一条。
4. 顶栏加「合集」入口，UI 要好看；后台可管理合集显示、信息与收录文章。
5. 管理后台左侧菜单支持子选项，右侧顶部加面包屑；站点配置不再用顶栏切换菜单，改放左侧子选项。
6. 挂件绳子太陡、有瞬移感、不柔顺（怀疑算法太硬无弹性）；另加：鼠标移出窗口快速渐隐，移回后 0.1s 快速渐显。
7. **bug**：文章浏览页走 SSR 后，右上角显示已登录，但点赞报未登录、再报 duplicated key、评论报外键约束违反。

### 追加 5 项

8. 首页同样走 SSR，已登录态下点需登录按钮仍弹登录窗（与 7 同源）。
9. 文章上传时，支持在文章信息填写界面创建分类。
10. 文章封面上传按钮太单调，参考后台轮播图上传样式；封面比例要锁定，不符比例要像轮播图那样裁剪。
11. **bug**：文章阅读页缩小到窄版响应式布局时页面自动不断下跳（怀疑目录组件）。

### 再加 3 个后台 bug（PDF `C:\Users\Jen\Desktop\1.pdf` 为证据）

12. 后台响应式布局崩坏（一压缩宽度就坏，截图为轮播图上传界面）。
13. 轮播图选完图片点上传后一直卡在「正在创建上传任务」。
14. 「图片资源库」加载不出（只有 ID 表头 + 无限转圈）。
15. **再追加**：刷新后台后卡着的状态消失、图片没传成功；之后**新**上传成功，但图片只进「图片资源库」，**没进右侧「轮播运营容器」**，刷新后仍没有。
16. **再追加（2026-11-02 晚）**：后台轮播列表不显示，但**首页轮播能展示**（后端创建链路通，坏在后台列表渲染）。
17. **首页轮播图有个大黑块挡着，需修复**。
18. **轮播图圆点容器太丑**：要求改成参考图那种「渐黑过渡容器」装标题 + 圆点；并且**给轮播图加一栏标题填写**（后台上传页）。

> 12–14 的证据 PDF：`C:\Users\Jen\Desktop\1.pdf`（3 页，768×960pt）。已用 LibreOffice Kit 渲染成 PNG，但当前模型不支持读图，**像素现象未目视确认**。

---

## 二、已确认事实（探子回报，带证据）

### 2.1 首页右栏（对应需求 1）

- 右栏整体委托给 `frontend/app/components/home/HomeSidebar.vue`，唯一调用点 `frontend/app/pages/index.vue:241-246`。
- 只有 **3 个模块**：站点公告（`HomeSidebar.vue:83-91`）、热门标签（`:93-115`）、推荐作者（`:117-144`）。无标签云 / 日历 / 统计卡；AI 卡片已被删且有门禁锁回潮（`frontend/test/design/ai-entry-retired.test.ts:50-57`）。
- 开关读 `runtimeConfig.index_right.list`（`frontend/app/utils/home.ts:19-31`），只识别 `site_notice` / `hot_tags` / `recommended_authors`。
- **配置段是 `site`，不是 `deco`**（`backend/conf/conf_site.go:13`；`backend/conf/site/enter.go:56-64`）。`deco` 段目前只有光标挂件（`site_default_settings.yaml:177-215`）。
- **半残契约**（关键）：后端只存 `[]ComponentInfo{Title, Enable}`、**不校验也不认识那三个 key**（全仓仅出现在 `frontend/app/utils/home.ts:17,27-29` 与前端测试）；前端只读 `title`、**忽略 `enable`**（`frontend/app/types/api.ts:45-49`）；`list` 当集合用、数组顺序不参与排序；三个中文标题硬编码在模板里。
- **后台目前无法编辑 `index_right`**：`SettingsSite.vue` 四组表单（基础信息 / 登录策略 / 发布与展示 / 内容审核）里没有对应控件，保存时整段透传（`frontend_admin/app/types/api.ts:178` 类型是 `unknown[]`）。
- 公告卡正文是硬编码占位串（`HomeSidebar.vue:65`，注释说明后端无此字段，#39 清死分支遗留）。
- 右栏三模块是全站唯一被 `index_right` 驱动的东西；文章页右栏不走这套配置。

### 2.2 预览机制（对应需求 1 的「预览」）

- **全仓 `iframe` 零命中**，没有任何"改后台 → 实时看读者站"的机制。
- 后台与读者站唯一连接：`AdminShell.vue:102-104` 的「打开 Web 前台」→ `window.open` 另开标签页。
- 已有「参数 → 预览」先例：**轮播封面取景**（`BannerFramingDialog` + `BannerFramingView`）。`frontend_admin/app/utils/framing.ts:246` 的 `framingStyle()` **只输出百分比与 `scale()`、零像素值**，因此预览几何 = 线上几何；读者站对等实现 `frontend/app/utils/bannerFraming.ts`，目前只被 `BannerCarousel.vue` 消费。
- 用户**明确拒绝 iframe**，选择「直接把组件状态复现」。共享可行性另派探子核查（见 §四）。

### 2.3 待删文案（对应需求 2）

两条都在 `frontend/app/pages/search.vue` 同一文件：
- `search.vue:304-306`：说明 `<p>`（要连 `<p>` 标签一起删，只删 305 会留空 `<p>`）。
- `search.vue:414`：分页条说明 `<span>`，**是 `justify-between` 的左侧占位**，删后按钮组会被推到左端。

**同义孪生（用户未列入，会造成不一致）**：
- `frontend/app/pages/index.vue:223`「已访问页会复用缓存，翻页不重复请求旧页。」
- `frontend/app/pages/article/[id].vue:602`「评论第 N 页…」（同类分页说明）
- `frontend/app/utils/searchState.ts:77` 的 idle `description` 与 A1 内容**几乎逐字重复**（删掉 A1 后，首次进搜索页的空态仍会显示同义说明）

读者站同类说明文字约 60 条，分四种形态：顶部说明 `<p>`、分页条 `<span>`、卡片脚注 `note`、空态 description/hints。**空态已收敛**到 `ProfileEmptyState.vue` + `utils/searchState.ts` / `utils/home.ts` 两个纯函数；**非空说明文字全硬编码在模板里，无集中出口**。

### 2.4 最近登录（对应需求 3）——前提被推翻

- 页面 `/studio/recent-logins` **读的不是登录日志，是会话表**：`GET /api/users/sessions`（`frontend/app/services/studio.ts:47-54`）→ `ListActive`（`backend/internal/auth/adapter/mysql/session.go:151-165`），返回 `revoked_at IS NULL AND expires_at > now()` 的行。
- 真实语义是「当前有效登录设备」，不是历史流水。空态文案「当前没有可管理的有效会话」（`recent-logins.vue:74`）才是准确的。
- **会话表只在首次登录成功时 INSERT**（`service.go:224-235`）；refresh 只 UPDATE 同行 ip/addr/ua（`session.go:112-124`）；每次鉴权只 UPDATE `last_seen_at`（`service.go:344`）。**但 `Create` 无条件 INSERT**（`session.go:52-82`，不判同 IP/UA 是否已存在）→ 同设备反复密码登录会产生多行。
- **完全没有「设备」维度**：全仓无 UA 解析库；只有 `ip` / `addr` / `ua` 三个原始字段；`recent-logins.vue:45` 直接把 UA 原文当副标题渲染。
- **无去重**：无 DISTINCT、无合并逻辑。
- **不分页**：前端传 `limit:20`（`recent-logins.vue:13`）、service 默认 10，**后端 handler 未绑任何 query**，SQL 无 LIMIT。
- **两套数据互不读取**：MySQL `user_session_models` 供读者站；ClickHouse `login_event_logs` 供后台用户列表 `last_login_at`（`user_wiring.go:526`）与日志中心登录页（`/logs/login`）。`GET /users/login/log` 被 `IsAdmin()` 硬拦且两端无页面使用。
- refresh 成功会另写 ClickHouse `token_refresh` 事件（`handler.go:151`，userID=0），MySQL 侧不新增行 → 两侧粒度不一致。

### 2.5 顶栏（对应需求 4 的入口）

- 清单在 `frontend/app/utils/chrome.ts:22-26` 的 `HEADER_NAV`，**只有 3 项**：首页 `/`、搜索 `/search`、创作 `/studio/write`。
- 数据结构 `HeaderNavItem { key, label, to, active }`，**无 icon 字段、无 children**。icon 在渲染层按 key 硬选（`AppHeader.vue:140` 三元表达式）→ 加第 4 项必须改这行。
- 「创作」受发布与展示开关过滤（`AppHeader.vue:51-53`，ADR-0017 决策 2）。
- 移动端**不是抽屉**，是同一份 DOM 的折叠面板（`is-open` class）；三档断点 `<640` / `640-1023` / `≥1024`。
- 单测钉住清单顺序：`frontend/test/utils/chrome.test.ts:20-26`。

### 2.6 合集（对应需求 4）——完全从零

- 全仓（backend Go / SQL / 契约 / 两端前端）搜 `collection|series|album|合集|专栏|专辑`：**零命中**（仅有 3 处无关：`docs/adr/README.md` 的「ADR 合集」、后台 `index.vue` 的 `growthSeries` 图表数据、`ProfileCollectionShelf.vue` 的**收藏架**）。
- 最接近的两种「容器」都不是合集：收藏夹（读者私人收纳）、热门置顶（管理员排序位，上限 12）。
- **新建表的既有做法**（区别于 GLOSSARY:174 的「三步」）：
  - GLOSSARY / ADR-0009 的「三步」是**接进 ES 搜索投影**的三步（`0009:47`）：① `article_search_projection.go:16-29` 加 case；② `projection_replay.go:131-169` 登记；③ 补用例。**只有想进 ES 才需要**。
  - 纯新建业务表三处登记：① `backend/internal/platform/mysql/ddl/schema.sql` 加 `CREATE TABLE IF NOT EXISTS`；② `backend/internal/testkit/schema/rows.go` 加 Row 结构；③ `backend/internal/testkit/schema/schema.go` 的 `groupModels` 登记。兜底：`schema_sql_integration_test.go:31-57` 双库逐列比对。
  - 实证：ADR-0017 加的 `article_hot_top_models`（schema.sql:69、rows.go:678-690、schema.go:181）。
- 模块样板可参考 `hottop`：domain 实体 → app 用例（`WithinTx`）→ adapter（repo + rows）；登记进 `Repos` 要改 `app/port.go:35` 与 `uow.go:78-91`，路由在 `adapter/http/router.go`。

### 2.7 后台菜单与面包屑（对应需求 5）

- 菜单唯一来源 `frontend_admin/app/utils/menu.ts:33-45`，**10 项扁平**：数据概览 / 文章审核 / 文章管理 / 热门置顶 / 用户管理 / 站点配置 / 媒体运营 / 全局通知 / 日志中心 / 同步死信。
- `AdminNavItem { label, path, icon: AdminNavIcon }`，icon 是字符串枚举（在 `AdminShell.vue:42-53` 映射成组件）。**不支持 children**（全仓 .ts 搜 `children` 零命中；ADR-0017:43 明写「后台菜单是扁平清单」）。
- 高亮是**精确 path 相等**（`AdminShell.vue:68`）→ 子路由靠自己成为菜单项才点亮。
- **面包屑：全仓零命中**（`breadcrumb|面包屑`）。替代物是 `AdminShell.vue:130-151` 的 eyebrow「Admin Console」+ 一级标题 + 静态副标题。
- 站点配置页 `pages/site/index.vue`（46 行纯壳）：`NTabs` + 8 段（站点 / AI 助手 / 邮件 / 对象存储 / 第三方登录 / 日志 / 上传策略 / 装饰 → `Settings{Site,Ai,Mail,Storage,OAuth,Log,Upload,Deco}.vue`）。`active = ref('site')`（:35）**纯组件内状态、不进路由**。
- **硬对账测试** `frontend_admin/test/unit/menu.test.ts`：每个 path 必须有页面文件（:39-48）、热门置顶位置固定（:50-58）、path 唯一（:74-80）、每个页面必须有菜单入口（:82-97）→ 加子菜单会同时撞这几条。
- 后台另一处 `NTabs`：`frontend_admin/app/pages/logs/index.vue:252-254`（日志类型切换）。

### 2.8 光标挂件（对应需求 6）

- 算法：**Verlet 积分 + 全量投影距离约束**（Gauss-Seidel），**无 spring-damper / 弹性 k**。核心在 `frontend/app/utils/decoCharm.ts:590-666`（组件与 composable 只是接线层）。
- 约束修正 `correction = (distance - rest) / distance` 是**全量投影**（无松弛因子）→ 不可伸长刚杆链，不是弹性绳。
- **迭代次数注释与代码不一致**：`decoCharm.ts:630` 实际 `config.segments * 30`（默认 300 次/子步），但 `:622-626` 注释仍写「2 × segments … 10 节点 × 20 次」。`git log -L` 证实 `5839cf6`（#42）改代码未同步注释末句。
- `segments` = 链节点数（默认 10，区间 4–24），且**每节 rest length = rope_length / segments**（默认 120/10 = 12px）。→ 拉到 24 时每节只剩 5px，节点越多链越容易被拉直，**这正是用户"拉到最大还是硬"的原因**。
- 弧垂只由 slack 驱动（`charmTether.ts:132-144`，`sagPeak = √(3·chord·slack/8)`），**重力完全不参与**；默认绳长 120px，指针一动 chord 逼近 120 → slack=0 → 弧垂严格为 0 → 视觉是直线。
- 指针端每子步**硬赋值**且 `previous[0]` 同步归零（`decoCharm.ts:591-595`），零插值；发布又被 `1000/fps_cap` 门控量化 → 瞬移感。
- 偏移唯一换算点 `decoCharm.ts:546-554` `charmPointerAnchor()`，仅被 `useCursorCharm.ts:348` 调用一次；渲染层确认没再偏。
- 默认参数：`rope_length` 120、`segments` 10、`gravity` 1600、`damping` 0.99、`idle_sway_hz` 0.8、`idle_sway_amp` 20、`fps_cap` 60、`logo_size` 48、`line_width` 2.5、`pointer_offset` 3/3。共 `enable` + 11 个数值参数，四源一致由 `scripts/check-deco-range-parity.mjs` 守。
- **后台无法调的**：stiffness / 弹性 / 松弛因子 / 头部平滑。
- 离开窗口处理：有 `pointerout`（仅 `relatedTarget == null`）/ `blur` / `focus` / `visibilitychange`，**无 `mouseleave` / `pointerleave`**。冻结时指针端停最后位置、链自然收敛。默认轻晃开着 → 永不因收敛停帧。
- ADR-0016/0018 锁死的常量：1/120s 固定步长、0.05s 累加器上限、单帧最多 8 子步（**迭代次数不在锁死清单**）。

### 2.9 分类（对应需求 9）——已存在，只是没接 UI

- **分类是一等公民**：表 `category_models`（`schema.sql:211-222`，唯一键 `uk_category_user_title (user_id,title)`）、领域 `backend/internal/article/domain/taxonomy/taxonomy.go:31-58`、文章表 `article_models.category_id` 存在（`schema.sql:112`）。river 投影表已含 `category_models`。
- 分类**按用户唯一**（个人分类），一篇文章只属一个分类。
- 创建接口 `POST /api/articles/category`（`router.go:59`，**authGroup = 登录用户即可**），body 只要 `{id, title}`（`dto.go:146-150`）。
- 标签则**管理员专属**：`PUT /api/articles/tags` 在 `adminGroup`（`router.go:71-72`）；`tag_models` 按 `title` 全局唯一，是公共词库。
- **没有「写文章时顺带新建」的先例**：`ArticleCreateRequest` 只收 ID（`dto.go:21-22`）；前端零处调用 saveCategory/saveTag，只调 `getTagOptions` / `getCategoryOptions`（`frontend/app/services/search.ts:36-69`）。
- 创作页下拉加载：`frontend/app/composables/useStudioArticle.ts:92-99`，分类选项**只在登录后请求**。

### 2.10 文章封面（对应需求 10）

- 库列 `article_models.cover`（URL，varchar 256）；契约写入用 `cover_image_id`、读出两者都有（`dto.go:23,65-66`）；`cover` 是已解析 URL（`app/dto.go:28-29`）。
- UI 在 `frontend/app/components/studio/StudioWritePublishDialog.vue:82-118`：隐藏原生 `<input type=file>` + 伪装 button，尺寸**写死 `194×130`**（`studio-write.css:231-232`，比例 ≈1.49，既非 16:9 也非 4:3），**无拖拽、无 hover、无尺寸/格式提示、无 aspect-ratio**。
- `handleCoverFileChange`（`useStudioArticle.ts:214-248`）**不读尺寸、不校比例、不取景**，直接上传。
- 展示端几何只有 `object-fit: cover` + `height: clamp(220px,38vw,420px)`（`tailwind.css:647-650`）。
- **文章契约里没有 crop / focus / zoom 字段**（轮播的 `BannerCrop` 是 `internal_site_adapter_http.BannerCrop`）。

### 2.11 后台轮播上传（需求 10 的参照 + 需求 12/13/14 的现场）

- `frontend_admin/app/pages/media/index.vue:570-632**：同样不用 NUpload，但 picker 用 `aspect-ratio: 16/9` + `width:100%` + `min-height:168px`（`admin.css:370-384`），外层渐变面板卡 + hover 过渡 + `NTag` 合规徽标。
- 上传前本地读像素尺寸过 **±1% 闸门**（`media/index.vue:358-410`）：不合规先弹取景窗，合规才上传。
- 取景窗 `BannerFramingView.vue`：pointer 拖拽 focus + 原生 `range` 缩放（1–2，step .05）+ 方向键 + 读数；比例锁 `.admin-framing__frame { aspect-ratio: 16/9 }`（`admin.css:467`）。
- `framingStyle()`（`framing.ts:256-265`）只输出 `objectFit:cover` + 两条百分比 + `scale(zoom)`，零像素值。
- **上传封装是两份逐行相同的拷贝**：`frontend/app/services/image.ts` 与 `frontend_admin/app/services/image.ts`（各 291 行，同名四导出）。流程：本地算七牛 qetag → `POST /api/images/upload-tasks` 建任务 → 命中 `skip_upload` 直接返回 → 否则 POST `upload.qiniup.com` → 轮询 `GET /api/images/upload-tasks/{id}` 到 `ready`（默认 1500ms/30s）。四 stage：`hashing / creating_task / uploading_to_qiniu / polling_status`。
- 卡在「正在创建上传任务」= stage `creating_task`（映射在 `useStudioArticle.ts:200-208`）。
- 后台无 `composables/` 目录，编排逻辑放 `utils/bannerCover.ts`（`requiresFraming:24` / `uploadBannerCover:42` / `buildBannerCreatePayload:54-66`）。

### 2.12 后台形态（通用）

- **Naive UI `^2.44.1`**（两端同版本）+ `@tabler/icons-vue`。列表统一 `NDataTable` + `h()` render 函数；表单 `NForm` + `NFormItem` + `reactive`，初值惯例 `ref({} as T)` + `loaded` 标志。
- 通信封装 `app/services/http/client.ts`（经 `plugins/00.api.ts` 注入 `$api`），**两端都走 openapi 生成类型**（`openapi-typescript ../backend/docs/swagger.json`），再经 `contract.ts` 的 `Schema<K>`（请求体）/ `RequiredSchema<K>`（响应体）派生。
- 后台首页 `pages/index.vue` **无右栏**，只有第 4 段的 `NGridItem :span="16"` + `:span="8"` 可作「主内容 + 右栏」参照骨架。

---

## 三、已作出的决策

| # | 决策 | 结论 |
|---|---|---|
| D1 | 拆票顺序 | **暂不拆**（用户后续另调技能拆 spec 与票），本文只记录 |
| D2 | 合集归属 | **管理员专属**（用户："管理员后台才能改，自然管理员专属"） |
| D3 | 后台菜单 | **全站统一两级化**（不是只改站点配置） |
| D4 | 挂件调参 | 用户**已去后台调过**：`segments` 拉到最大 24 仍硬 → **确认为算法弹性不足**，非参数问题 |
| D5 | `segments*30` 是否笔误 | 用户倾向：**不是** segments 的问题；按「算法缺弹性项」处理 |
| D6 | 预览方案 | **放弃 iframe**，走「直接复现组件状态」 |
| D7 | 重复登录记录位置 | 用户截图确认为**读者站 `/studio/recent-logins`**（会话表侧），不是后台日志 |
| D8 | 右栏管理粒度 | **C 完整装修**：内置类型可增删排序 **+ 自定义富文本卡 + 合集推荐卡** |
| D9 | 站点公告卡 | **删掉**硬编码占位串，改成后台可增删的卡片 |
| D10 | 设备判据 | **A 纯 UA 解析**（用户已知晓代价：合并的是「设备类型」不是「设备实例」） |
| D11 | 合并后显示 | **首次 + 最近 + 共 N 次 全显示** |
| D12 | 登录记录策略 | **合并显示 + 吊销旧会话** |
| D13 | 预览实现方式 | **两份拷贝 + 新增同步门禁**（探子已确认共享不可行，与此一致） |
| D14 | 封面锁定 | **锁 16:9 并复用取景窗** |
| D15 | 后台响应式 | **全后台统一审断点** |
| D16 | 挂件渐隐时机 | **指针离开文档**（与现有 `pointerout` 冻结机制对齐） |
| D17 | 挂件 0.1s 延迟 | **是防抖**：0.1s 内又移出则取消显示 |
| D18 | 挂件弹性 | **新增 stiffness 松弛因子参数**进后台；另：`segments` 区间改 **4–30**、`logo_size` 改 **15–70**、`damping` 改 **0.9–1**；全部做成**滑块 + 数字可输入** |
| D19 | 合集存储 | **中间表（含排序 + 软删）** |
| D20 | 合集入口 | **顶栏加一项 → 独立页 `/collections`** |
| D21 | 读者站合集页 | **列表页 + 详情页** |
| D22 | 合集信息结构 | **标题 + 描述 + 封面 + 文章（排序）** |
| D23 | 窄屏下跳修法 | **改 `scrollTop` 方案**（永不碰 document）；**窄屏不显示目录**；目录点击跳转保留 |
| D24 | SSR 登录态 | **两个都修**（isLoggedIn 校验有效性 + isAuthLikeError 不再按中文关键词） |
| D25 | 后台三 bug | **全后台断点 + refresh 超时一起修** |
| D26 | 轮播三问题 | **黑块 + 渐黑容器 + 标题一起做** |
| D27 | 轮播标题存储 | **DROP 并重建 `banner_models`**（用户：项目未上真生产，允许重建，不需备份、可直接清空服务器备份） |
| D28 | 取景表 | **合并成通用表 `picture_crop_models`**（存 banner + 文章封面，加 `target_type` / `target_id`） |
| D29 | 渐黑容器 | **方案 1**：改 `.banner-scrim` 为只底部渐黑（上半全透明），标题 + 圆点住底部渐黑区；同步改 `editorial-hero.test.ts` 的 70%/95% 断言。**同时去掉遮挡轮播的大黑块** |
| D30 | 轮播标题字段 | **只要标题**（不要副标题/CTA）；**标题不要太大，不要喧宾夺主** |
| D31 | 分类归属 | **作者本人分类（现状语义）**：按 `(user_id,title)` 唯一，后端已有 `ErrTitleDuplicated` 可读提示，零改动 |
| D32 | 文案删除 | **两条 + 所有孪生句一起删**（`search.vue:305/414` + `index.vue:223` + `article/[id].vue:602` + `searchState.ts:77`） |
| D33 | 右栏卡片类型 | **现有两种（热门标签/推荐作者）+ 富文本卡 + 合集推荐卡 + 再加三种内置卡**（候选：热门文章 / 最新评论 / 站点统计） |
| D34 | 站点配置路由 | **每段独立路由**（`/site/deco` 等）**+ 探测后台其它页面有没有可下移的子菜单，一起适配** |
| D35 | 合集字段 | **标题 + 描述 + 封面 + 显示开关 + 文章排序** |
| D36 | 挂件 stiffness | **默认 0.5**，后台可调 |
| D37 | 合集封面 | **单独上传封面；无封面时自动取首篇文章封面** |
| D38 | 拆票前置 | **先验两条事实再拆票**（见 §七） |
| D39 | 站点配置路由 | **8 段全拆**为独立子路由（新增 7–8 个页面文件，同步改 `admin-flip.test.ts:181` 与 `legacy-colors.test.ts:141` 的 `toBe(12)`） |
| D40 | 菜单子项 | **`/articles/hot-top` 与 `/logs/dead-letter` 两个都改成子项**（需改 `menu.test.ts:50-58` 的 ADR-0017 位置断言） |
| D41 | 日志中心 6 类 tab | **保持页内 tab**，不下移（六类交互同构，拆开等于抄六遍） |
| D42 | 面包屑数据来源 | **路由 meta**（需给各页面加 `definePageMeta`；全仓目前只有 `login.vue:7` / `setup.vue:7` 用了） |
| D43 | media 页 | **拆成「轮播 / 图片库」两个子路由** ⚠️ 与 §⑬ 冲突：会断掉 `media/index.vue:318-319`「上传封面后刷新图片库」的联动，需另找补偿机制 |
| D44 | 挂件 damping | **坚持 0.9–1**（用户接受无阻尼风险）→ 须同步改 ADR-0016 约束 5「上限必须 < 1」的表述 |
| D45 | 右栏再加 3 种内置卡 | **热门文章 + 最新评论 + 标签云**（标签云按字号权重，与已有的"热门标签"chip 平铺区分） |
| D46 | 文章封面 | **详情页也锁 16:9** —— 改 `.article-cover`（`tailwind.css:647-650`，现状 `height: clamp(220px,38vw,420px)` 随视口变比例） |
| D47 | `/site` 父级 | **父级不可点、不做页面、不自动跳第一段**；点击只展开/收起子项 |
| D48 | 合集详情路由 | **`/collections/{id}`**（不用 slug） |
| D49 | 文章页反向显示合集 | **不显示**，收录关系只在 `/collections` 内可见 |
| D50 | 轮播 title | **可空**，不填则不渲染文字区（只渐黑 + 圆点） |
| D51 | 上传修复 | **修超时 + 后台可配置「前端是否调 complete」，默认不调**；开关放 **upload 段**（如 `upload.frontend_confirm`）。⚠️ 默认不调 ⇒ `callback_url` 未配时上传仍会失败，只是不卡死 |
| D52 | banner 重建执行流程 | **完整流程**：开 ADR-0020 → 备份 → 手工 DROP → 改代码 → 跑 `-db` → 清孤儿引用行 |
| D53 | 预览区加载态 | **不做骨架**，加载完直接渲染 |
| D54 | 预览区宽度 | **只渲染 310px 右栏**（与首页 `.hero-grid` 的 `lg:grid-cols-[minmax(0,1fr)_310px]` 一致） |
| D55 | 富文本卡格式 | **HTML + 后台富文本编辑器**，读者站 `v-html` 渲染（须做 XSS 清洗） |
| D56 | 合集接口权限 | **CRUD 全管理员 + 批量收录 + 读公开 + 隐藏态**（比原推荐多"批量"与"隐藏态"） |
| D57 | 拆票时机 | **等用户验完两条事实再拆**（见 §七） |
| D58 | 右栏卡片存储 | **新建 `home_sidebar_card_models` 表**（不用配置 JSON 装富文本与参数）⚠️ 现有 `site.index_right` 与 `homeSidebarVisibility`（`frontend/app/utils/home.ts:19-31`）要退役或改造，`ai-entry-retired` 门禁（`test/design/ai-entry-retired.test.ts:50-57`）仍需守住 |
| D59 | 合集失效文章 | **沿用热门置顶的失效行模式**（ADR-0017 决策 4–5）：收录行保留、前台不展示、后台标「已失效」可一键清理 |
| D60 | UA 解析 | **后端解析，契约加 `device` 字段** ⚠️ 需引入 UA 解析依赖——探子已确认**全仓无任何 UA 解析库**（Go/前端均零命中），后端 `go.mod` 也只有无关的 `gopher-lua`。这是本次唯一要新增的第三方依赖 |
| D61 | 吊销旧会话 | **登录时自动吊销同 UA 旧会话**（用户无感，列表自然只剩一条） |
| D62 | `/collections` SSR | **不 SSR**（与 `/search` 一致；不动 ADR-0019） |
| D63 | 右栏热门文章卡 | **按点赞数排序**（与主栏热门置顶区分）需确认有无现成接口可复用 |
| D64 | 合集列表排序 | **后台手动排序**（与收录文章、热门置顶手感统一） |

> **D43 冲突提示（拆票时必须处理）**：`media/index.vue:318-319` 在 `uploadPendingBannerCover` 成功后执行 `imagePage.value = 1; await refreshImages()`。拆成两个独立路由后，两个页面各自挂载，这条跨区块刷新**没有宿主**。可选补偿：① 上传成功后写入一个共享 store / 事件，图片库页挂载时消费；② 轮播页保留一个"上传后提示去图片库查看"的跳转；③ 放弃自动刷新，改为用户翻页/手动刷新时自然拉取。**这一条要写进 spec，不能默认它还在。**

---

## 三·补、已确认的根因（探子回报）

### ① 窄屏自动下跳 —— TOC scroll-spy 回环（**已确认**）
- 元凶 `frontend/app/components/article/ArticleTocAnchor.vue:23-38`：active heading 一变就 `currentLink?.scrollIntoView({ block: "nearest", behavior: "smooth" })`。
- `scrollIntoView` 滚**所有祖先滚动容器含 document**；**全仓零命中** `isProgrammaticScroll` / `scrollLock` 之类防重入标志。
- 宽屏 `.article-aside` 是 `position: sticky`（`tailwind.css:914-919`）→ 目录常驻视口 → `nearest` 对 document 是 no-op → **bug 被掩盖**。
- 窄屏变 `static`（`tailwind.css:882-886`）且 grid 单列 → aside 掉到**文档底部** → 开始滚 document 拽向页尾；滚了又触发新 active → 回环；`smooth` 异步动画持续重投 → 停不下来。
- 顺带：`[id].vue:389` 的 `document.getElementById(id)?.scrollIntoView()` fallback 对 shadow DOM 标题恒 null，是**死代码**。
- 已排除：naive `NAnchor`（用 `document.getElementById` 找 shadow DOM 标题，恒 null，空转）、进度条、回顶按钮、页头显隐（只动 transform）、Jelly/挂件（全 fixed）、SWR（只刷新一次且与窄屏无关）。

### ② SSR「已登录但点赞/评论失败」——「已登录」是假象 + 重试误判（**已确认/高置信推断**）
- `frontend/app/stores/auth.ts:294` `isLoggedIn = computed(() => !!accessToken.value)` —— **只看 localStorage 里有没有字符串，不看是否过期/被吊销**。后端 `Authenticate` 有 5 道独立失效判定（`service.go:303-354`）。
- `frontend/app/services/http/client.ts:154-171`：`isAuthLikeError` 按**中文关键词**匹配（`errors.ts:46-53` 含"登录"），后端返回的「请先登录」必然命中 → 自动 refresh → **用新 token 重发同一个 PUT** → 服务端真写入了但用户看到第一次的失败 → 再点撞 `uk_article_digg (article_id, user_id)`（`schema.sql:63`）。此脆弱性仓库已自记一次（`docs/notes/2026-10-29-regression-review.md:103`）。
- 后端 **HTTP 状态码恒 200**（`res/enter.go:38`），成功失败只能靠 body 的 `code` 区分。
- **评论 FK 与点赞不是同一根因**：`user_id` 不可能为 0（`authGroup` 中间件 + `viewer.IsZero()` 双守卫）；最可能是 `article_id` 在 MySQL 不存在。附带：普通用户评论默认 `StatusExamining`，**列表里看不到**，用户可能把两者混述。
- 澄清：SSR 读不到 localStorage **不是**点赞失败原因（点赞是浏览器直发，不过 Nitro 代理）；后端只从 `Authorization`/`token`/`Token` 头取令牌、**完全不看 cookie**（`jwts/enter.go:93-106`）。
- **本地 dev 匿名 SSR 缓存是关的**（`plugins/ssrAnonymousCache.ts:33-35`）→ 缓存相关现象本地复现不出。
- **待用户在浏览器 Network 确认**：`PUT /api/articles/{id}/digg` 是否发了**两次**（第一次 code=1001、第二次 code=0）。

### ③ 后台上传卡死 + 资源库转圈 —— 同一根因（**已确认**）
- `frontend_admin/app/services/auth.ts:32-40` `refreshAccessToken()` 是全仓**唯一绕过 15s 超时兜底**的裸 `$fetch`（无 signal）→ 连接一停顿就永不 settle → 上传卡在 `creating_task`、资源库永久 spinner。
- 另一个无界挂起点：`services/image.ts:201-205` `uploadToQiniu` 裸 `fetch` + `signal: undefined`（`media/index.vue:315` 未传 signal）。
- **前端从不调 `POST /upload-tasks/complete`**（全目录无调用点）→ pending→ready 只靠七牛回调；而 `callback_url` 默认空（`site_default_settings.yaml:154`），`README.md:1509` 已记录此故障模式。
- `waitUploadReady` 有墙钟 30s 超时（`image.ts:217`），**不会无限轮询**。
- 「刷新后状态没了」= 页面重载 ref 重置，与后端是否成功无关。

### ④ 「只进资源库、不进轮播容器」——**是流程，不是 bug**（**已确认**）
- `uploadBannerCover`（`bannerCover.ts:42-51`）只上传+校验，**不发创建请求**；`buildBannerCreatePayload` 是纯函数；`createBanner` 只在 `submitBanner`（`media/index.vue:286`），只由按钮触发（`:644`）。
- **但有一个独立真 bug**：`refreshBanners` 未传 `show`（`media/index.vue:209`）→ `Show=false` → SQL ``WHERE `show` = false``（`banner.go:47-48`），而新建默认 `show=true`（`:59`）⇒ **新建的轮播即使创建成功也永不显示在后台列表**。
- 判别 SQL：`select id,show from banner_models where deleted_at is null;` —— 有 show=true 行 = 命中列表过滤 bug；无行 = 没点创建。

### ⑤ 两端共享组件 —— **不可行**（**已确认**，三条独立阻断）
1. 仓库根无 `package.json`/`pnpm-workspace.yaml`/`turbo.json`/`packages/`；`docs/adr/0012-...:61` 明文「**抽共享包被否决**（两端各有 workspace 与独立 lockfile，构建耦合风险高于收益）」。
2. 跨 app import 在 CI 会炸：`bannerFramingContract.test.ts:8-11` —— 触发对端 `.nuxt` 解析，TSConfckParseError；两端零 import 先例。
3. `HomeSidebar.vue` **11 项依赖里 8 项后台没有**：`~/utils/home`、`~/utils/avatar`、`~/utils/chrome`、`~/utils/motion`、两个 composable、`AppAvatar.vue`、`brandProjectTitle`；`motion-v` 后台未装；CSS `blueprint-panel`/`blueprint-chip`/`section-title` 等只在读者站 `tailwind.css` 定义。
- **既定惯例 = 两份拷贝 + 门禁防漂移**（ADR-0012:61 明文）。门禁清单：`check-token-parity.mjs`（逐字节）、`check-deco-range-parity.mjs`（四源）、取景契约表、16:9 静态断言。
- **关键缺口**：`tokens.css` 有同源门禁，但**组件类层（`tailwind.css` vs `admin.css`）没有**，且已实际漂移 —— `eyebrow` 读者站无 `mb-2`（`tailwind.css:63-66`）vs 后台有（`admin.css:78-81`）。
- 更正前探子两处：`bannerFraming.ts`(77行) vs `framing.ts`(265行) **不是逐行拷贝**（同步靠两端各钉同一张字面量契约表）；`image.ts` 那对**才是**真正近拷贝（约 250 行同构）且**无任何门禁**。

### ⑥ 后台窄屏崩坏（**已确认**，纯 CSS，独立于③④）
- 骨架是后台通病：全站 8 处 `NDataTable` 全无 `scroll-x`；`admin.css` **全站只有一条 `max-width: 1024px` 断点**（`:542-550`）；`AdminShell.vue:117` 侧栏 `:width="276"` 写死且**无任何折叠触发器**。
- media 页三处局部放大器：`NGridItem :span="9"` 无断点后缀（`index.vue:567`）；上传面板 `min-height:168px` 与 `aspect-ratio:16/9` **双约束冲突**（`admin.css:372-373`）；`__head` 的 `shrink-0` 按钮组 + 无 `min-w-0` 的长中文提示（`admin.css:352/406`）。

| # | 决策 | 结论 |
|---|---|---|
| D1 | 拆票顺序 | **暂不拆**（用户后续另调技能拆 spec 与票），本文只记录 |
| D2 | 合集归属 | **管理员专属**（用户："管理员后台才能改，自然管理员专属"） |
| D3 | 后台菜单 | **全站统一两级化**（不是只改站点配置） |
| D4 | 挂件调参 | 用户**已去后台调过**：`segments` 拉到最大 24 仍硬 → **确认为算法弹性不足**，非参数问题 |
| D5 | `segments*30` 是否笔误 | 用户倾向：**不是** segments 的问题；建议按「算法缺弹性项」处理，并**在后台装饰页加手感可调设置** |
| D6 | 预览方案 | **放弃 iframe**，走「直接复现组件状态」 |
| D7 | 重复登录记录位置 | 用户截图确认为**读者站 `/studio/recent-logins`**（会话表侧），不是后台日志 |

---

## 四、待补事实（探子进行中 / 未回）

1. **文章页 SSR 登录态丢失**（需求 7/8）根因链 —— 探子未回。
2. **窄屏自动下跳**（需求 11）根因链 —— 探子未回。
3. **后台三 bug**（需求 12/13/14）根因链 + PDF 现象描述 —— 探子未回。
4. **两端共享组件可行性**（对应 D6）—— 探子未回。关键待查：有无 `packages/` / workspace；`frontend/shared/` 是否只被 frontend 用；`bannerFraming.ts` vs `framing.ts`、`image.ts` 两份拷贝是否有同步门禁；`HomeSidebar.vue` 的依赖面（Pinia / tailwind 类 / icon）。

---

## 六·补、轮播黑块与标题（探子回报，已确认）

### ⑦ 首页轮播大黑块 = `.banner-scrim` 罩层（**已确认**）
`frontend/app/components/article/BannerCarousel.vue:159-170`：
```css
.banner-scrim {
  position: absolute; inset: 0; z-index: 1;
  background: var(--color-surface-inverse);      /* 降级：不透明深墨面 */
  background: linear-gradient(180deg,
    color-mix(... 70%, transparent) 0%, color-mix(... 95%, transparent) 100%);
}
```
- **是有意设计**（注释：保证反白文字 ≥4.5:1），后果是整图被压暗 70–95%。
- 降级路径：不支持 `color-mix` 时 = **100% 纯黑不透明矩形**（注释自述"图没了，字仍然可读"）→ 与用户"一大块黑矩形"吻合。
- 叠加上 `index.vue:385` 面板兜底黑底（暗色 `--color-surface-inverse: #101011`）；`<img>` **无 `@error` 兜底**（`BannerCarousel.vue:59-72`）→ 图片加载失败时 = 黑底 + 黑罩层 = 整块纯黑。
- **已排除**：取景 zoom 露底（`MIN_ZOOM=1` + `object-fit:cover`，只会盖更满）、视差露边（10px < 12px 外扩余量，父级 `overflow: clip`）。
- 暗色主题**不新增**覆盖层（`tokens.css:146-196` 只换 token 值），但黑得更黑。
- `.home-cover__lines`（`index.vue:391-398`）直角/偏右下/可越出面板，但颜色是反白/黄/蓝/珊瑚，**不是黑色**；`.home-cover__capsule`（VOL.01，`:553-562`）确是黑底矩形但在右栏、带圆角、尺寸小 → 均推断不是。

### ⑧ 圆点容器现状（**已确认**）
`BannerCarousel.vue:77-110` + `:178-252`：底部居中 `glass-thin` pill（`left:50%` + `translateX(-50%)`），装 `‹ 圆点组 ›`，active 点拉成 28×8 条，**44px 触控区**，**无 hover 态**。暗色 `--glass-thin-fill: rgb(22 22 23 / .72)` → 深色半透明胶囊。
- **改造会撞三条既有设计测试**：`editorial-hero.test.ts:86-94`（钉 70%/95% 字面值）、`:312-317`（钉 `glass-thin` + `radius-pill`）、`glass.test.ts:38`（记录 BannerCarousel 允许 1 处玻璃类）。

### ⑨ 轮播标题三层全无（**已确认**）
契约 `openapi.d.ts:5697-5708`、DDL `schema.sql:200-209`、payload `bannerCover.ts:54-66` 均无 `title`；读者站轮播上**零可见文字**。
- 陷阱：`BannerCarousel.test.ts:13-15` 夹具写了 `abstract: "第一张"`，但**组件从不读 `abstract`**，契约也无此字段（`as never` 强转所以编译不报错）。

### ⑩ 后台轮播列表为空的根因（**已确认，且渲染函数无辜**）
`media/index.vue:209` 未传 `show` → `Show=false` → SQL ``WHERE `show` = false``（`banner.go:47-48`）；新建默认 `show=true`（`:59`）⇒ 永不显示。
- **不是** `bannerColumns` 字段缺失：六列消费的字段（`cover`/`href`/`show`/`updated_at`/`id`/`cover_image_id`）契约里全有，且**无一列读 `title`**。
- 接口就是 `GET /banners`（后台 `admin.ts:207` / 读者站 `site.ts:40` 同一接口），返回 `HasMoreList{list, has_more}`。
- 连带：`has_more=false` → 「下一页」恒 disabled；创建/删除/重新取景后的 `refreshBanners`（`:303/:473/:536`）都会命中同一 bug。

### ⑪ 改造落点（已确认）
宿主 = `.banner-stage`（`BannerCarousel.vue:43`，即 `.home-cover__media`），`position:absolute; inset:0`。现有 `.banner-scrim`（z-index 1）与 `.banner-controls`（z-index 6）是其直接子节点。
- **坐标系差异**：图片层 `.banner-photo` 是 `inset: -12px -21.3333px`（比面板大），而 scrim/controls 相对面板 content-box 定位 → 若渐黑容器按面板定位，与图片可见区有 12/21.33px 错位。
- `.glass-thin` 有 `isolation: isolate` → 会新建叠放上下文，影响子元素 z-index。

### ⑫ 「零 DDL」对 banner 的适用性（待探子最终确认）
- 硬约束理由（GLOSSARY:50-53）是「ES 投影按列名解析 binlog 行」，而进 ES 的 5 张表是 `article_models / user_models / category_models / tag_models / article_tag_models` —— **banner 不在其中**。
- 但对 banner 开例外仍有连带：`testkit/schema/schema.go` 的表数注释与 `flag_db_integration_test.go:35` 的表数断言；`entrypoint.sh` 以「`runtime_config_models` 是否存在」判断初始化，**已存在的表被删后不会自动重建**（需手动跑 `-db`）。
- `banner_crop_models` 改名还会牵动 GLOSSARY（明写该表名与 `banner.crop_invalid` 属历史命名）与错误码。

---

## 七、✅ 已在线验证定案（2026-11-02，直连 43.155.228.146 与生产 MySQL/ClickHouse）

**验证方式**：F12 抓包 + JWT 解码 + 生产库直查 + 用真实 token 回放接口（测试产生的数据已清理，`article_digg_models` 已回到 0 行）。

### ⑳-A 轮播列表为空 —— **确认命中列表过滤 bug**
生产库实测：`banner_models` 共 **2 行，两行 `show=1`**（`375330790029922304` / `375329868482613249`）。
→ 后端行是对的，首页正常；后台 `refreshBanners` 未传 `show` → `WHERE show=false` → 查不到。**根因成立**。

### ⑳-B 点赞「duplicated key not allowed」—— **确认，且线上原样复现**
- 唯一索引 `uk_article_digg (article_id, user_id)` **不含 `deleted_at`**（`SHOW INDEX` 实测）。
- 「取消点赞」走 `Remove` → **软删**（实测 `deleted_at=2026-11-02 02:28:33.974` 仍在表里）。
- `Exists` 带 `deleted_at IS NULL` 过滤 → 软删后返回 0 → `Add` 走 INSERT → **撞唯一键**。
- **线上回放复现**：`{"code":1001,"data":{},"msg":"duplicated key not allowed"}` —— 与用户报错逐字一致。
- 结论：**点赞→取消→再点赞，必然报这个错**。这是 `Exists` 与唯一索引口径不一致。

### ⑳-C 点赞「未登录/登录状态已过期」—— **确认是 token_version 不一致**
同一 session（`375325762284163072`）、同一 user（`373950800474411008`）的两枚 token：

| token | 来源 | `token_version` | 实测接口返回 |
|---|---|---|---|
| A | 登录签发（exp 11-03 00:50） | **1**（与 DB 一致） | `{"code":0,"is_digg":true,"msg":"成功"}` ✅ |
| B | **refresh 签发**（exp 11-03 02:15） | **0**（与 DB 不一致） | `{"code":1001,"msg":"登录状态已过期，请重新登录"}` ❌ |
| — | 匿名 | — | `{"code":1001,"msg":"请先登录"}` |

- 生产库 `user_models.token_version = 1`（master/slave 一致）。
- 后端 `Authenticate` 第 4 步 `account.TokenVersion != claims.TokenVersion` → `CodeUserSessionExpired`（`service.go:333`）。
- UI 仍显示已登录，因为 `isLoggedIn` **只看 localStorage 有没有 token 字符串**（`frontend/app/stores/auth.ts:294`）。
- **待查**：refresh 为何签发 `0`。已排除：`FindByID` 是 `SELECT *`、`toUser`/`toAuthAccount`/`claimsOf`/`jwtIssuer.Issue`/`GetToken` 全部整体拷贝 `TokenVersion`；主从一致；`BumpTokenVersion` 是 `token_version+1`；02:15 前后 `action_audit_logs` 无任何账号变更。**下一步要加日志**（在 `Refresh` 里打印 `account.TokenVersion`）才能定位。

### ⑳-D 评论「violates foreign key constraint」—— **确认（结构性问题）**
- 生产库 `comment_models` 的外键：`fk_comment_models_parent_model (reply_id → comment_models.id)`。
- `reply_id` 列 `DEFAULT '0'`，而 `comment_models` **当前 0 行** → **库里不存在 id=0 的行**。
- `ctype.ID.Value()` = `int64(id.Uint64())`（`ctype/id.go:136-137`）→ 根评论 `ReplyID` 为零值时**落库写 0** → 必然违反该外键。
- 结论：**任何根评论（无父）都会 FK 失败**，不是偶发。

### ⑳-E 已顺带证实的
- 抓包 4 个请求 = `digg → refresh → detail → digg`，**确认两次 digg**，与"自动刷新重试"推断一致。
- 后台「轮播运营」空列表**不是渲染问题**：六列消费字段契约里全有。
- ClickHouse `token_refresh` 事件确实在 02:15:58 成功（`user_id=0`，印证探子说的"refresh 事件传 0"）。

> 另：本地 dev 匿名 SSR 缓存是**关闭**的（`plugins/ssrAnonymousCache.ts:33-35`），缓存相关现象需在 test/production 验。

---

## 六·补二、后台子菜单盘点（探子回报，已确认）

### ⑬ 全后台页面级分段只有 2 处 + 1 处筛选型
| 位置 | 形态 | 判定 |
|---|---|---|
| `pages/site/index.vue:24-45` | `NTabs` 8 段（站点/AI/邮件/存储/OAuth/日志/上传/装饰） | **导航型** → 下移（D34 已定） |
| `pages/logs/index.vue:252-254` | `NTabs` **6 类**日志（runtime/action/login/cdc/replay/ai，**无 access**） | 形态导航但交互完全同构；页内注释 `:8-9` 明写「五类交互完全一样，做成五个页面等于抄五遍」→ 探子建议**保持页内 tab** |
| `pages/index.vue:163-168` | `NRadioGroup` 今日/本周/本月/本年 | **筛选型**，不下移 |

- `pages/media/index.vue` **不是 tab**，是「一页多区块」3 张 NCard（新增轮播/轮播运营/图片资源库）。拆成子路由会**断掉联动**：`media/index.vue:318-319` 上传封面后 `imagePage.value = 1; await refreshImages()`。
- 筛选型不算二级导航：`review/index.vue:144`、`logs/dead-letter.vue:184-185` 的 `NSelect`。

### ⑭ 语义子项只有两条，且改 children 必撞测试（**已确认**）
| 候选 | 现父项 |
|---|---|
| `热门置顶 /articles/hot-top`（`menu.ts:38`） | `/articles` 文章管理 |
| `同步死信 /logs/dead-letter`（`menu.ts:44`） | `/logs` 日志中心 |

- `menu.test.ts:50-58` 是 ADR-0017 决策 5 的位置断言：`ADMIN_NAV[文章管理index + 1]` 必须是「热门置顶」且 path `/articles/hot-top`。**挪进 children 后 :56 与 :57 双双失败**。
- `/logs/dead-letter` 挪进 children 撞 `:82-97`（页面无菜单入口 → 红）；同时 `:39-48`（path→文件）与 `:74-80`（path 唯一）会对 children **静默漏检**。
- 根因：4 条对账全部假设 `ADMIN_NAV` 是**扁平数组**。必须先引入 `flattenNav()` 之类扁平视图再改断言。

### ⑮ ⚠️ 最大的隐藏地雷：两处 `toBe(12)`（**已确认**）
- `frontend_admin/test/unit/admin-flip.test.ts:181` → `expect(pages.length).toBe(12);`
- `frontend_admin/test/unit/legacy-colors.test.ts:141` → `expect(pageFiles.length).toBe(12);`
- 注释写明「12 = 11 个既有页面 + ADR-0017 新增的热门置顶页」。
- **每新建一个页面文件就红一次**，站点配置 8 段全拆 = 新增 7–8 个文件 → 这两个数字必须同步改。

### ⑯ 页面文件放法（**已确认**）
- `pages/site/deco.vue`（与 `index.vue` 同级平铺）或 `pages/site/deco/index.vue` —— 两种 `pageFileCandidates` 都认，**都可行**。后台不需要嵌套壳（AdminShell 已由 `layouts/default.vue` 全局套住）。
- **不可行**：只把 tab 状态搬进 URL 而不建文件 → 运行时 404 + 撞 `:39-48`（这正是 `menu.ts:5-7` 注释里记过的「侧边栏挂 /logs 而目录不存在 → 404」同类事故）。

### ⑰ 面包屑落点与连带（**已确认**）
- 全仓 `breadcrumb|面包屑` 零命中，需新建。建议落 `app/components/admin/`（**放子目录会带路径前缀注册名**，见 `scripts/check-components.mjs:143` 与 `SettingsAi.vue:5-7` 踩坑注释：不报错、整块 UI **静默不渲染**）。
- 数据来源两个选项：**从 `menu.ts` 的 path 层级推导**（改动面小，与「菜单是唯一来源」一致）vs **路由 meta**（需给 10 个页面加 `definePageMeta`，全仓目前只有 login/setup 用了）。探子推荐前者。
- `AdminShell.vue:68` 的 `routeMeta = navItems.find(item => item.path === route.path)` 是**精确 path 相等** → `/site/deco` 命中不到会静默 fallback 到「数据概览」（`:67`），标题与高亮都错。
- 样式约束：新文件 `border-radius` 必须走 `var(--radius-*)`（`admin-flip.test.ts:99-110`），`admin.css` 不得出现裸色值（`admin-flip.test.ts:119-152`、`legacy-colors.test.ts:139-152`）。
- `menu.ts` 在 vitest 覆盖率范围（`vitest.config.ts:41`，门槛 95/90/95/95）→ 新增 util 函数必须同时补用例。
- 只要 8 个 `Settings*.vue` **不改名不移动**，`site-config.test.ts:26` 与 `settings-wording.test.ts:20-24` 就不会红。

---

## 六·补三、banner 表 DROP 重建影响面（探子回报，已确认）

### ⑱ ES 投影不受影响，但 **image-ref 受影响**（**已确认，且修正了先前判断**）
- **两张表都不在 ES 投影里**：`article_search_projection.go:16-29`、`projection_replay.go:131-169`、`settings.yaml:197-207` 的 river 监听范围，都只有那 5（+1）张表。
- **但 `banner_models` 在 image-ref CDC 链路里**：`imageref/river.go:57` `imageRefTables = []string{"article_models","user_models","banner_models","favorite_models"}`，且**同样按列名解析**（`banner_refs.go:37` `snapshot.RequireString("cover")`；`row_snapshot.go:35-53` 按 `id`/`deleted_at`）。
  ⇒ **「零 DDL」的理由表述不完备**：GLOSSARY:50-53 / README:1207 写的是「ES 投影按列名解析」，准确说法应是「**CDC 按列名解析**」。
- **好消息**：go-mysql v1.13.0 `canal/sync.go:246-253` 会在 QUERY_EVENT 命中 DDL 后 `ClearTableCache` + `OnTableChanged`，而本项目 `imageref/river.go:174-176` 把 `OnDDL`/`OnTableChanged` 实现为**空返回** → DROP+CREATE **不会卡死、不会位点错乱**，会自动拿到新表结构。
- **`banner_crop_models` 不进任何 CDC 链路**（不在那四张表里）→ 改名/加列**零 CDC 风险**，这是最干净的一块。
- **安全边界**：image-ref 按列名取 `cover` ⇒ **加列安全，改/删 `cover` 列不安全**。本次只加 `title`，安全。

### ⑲ 孤儿引用行（**残留风险，已确认**）
DDL 的 DROP 是 QUERY_EVENT，**不走 `OnRow`**（`imageref/river.go:117-129` 只对 `DeleteAction` 做 `DeleteOwnerRefs`）
⇒ `image_ref_models` 里 `ref_type=banner`（`image_ref_enum/enter.go:13`）的引用行**不会被清理**。当前代码不做引用计数校验（`media/adapter/mysql/image.go:178-190` 只删引用再硬删图），实际影响是"后台图片库可能误判某张图仍被引用"。**建议重建后手工清一遍**。

### ⑳ 代码外必须做的 2 项（**已确认**）
1. **DROP 必须手工在 MySQL 执行** —— 全仓无 DROP 通道（`ddl.go:51-71` 的 `Apply` 只做 CREATE）。不是"改代码就能完成"的操作。
2. **重建后必须手工跑一次 `-db`** —— `entrypoint.sh:76-85` 只认 `runtime_config_models`，那张表还在就跳过自举（`README.md:948-953` 已写死此副作用）。命令见 `README.md:1046-1047`：
   ```bash
   docker compose exec blogx_server /app/server -f config/settings.yaml -db
   ```
   `-db` 只做 `CREATE TABLE IF NOT EXISTS`，幂等，只补建缺失表、不动其它表。

### ㉑ 流程上必须先开 ADR-0020（**已确认，有明文规定**）
- `docs/adr/README.md:47-61`：新增 ADR 四位递增（下一条 **0020**）、8 段固定结构、**须登记索引表**。
- `docs/adr/README.md:51`：已交付的 ADR **不改结论** → **ADR-0015 不能被改写**，只能在「后续修订」追加一条指向 0020。
- `docs/agents/domain.md:42-46`：与既有 ADR 冲突必须**显式指出**，有 `_Contradicts ADR-00XX` 句式模板。
- 需同步的**无条件表述**四处：`GLOSSARY.md:50-53`、`docs/architecture/overview.md:575`（该行已写"放宽的前提是有列名契约测试"，正可指向 0020）、`README.md:1207`、`docs/notes/memory/MEMORY.md:25`。
- 另需改：`GLOSSARY.md:416`（「取景」词条把**表名与错误码并列**写在"历史命名例外"里，须拆开重写）、`docs/adr/0015:46` 与 `:70`。
- **好消息**：**契约字段 `crop` 与错误码 `banner.crop_invalid` 都不受表名影响**（前者是 HTTP 契约字段，后者属 `banner.*` 域），前端只认字段名不认表名 → 改名**不需要动错误码与前端类型**。

### ㉒ 连带量（已确认）
- 代码内必改 ≈ 20 处：DDL 2、Go row 4、testkit 登记 6、契约 DTO/handler 8、domain/app/apptest 3。
- 生成产物重跑 4 条命令：`cd backend && make openapi` → 两端 `pnpm openapi:types`。
- 测试敏感度：`TestFlagDB`（`flag_db_integration_test.go:35`，表数从 `All` 组推导**不是写死**）与 `TestGeneratedRowsMatchSchemaSQL`（`schema_sql_integration_test.go:43`，双库表清单比对）对**删表/加表**敏感；`columns_integration_test.go` **不含** `banner_crop_models`，故改它的列不红那条，但 `banner_models` 加列会红（除非 DDL 与 `BannerRow` 同步）。
- 数据：无 CLI 导出（`cmd/cli/enter.go:43-63` 只有 `-db`/`-es`/`-t`/`-version`），只能 mysqldump 或裸 SQL；回填需手工 INSERT，且 **`banner_models.id` 无 AUTO_INCREMENT 必须显式带原 id**、`show` 是保留字需反引号。

### ㉓ 判定
**可行**，不是"不建议"。前提：先开 ADR-0020 + 手工 DROP + 手工 `-db` + 备份 + 清理孤儿引用行。

---

## 八、拆票结果（2026-11-02，已发布到 GitHub Issues）

按 `/to-spec` 输出 7 份 spec，全部带 `ready-for-agent`：

| # | Issue | 范围 |
|---|---|---|
| 1 | [#89](https://github.com/Jenlybein/NewblogX/issues/89) | 缺陷修复：点赞重复键 / 登录态误判 / 评论外键 / 窄屏目录下跳 / 后台轮播列表为空 |
| 2 | [#90](https://github.com/Jenlybein/NewblogX/issues/90) | 合集：管理员编排的文章集合 + 读者侧入口 |
| 3 | [#91](https://github.com/Jenlybein/NewblogX/issues/91) | 后台左侧菜单两级化 + 面包屑 |
| 4 | [#92](https://github.com/Jenlybein/NewblogX/issues/92) | 首页右栏卡片后台管理 + 实时预览 |
| 5 | [#93](https://github.com/Jenlybein/NewblogX/issues/93) | 轮播改造：标题字段 / 底部渐黑容器 / 去掉遮挡大黑块 |
| 6 | [#94](https://github.com/Jenlybein/NewblogX/issues/94) | 光标挂件：挂绳弹性 + 进出场渐隐渐显 |
| 7 | [#95](https://github.com/Jenlybein/NewblogX/issues/95) | 封面取景 16:9 / 创作页建分类 / 文案清理 / 登录设备去重 |

**与最初"6 个"的偏离**：原 6 组分组未覆盖「轮播改造」与「窄屏目录下跳」。下跳并入 #89（都是 bug），轮播改造单独成 #93（表重建 + 视觉重设，体量接近合集）。

**跨票依赖**：
- **#93 拥有两件跨票资产**：ADR-0020（零 DDL 例外 + 通用取景表改名合并）、轮播表的 DROP 重建与手工建表命令。#95 的封面取景依赖 #93 的通用取景表。
- #93 未落地时，#95 只能先做「锁 16:9 不存取景」。

## 九、拆票（/to-tickets，2026-11-02 已发布）

36 张票，编号 **#96–#131**，全部为对应 spec 的**子 issue**，全部带 `ready-for-agent`。
编号即依赖顺序（阻塞者先建），票面「Blocked by」引用真实票号。

| 票 | 标题 | 父 | 阻塞于 |
|---|---|---|---|
| #96 | 点赞关系行口径统一 | 89 | — |
| #97 | 评论根评论外键修复 | 89 | — |
| #98 | 前端登录态真实性（重试判别 + 已登录判定） | 89 | — |
| #99 | 刷新签发的访问令牌版本与会话账号一致 | 89 | — |
| #100 | 窄屏目录只滚动自身容器且窄屏不渲染目录 | 89 | — |
| #101 | 后台轮播列表改为三态筛选 | 89 | — |
| #102 | 合集最小闭环（建表/建合集/收录一篇/读者侧可见） | 90 | — |
| #103 | 后台合集管理页（增删改 + 取景封面 + 展示开关） | 90 | #102 |
| #104 | 合集编排（批量收录 + 两级手动排序） | 90 | #102 |
| #105 | 合集失效行（保留占位/标记/一键清理） | 90 | #102 |
| #106 | **预重构**：菜单拍平视图与对账改造 | 91 | — |
| #107 | 菜单支持子项（分组/展开/高亮/标题） | 91 | #106 |
| #108 | 站点配置八域拆成独立路由 | 91 | #107 |
| #109 | 热门置顶与同步死信归入对应父项（含 ADR 修订） | 91 | #107 |
| #110 | 后台面包屑 | 91 | #107 |
| #111 | 右栏卡片最小闭环（数据驱动渲染） | 92 | — |
| #112 | 后台卡片管理界面（增删/排序/标题/启用） | 92 | #111 |
| #113 | 右栏三种内置卡（热门文章/最新评论/标签云） | 92 | #111 |
| #114 | 右栏合集推荐卡 | 92 | #111, #102 |
| #115 | 右栏富文本卡与清洗 | 92 | #111 |
| #116 | 右栏预览区 | 92 | #112 |
| #117 | 下线旧右栏配置与旧可见性判定 | 92 | #111 |
| #118 | **ADR-0020**：零 DDL 例外授权与通用取景表 | 93 | — |
| #119 | 轮播表重建与通用取景表合并 | 93 | #118 |
| #120 | 轮播标题（契约 + 后端 + 后台表单与列表） | 93 | #119 |
| #121 | 轮播视觉改造（底部渐黑/控件并入/去掉黑块） | 93 | #120 |
| #122 | 挂件刚度参数（挂绳从刚杆变弹性绳，含迭代次数修正） | 94 | — |
| #123 | 挂件指针端平滑跟随 | 94 | #122 |
| #124 | 挂件参数区间放宽与滑块/数字双输入（含阻尼 ADR 修订） | 94 | #122 |
| #125 | 挂件进出场渐隐渐显 | 94 | — |
| #126 | 读者站解释性文案清理 | 95 | — |
| #127 | 创作页内联创建分类 | 95 | — |
| #128 | 文章封面锁 16:9 与上传体验改造 | 95 | — |
| #129 | 文章封面取景存取 | 95 | #128, #119 |
| #130 | 登录设备：UA 解析与按设备聚合展示 | 95 | — |
| #131 | 登录时自动吊销同设备旧会话 | 95 | #130 |

**可直接开工的 frontier（16 张，无阻塞）**：
#96 #97 #98 #99 #100 #101 #102 #106 #111 #118 #122 #125 #126 #127 #128 #130

## 十、实施进展（/implement-spec，2026-11-02）

集成分支 `integration/89-auth-and-interaction-fixes`（基于 `main` @ `eca7edc`），draft PR [#133](https://github.com/Jenlybein/NewblogX/pull/133)。

| 票 | 提交 | 状态 |
|---|---|---|
| #101 轮播三态筛选 | `95508f1` | ✅ 已合入 |
| #96 点赞关系行口径 | `4e446d9` + `e68c03c`（对账工具） | ✅ 已合入 |
| #99 刷新令牌版本 | `18f59c1` | ✅ 已合入 |
| #97 评论外键 | `6d6bcd3` | ✅ 已合入（父字段落 NULL，连带修正 `reply_id IS NULL`） |
| #98 前端登录态 | `69e4229` + `59479cc` | ✅ 已合入（写请求不重放 + 已登录判定改口径 + 校验去重） |
| #100 窄屏目录 | `70452c2` | ✅ 已合入（纯函数 + 只滚自身容器 + 窄屏不渲染） |

**6 张票全部完成**，PR [#133](https://github.com/Jenlybein/NewblogX/pull/133) 已转正式评审，#96–#101 已关闭，6 个 worktree 已清理。

#98 的实现与原计划不同（值得记一笔）：票据原写"重试判别改用稳定业务码"，但**后端业务码只有 0/1001/1002 三个值，所有业务失败都塌缩成 1001，无法区分"未登录"与"参数错误"**，所以判别依据改为放在**请求方法**这一侧——只有幂等的读可以重放，写请求一律不重试。待后端区分错误类别后再收敛。

#100 的实现要点：新增纯函数 `resolveTocFollow`（在既有目录纯函数模块），只计算目录容器的 scrollTop；组件改用它并加程序化滚动抑制窗口；窄屏按与侧栏 sticky→static 同一断点不渲染目录。顺带清理了对 shadow DOM 标题恒为 null 的 `document.getElementById` 回退。

审查阶段的自我修正：#98 的 `markPossiblyExpired` 会在每次鉴权失败时发起校验，并发失败会重复打接口——已用 `verifyPromise` 收敛（`59479cc`）。另核查确认两处高风险连带无误：轮播三态改完后读者站仍显式只取展示中；评论父字段可空后依赖该列的查询仅一处且已改为 `IS NULL`。

### ⚠️ 本次挖出的更大根因：**数据库查询缓存污染**（已单开 [#132](https://github.com/Jenlybein/NewblogX/issues/132)，挂为 #89 子票）

这是 #99 诊断时的意外收获，也反向解释了 §⑳-C 里"为什么此前所有排除项都干净"。

**机制（已核实插件源码 + 实测复现）**：
- `core/init_db.go` 给全局 `*gorm.DB` 装了 `go-gorm/caches` 插件（Redis 后端）。
- 插件的缓存标识**只由「SQL + 参数」组成，不含目标结构体类型** —— 已核对 `go-gorm/caches@v4.0.5` 的 `identifier.go`：`buildIdentifier` = `fmt.Sprintf("%s%s-%s", prefix, query, queryArgs)`。
- 命中时把缓存的 `Query[any]{Dest}` JSON 用 `json.Unmarshal` 解码进**本次查询的 Dest**。

**两个查询撞同一个 key**：
- 文章模块 `userPort.one`：`Take(&m, "id = ?", id)`，目标 `userRow` —— **局部行，无 `token_version` 列**
- 用户模块 `FindByID`：`Take(&row, "id = ?", id)`，目标 `UserRow` —— `SELECT *`，**含 `token_version`**

两者生成的 SQL 完全相同（`SELECT * FROM user_models WHERE id = ? LIMIT 1`）→ 文章页渲染先跑窄行查询存进缓存 → 紧接着的 refresh 命中同一 key → 窄行 JSON 解码进 `UserRow` → 字段不存在 → **静默零值 0**。

实测（临时用例，已删）：
```
{"Dest":{"id":"1001","username":"alice","role":2},"RowsAffected":1}
→ id="1001" token_version=0 status=0 role=2
```

**为什么这解释了当初的"已排除"**：值是在**进入 Go 结构体之前**就没的，所以逐层拷贝链路自然每一层都干净。也解释了登录签发为何正确（`FindByUsername` 的 SQL 不同 → 不同 key）。

**影响面**：不止令牌版本。任何"同表 + 同 WHERE + 不同目标结构体"的组合都会中招，且表现为**字段静默归零而非报错**。`Authenticate` 也走 `FindByID`，故 #97/#98 的登录态误判可能还有别的入口。
→ **#99 只修了 refresh 一条路径的后果，根因仍在**，故单开 #132（让缓存标识带上目标类型，或关闭该插件并评估其"写操作触发全表 SCAN 式失效"的成本）。

### #132：查询缓存污染已修复（根因核实 + 红→绿实证）

**根因成立并已实测复现**。`Take` 不带 Select 时生成的是 `SELECT *`，所以窄结构体查询缓存的 JSON **本来含全列**——这一点最初让我误以为根因不成立。真正的开关是**软删条件**：`deleted_at IS NULL` 是拼进 SQL 的，窄结构体若不带 `DeletedAt`，SQL 就不同、命中不到同一个键，污染复现不出来（**测试会假绿**）。真实的 article `userRow` 内嵌 `BaseRow`（含 `DeletedAt`），与整行结构体 SQL **完全一致**，所以线上会中招。护栏用例正是照这个形状构造的。

复现输出（修复前）：
```
缓存 JSON = {"Dest":{"ID":"7201","Username":"cache_user","Nickname":"缓存用户","Role":0,...}}
整行查询被窄行缓存污染：token_version 应为真实库值 7，实际 0
```

**修法**：Cacher 拿得到 `Dest`，在这一层把形状（类型名）补进键里——不改插件内部、不替换插件。实现放 `internal/platform/mysql/querycache.go` 并**只此一份**，生产 `core/init_db.go` 与测试基建 `testkit/mysqlcontainer` 共用（两份的话测试验证的就不是生产行为）。用类型名而不是值：值随行内容变化会让缓存永远命中不了。

**成本评估结论：建议后续关闭该插件（未在本票内关闭）**。插件在 Create/Update/Delete 回调里无条件 `Invalidate`，而 `redisCacher` 的实现是 `SCAN gorm-caches::*` 全量扫描 + DEL —— **任何一次写都清空全部缓存**（已加 `TestAnyWriteWipesTheWholeCache` 钉住）。本站每次文章浏览都要写浏览计数，写极频繁 → 缓存刚存进去就被清空，命中率趋近于 0，却每次写都付一次 O(N) 的 Redis SCAN。关闭会改变生产读性能特征，故单独评估，不在本票一并处理。

### 附带修掉的既有基建缺陷（影响面超出 #96）

`testkit/mysqlcontainer` 的 `DB()` 未开 `TranslateError`，而生产 `core/init_db.go:109` 是开的。后果：`RestoreOrCreateUnique` 靠 `gorm.ErrDuplicatedKey` 判别"唯一键冲突 == 已存在"，测试容器里该分支走不到，**在生产不会报错的地方假红**；同时让评论/关注模块的既有集成测试对唯一键行为失去保护。已修（1 行）。

### #96 的修法与理由

选「让判定不再排除软删行」而非「硬删」：
1. 与评论点赞（`comment/adapter/mysql`）已是同一口径（同一个平台助手）
2. 零 DDL，不碰 `schema.sql`
3. 线上存量软删残行**自动被修复覆盖**，不清理也能再点赞 → 对账降级为可选而非前置步骤

对账工具 `./blogx-server -t reconcile -s digg`：**默认只读**，`-apply` 才物理删；孤儿行（指向已软删文章）只统计不自动删（避免"用户恢复文章后点赞全没了"）。

**四条关键链**：
- 轮播：`#118 → #119 → #120 → #121`（文档先行，再动表，再做视觉）
- 封面：`#128 → #129`，且 `#129` 还依赖 `#119` 的通用取景表
- 导航：`#106 → #107 → {#108, #109, #110}`（预重构先行）
- 右栏：`#111 → #112 → #116`；合集推荐卡 `#114` 依赖 `#102`

---

## 五、开放问题（**已全部有答案，见 §三 D1–D64**；以下为 2026-11-02 晚新开的缺口）

- **Q-A**：右栏「完整装修」（可增删 / 排序 / 参数 / 富文本）的**配置存储结构**未定。现有 `site.index_right.list` 只是 `[]ComponentInfo{Title,Enable}`，装不下 type / title / params / richtext / order。
- **Q-B**：合集中收录的文章**失效**（被删 / 转私密 / 撤稿）后怎么处理。热门置顶有现成先例（ADR-0017 决策 4–5：失效行保留、前台不展示、后台标「已失效」可一键清理）。
- **Q-C**：UA 解析放哪一侧。**发现**：合并判据就是 UA 字符串本身，后端 `GROUP BY ua` 即可分组，**不必解析设备名**；设备名只是显示层需求，可纯前端解析（零后端依赖、零契约改动）。代价：UA 原文不同（Chrome 141 vs 142）会算成两台。
- **Q-D**：「吊销旧会话」的触发时机（登录时自动 / 用户手动 / 定时任务）。
- **Q-E**：`/collections` 是否进 SSR 白名单。**硬约束**：GLOSSARY §十「渲染边界」白名单只有 `/` 与 `/article/**`，新增 SSR 页面**必须先改 ADR-0019** 再进白名单。
- **Q-F**：右栏「热门文章」卡的排序依据（浏览量 / 点赞数 / 复用既有接口）。注意：主栏已有「热门置顶」，右栏若取同一份数据会重复。
- **Q-G**：合集列表页自身的排序（后台手动 / 创建时间 / 更新时间）。

---

## 六、术语与口径提醒（给后续拆票）

- 右栏配置在 **`site` 段**（`index_right`），不是 `deco` 段；`deco` 段只有光标挂件。
- 「三步代码登记」有两种含义，别混：**接进 ES 投影**的三步（ADR-0009）vs **新建业务表**的三处登记（schema.sql / testkit rows / testkit groupModels）。
- 分类（category）**已存在且是用户级的**；标签（tag）是全局公共词库、管理员维护。两者不是一回事。
- 「最近登录」页的真实语义是「当前有效登录设备」，不是登录流水。
- 收藏夹（`favorite_models`）不是合集；热门置顶是排序位不是合集。
