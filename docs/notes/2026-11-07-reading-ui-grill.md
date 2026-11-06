# 2026-11-07 阅读页/评论区/顶栏/合集卡片 grill 记录（已交付）

> 本文是 `/grill-with-docs` 会话的**决策记录**；spec、子票、实现与验收见「§九 交付记录」。
> 代码与本文冲突时，以本文件的决策 + [ADR-0012 后续修订](../adr/0012-neutral-glass-visual-system.md) 为准。
> 状态：已交付（2026-11-07），六项需求全部落地。

---

## 一、需求清单（用户原始表述，共 6 项）

### 首批 5 项

1. 文章阅读页评论区的**子评论展开要有动效**。
2. 文章阅读页评论区的**头像和昵称点击进个人主页**。
3. 顶栏右侧「创作 / 消息 / 历史」三枚按钮**压窄宽度**，仍保持「图标 + 文字」，只压缩宽度。
4. 改**阅读页目录的缩进与箭头样式**，参考给出的两段组件代码。
5. **合集和项目的卡片改文件夹样式**，里面的文件变成合集/项目的封面。

### 追加 1 项（第二轮问答中提出）

6. **消息中心页顶栏**（评论与回复 / 点赞与收藏 / 系统通知 / 全局通知 / 私信）参考 gooey 分段条。

### 用户带来的三段参考代码

| 参考 | 落点 | 关键机制 |
| --- | --- | --- |
| `RailToc`（飞机沿轨道走） | **右栏标题大纲** | SVG 路径测长 + 按 y 二分求长度 + 两层 mask 挖洞 + 弹簧 travel/turn + 点击 pinned |
| `BounceSidebar`（圆点弹跳） | **左边缘抽屉的合集/项目目录树** | 逐项 `offsetTop` 测量 + 弹簧/`easeOut` 位移 + `arc()` 弧线路径 |
| `GooeyNav`（颈部形变） | **消息中心页 tab 条** | 逐段 `margin-left` 弹簧 + 接缝 SVG 渐变 + `borderRadius` 弹簧 + `usePathname` 派生激活 |

用户原话把"缩进"和"箭头"当成一件事，实际落在**两个不同组件**上：缩进/圆点在目录树，箭头（飞机）在标题大纲。本记录按这个拆分执行。

---

## 二、事实底座（带证据，均已读代码确认）

### 2.1 评论区

- `frontend/app/components/comment/CommentThread.vue:99` 用 `v-if="expanded[comment.id] && comment.replies?.length"` 直接插入回复块，**没有高度动效**。
- 回复是**点开才请求**：`CommentThread.vue:36-41` `toggleReplies()` 先翻 `expanded` 再 `emit("loadReplies")`；取数在 `frontend/app/composables/useArticleComments.ts:143-182`（`replyPageSize` 默认 **3**，见 `:89`）。因此"展开瞬间列表是空的"，动效必须覆盖「骨架 → 条目」这一段。
- 样式在 `frontend/app/assets/css/tailwind.css`：胶囊 `.comment-capsule` `:1055-1075`（chevron 翻转已在），回复容器 `.comment__replies` `:1077-1079`（`mt-4 + border-t + pt-4`），分页 `.comment__pagination` `:1081-1087`。
- 评论项字段：`CommentRootItem` / `CommentReplyItem` 带 `user_id` / `user_avatar` / `user_nickname`；`AppAvatar` 是 Naive `NAvatar`（`frontend/app/components/common/AppAvatar.vue`），**本身不是链接**。
- 现成范式：`frontend/app/components/article/ArticleFeedItem.vue:90-101` —— `NuxtLink :to="/users/{id}"` 包「头像 + 昵称」，无 id 时退化成静态 `span`。

### 2.2 个人主页路由

- `/users/:id` 已存在：`frontend/app/pages/users/[id].vue`；站内已有 5 处入口（`ArticleFeedItem`、`ProfileRelationList`、`StudioSidebarNav`、`useHeaderUserMenu`、fans/follows 页）。**本轮不新增路由。**

### 2.3 顶栏三按钮

