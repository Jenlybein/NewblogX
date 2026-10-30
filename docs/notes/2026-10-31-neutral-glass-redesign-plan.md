# 中性玻璃改造方案（JellyBin 视觉立场反转）

- 日期：2026-10-31
- 决策出处：[ADR-0012](../adr/0012-neutral-glass-visual-system.md)（已取代 [ADR-0010](../adr/0010-visual-system-and-motion-budget.md) 的视觉立场）· [ADR-0013](../adr/0013-editorial-hero-cover.md)（首屏编辑部式封面例外）
- spec：[#17](https://github.com/Jenlybein/NewblogX/issues/17)（[spec 笔记](./2026-10-31-neutral-glass-redesign-spec.md)）——本文是它的实现侧展开，取值为最终口径
- 事实来源：[视觉主题勘查结论](#八勘查事实摘要)（两端已完全 token 化，门禁 0 违规）· [Apple 液态玻璃解构](./2026-10-30-apple-liquid-glass-deconstruction.md)
- 状态：方案已评审通过，**代码未动**

## 一、一句话

把"克莱因蓝主色 + 圆角归零 + 图纸工程感"换成"中性石墨阶 + 圆角阶梯 + 只落在导航/控制层的液态玻璃"，并用逐字节 token 同步门禁、扩展后的漂移门禁与更严的设计测试把新立场变成可执行约束。

## 二、决策溯源（grilling 会话逐条结论）

| # | 决策 | 结论 |
| --- | --- | --- |
| Q1 | ADR-0010 是否被取代 | 新建 ADR-0012 显式 supersede 部分条款，ADR-0010 状态改为 `已取代` |
| Q2 | 蓝色退役范围 | 退役铺底 / 墨色 / 结构线 / 主色，蓝色不留主色 |
| Q3 | 圆角政策 | token 阶梯三档 + 胶囊 full（最终 5 档） |
| Q4 | 玻璃作用范围 | 只给导航 / 浮层 / 悬浮控件；正文与卡片保持实体面 |
| Q5 | 动效预算 | 放宽到档位 C（滚动进场 + 导航转玻璃 + 视差 ≤10px） |
| Q6 | 拉丁字体 | 字标字体不改；**全站不用衬线体**；其余按本方案（Sora = 字标 + 大标题，Inter = 正文/UI） |
| Q7 | 范围与批次 | **两端全做**；后台以保证信息对比度为前提；分 4 批 |
| Q8 | 顶栏 | 改成吸顶玻璃导航 + 滚动方向感知（下滚隐藏、上滚出现） |
| Q9 | Logo 白边 | 作为独立 bugfix 立刻修 |
| Q10 | 强调色 | 中性墨水做强调；珊瑚橙只用于状态/危险；保留功能蓝（仅链接与焦点环） |
| Q11 | 色相集合 | 五色相保留：黄只做记号笔高亮底（不作字色、不作装饰） |
| Q12 | 形状语义 | 整体退役，几何语言统一为"圆角 + 玻璃" |
| Q13 | 亮色底 | 层次反转：底用暖灰（提亮派生 `#F5F3EE`），面用奶油白/纯白 |
| Q14 | 暗色底 | 苹果实测中性暗阶 `#161617` / `#1D1D1F` / `#2C2C2E` |
| Q15 | 玻璃配方 | 两档 token（thin / thick），以苹果实测数值为基线 |
| Q16 | 性能与降级 | 同屏可见玻璃层 ≤3；移动端 blur ≤12px、关指针高光；两种降级都做 |
| Q17 | 品牌图形 | **保留电光蓝 3D 渲染图作为品牌例外**（同 App 图标），只修白边 |
| Q18 | token 单源 | 保持两份，加"逐字节一致"同步门禁 |
| Q19 | 功能蓝 | 只做"可交互指向"：链接 + 焦点环，CTA 留墨水 |
| Q20 | 危险色 | 珊瑚橙保留作填充，文字用派生暗橙 `#C2401F` |
| Q21 | 字体分工 | Sora = 字标 + 大标题；Inter = 正文与 UI |
| Q22 | 亮色底值 | `#F5F3EE` |
| Q23 | 滚动进场 | **全站所有区块都做**（正文段落不做） |
| Q24 | 门禁扩展 | 补 `.ts` CSS 串、修 `_` 转义、补阅读站 `legacy-colors` 测试 |
| Q25 | 后台暗色 | **同步启用**，且每个页面都要有可见开关（`/studio/write` 必须补） |
| Q26 | 收尾方式 | 文档先行，经评审后再动代码 |
| Q27 | 主旨句去留 | 「一个会呼吸的个人博客」页头 / 首屏 / 页脚**三处全删**，`BRAND.tagline` 字段删除 |
| Q28 | 首屏容器 | 去掉边框与纸面底色（后被 Q31–Q35 的满幅版式取代） |
| Q29 | 首页 h1 | ~~把「JELLYBIN · 果冻盒」小字升为 h1~~ → 被 Q36 取代：h1 = 首屏超大标题 |
| Q30 | tagline 字段 | 从 `brand.ts` 删除 |
| Q31 | 版式范围 | **只首屏**用编辑部式版式，其余保持苹果式克制 |
| Q32 | 版式元素 | 色带 + 超大黑体与方括号/箭头 + 网格刻度 + 胶囊小签；**放弃人像抠图**（无素材） |
| Q33 | 色带配色 | 黄 + 功能蓝 + 珊瑚三色带，**仅首屏例外**（需在 ADR 开例外） |
| Q34 | 首屏图像 | 运营上传的首张轮播图满幅铺底 |
| Q35 | 三格事实 | 直接删掉（`homeHeroFacts()` 与其单测一并删） |
| Q36 | 超大文字 | `JELLYBIN` + 方括号副标 `[果冻盒]`，这行即首页 h1 |
| Q37 | 轮播 | 控件并入首屏（玻璃胶囊箭头 + 圆点），≥2 张才显示 |
| Q38 | 文档落点 | 新开 [ADR-0013](../adr/0013-editorial-hero-cover.md)：首屏编辑部式封面 |

## 三、Token 全表

### 3.1 原始层（五个锁定色相 + 派生）

| token | 值 | 用途 |
| --- | --- | --- |
| `--jelly-cream` | `#FDFBF6` | 纸面主色 |
| `--jelly-warm` | `#F2EEE6` | 次级底色 |
| `--jelly-warm-100` | `#F5F3EE` | 页面底（暖灰提亮派生） |
| `--jelly-yellow` | `#F5C518` | **仅**记号笔高亮底（`rgb(245 197 24 / .22)`） |
| `--jelly-coral` | `#FF6B4A` | 危险/状态填充 |
| `--jelly-coral-700` | `#C2401F` | 危险文字（对奶油白 5.0:1） |
| `--jelly-interactive` | `#0066CC`（亮）/ `#2997FF`（暗） | 功能蓝：链接与焦点环 |
| `--graphite-rgb` | `28 27 25`（亮）/ `245 245 247`（暗） | 石墨阶派生基底 |
| `--jelly-coral-rgb` | `255 107 74` | 珊瑚橙 alpha 派生基底 |
| `--jelly-yellow-rgb` | `245 197 24` | 记号笔底派生基底 |
| `--interactive-rgb` | `0 102 204`（亮）/ `41 151 255`（暗） | 功能蓝 alpha 派生基底 |

> 克莱因蓝 `#002FA7` 与 `--jelly-blue-*` 八档派生**全部删除**；`--jelly-warm-800` 保留供发丝线派生，其余无消费者的一并删除。

### 3.2 语义层（亮色）

| token | 值 | 备注 |
| --- | --- | --- |
| `--color-bg-page` | `#F5F3EE` | 页面底 |
| `--color-bg-subtle` | `#F2EEE6` | 次级底 |
| `--color-surface` | `#FDFBF6` | 纸面 |
| `--color-surface-raised` | `#FFFFFF` | 抬升面 |
| `--color-ink` | `#1C1B19` | 正文墨（对纸面 ≈16.9:1） |
| `--color-ink-muted` | `#625C52` | 次级墨（≈5.9:1） |
| `--color-ink-faint` | `#8A8378` | 极淡（仅大字/装饰） |
| `--color-line-faint` | `rgb(var(--graphite-rgb) / .10)` | 发丝线（极淡） |
| `--color-line` | `rgb(var(--graphite-rgb) / .16)` | 发丝线（默认） |
| `--color-line-strong` | `rgb(var(--graphite-rgb) / .32)` | 结构线（强） |
| `--color-link` | `var(--jelly-interactive)` | 文字链接（`#0066CC`，4.5:1） |
| `--color-focus` | `var(--jelly-interactive)` | 焦点环 |
| `--color-focus-soft` | `rgb(var(--interactive-rgb) / .10)` | 强调底 |
| `--color-highlight` | `rgb(var(--jelly-yellow-rgb) / .22)` | 记号笔底 |
| `--color-danger` | `var(--jelly-coral)` | 危险填充 |
| `--color-danger-ink` | `var(--jelly-coral-700)` | 危险文字 |
| `--color-danger-soft` | `rgb(var(--jelly-coral-rgb) / .12)` | 危险底 |
| `--color-danger-line` | `rgb(var(--jelly-coral-rgb) / .40)` | 危险线 |
| `--color-on-strong` | `#FFFFFF` | 墨水实心 CTA 上的字 |
| `--color-shadow` | `rgb(var(--graphite-rgb) / .12)` | 阴影 |

### 3.3 语义层（暗色映射，`.dark` / `.theme-dark`）

| token | 值 |
| --- | --- |
| `--color-bg-page` | `#161617` |
| `--color-bg-subtle` | `#101011` |
| `--color-surface` | `#1D1D1F` |
| `--color-surface-raised` | `#2C2C2E` |
| `--color-ink` | `#F5F5F7` |
| `--color-ink-muted` | `#A1A1A6` |
| `--color-ink-faint` | `#86868B` |
| `--color-line-faint / - / -strong` | `rgb(245 245 247 / .08 / .14 / .28)` |
| `--color-link` / `--color-focus` | `#2997FF` |
| `--color-focus-soft` | `rgb(41 151 255 / .16)` |
| `--color-highlight` | `rgb(245 197 24 / .20)` |
| `--color-danger` / `-ink` | `#FF6B4A` / `#FF9B85` |
| `--color-danger-soft` / `-line` | `rgb(255 107 74 / .18 / .50)` |
| `--color-on-strong` | `#1D1D1F`（暗色下 CTA 反转为浅墨填充） |
| `--color-shadow` | `rgb(0 0 0 / .55)` |

> 暗色采用**中性**石墨（不取暖黑）：暖黑压上白玻璃会发浊。明暗色温断层由玻璃边缘带用暖白 `rgb(255 253 246 / α)` 接住。

### 3.4 玻璃组

| token | 亮色 | 暗色 |
| --- | --- | --- |
| `--glass-thin-fill` | `rgb(255 255 255 / .72)` | `rgb(22 22 23 / .72)` |
| `--glass-thin-blur` / `-saturate` | `20px` / `180%` | 同 |
| `--glass-thick-fill` | `rgb(253 251 246 / .80)` | `rgb(29 29 31 / .82)` |
| `--glass-thick-blur` / `-saturate` | `28px` / `160%` | 同 |
| `--glass-rim` | `rgb(255 255 255 / .62)` | `rgb(255 255 255 / .30)` |
| `--glass-rim-warm` | `rgb(255 253 246 / .50)` | `rgb(255 253 246 / .22)` |
| `--glass-grain-opacity` | `.055` | `.045` |
| `--glass-specular` | 由 `--mx/--my` 驱动的径向渐变 | 同 |
| `--glass-blur-mobile` | `12px`（`max-width: 768px` 覆盖） | 同 |

实现要点（已核实）：
- **边缘折射带**用 `.glass::before` + `padding: 1px` + `mask-composite: exclude`，不用 `border`（`border` 会和 `overflow:hidden` 裁掉高光）。
- **噪点层**用 `.glass::after` 内联 `feTurbulence` 的 data-URI SVG，压住渐变带色阶。
- **`backdrop-filter` 会让元素成为 backdrop root** —— 嵌套玻璃无法糊到页面、只会读成平灰。因此"同屏 ≤3 层玻璃"是硬约束，嵌套卡片一律不透明。
- 降级链：`@supports not (backdrop-filter: blur(1px))` → 不透明面；`prefers-reduced-transparency: reduce`（Safari）→ 不透明面；`prefers-reduced-motion: reduce` → 关指针高光与进场。
- 移动端（`max-width: 768px`）：`blur` 降到 `--glass-blur-mobile`，关闭指针镜面高光（省一次每帧重绘）。

### 3.5 圆角组

`--radius-sm: 8px` · `--radius-md: 14px` · `--radius-lg: 20px` · `--radius-xl: 28px` · `--radius-pill: 999px`
映射：输入/按钮/控件 8–14；卡片/面板 20；弹窗/抽屉/大浮层 28；胶囊/徽标 pill。嵌套子圆角 = 父圆角 − 内边距（同轴曲率）。

### 3.6 排版与动效

| token | 值 |
| --- | --- |
| `--font-display` | `"Sora", "Sora Fallback"`（字标 + 大标题） |
| `--font-latin` | `"Inter", "Inter Fallback"`（正文/UI） |
| `--font-title` | `var(--font-display)`（**新增，修 `pages/index.vue:217,270` 的未定义**） |
| `--font-mono` | `"JetBrains Mono", ui-monospace, monospace`（**阅读站新增，修 `markdownTheme.ts:358` 的未定义**） |
| `--type-tracking-title` | `-0.03em`（大标题，苹果式紧字距） |
| `--motion-reveal-offset` | `12px`（滚动进场位移上限） |
| `--parallax-max` | `10px` |
| `--motion-*` | 120 / 200 / 320ms + `cubic-bezier(.2,.8,.2,1)` + stagger 40ms（不变） |

## 四、首屏封面规格（ADR-0013）

分层（自下而上）：

| 层 | 内容 | 实现要点 |
| --- | --- | --- |
| 0 底图 | 首张轮播图 `BannerItem.cover`，`object-fit: cover` | 无图时色带 + 文字仍成立（图是"底"不是"主体"） |
| 1 罩层 | 中性石墨渐变，保证文字对比度 ≥4.5:1 | 亮色自下而上加深，暗色反向 |
| 2 色带 | 2–3 条横向带：黄 `#F5C518` / 功能蓝 `#0066CC` / 珊瑚 `#FF6B4A` | **仅此处**可作装饰；主带 8–14px，最长一条可压过文字行 |
| 3 网格+刻度 | 细网格线（墨色 6%）+ 下沿刻度数字 | ≥1024px 才显示；刻度用 `--font-mono` |
| 4 超大 h1 | `JELLYBIN`（700，字距 `-0.04em`）+ 方括号副标 `[果冻盒]` | 整个 `<h1>` 是唯一边界；字号 `clamp(2rem, 9vw, 6rem)` |
| 5 记号 | 右上箭头、方括号收尾、黑底白字胶囊小签 | 纯装饰，全部 `aria-hidden` |
| 6 控件 | 轮播箭头 + 圆点，`--glass-thin` 胶囊 | 仅 `banners.length ≥ 2` 时渲染 |

满幅实现：在 `page-shell` 内用 `width: 100vw; margin-inline: calc(50% - 50vw)` bleed，外层 `overflow-x: clip` 防横向滚动条；高度 `min-height: clamp(380px, 62vh, 620px)`；内容用两栏 grid（左文字 / 右刻度）。

文案与语义（Q27 / Q36）：`h1` = `JELLYBIN` + `[果冻盒]`；保留描述句（`brandDescription`）与两个入口（搜索文章 / 开始创作）；**删除**三格事实条、页头 `.site-header__abstract`、页脚 tagline，并把 `BRAND.tagline` 字段从 `brand.ts` 移除。

窄屏（<768px）：色带减到 1 条，隐藏刻度/方括号/箭头，标题走 `clamp(2rem, 9vw, 3rem)`，内容纵向堆叠。

## 五、四批实施

### 批次 0：Logo 白边独立 bugfix（可先发，不阻塞）

| 文件 | 改动 |
| --- | --- |
| `frontend/app/components/layout/AppHeader.vue:118` | 删掉 `uiStore.theme === 'dark' ? 'white' : 'render'`，固定 `render` |
| `frontend/app/components/brand/Logo.vue:19-21` | 删 `white` 变体分支，`variant` 只留 `render` / `flat` |
| `frontend/app/utils/brand.ts:26,31` | 删 `logoWhite`（`BrandAssetKey` 与 `BRAND_ASSETS`） |
| `frontend/public/brand/logo-512-white.png` | 删除（`logo-256.png` 已含 alpha，四角 `(0,0,0,0)` 透明） |
| `frontend/test/**` | 补断言：品牌资产清单不含按主题切换的变体；Logo 组件只接受 `render`/`flat` |

> 验证：暗色顶栏不再出现白色方块；`grep -r logoWhite` 为 0。

### 批次 1：token 层重建（单一颜色来源）

| 文件 | 改动 |
| --- | --- |
| `frontend/app/assets/css/tokens.css` | 按 §三 重写：原始层换色相、语义层走 `rgb(var(--*-rgb)/α)`、暗色换中性暗阶、新增玻璃组/圆角组/字体组/动效补充 token；**删除过渡层 `:156-179`** |
| `frontend_admin/app/assets/css/tokens.css` | 与读者站**逐字节一致**（后台特有变体合并进主表） |
| `frontend/tailwind.config.ts` | 色板映射更新（新增 `link` / `glass` / `ink-faint`；删 `jelly.blue*`）；加 `borderRadius` 从 `--radius-*` 取值 |
| `frontend_admin/tailwind.config.ts` | 同上 |
| `frontend/app/utils/readerTheme.ts` | Naive `themeOverrides` 重算：`primaryColor` = 墨色（CTA 是墨水实心胶囊）、`infoColor` = 功能蓝、`borderRadius` = 14 / `borderRadiusSmall` = 8、字体从 token 取 |
| `frontend_admin/app/utils/theme.ts` | 322 行重写：中性调色板、去掉硬编码 alpha 常量、`shade()` 派生改走 `--*-rgb`、圆角与暗色映射 |
| `frontend/app/assets/css/fonts.css` | 新增 `Inter` 自托管（`@fontsource-variable/inter`）+ 度量匹配 fallback；`font-display: swap` |
| `frontend_admin/app/assets/css/fonts.css` | 同步 Inter（后台无 Sora 800 预加载） |
| `frontend/app/utils/motion.ts` | 新增 `revealOffset` / `parallaxMax` 常量，与 tokens 交叉校验 |

### 批次 2：全局层（形状 + 颜色语义解散）

| 文件 | 改动 |
| --- | --- |
| `frontend/app/assets/css/tailwind.css` | `.surface-card` 去 `backdrop-blur-xl`、改圆角 20 + 发丝线；`.surface-section`、`.feed-entry*`、`.article-*`、`.comment-*`、`.blueprint-*`、`.soft-tab*`、`.search-*`、`.studio-*` 全部改中性 + 圆角；删 40px 网格底纹 |
| `frontend/app/components/layout/AppHeader.vue:284-290` | 删黄三角（形状语义退役） |
| `frontend/app/components/profile/ProfileEmptyState.vue:39-47` | 三角/方/圆三色记号 → 中性几何或删除 |
| `frontend/app/components/layout/AppFooter.vue:63-67,84-89,144` | 克莱因蓝满铺 → 深墨面或玻璃条；次级字改中性 |
| `frontend/app/components/article/ArticleCover.vue:65-68,84,114-122,129` | 蓝色 `135deg` 渐变 → 中性石墨渐变 + 暖灰；徽标改玻璃（thin）；补 `--color-on-strong-*` 定义或删除引用 |
| `frontend/app/components/article/BannerCarousel.vue:150-154` | 圆点激活色改功能蓝 |
| `frontend/app/utils/markdownTheme.ts` | 573 行 CSS 字符串：链接/悬停、blockquote、code、表格、hljs 全配色改走新语义层；补 `mermaid themeVariables`（`MarkdownShadowPreview.vue:186-190` 传参） |
| `frontend/app/layouts/write.vue:6` | 删硬编码 `#fefcf8→#f7fbff` 与 `text-slate-900`，改 `bg-page` + 圆角面 |
| `frontend/app/assets/css/studio-write.css` | 视觉层跟 token；布局不动 |
| `frontend_admin/app/assets/css/admin.css` | 全量改中性 + 圆角；登录页渐变、图表柱、封面兜底全改 |
| `frontend/app/pages/studio/settings.vue:364-491`、`history.vue:111`、`favorites.vue:168`、`InboxChatPanel.vue:181`、`FavoriteFolderModal.vue:162` | 5 处圆角逃逸归 token |

### 批次 2b：首屏编辑部式封面（ADR-0013）

| 文件 | 改动 |
| --- | --- |
| `frontend/app/pages/index.vue:125-145,198-281` | 首屏重写为满幅封面（§四 规格）：删 `.home-hero` 的边框/纸面底色/内边距、删三格事实条、`h1` 改为 `JELLYBIN` + `[果冻盒]`、`.home-hero*` 更名 `.home-cover*`；新增 bleed、罩层、色带、网格刻度、记号、胶囊小签 |
| `frontend/app/components/article/BannerCarousel.vue` | 并入首屏：删 `.banner-shell` 边框与独立卡片形态（`:83-87`）；控件改 `--glass-thin` 胶囊、圆点激活色改功能蓝（`:150-154`）；空态沿用 `ProfileEmptyState` |
| `frontend/app/components/layout/AppHeader.vue:45-50,123,292-303,464` | 删 `projectAbstract` 计算属性、`.site-header__abstract` 元素与其样式（含响应式块 `:464`） |
| `frontend/app/components/layout/AppFooter.vue:31,95` | 删 tagline 行与样式，页脚版式重排 |
| `frontend/app/utils/brand.ts:22` | 删 `tagline` 字段 |
| `frontend/app/utils/home.ts:164`（`homeHeroFacts` 与 `HomeHeroFact` 类型） | 删除 |
| `frontend/test/utils/home.test.ts:130` | 删除 `homeHeroFacts` 断言 |
| `frontend/test/design/home-entrance.test.ts` | 首屏进场断言改为新结构（封面分层进场 + 视差上限） |

### 批次 3：玻璃层 + 吸顶顶栏 + 动效

| 文件 | 改动 |
| --- | --- |
| `frontend/app/components/layout/AppHeader.vue` | 改 `position: sticky`、高度 56–64px、`--glass-thin`、底部 1px 边缘高光；滚动方向感知（下滚隐藏/上滚出现）；`AppHeader` 仍是唯一开关落点 |
| 新增 `frontend/app/composables/useScrollChrome.ts` | 滚动方向 + 阈值 + `prefers-reduced-motion` 守卫（替代 `layouts/default.vue:50` 的"非 sticky"注释） |
| 新增 `frontend/app/composables/usePointerSpecular.ts` | `--mx/--my` 写入，移动端关闭，`reduced-motion` 关闭 |
| `frontend/app/assets/css/tailwind.css`（`.glass-*` 组件类） | `.glass-thin` / `.glass-thick` + 边缘带 + 噪点层实现（§3.4） |
| `frontend/app/components/article/ArticleScoreModal.vue`、`favorite/FavoriteFolderModal.vue`、`auth/AuthModal.vue`、`jelly/JellyAiPanel.vue`（`:277-281`）、`jelly/JellyLauncher.vue`（`:300-311` 气泡）、`studio/StudioSelectionAiToolbar.vue`、`studio/StudioWriteHeader.vue` | 浮层改 `--glass-thick`，圆角 28；Naive modal/drawer/popover `themeOverrides` 同步 |
| `home/HomeFeedSection.vue:45-52`、`home/HomePinnedSection.vue`、`home/HomeSidebar.vue:65,94-103`、`pages/article/[id].vue` 侧栏、`studio` 页区块 | 用 `motion-v` 的 `useInView` 接滚动进场（位移 ≤`--motion-reveal-offset`，只 transform/opacity，单区块只进一次）；首屏封面自带的分层进场与视差在批次 2b 内实现 |
| `frontend/app/layouts/default.vue:47-66`、`studio.vue`、`write.vue` | 补 `scroll-padding-top`；`write.vue` 补主题开关（或补 `AppHeader`） |
| `frontend_admin/app/components/admin/AdminShell.vue`（`:128-147` 的 `admin-header`） | 顶栏改 `--glass-thin` 条带（仅导航层，内容面不透明）；新增暗色开关 |
| `frontend_admin/app/layouts/auth.vue`、`login.vue`、`setup.vue` | 也放开关（"每个页面都要有"） |
| 新增 `frontend/app/components/common/ThemeToggle.vue`（或两端各一份） | 复用现有 `uiStore.toggleTheme()` + `themeToggleLabel()`，样式走玻璃胶囊 |

### 批次 4：校准 + 门禁归零 + 文档收口

- 逐页目视：阅读站 15 页 × 明暗 2 套 × 移动/桌面；后台 11 页 × 明暗。
- 对比度核验：正文 4.5:1、非文字（焦点环/图形）3:1 —— 用 `readerTheme`/`theme.ts` 里已有 `withAlpha`/`shade` 的同一套判定。
- 同屏玻璃层 ≤3 审计（列表页、弹窗叠面板、Jelly 面板 + 抽屉）。
- 降级四连测：`prefers-reduced-motion`、`prefers-reduced-transparency`、`@supports not`、移动端 blur。
- 删除死代码：`tokens.css` 过渡层、`--jelly-blue-*`、`--motion-slow`/`--motion-stagger` 若无消费者、`frontend/package.json` 未使用的 `echarts` / `vue-echarts`。
- 首屏专项核验：`<768px` 时色带 ≤1 条、刻度与记号隐藏；首页有且仅有一个 `h1`；`grep -r "一个会呼吸的个人博客" .` 为 0；`grep -r "homeHeroFacts" .` 为 0；色带/刻度/记号类名只出现在首屏封面组件内。
- 文档：ADR-0012 状态改 `已交付`；本文件补"实际 vs 预估"偏差。

## 六、门禁与测试改动

| 项 | 现状 | 改动 |
| --- | --- | --- |
| `scripts/check-design-tokens.mjs` | 只扫 `.vue` / `.css`；hex 正则被 `_` 绕过 | 扩到 `.ts`（抽 CSS 字符串）；修正则；新增玻璃参数白名单（只许 token） |
| 新增 `scripts/check-token-parity.mjs` | 无 | 两端 `tokens.css` 逐字节比对，不一致即失败 |
| `scripts/check.sh` / CI | 已接 token 门禁 | 接入 parity 门禁 |
| `frontend/test/design/tokens.test.ts` | 断言旧五色 | 改断言新五色 + `rgb(var(--*-rgb)/α)` 派生 + 无 `rgba(` 字面量 |
| `frontend/test/design/dark-tokens.test.ts` | 断言克莱因蓝暗阶 | 改断言中性暗阶与玻璃边缘带 |
| `frontend/test/design/rounding.test.ts` | 只查少数结构类 | 扩到全站 `.vue` + `.ts`，断言无反例圆角字面量 |
| 新增 `frontend/test/design/glass.test.ts` | 无 | 断言玻璃只出现在允许的组件清单；同屏 ≤3；两档 token 被消费 |
| 新增 `frontend/test/design/legacy-colors.test.ts` | 阅读站缺（后台有） | 禁 Tailwind 默认调色板类名（`bg-slate-*`、`text-slate-*` 等） |
| `frontend/test/design/legacy-colors`（后台 `frontend_admin/test/unit/legacy-colors.test.ts`） | 已有 | 同步到新色板 |

## 七、验收清单

- [ ] `bash scripts/check.sh` 全绿（含新 parity 门禁）
- [ ] `pnpm -C frontend test`、`pnpm -C frontend_admin test` 全绿，`passWithNoTests` 仍为 `false`
- [ ] `node scripts/check-design-tokens.mjs`：两端 0 违规
- [ ] `grep -r "002FA7\|002fa7" frontend/app frontend_admin/app` 只剩品牌资产例外
- [ ] `grep -r "rgba(0, 47, 167" .` 为 0
- [ ] `grep -r "logoWhite\|logo-512-white" .` 为 0
- [ ] 明暗 × 两端 × 每页，正文对比度 ≥4.5:1、非文字 ≥3:1
- [ ] 每页都有可见主题开关（含 `/studio/write`、后台 login/setup）
- [ ] 同屏玻璃层 ≤3；移动端 blur ≤12px；四类降级生效
- [ ] `prefers-reduced-motion` 下无进场、无指针高光、无导航过渡
- [ ] 首屏专项：`<768px` 色带 ≤1 条；首页只有一个 `h1`；`grep -r "一个会呼吸的个人博客"` 为 0；`grep -r "homeHeroFacts"` 为 0；色带/刻度/记号类名只在首屏封面组件内出现

## 八、风险登记

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| 全站滚动进场（用户选定） | 长文页多处同时进场，低端机掉帧、观感眩晕 | 位移 ≤12px、单区块只进一次、正文段落不进、stagger 40ms 上限、强制 reduced-motion |
| `backdrop-filter` 重绘成本 | 移动端滚动掉帧 | 同屏 ≤3、移动端 blur 12px、关指针高光、避免嵌套玻璃 |
| 后台暗色逐页回归量大 | 密集表格对比度不足 | 后台不上玻璃、Naive 调色板重算走 token、按页签核表 |
| 两份 tokens.css 漂移 | "改了没生效"的静默故障 | 逐字节同步门禁进 CI |
| Naive 平行主题层 | token 改了但组件还是旧色 | `readerTheme.ts` / `theme.ts` 与 token 同批改，设计测试交叉校验 |
| 首屏例外区被复制扩散 | 色带/刻度出现在列表页与详情页，站点变成"到处是图形" | 样式集中在首屏封面组件；新增设计测试断言这些类名只在 `pages/index.vue` 首屏出现 |
| 首屏满幅素材依赖接口 | banner 接口失败时封面无图、版式塌 | 图只作"底"，无图时色带 + 超大标题仍成立；沿用 `ProfileEmptyState` 空态 |
| 首屏窄屏溢出 | `100vw` bleed 在移动端出现横向滚动条 | 外层 `overflow-x: clip`；色带与刻度在 <768px 收缩 |
| 明暗色温断层 | 亮色暖、暗色中性，品牌感分裂 | 玻璃边缘带用暖白；大标题与字标 Sora 守住品牌 |
| 品牌例外被滥用 | 未来又冒出"暗色专用 Logo" | 门禁写明例外清单；ADR 约束 #12 |

## 九、勘查事实摘要

两端**已完全 token 化**，换色本身便宜：
- 颜色只存在于两份 `tokens.css`（`frontend/app/assets/css/tokens.css` 179 行、`frontend_admin/app/assets/css/tokens.css` 146 行）。
- 门禁实测：`frontend` 0 违规 / 61 文件，`frontend_admin` 0 违规 / 29 文件；无内联色绑、无 canvas、无 CDN 字体、无 `dark:` 工具类、无 `!important` 颜色覆盖。
- Typography/批次涉及的**真实硬伤**：16 个语义 token 写死 `rgba(0, 47, 167, α)`；`markdownTheme.ts` 573 行 CSS 字符串不在门禁内；`layouts/write.vue:6` 唯一一处裸色字面量（`_` 转义绕过门禁）；`--font-title` / 阅读站 `--font-mono` / `--color-on-strong-soft` / `--color-on-strong-line` 未定义；5 处圆角逃逸；mermaid 无 `themeVariables`；后台 12 个外部 markdown 主题与 highlight.js 会重绘写作预览。

## 十、实际 vs 预估偏差

八张票（#18–#25）全部合入集成分支后，实现与本文口径的差异如下。**刻度与配色取值本身没有偏差**，差异都在"实现方式"与"未做的部分"。

| 票 | 偏差 | 为什么 | 处置 |
| --- | --- | --- | --- |
| #20 | 页脚与封面新增语义反白组 `surface-inverse` / `on-inverse`（暗色取 `#101011`） | 原方案的 `on-strong` 在暗色会反转为浅墨填充，反白字落不上去 | 保留新组；`#25` 的对比度测试已断言反白字 ≥4.5:1 |
| #20 | 轮播激活点取墨色，而非方案里写的"功能蓝" | 验收限定功能蓝只用于链接与焦点环 | 保留墨色，与 ADR-0012 约束 #1 一致 |
| #20 | 封面内层圆角取 14（20 − 16 = 4 不在五档） | 五档阶梯没有 4px，取最近的既有档位 | 保留 14；由设计测试锁住"圆角只来自五档" |
| #20 | `--color-focus-soft` 有定义但当前无组件消费 | 保留给后续焦点/选区附近极淡底用 | 保留定义（不是死代码：语义层的一员） |
| #20 | 写作页布局改为**渲染 `AppHeader`**，而非方案里"补一个开关" | 复用既有开关与根类机制，改动更小 | 保留 |
| #22 | 后台暗色开关落在 AdminShell 顶栏图标 + `auth` 布局右上浮动（覆盖全部 11 页，含登录/初始化） | "每个页面都要有"要求 | 保留 |
| #22 | 后台**未做玻璃**（明确排除项）：侧栏改不透明、统计卡仅半透明色无 blur、后台顶栏为实心面 | 密集信息面以对比度为先（用户在 Q7 明确要求） | 与方案一致，非偏差 |
| #22 | 新增 `themePreference.ts` / `stores/theme.ts` / `ThemeToggle.vue`（后台原本没有这套机制） | 后台此前只有暗色映射没有开关 | 保留 |
| #23 | 创作区按**布局容器**进场，而非逐区块 | 创作区是固定工作区，逐区块动会分散注意力 | 保留 |
| #23 | 弹窗与捏捏面板叠开时，"同屏 ≤3"依赖两者互斥 | 静态测试只能锁常驻三层（页头 + 捏捏把手 + 面板） | 已锁；叠开场景靠人工目视 |
| #24 | 首屏色带直接消费原始层 `--jelly-yellow` / `-interactive` / `-coral` | 语义层把三色分别锁给"记号笔底 / 链接 / 危险"，色带没有对应语义 token；避免为一次性装饰污染语义层 | 属 ADR-0013 首屏例外；`#25` 测试断言这些类名/取值**只出现在 `pages/index.vue`** |
| #24 | 无图降级用**深墨兜底**（同 `ArticleCover` 口径）而非透明页底 | 保证反白字 4.5:1 | 保留 |
| #24 | 首屏 CTA 在深色封面上反白覆盖 | 否则墨字在深色封面上不可读 | 保留 |
| #23/#24 | 视差接在文章页装饰记号与首屏底图，未接段落 | 正文不进场的约束 | 与 ADR-0012 约束 #9 一致 |

**未验证 / 环境限制**

- 本地 WSL 缺 `go` / `make` / 全局 `node`，`bash scripts/check.sh` **无法整体跑通**；其中涉及 node 的节（设计门禁、parity 门禁、两端 vitest、typecheck）已在 Windows 侧单独实跑为绿，`verify-gates.sh` 的 parity canary（12/12）也单独验过红→绿。整体一键门禁需在 CI 或有完整工具链的环境里最终确认。
- **人工目视签核仍待做**：逐页明暗（读者站 × 桌面/移动、后台 × 桌面）没有截图或真机证据，本次只做到"结构与对比度断言为绿"。这是唯一未闭环的一条。

**与 spec #17 的口径差异**：无。spec 的验收清单已由 #25 的两组测试（`closeout-contrast` 对比度、`closeout-review` 复核项为 0 与降级链）固化为可执行断言。
