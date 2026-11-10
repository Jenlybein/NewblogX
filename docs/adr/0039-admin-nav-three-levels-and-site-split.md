# ADR-0039：后台菜单引入三级；「站点」拆四页、「媒体运营」拆两页

- 状态：已交付
- 日期：2026-11-10
- 来源：[#336](https://github.com/Jenlybein/NewblogX/issues/336)（与 ADR-0038 同一票）

## 背景与问题

「站点配置 → 站点」是一个 306 行、8 张卡、32 个控件的长页：基础信息（含 SEO）、登录策略（11 个字段）、发布与展示、内容审核、投稿配额、质量闸门、机审词表、私信全塞在一张表单里，一个「保存站点配置」按钮管全部。管理员改一个备案号要滚完整页；页首说明还写着"下面六组设置"（实际 8 张卡），已经和内容对不上。

「媒体运营」同样是一页两张互不相干的表：轮播列表与图片资源库共用一个页面、一次加载、一套分页翻页按钮。

两件事都不是"再加一个开关"，而是**编辑面本身的结构**：

1. 「站点」这一页里的各组字段属于**同一个配置段**（后端 `site` 段、一对整段端点）。随便拆成几个组件各自读写整段，会互相把对方刚保存的值覆盖回去——现有代码注释里已经踩过这个坑。
2. 拆页之后，`about` / `project` / `reconcile` 三块**后台没有 UI**，今天靠"整段读 → 整段写"原样往返。任何只提交自己那几块的保存方式都会把它们清成零值。
3. 「媒体运营」拆成两页后，旧地址 `/media` 不能直接 404。

## 决策

1. **侧栏支持三级**。「站点配置（纯分组）→ 站点（纯分组）→ 站点基础 / 登录与发码 / 发布与审核 / 私信」；其余七个编辑域与「右栏卡片」仍直接挂「站点配置」。
2. **分组 key 改成层级链**（`group:站点配置 / 站点`），不再只取标签名：两层同名前缀共用一份展开态会在收起一层时塌掉另一层。
3. **补 depth=2 的缩进样式**：只给第三层补 `padding-left`，不重做侧栏结构。
4. **每页仍是一个薄壳 + 一个配置组件**，组件按域拆分：`SettingsSiteBasics` / `SettingsSiteLogin` / `SettingsSitePublish` / `SettingsSiteChat`；页面只 import 自己那一个组件（既有结构对账门禁继续生效）。
5. **保存走「读-改-写」**：取最新整段 → 只覆盖本页拥有的顶层块 → 写回。合并逻辑抽成纯函数 `applySiteSectionPatch`，四页共用：
   - 站点基础拥有 `site_info` / `seo`；
   - 登录与发码拥有 `login`；
   - 发布与审核拥有 `article` / `comment`；
   - 私信拥有 `chat`。
6. **路由**：站点基础复用 `/site`（旧书签不失效），其余三页是 `/site/login`、`/site/publish`、`/site/chat`。
7. **媒体运营**变成纯分组：轮播运营 `/media/banners`、图片资源库 `/media/images`；两页各自只拉自己的分页与加载状态；`BannerFormModal` 只归轮播页。`/media` 保留为跳转页（`navigateTo('/media/banners')`），并登记进"不进菜单但确实存在"的跳转页白名单。
8. **零后端变更**：不新增端点、不改 `site` 段形状；四页仍用同一对整段端点。

## 理由与已否决方案

- **否决「保持两级、把站点拆出的四页提平挂到站点配置下」**：分组会变成 11 个同级子项，管理员再也看不出"哪几项是站点段、哪几项是别的段"；用户也明确要求"再拆子菜单"。
- **否决「把 `site` 段在后端拆成多个段」**：九段注册表（ADR-0029）、契约、默认值与历史数据都要跟着迁移，代价远大于收益；而且 `about` / `project` / `reconcile` 本来就没有编辑页，拆段后仍然要"原样往返"。
- **否决「后端加按字段合并写的端点」**：会让 `site` 段从一个具名整段退化成"部分字段写"，与 ADR-0029 的方向相反，还要为每个字段定义可空语义。
- **否决「每页各自加载整段、改自己那片、整段回写」**：拆页后两个页面同时开着必然互相覆盖；更糟的是它保留了"无 UI 块被清空"的窗口。
- **否决「保留 `/media` 作为轮播运营的地址」**：两页地址不对称（`/media` 与 `/media/images`），后人会以为 `/media` 是"媒体总页"；统一用 `/media/banners` 后语义一致，旧地址由跳转页承接。

## 代价 / 不做什么

- **每次保存多一次 GET**：读-改-写不是免费操作；换来的是拆页后不会互相覆盖，且三个无 UI 块原样往返。
- **站点页仍有长页特征**：「发布与审核」一页仍有 5 张卡（发布、审核、配额、闸门、词表），这是有意的——它们同属"发布治理"一件事，再拆一次会让"闸门依赖 AI、配额依赖投稿"的关联更难解释。
- **三级菜单只有「站点」这一处**：没有为所有菜单铺开三级，也没有做菜单项级权限门控（`AdminNavNode` 仍无权限字段）。
- **不做**：英文/多语言文案（后台没有 i18n）、菜单动画、`about` / `project` / `reconcile` 的编辑 UI（仍无 UI）。
- **旧地址 `/media` 是一次客户端跳转**，不是 301：后台是 SPA，服务端没有对应路由入口。

## 当前生效约束

1. 每页只能 import **一个** `components/settings/*` 组件；页面里不得出现 tab 容器或活动态内存变量（既有门禁会红）。
2. 站点四页的保存必须调用 `applySiteSectionPatch`，且 `OWNED_BLOCKS` 与 `utils/siteSettings.ts` 里该域的声明一致；新增/调整归属要同时改两处与对账测试。
3. `navGroupKey` 的入参是**层级链**；新增分组不得退回只传标签名。
4. 不进菜单但存在的页面只能登记在 `ADMIN_REDIRECT_PAGES`，且只允许是跳转页。
5. 媒体两页各自持有自己的加载与分页状态，不得共享同一份 list 数据。

## 相关代码与文档

- 菜单与导航：[`menu.ts`](../../frontend_admin/app/utils/menu.ts) · [`siteSettings.ts`](../../frontend_admin/app/utils/siteSettings.ts) · [`siteSectionSave.ts`](../../frontend_admin/app/utils/siteSectionSave.ts)
- 页面与组件：[`app/pages/site/`](../../frontend_admin/app/pages/site/) · [`app/components/settings/`](../../frontend_admin/app/components/settings/) · [`app/pages/media/`](../../frontend_admin/app/pages/media/)
- 样式：[`admin.css`](../../frontend_admin/app/assets/css/admin.css)（`data-depth='2'`）
- 测试：[`site-settings.test.ts`](../../frontend_admin/test/unit/site-settings.test.ts) · [`menu.test.ts`](../../frontend_admin/test/unit/menu.test.ts)