- `frontend/app/components/layout/AppHeader.vue:495-520`：
  - `.site-action`：`min-width: 2.75rem`、`min-height: 2.75rem`、`padding: 0 0.5rem`、`gap: 0.375rem`、`font-size: 0.8125rem`。
  - `.site-action--equal`：`min-width: 5.5rem`（88px），注释写明"三枚动作按钮等宽"是**既有不变量**。
- 内容实宽 ≈ 图标 18 + gap 6 + 两个汉字 ≈26 + padding 16 ≈ **66px**，5.5rem 里有约 22px 冗余。
- 出现档位：`创作` 从 640px 起显文字（`:712-714`），`消息` ≥1024 显文字（`:736-738`），`历史` ≥1024 常显（`:741-743`）；<640 三者都只留图标，但**宽度仍受 `.site-action--equal` 的 5.5rem 支配**（即窄屏也是 88px 的空盒子）。

### 2.4 阅读页的两个「目录」

| 组件 | 位置 | 现状 |
| --- | --- | --- |
| `frontend/app/components/article/ArticleTocAnchor.vue` | 右栏 `article-paper` 卡（`pages/article/[id].vue:657`） | naive `NAnchor`（`:120`，`bound=96`、`:show-rail="false"`）+ 自带进度条与百分比（`:108-117`）；<1024 整块不渲染 |
| `frontend/app/components/article/ArticleOutlineAside.vue` → `OutlineTree.vue` | 贴左边缘的抽屉（`pages/article/[id].vue:672`） | 目录树：`▸`/`▾` 折叠（`OutlineTree.vue:69`）、每层 1.5rem 缩进（`:98-101`）、每层一条发丝引导线（`:103-111`）、文章行带 5rem 封面缩略图（`:76-78`、`:160-166`） |

- `OutlineTree.vue` 被**三处**共用：`/collections/[id]`、`/projects/[id]`、阅读页抽屉（`ArticleOutlinePanel.vue:44`）。
- 树的"当前文章"高亮来自 `markCurrentOutlineRows`（`frontend/app/utils/outlineState.ts`），一页最多一个；折叠状态由 `useOutlineCollapsed` 持久化。
- `ArticleOutlinePanel` 的 `:key="panelKey"`（`ArticleOutlinePanel.vue:33`）是 `kind:owner_id` —— **同树内换文章不重挂载**，所以圆点可以在同一次页面会话里"动"；跨树才重建。
- 结构提示：抽屉面板是 `transform: translateX(-101%)` + `visibility: hidden`（`ArticleOutlineAside.vue:193-215`），收起态下 `offsetTop` 仍然可测，但**首帧测量要等字体就绪**（参考组件自己也 `document.fonts.ready.then(snap)`）。

### 2.5 合集/项目卡片

- 读者侧两页各自实现一份**同形**卡片：`pages/collections/index.vue`（`.collection-grid` / `.collection-card`）与 `pages/projects/index.vue:115-201`（`.project-grid` / `.project-card`，文件头注释明写"复用合集卡片的视觉语言"）。
- 网格：`repeat(auto-fill, minmax(min(100%, 20rem), 1fr))` + `gap: 1.25rem`；卡片 = 16:9 封面 + 标题 + 两行描述 + 篇数。
- 契约：`CollectionCardResponse{ID,Title,Description,Cover,ArticleCount,CreatedAt}`（`backend/internal/article/adapter/http/`），**只有单张 Cover**，且后端已把空封面回退到"目录树 DFS 第一篇公开文章封面"。
- 卡片数据形状：`utils/collection.ts:55-69` `collectionCardView()`、`utils/project.ts` `projectCardView()`。
- 封面组件：`components/article/ArticleCover.vue` —— 固定 16:9、`object-fit: cover`、失败/空时渲染透明占位。

### 2.6 消息中心 tab 条

