# 2026-11-04 阅读页互动、消息中心与历史记录：决策清单

- 触发：读者反馈「点赞后刷新失效、30 秒后计数才 +1、按钮不显示已赞」；裁剪弹窗铺满窗口；子评论 UI 与点赞；消息中心/历史记录要独立成页
- 结论状态：设计树已走完（4 轮），本文件是唯一决策来源；实现按票拆开
- 相关 ADR：[ADR-0019](../adr/0019-reader-rendering-and-cache-strategy.md)（渲染边界与两层缓存）、ADR-0022（私信双通道）；本批新增 **ADR-0024 阅读页用户态与计数真相**

## 0. 事实基线（勘察结论，全部有代码证据）

### 点赞/收藏链路

- 点赞态 `is_digg` **不进 SSR payload**：`useArticlePageSources.ts:105-106` 主动剔除私有字段（`utils/readerPrivateFields.ts:10` = `is_digg`/`is_favor`/`relation`）
- hydration 把这份**匿名 payload 预热进 30 秒客户端 SWR**（`useReaderSwrAsyncData.ts:93-97`）
- `useReaderSwrAsyncData.ts:112-116` 对**任何** cause 的 `refresh()` 在 fresh 时一律短路 → ADR-0019 承诺的「客户端补齐用户态」在刷新/硬刷新时**根本不会发生**（现有测试 `useReaderSwrAsyncData.test.ts:79-96` 把这个行为钉死了）
- 计数真相 = 库内快照 + Redis 增量（`app/query.go:153-163` `enrichCounters`），读时已叠加；写路径 `PUT /api/articles/{id}/digg` 是 **toggle**（`app/interaction.go:122-134`）
- **危险点**：按钮显示「未赞」时再点一次 → 发 toggle → 后端发现已存在 → **Remove**，把真实点赞取消
- 数字滞后来自两层 30s：Nitro 匿名 SSR HTML 缓存（`server/utils/ssrAnonymousCache.ts:25`）+ 客户端 SWR
- 收藏同一计数链路，但没有 toggle（已收藏再 POST 会撞 `ErrItemExists`）；`FavoriteFolderModal.vue:71-84` 在「已收藏→移除」分支仍发 POST（既有缺陷）
- 分享是纯前端 clipboard，无接口无计数

### 阅读页版式

- 三栏：左 `ArticleOutlineAside`（216px，仅当文章有归属树）／中 `.article-main`／右 `.article-aside`（320px）；网格在 `tailwind.css:630-638`，`lg`=1024px
- 右栏顺序：作者卡（≈340px）→ 目录卡（≈490px，内部 `max-h-[380px]` 滚动）；`.article-aside` 在 ≥1024 是 `sticky; top:88px` → **视口低于约 938px 就裁掉底部**
- hero 里 `.article-actions` = 质量评分 + 点赞 + 收藏 + 分享；`.article-meta` 行尾 = 编辑；正文底部无互动条
- 全仓没有 `vh/dvh`、没有 `@media (max-height)` 断点

### 评论区

- 子评论点赞渲染成纯文本 `<span>点赞 N</span>`（`CommentThread.vue:90`），无 `@click`/`emit`/`disabled`；**后端完全支持二级点赞**（`comment/adapter/http/handler.go:123-138` 只按 id 找评论；`comment_digg_models` 无层级约束）
- 点完只 `commentsPager.refreshCurrentPage()`（`article/[id].vue:251`），`replyStates` 从不重取 → 子评论计数不更新
- 回复按钮传的是**根评论 id**（`CommentThread.vue:91`）→ 回复子评论会落到根楼层
- `handleReply` 只写 `replyTarget`，不聚焦输入框、不滚动（`article/[id].vue:324-331`）
- 互动无登录前置：游客点击 → 打接口 → 「请先登录」→ `markPossiblyExpired()` 把 `isLoggedIn` 置假（`services/http/client.ts:170-172`）
- 「已登录，可直接评论」在 `CommentComposer.vue:42-44`；根评论每页 7、回复每页 3

### 裁剪弹窗

