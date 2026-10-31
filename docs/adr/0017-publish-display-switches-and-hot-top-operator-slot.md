# ADR-0017：站点发布/展示开关与热门置顶运营位

- 状态：提议
- 日期：2026-10-31
- 来源：产品需求 + 设计对话（"站点模式和社区模式的区别是什么"引出的死配置排查）；落地票 [#54](https://github.com/Jenlybein/NewblogX/issues/54)

## 背景与问题

三件事同时暴露了同一类问题——**配置与数据模型写了意图，但没有任何消费点**：

1. **站点模式是死配置**。`site.site_info.mode`（1 社区 / 2 博客）只有枚举声明（[site_mode.go](../../backend/internal/kernel/enum/site_mode.go)）、默认值与后台下拉框；全仓没有一处读它。默认值文件里的注释"博客模式下普通用户发布文章会受到更严格的限制"从未实现（[site_default_settings.yaml](../../backend/config/site_default_settings.yaml)）。它给管理员的是一个**看起来能改定位、实际什么都不改变**的开关。
2. **首页「热门置顶」区块恒为空**。前端调 `/api/articles/top` 时传的是 `type: 2`（[services/article.ts](../../frontend/app/services/article.ts)），后端该接口只认 `user_id`；匿名访客的目标用户 ID 为 0，查询 `WHERE user_id = 0` 永远无结果（[handler_sub.go](../../backend/internal/article/adapter/http/handler_sub.go)、[social_repo.go](../../backend/internal/article/adapter/mysql/social_repo.go)）。`type=2` 是 v1「全站运营置顶」的遗留参数，迁移到 v2 后没有接上，于是这个区块对每个访客都渲染空态（[HomePinnedSection.vue](../../frontend/app/components/home/HomePinnedSection.vue)）。
3. **"管理员置顶"存在但不可达**。ES 文档里的 `admin_top` 按"置顶人是管理员"推导（[article_sync_load.go](../../backend/internal/platform/es/articlesync/article_sync_load.go)），但没有任何接口允许管理员置顶别人的文章（`CanBeTopped` 只允许作者置顶自己的文章），搜索也从不按它排序、读者站与后台都不消费。同时站点完全没有"谁能发文 / 非管理员的文章是否展示"的开关，任何拿到令牌的用户都能发布（`skip_examining` 默认 true 时提交即发布）。
4. **访客角色名存实亡**。`role=3`（guest）与库里 `role=0`（列默认值，不在枚举里）在鉴权时被折叠成"普通用户"（[article_wiring.go](../../backend/app/article_wiring.go) 的 `viewerFromContext`），因此 guest 实际拥有全部作者权限。

## 决策

1. **删除站点模式**：删 `conf/site.SiteInfo.Mode`、`kernel/enum/site_mode.go`、默认值里的 `mode: 1` 与那两行注释、后台下拉框与两端手写类型。库里已在 `runtime_config_models` 的 site 段里残留的 `"mode": 1` **不做迁移**：深度合并会把它留在 map 里但不再进类型（[merge.go](../../backend/internal/platform/config/merge.go)），管理员下次保存整段即自然消失。零 DDL。
2. **新增三个布尔开关**（都在 `site.article` 段，默认 `true`，后台同一张卡「发布与展示」）：
   - `allow_non_admin_publish`（允许非管理员发布文章）：关 —— 非管理员不能"创建即发布"、不能提交审核、不能发布自己的草稿、不能更新已发布文章。
   - `allow_non_admin_draft`（允许非管理员创建草稿）：关 —— 非管理员不能新建草稿、不能更新草稿；存量草稿可打开、可删除，不可保存。
   - 两者都关 —— 非管理员只能查看和删除自己的存量草稿。
   - `show_non_admin_articles`（显示非管理员发布文章）：关 —— 读者端所有公开读面过滤掉"作者不是管理员"的文章。
   - 读者站写作区同步做体验：发布开关关闭时隐藏「写文章」入口、直接访问提示"当前不允许发布"；草稿开关关闭时编辑器只读、保存与发布按钮禁用并说明原因（服务端判定仍是唯一边界）。
3. **guest / role=0 在鉴权层视同未登录**：拦截点放在 [auth_verifier.go](../../backend/app/auth_verifier.go) 每请求重建 claims 的地方——`role ∉ {1,2}` 一律按"未登录"处理。这是唯一的收敛点：中间件、文章 Viewer、搜索、评论、私信、收藏/关注、媒体、通知、AI 都从它拿身份，改这一处就全覆盖；反过来，其中 6 个模块把调用者压成 `uint64`/`bool`，结构上无法表达 guest，逐个改只会留下漏口。
   - 保留 `domain.RoleGuest` 与 `role=3` 这个值本身：后台角色下拉保留 guest 并标注「受限访客（无法写入）」，`role=0` 的标签由"注册用户"改为"未知角色（按受限访客处理）"（`frontend_admin/app/utils/roles.ts` 与 `roles.test.ts` 同步改）。
   - 它们能浏览公开内容，不能创建/发布/互动，也看不到自己的历史内容。
   - **不改登录与刷新链路**：本项目尚无 guest 与 role=0 存量数据，登录仍会成功，但令牌在鉴权层不被承认（写操作 401，公开页按匿名渲染）。将来真有 guest 时再补登录期拦截。`role=2`（普通用户）才是三个开关作用的"非管理员"。
4. **"显示"开关在服务端实现**，不做前端隐藏：
   - ES 文章文档新增 `author_is_admin`（由作者 `user_models.role` 投影；用户角色变更走已有的"用户行 → 重算文章文档"分支），公开查询（type 1/2/3，含 AI 搜索）在开关关闭时加 `author_is_admin = true` 过滤；type 4（自己的文章）与 type 5（后台）不过滤。
   - 文章详情在开关关闭且作者非管理员时返回 404；收藏夹内容、浏览历史同样过滤；非管理员作者主页保留页面但文章列表为空态。
   - **不做前台管理员例外**：读者站列表是 `auth:false` 的公开请求、`useAsyncData` 缓存键不区分身份，加例外会让缓存串号。
5. **热门置顶改为独立运营位**：
   - 新表 `article_hot_top_models`（`article_id` 唯一、`sort_order`、软删），与 ES 无关；最多 12 条，新置顶排末尾，取消后序号压实。
   - 只有"已发布且可见"的文章可置顶；文章后来被隐藏/退回草稿/删除时，置顶行**保留**、首页容器 JOIN 过滤掉，后台在热门置顶页标"已失效"并提供一键清理。
   - 接口：`GET /articles/hot-top`（公开，返回卡片列表）、`POST|DELETE /api/articles/{id}/admin/hot-top`（幂等开关）、`PATCH /api/articles/hot-top/order`（全量顺序替换）。
   - 后台：文章管理列表每行加「热门置顶 / 取消」；后台侧边栏在「文章管理」之后新增「热门置顶」页（后台菜单是扁平清单，没有分组）负责排序（上移/下移/移除）、失效标记与清理。文章列表的置顶状态来自 `GET /articles/hot-top` 的 ID 集合，**不依赖 ES 字段**。
   - 首页「热门置顶」区块**只在有数据时渲染**：无数据、加载中、请求失败都整块不出现，同时删掉现有空态分支与文案。
6. **清理 ES 里无人消费的置顶派生字段**：删除 `admin_top`、`author_top` 与搜索结果里的 `top` 对象（连同 `search/domain` 的 `Top`/`UserTop`/`AdminTop`）；作者置顶功能保留，继续走 `user_top_article_models` 与 `/articles/top?user_id`，只是不再进 ES。映射变更需要一次索引重建 + 全量回填（`-es` 后接 `-es -s article-sync`），期间搜索有短暂空窗。

## 理由与已否决方案

**为什么拆成三个开关而不是一个**：原设计只有一个"允许发布"，但发布与存草稿是两种权限——只拦发布会让作者写完直接发布绕过审核策略，只拦草稿会让存量草稿变成打不开的孤儿。用户要求把两者拆开，并把"存量草稿可打开、可删除、不可更新不可发布"作为兜底语义。

**热门置顶为什么新建表，而不是复用 `user_top_article_models`**：复用（管理员置顶任意文章 = 一行 `user_id=管理员`）看起来更省，因为它能让现有 `admin_top` 推导自动正确；但它是**个人置顶**关系，管理员给自己主页选的文章与"站点运营位"是两件事，复用会让管理员个人页置顶列表被运营位污染，并且必须放宽 `CanBeTopped`（作者才能置顶自己的文章）这条规则。新建表让个人置顶规则保持不变，代价是多一张表、后台多一页、多三个接口。

**已否决**：

- **保留站点模式并真正实现它**：需求本身不再要"社区/博客"二分，两个新开关直接覆盖了它原本想表达的差异；留着只会继续误导。
- **把热门置顶写进 ES**：读写都极少（后台改、首页读），MySQL 一次 JOIN 足够；写进 ES 要在投影里登记第三张表，并且"失效"状态要维护两套真相。
- **只在前端过滤"非管理员文章"**：详情直链能绕过去，等于开关没生效；且 ES 分页总数会与可见条目对不上。
- **前台给管理员做例外**：见决策 4 的缓存理由。
- **登录期直接拒绝 guest**：无存量数据，收益小于改登录/刷新链路的成本。

## 代价 / 不做什么

- **ES 需要重建索引**：mapping 加/删字段只能重建；重建期间搜索短暂返回空。若要不停机，需要双索引 + 别名切换，本次不做。
- **新表不会被容器自举创建**：entrypoint 只在 `runtime_config_models` 缺失时跑 `-db`（[entrypoint.sh](../../deploy/docker/api/entrypoint.sh)），部署新版本时必须手动执行一次 `-db`，否则热门置顶接口会报表不存在。
- **显示开关关闭后旧链接会 404**：收藏、历史、分享出去的文章链接都一样；不做"该文章已隐藏"的落地页。
- **guest 仍能拿到令牌，但令牌不被承认**：登录接口不拦，鉴权入口把 `role ∉ {1,2}` 当未登录，于是 guest 连 `/users/detail`、`/logout`、`/sessions` 这些"自己的"接口也会 401，个人页与历史内容一律看不到。前端此时仍认为"已登录"（读者站 store 只有 `isLoggedIn = !!token`，且 `/users/detail` 不返回角色），所以只会在写操作上看到 401 提示；要彻底对齐需要给 user detail 契约补 role/可写标记，本次不做。
- **不做**：热门置顶的缓存层、访问计数、到期自动下架；"显示"开关的按分类/标签细化；站点模式的历史数据清理脚本。
- **不做**：把"非管理员文章"的隐藏做成可回滚的软标记——它只是读过滤，开关一开就恢复。

## 当前生效约束

1. `site` 配置段不得再出现 `mode`；新增的发布/展示开关必须放在 `site.article` 段、`bool`、默认 `true`，且同时带 `yaml` 与 `json` tag（`TestAllSections_都有jsonTag` 会拦）。
2. 发布/草稿权限判定必须在 app 层（`internal/article/app/create.go`、`update.go`）实现，并配 `domain/rule.go` 的纯规则 + `rule_test.go` 用例；不得只靠路由中间件或前端隐藏。
3. "显示"开关的过滤必须在服务端（ES DSL / MySQL 查询），禁止前端过滤；公开读面（type 1/2/3 + 详情 + 收藏内容 + 历史）一致，后台（type 5）与自己（type 4）不过滤。
4. 热门置顶的唯一真相是 `article_hot_top_models`；不得写入 `user_top_article_models`，不得写进 ES，也不得依赖任何 ES 字段表示置顶状态。
5. 作者置顶的规则不变：只有作者本人能置顶自己的文章，且只影响个人页；它与热门置顶是两套数据。
6. 首页热门置顶区块在无数据、加载中、失败时都不渲染（含标题与空态）。
7. 搜索 ES 文档保留 `author_is_admin`，删除 `admin_top`/`author_top`/`top`；改这几个字段必须同步更新 `search/app/testdata/dsl/` 的 golden。

## 相关代码与文档

- 配置：[backend/conf/site/enter.go](../../backend/conf/site/enter.go)、[site_default_settings.yaml](../../backend/config/site_default_settings.yaml)、[platform/config/merge.go](../../backend/internal/platform/config/merge.go)、[SettingsSite.vue](../../frontend_admin/app/components/settings/SettingsSite.vue)
- 发布权限：[article/app/create.go](../../backend/internal/article/app/create.go)、[article/app/update.go](../../backend/internal/article/app/update.go)、[article/domain/rule.go](../../backend/internal/article/domain/rule.go)、[article_wiring.go](../../backend/app/article_wiring.go)
- 可见性：[search/app/search.go](../../backend/internal/search/app/search.go)、[articlesync/article_sync.go](../../backend/internal/platform/es/articlesync/article_sync.go)、[articlesync/mapping/article_mapping.json](../../backend/internal/platform/es/articlesync/mapping/article_mapping.json)、[article/app/query.go](../../backend/internal/article/app/query.go)
- 热门置顶：[schema.sql](../../backend/internal/platform/mysql/ddl/schema.sql)（新表 DDL）、[article/adapter/http/router.go](../../backend/internal/article/adapter/http/router.go)、[article/domain/top](../../backend/internal/article/domain/top)、[index.vue](../../frontend/app/pages/index.vue)、[HomePinnedSection.vue](../../frontend/app/components/home/HomePinnedSection.vue)、[frontend_admin 文章管理](../../frontend_admin/app/pages/articles/index.vue)
- 部署：[README §八](../../README.md)、[entrypoint.sh](../../deploy/docker/api/entrypoint.sh)
- 术语：[GLOSSARY · 热门置顶 / 作者置顶](../../GLOSSARY.md)