- `frontend/app/components/inbox/InboxSubNav.vue`（34 行）：`NuxtLink` × 5 + `shouldShowUnreadCount` 角标，激活态靠 `is-active` 类。
- 样式在 `tailwind.css`：`.message-center__subnav`（`gap-1` + 边框 + `p-1` + `self-stretch` + `overflow-x: auto`）、`.message-center-tab`（`rounded-control px-4 py-2`、`shrink-0`）、`.message-center-tab.is-active`（抬升面 + **`inset 0 -2px 0` 墨色下划线** + 600 字重）。
- 五个子项与顺序的唯一来源：`utils/messageCenter.ts:19-36`（`comments/digg/system/global/chat`，标签「评论与回复 / 点赞与收藏 / 系统通知 / 全局通知 / 私信」）。顶栏 hover 面板消费同一份数据。

### 2.7 动效与材质的硬约束（本轮最大的冲突源）

- `frontend/app/utils/motion.ts:17-28`：`MOTION_DURATIONS_MS = {fast:120, base:200, slow:320}`、`MOTION_EASE = [0.2,0.8,0.2,1]`、`MOTION_STAGGER_MS = 40`；`tokens.css` 有对应变量，`frontend/test/design/motion-foundation.test.ts:75-110` 逐条锁定。
- ADR-0012 **约束 9**："动效三档 + 单一缓动 + stagger 40ms；只动 `transform`/`opacity`；强制尊重 `prefers-reduced-motion`"。
- ADR-0012 **约束 6**："液态玻璃只用于导航/控制层；正文、卡片、列表等内容面一律不透明；**同屏可见玻璃层 ≤3**"。
- ADR-0012 **约束 7**：玻璃必须带 `@supports not` 与 `prefers-reduced-transparency` 的不透明降级；移动端 `blur ≤12px`。
- 门禁事实：
  - **没有任何门禁禁止弹簧**（`frontend/test/utils/motion-tier-c.test.ts` 只锁纯函数行为，`motion-foundation.test.ts` 只锁 token 数值）。
  - `frontend/test/design/reduced-motion.test.ts:26-37` **只扫 CSS 的 `@keyframes` / `animation:`** —— JS 弹簧完全在门禁视野外。
  - `scripts/check-design-tokens.mjs` 扫描 `.vue`/`.css`/`.ts`，**裸 hex 与 `rgb()/rgba()/hsl()` 一律红**；豁免只有 `tokens.css`、`assets/brand/`、`assets/og/`。
- 依赖事实：`motion-v@2.9.0` 只转出 `framer-motion/dom`；参考代码里的 `arc()` 来自 **`motion` 包（本仓库未安装）**。`motion-v` 提供 `motion` 组件、`AnimatePresence`、`useMotionValue/useSpring/useTransform/useAnimate`（`framer-motion/dom` 侧）。全局 `MotionConfig reduced-motion="user"` 已在 `app.vue`。

---

## 三、决策（逐项最终口径）

### 3.1 子评论展开动效

- **实现**：纯 CSS `grid-template-rows: 0fr → 1fr` + 透明度，**展开 200ms（`--motion-base`）+ 逐条 40ms stagger**；**收起 120ms（`--motion-fast`）、无 stagger**；**翻页（上一页/下一页）只做交叉淡入**，不做高度动画（两页条数不同会让容器高度起伏）。
- **先骨架后条目**：点开立刻展开容器并注入 3 行骨架（与 `replyPageSize` 的 3 条一致），数据到货后条目淡入，**不二次改高度**。
- `prefers-reduced-motion` 下由 token 归零自动静态，不写第二套开关。
- 不改数据流（仍然点开才请求），不改分页语义（每页 3 条）。

### 3.2 评论头像/昵称 → 个人主页

- 根评论与子评论**都做**；`user_id` 缺失时**不渲染链接**（退化静态，同 `ArticleFeedItem` 范式）。
- 语义用 `NuxtLink`（可中键/可复制/可被读屏当链接），不用 JS `navigateTo`。
- 视觉：沿用 `.comment__author` 的墨色，**不改链接色**，加 `hover` 下划线 + 全站统一焦点环 `--color-focus`。
- 不做「回复 XXX」里的昵称链接（那是关系说明，不是身份入口）。

### 3.3 顶栏三按钮压窄