- `ImageCropDialog` 的 `NModal` 只有 `preset="card"`，**无 style/class**；naive-ui `NCard` 自带 `width:100%`、`.n-modal` 无宽度 → 被最长中文提示行撑到接近窗口宽
- 取景框 `width:100%` + 内联 `aspect-ratio` → 高度按宽度派生（头像 1:1 时高≈宽）→ 观感铺满
- 读者站与后台两份组件**逐字节相同**，有同源门禁（`test/utils/image-shaping-parity.test.ts`），必须同时改

### 消息中心

- 现状：唯一页面 `pages/studio/inbox.vue`，`tab=site|chat` + `group=1|2|3|global` 走 query；侧栏两条菜单（消息中心 / 私信）各带 badge
- 后端三源：站内信 `article_message_models`（`/api/sitemsg`，分组 1/2/3=评论与回复、5/7=点赞与收藏、9=系统）、全局通知 `global_notif_models`+`user_global_notif_models`、私信 chat 三表
- **统一未读汇总已存在**：`GET /api/sitemsg/user` 返回 5 个未读数，前端 `totalUnread` 相加 → 不需要新接口
- 顶栏 `AppHeader.vue` 目前**没有**任何消息入口；hover 下拉有先例（`NPopover trigger="hover"`）
- 加两个入口后动作区约 294px，360px 屏内容宽约 328px → **会挤压溢出**，必须补窄屏规则

### 历史记录

- 现状：`pages/studio/history.vue`（studio 布局），16:9 卡栅格、每页 20、「上一页/下一页」+`has_more`；无分组/搜索/批量
- 表 `user_view_article_history_models` 只有 `article_id/user_id/时间戳`，`UNIQUE(article_id,user_id)` + upsert → **去重已是现状**；浏览时刻 = `updated_at`
- 接口 `GET /api/articles/history`（仅 `page/limit`）、`DELETE /api/articles/history`（`id_list` 必填，无清空）
- 卡片需要的封面/标题/时间**现有接口都有**；作者/分类/阅读时长拿不到，但也不需要
- 缺口：①无 `keyword` ②offset 分页撑不起按天分组（要游标）③无清空接口 ④无 90 天清理任务

## 1. 点赞与收藏：用户态与计数真相（ADR-0024）

1. **SWR 语义收窄**：`useReaderSwrAsyncData` 只在 `cause === 'initial'` 时允许 fresh 命中；手工 `refresh()` 必须真发请求。同步改写钉住旧行为的测试。
2. **用户私有态独立取数**：新增 `article-private-<id>` 取数（`server: false`、`getCachedData: () => undefined`），只取 `is_digg/is_favor/relation`，登录后必取；永不进 payload、永不进 SWR。ADR-0019 约束 2（SSR 只出匿名内容）保持不变。
3. **点赞改幂等目标态**：`PUT /articles/{id}/digg` 带目标态 `{liked: boolean}`，服务端按目标收敛，重复同态即无操作。契约加字段（兼容性小改）。
4. **写响应回填 + 主动失效**：响应返回最新 `digg_count`/`favor_count`，前端直接 patch 并 `readerSwrCache.delete('article-<id>')`，不等 TTL。
5. **共享 SSR HTML 缓存不加大范围 purge**：接受「别人最多滞后 30 秒」；多实例进程内 Map 无法跨进程失效，不做假能力。
6. **收藏附带修复**：`FavoriteFolderModal` 的「已收藏→移除」分支改走 DELETE；收藏计数同样走回填。
7. **门禁**：「hydration 后必须发出一次带鉴权的用户态请求」——源码门禁 + composable 行为用例。
8. **ADR-0024** 记录以上取舍，ADR-0019 追加「后续修订」指向它。

## 2. 阅读页版式

1. **hero 精简**：只留「质量评分」+「编辑」（编辑紧贴质量评分右侧，仍受 `articleEdit` 门控）。
2. **四个互动按钮移到右栏**：点赞、收藏、分享、评论，插在「作者简介」与「目录」之间；顺序固定为 作者简介 → 互动条 → 目录。
3. **窄屏兜底**：<1024px 右栏消失时，互动条降级为**底部固定条**（四个图标 + 数字），避免窄屏无入口；不在正文底部另放一份。
4. **右栏高度响应式**：`.article-aside` 加 `max-height: calc(100dvh - 88px); overflow-y: auto`；目录体 `max-h` 改 `clamp(160px, 32dvh, 380px)`；新增 `@media (max-height: 820px)` 压作者卡 padding 并折叠 stats。作者简介限高两行截断。
5. 全部写在 `tailwind.css`；页面不得加 `<style>`（有门禁断言）。