- `.site-action--equal` 的 `min-width` 从 **5.5rem → 4.25rem（68px）**，`padding` 与 `gap` 不变、图标与文字都不变。
- 保留"三枚等宽由声明保证"这一不变量（不改成内容自然宽）。
- 640px 以下三枚原本也是 88px 的空盒子，压缩后一并收窄；`min-height: 2.75rem`（44px 触控目标）与焦点环不变。

### 3.4 目录树（左边缘抽屉）：沟槽圆点

- **封面退场**：`OutlineTree` **全站不再渲染封面缩略图**（抽屉与两个详情页一致），文章行变纯文字；`outlineTreeRows` 的 `cover` 字段与 `ArticleCover` 依赖一并清理。
- **缩进保持 1.5rem/层**；**层级发丝引导线保留**。
- **圆点**（沟槽圆点）**随层级横移**：`x = depth × 1.5rem − 0.75rem`，正好落在该层发丝线上、**线从圆点里穿过**。`depth 0` 没有线，圆点压在最外侧缩进起点（`x = 0`）。
- **圆点只跟随"当前文章"**；`hover`/键盘聚焦不动它。
- **移动时机**：同树内切文章（上一篇/下一篇/点树内文章）+ 文件夹折叠导致目标行位移；**首次打开抽屉直接落位、不播动画**。
- **路径**：`x` 与 `y` 同时变化时**沿位移向量画弧**（斜向鼓出），不是两段式。
- **弹簧 + 弧线的调和**：一个弹簧驱动 `0 → 1` 的进度，位置由 `useTransform` 派生（线性插值 + 正弦弧偏移），**不依赖 `motion` 包的 `arc()`**。弧强度沿用参考的量级（`min(0.8, 14/distance)`）。
- 文件夹箭头保留 `▸`/`▾`（"箭头"指的是标题大纲的飞机，不是这两个）。
- 折叠状态、`aria-current`、键盘顺序、来源链接（`?from=collection:<id>` / `project:<id>`）全部不变。

### 3.5 标题大纲（右栏）：轨道 + 飞行器

- **保留 naive `NAnchor` 作为行为层**（`bound=96` 的滚动绑定、`active` 判定、aria、键盘），只在它上面**叠一层视觉**：SVG 轨道（拐弯路径 + `2 5` 虚线）+ 节点圆点 + 飞机 + 已读填充。
- **保形降级**：轨道形状、节点、飞机朝向、`depth` 缩进（`14px`）全部保留；**不做 mask 挖洞**（改用 `stroke-dasharray` + 分段上色）；不做 `useId` 派生的 mask id。
- **进度双份表达保留**（用户明确选 B）：卡头的进度条与百分比**继续存在**，轨道的已读填充同时存在。
- 点击 pinned 行为（点完标题后飞行器不跟着滚，直到读者自己滚动）**保留**。
- `<1024` 整块目录仍不渲染（现状不变）。

### 3.6 合集/项目文件夹卡

- **形态**：背板 + **一张 16:9 封面卡** + 前盖（flap）。**不做三张扇形**，**不改契约**（`covers[]` 方案被否，仍用现成的单张 `Cover`）。
- **卡与 flap 的关系**：卡片插在背板与 flap 之间，**常驻露出上 2/3**，flap 从下盖住卡片下沿；`flap` 常态 `rotateX(-15°)`。
- **交互**：`hover`/`focus-visible` → 卡片**上抽 30px + 旋转 6°**，flap 前倾到 **-45°**；**点击直接进详情**（不做"第一次点击炸开、第二次点击进详情"）；移出即回位。
- **文字区保留**：文件夹在上、标题 + 两行描述 + 篇数在下（不把标题叠到 flap 上）。
- **尺寸由容器决定**：网格列宽**不动**（`minmax(min(100%, 20rem), 1fr)`），文件夹改成按容器宽度自适应（内部几何走比例 / `viewBox`，缩放 = 列宽/321），**不写死 321×270**。
- **配色**：镜像成**一套语义 token**（背面 / 文件夹面 / 卡片面 / 发丝线），亮暗自动切换；**不保留 black/white/blue 三色主题**（会新增色相，撞 ADR-0012 约束 1）。
- **blur 常开**：flap 的 `backdrop-filter` 按参考常开（用户明确选择，覆盖性能建议）；必须带 `@supports not` 与 `prefers-reduced-transparency` 的不透明降级，移动端 `blur ≤12px`（ADR-0012 约束 7 照旧生效）。
- **实现落点**：抽一个共用组件（两页共用），删掉 `collections/index.vue` 与 `projects/index.vue` 里两份重复的卡片 CSS。

### 3.7 消息中心 tab 条（gooey）

- **形态**：内容宽度的一体化分段条（不再 `self-stretch` 拉满），5 段**相邻贴合**，激活块有圆角，邻接**接缝处生成颈部**（两段凹曲线 + 线性渐变，渐变两端取左右相邻段的填充色）。
- **激活配色**：`--color-surface-inverse` 铺底 + `--color-surface` 文字（深墨面 + 反白，亮暗自动反转）；不新增"激活垫"色相。
- **未读角标**：保留 `shouldShowUnreadCount` 规则（0 不显示），在深墨块内用半透明反白底。
- **动效**：逐段 `margin-left` 弹簧（**布局属性，ADR 破例**）+ `borderRadius` 弹簧 + 颜色 `transition-colors`。
- **窄屏**：保留横向滚动（整条拖动），不换行、不截断。
- **范围**：只改页面内这条 tab 条；**顶栏 hover 面板不动**；路由、子项顺序、标签文案、`messageCenterNavItems` 数据源全部不变。

### 3.8 跨项的动效口径（用户明确选的例外）

- **三处都用弹簧**：标题大纲飞行器（travel/turn）、目录树圆点、消息中心颈部。
- **参数集中**：`utils/motion.ts` 新增 `SPRING_*` 预设（建议 `SPRING_TRAVEL{140,26,0.6}`、`SPRING_TURN{260,30}`、`SPRING_DOT{·}`、`SPRING_NECK{200,28,1}`），组件**禁止内联 `stiffness`/`damping` 字面量**；`tokens.css` 不动（CSS 表达不了弹簧）。
- **减少动态**：弹簧一律 `jump()` 到终点（不是放慢）；弧线直接落位。
- **布局属性**：只有消息中心颈部允许 `margin-left` 变化（5 个节点的小子树），其余仍只动 `transform`/`opacity`。
- 三档时长与单一缓动**继续服务 CSS 侧**（评论展开/收起、路由过渡、hover 反馈），不因弹簧退役。

### 3.9 门禁补写（与实现同批）

1. 弹簧组件在 `prefers-reduced-motion` 下必须 `jump`（静态），不得保留过渡；
2. 组件里不得出现裸 `stiffness`/`damping` 数字，只能引用 `utils/motion.ts` 的 `SPRING_*`。

两处以现有风格落 `frontend/test/design/*`（结构断言，不引 Playwright、不做截图比对）。

---

## 四、默认推定（未单列提问、按此执行；如要改请说）

1. `OutlineTree` 的两个详情页在失去封面后，**行高会变矮**；不做额外补偿，也不加"替代缩略图"。
2. 卡片原有的 `:hover { translateY(-2px) }` 被文件夹自身的 30px 上抽**取代**，不再叠加。
3. flap 上**不加**「合集 / 项目」小签（保持参考的纯几何形态，文字只在下方文字区）。
4. 文件夹里封面为空时，沿用 `ArticleCover` 的空占位（透明），落在背板底色上。
5. 骨架固定 3 行（与 `replyPageSize` 一致），不随 `reply_count` 变化。
6. 顶部三枚按钮压缩后，**所有断点下宽度都由 `.site-action--equal` 支配**（640–1023px 只显图标的消息/历史同样是 4.25rem，不是 2.75rem 方块）；`.site-action` 的 `min-height: 2.75rem` 才是那条 44px 触控目标。
7. 「沟槽圆点」的 `depth 0` 位置取 `x = 0`（最外侧缩进起点）。
8. 三处弹簧的**具体数值**可在实现时微调（只准改 `utils/motion.ts` 一处），手感以"不越界、不抖"为准。