## 3. 评论区

1. **子评论点赞补按钮**（后端已支持），并让 `replyStates` 在点赞后局部重取。
2. **回复目标精确到被回复的那条评论**（不再传根 id）。
3. **点「回复」后滚动到评论框并聚焦**（暴露 textarea ref）。
4. **点赞/回复改图标版**：点赞用实心/空心两态、回复用箭头图标，均带 `aria-label`。
5. **「展开 N 条回复」改成胶囊按钮**（浅底 + 主题色文字 + chevron）。
6. **删除「已登录，可直接评论」**提示（未登录仍显示「登录后可参与讨论」）。
7. **互动统一登录前置**：点赞/收藏/分享/关注/评论提交，未登录时先弹登录框、**不打接口**（根治"点一下把自己登录态打成失效"）。

## 4. 裁剪弹窗

- `NModal` 显式 `width: min(92vw, 560px)`（桌面 720px 档可选）与 `max-height: calc(100dvh - 48px)`；取景框改为**由高度决定宽**（`height: min(46dvh, 360px)` + aspect-ratio 反推）；内容区内部滚动；底部操作区固定。
- 移动端保留 4vw 边距。两端两份组件必须同步改（同源门禁）。

## 5. 消息中心（顶级页）

1. 路由：`/messages` 顶级页 + `auth`；**五个子项**：评论与回复 / 点赞与收藏 / 系统通知 / 全局通知 / 私信 → `/messages/{comments,digg,system,global,chat}`；旧 `/studio/inbox` 重定向。
2. 布局：左子项导航 + 右内容；<1024px 左导航变**顶部横向 tab**（带未读小胶囊）。
3. 未读表达：左导航选中态 = 抬升面底 + 左侧 2px 强调条 + 字重；未读数为 0 时**不显示数字**。列表行未读 = 左侧强调条 + 标题加粗 + 右上小圆点（不用 chip）。
4. 列表行内容：文章标题 + 一行内容摘要（评论/子评论正文截断）+ 时间；删除按钮 hover 才出现。
5. 批量动作：**全部已读** + **清空当前分组**（二次确认）+ 单条删除；不做多选。
6. 加载：滚动加载更多（每页 20，"没有更多了"收尾）。
7. 私信子项：进入后是会话列表（未读在会话行）+ 聊天窗；宽屏双栏、窄屏二选一；复用 `InboxChatPanel`。
8. 未读来源：复用 `GET /api/sitemsg/user`；顶栏入口订阅 `useMessageStore`，并把刷新点从「仅 studio 布局」扩大到「登录取后 + WS 事件 + 消息中心访问」。
9. 顶栏入口：`消息中心`（hover 展开五子项，子项右侧显示未读数）与 `历史`（普通链接，hover 无效果）。窄屏规则：<1280 历史收进头像菜单；<1024 消息中心仅图标 + 红点；<640 两者进折叠面板。

## 6. 历史记录（独立页 `/history`）

1. 路由：`/history` 顶级页 + `auth`；个人中心移除入口；旧 `/studio/history` 重定向。
2. 版式照参考图：左侧时间轴（竖线 + 圆点 + 分组标签「今天 / 昨天 / 近一周 / 更早」）+ 右侧卡片栅格（4–5 列自适应）。
3. 卡片**只显示封面、标题（两行截断）、浏览时间**，不要分类/标签/作者/时长角标；标题行右侧垃圾桶（单条删除）。
4. 顶部操作：搜索标题（服务端 keyword）+ 清空历史（二次确认）+ 批量管理（多选态：勾选框 + 已选 N 条 + 删除 + 取消）。
5. 后端缺口：①`keyword` ②**游标分页**（`before_updated_at + before_id`，替代 offset，保证分组不被页切断）③清空接口 ④90 天清理任务（cron 每日，软删 `updated_at < now-90d`）。
6. 去重沿用现有 `UNIQUE(article_id,user_id)` upsert（同一篇只留最近一次）。
7. 视觉：结构照图，色值全部映射现有 token（`--color-surface-raised` / `--color-line-faint` / `--color-line` / `--color-ink`），不引入新色相。

## 7. 陌生人私信（并入本批，原 #228）