---

## 五、风险与代价（已知，不隐藏）

1. **blur 常开的性能账**：24 张卡片同时是 `backdrop-filter` 元素 = 24 个 backdrop root。ADR-0012 原本的结论正是"内容面不透明"。缓解手段只剩约束 7 的不透明降级与移动端 12px；若线上出现滚动掉帧，**第一顺位回退方案是"仅 hover 开 blur"**（§三 3.6 的备选，已在第二轮问答里讨论过）。
2. **弹簧绕过了现有门禁**：`reduced-motion.test.ts` 只扫 CSS 动画，JS 弹簧在门禁视野外。这是 §三.3.9 要补两条断言的原因；不补的话，"档位 C"会静默退化。
3. **颈部是布局动画**：`margin-left` 每帧触发该行的重排与重绘。5 个节点范围内可接受，但它**确实**是 ADR-0012 约束 9 的例外，必须登记，否则下一个人会当 bug 修掉。
4. **弧线不能依赖 `arc()`**：`motion` 包未安装且 `motion-v` 不转出该 API；已改用"弹簧驱动进度 + `useTransform` 派生弧偏移"，代价是弧的形状由我们自己定（正弦近似），与参考的视觉会有细微差别。
5. **两处"目录"同名**：侧栏卡与抽屉都叫"目录"，本轮没有改名（不在需求内），但新增视觉后更容易混淆；若后续要拆名字，属于领域词汇层改动。
6. **`OutlineTree` 去封面是跨页面的**：合集/项目详情页会一起变成纯文字树。这是用户明确选择（B），但它改的是**三个**页面。
7. **六项一次交付**：改动面覆盖 2 个共用组件 + 1 个新分段条 + 1 个新卡片形态 + 1 个新预设模块 + 3 份文档 + 2 条门禁。一次性验收的回归面较大。
8. **不做视觉截图回归**：沿用仓库现状（ADR-0012 已否决 Playwright 比对），文件夹卡这种几何复杂的形态**只能靠人眼验收**。

---

## 六、验收清单（实现后逐条过）

### 评论

- [ ] 点「展开 N 条回复」：容器 200ms 展开、骨架先出现、条目逐条 40ms 淡入，**无二次高度跳动**。
- [ ] 点「收起回复」：120ms 收回，无 stagger。
- [ ] 翻页（上一页/下一页）：只交叉淡入，容器高度不抖。
- [ ] `prefers-reduced-motion: reduce`：全部即时，无过渡。
- [ ] 根评论与子评论的头像 + 昵称都进 `/users/{id}`；键盘可 Tab 到、焦点环可见；`aria-current` 语义不乱。
- [ ] `user_id` 缺失时是纯文本，不是死链。

### 顶栏

- [ ] 创作/消息/历史三枚等宽 68px，图标 + 文字不换行。
- [ ] 640–1023px：消息/历史只显图标，但宽度同为 68px（不再是 88px 空盒）；创作显文字。
- [ ] 焦点环、hover 边框、未读红点位置不变。

### 目录树

- [ ] 三种页面都不再出现封面缩略图。
- [ ] 圆点落在当前行所属层级的发丝线上，线穿点而过；`depth 0` 在最外侧起点。
- [ ] 同树内切文章：圆点沿弧线滑到新位置（含跨层级）；首次打开抽屉：直接落位。
- [ ] 折叠/展开文件夹后，圆点重新对准当前行。
- [ ] `reduce` 下圆点瞬移；`aria-current`、键盘顺序、`?from=` 来源链接不变。

### 标题大纲

- [ ] `NAnchor` 的滚动高亮仍按 `bound=96` 工作；点标题跳转 + pinned 行为不变。
- [ ] 轨道虚线、节点空心/实心、飞机朝向、已读填充都随滚动更新。
- [ ] 进度条与百分比**仍在**（双表达）。
- [ ] `reduce` 下无滑动动画；`<1024` 仍不渲染。

### 文件夹卡

- [ ] 合集页与项目页都是文件夹形态，列宽自适应、不溢出网格。
- [ ] `hover`/`focus`：卡片上抽 + 旋转，flap 前倾；点击进详情；移出回位。
- [ ] 亮暗两态都由 token 给出，组件里**零裸色值**（门禁绿）。
- [ ] 空封面、长标题、24 张卡滚动下不掉帧到肉眼可见。
- [ ] `@supports not (backdrop-filter)` 与 `prefers-reduced-transparency` 下退化为不透明面。

### 消息中心

- [ ] 五段贴合、激活块深墨 + 反白、接缝有颈部、未读角标正常。
- [ ] 路由切换激活态与顶栏 hover 面板数据一致。
- [ ] 窄屏（375px）横向滚动可用，无换行/截断。
- [ ] `reduce` 下颈部与圆角瞬变。

### 门禁

- [ ] `check-design-tokens`、两端 `tokens.css` 同源、`reduced-motion` 审计全绿。
- [ ] 新增两条设计测试（reduce 必须 jump；弹簧参数必须来自 `motion.ts`）**能被人造违规打红**。

---

## 七、非范围

- 后台（`frontend_admin`）不动；后台的合集/项目编辑与树编辑器不变。
- 顶栏的消息 hover 面板不动（仍用现有胶囊子项）。
- **不改 OpenAPI 契约**（不加 `covers[]`，不加字段）。
- 不做视觉截图回归、不做 e2e、不做 i18n。
- 不重命名"目录"这一同名概念（见 §五.5）。
- 不新增色相、不新增字体、不动 `tokens.css` 的既有取值（除可能的弹簧无关项）。

---

## 八、文档联动（本轮一并完成）

1. 本文（grill 记录）。
2. [ADR-0012](../adr/0012-neutral-glass-visual-system.md) 追加「后续修订（2026-11-07）」：封面层 flap 玻璃、颈部布局动画、三处弹簧。
3. [GLOSSARY.md](../../GLOSSARY.md)：新增「合集/项目文件夹卡」「沟槽圆点」「颈部」「弹簧预设」四条词条；修订「液态玻璃层」「动效档位」两条，各加一行例外指针。

## 九、交付记录（2026-11-07）

- spec：**#289**；子票 8 张 **#290–#297**（原生 sub-issue + 原生 blocking 边；`#290` 是唯一前置票，`#295/#296/#297` 依赖它，`#294` 依赖 `#292`）。
- 集成分支 `feat/reading-ui-batch`，基线 `main`@`125066a`；每张票一个独立 worktree + 独立分支，逐个合并。
- 六个需求 → 八张票的映射：动效地基（弹簧预设 + 两条结构门禁）、顶栏宽度、评论作者链接、子评论展开动效、目录树圆点、标题大纲轨道、文件夹卡、消息中心颈部。
- 验收：全量前端测试 **206 文件 / 2460 用例**通过；`pnpm typecheck` 通过；两端设计 token 门禁与 token 同源门禁通过；覆盖率 **83.06 / 75.11 / 82.54 / 83.68**（门槛 60/50/66/60）。
- 双轴评审结果：
  - **Standards 轴**：0 条硬性违规。6 条判断题已修 —— 3 处文件带 UTF-8 BOM、评论作者块重复、字体就绪重测逻辑重复、文件夹卡视图类型重复、两处过度设计（未用入参、只被测试调用的悬空 API）、过期注释与文档。
  - **Spec 轴**：1 条实现 bug（`OutlineTree` 字体就绪后把"是否已移动"取反 → 首次进入会滑一段）+ 1 条越界（`ArticleTocAnchor` 用 `:deep` 抹掉 NAnchor 的激活竖条，违反「行为层不变」），均已修；1 条被判**不成立**——`.comment__empty-replies` 的「加载回复」兜底按钮在 `main` 上本就存在，属既有行为，不是本批新增。
- 仍需人眼验收（无法在现有接缝里断言）：68px 顶栏不换行、文件夹卡悬停抽取与玻璃降级、圆点压线、飞机朝向、颈部形变、暗色下的实际观感。