- 语义：**站点总闸 AND 接收方个人开关**（后者在账号设置隐私区，默认开启）。
- 端口：`StrangerPolicy.StrangerEnabled(ctx, receiverID)`；`app/chat_wiring.go` 两级取值；`chat/app/service.go` 判定传接收方。
- 个人中心范围：账号设置 + 内容管理（我的文章/收藏）；不再有任何消息或历史入口。

## 8. 正文 markdown 色板（放松五族锁定，仅限正文渲染区）

用户决定：**正文 markdown 渲染可以特殊，不受 ADR-0012 五族色相锁定**，H4–H6 也要特殊样式与不同颜色。选定「方向 A：暖调编辑」。

| 角色 | 色值 | 装置 |
| --- | --- | --- |
| H1 | `#1A1815` 墨 | 定标线 + 居中记号块 |
| H2 | `#B2442A` 赤陶 | 左侧 3px 色带 |
| H3 | `#4E6B3C` 苔绿 | 行首三角 |
| H4 | `#35597D` 靛青 | 短下划线（约 40% 宽） |
| H5 | `#6B4A78` 梅紫 | 上下双发丝线 |
| H6 | `#8A6524` 赭黄 | 大写字距 + 顶横线 |
| 加粗 | `#0F0D0B` 深墨 + 字重 650 | — |
| 斜体 | `#5F5A52` 暖灰 + 真斜体 | — |
| 删除线 | 线 `#B2442A`、文字 `#6E6961` | 线宽 1.5px |
| 下划线 | 线 `#35597D`（靛青）、文字继承 | `text-underline-offset: 3px`，悬停加粗 |
| 有序列表数字 | `#B2442A` | — |
| 无序列表圆点 | `#4E6B3C` | — |
| 代码 | 关键字 `#B2442A`、字符串 `#4E6B3C`、数字 `#35597D`、函数 `#6B4A78`、类型 `#8A6524`、注释 `#7A756C`、变量 `#2A2724` | 代码块底色比正文纸面降一档；正文对比度 ≥ 4.5:1 |

治理：色值以 `--md-*` 语义 token 落在 `tokens.css`（只供 markdown 渲染区），ADR-0012 追加「后续修订」说明正文区不锁五族、色板单独治理；裸色值门禁继续生效，组件里只能用 token。下划线与「重点上色」（黄底）分离：`<u>` / `++文字++` 只用真下划线，不再加底色。

## 9. 渲染边界

- `/messages/**` 与 `/history` **都 `ssr: false`**（私人数据、不索引），显式写入渲染边界表 `frontend/shared/readerRenderingBoundary.ts`，并与 `nuxt.config.ts` 的 `routeRules`、门禁同步。

## 10. 票务

一张 spec（本文件所指）+ 11 张子票：

1. 点赞/收藏真相（ADR-0024 + 5 处改动 + 门禁）
2. 右栏互动条重排（hero 精简 + 右栏四按钮 + 窄屏底部条）
3. 右栏高度响应式与作者简介压缩
4. 评论区改造（子评论点赞/回复目标/焦点/图标化/胶囊/删提示/登录前置）
5. 消息中心顶级页骨架（路由 + 五子项 + 未读 + 顶栏 hover + 移动端 tab）
6. 私信并入消息中心 + 旧路径重定向 + 未读订阅与 WS 生命周期
7. 历史记录独立页（前端版式 + 后端 keyword/游标/清空/90 天清理）
8. 裁剪弹窗尺寸（两端同源）
9. 陌生人私信用户级开关（后端两级 + 账号设置 UI）
10. **正文 markdown 色板与装置**（`--md-*` token、六层标题装置、加粗/斜体/删除线/下划线、列表标记、代码高亮，ADR-0012 修订）
11. 文档与门禁收口（ADR-0024、ADR-0012 修订、GLOSSARY、README、发布说明、渲染边界与门禁）

## 9. 已知取舍

- 别人看到的点赞数可能滞后 ≤30 秒（共享 SSR HTML 缓存），不为此引入跨进程失效。
- 消息中心「清空当前分组」是不可逆操作，前端二次确认；后端按分组软删。
- 历史页 90 天清理会真实删除用户历史（软删），属数据策略变更，需在发布说明写明。
- 顶栏两个新入口在 360px 屏必须靠折叠规则，否则挤压品牌区。
