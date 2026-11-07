# ADR-0012：中性玻璃视觉系统（取代 ADR-0010 的色相、圆角、结构线与动效档位）

- 状态：已交付
- 日期：2026-10-31
- 来源：2026-10-31 的 grilling 会话（用户反馈："当前主题不好看，蓝色作为主色调太伤眼"；目标：apple.com 式高级感美学 + 液态玻璃 + 圆角）。spec 为 [issue #17](https://github.com/Jenlybein/NewblogX/issues/17)（`ready-for-agent`），落地分票 #18–#25 已全部合入集成分支；视觉解构见 [Apple 液态玻璃解构](../notes/2026-10-30-apple-liquid-glass-deconstruction.md)，改造方案见 [中性玻璃改造方案](../notes/2026-10-31-neutral-glass-redesign-plan.md)
- 后续修订：**决策 8 与约束 10 中"中文继续 Noto Sans SC"的部分已由 [ADR-0019](./0019-reader-rendering-and-cache-strategy.md)（已交付，落地票 #80–#87）取代** —— 中文字体改为系统字体栈，以消除首屏 2.3 MB 自托管字体；Sora = 字标/大标题、Inter = 正文/UI、全站无衬线这些分工继续生效。差异表见文末「后续修订」。
- 后续修订（2026-11-04）：**约束 1 / 2 / 3 / 4 对"正文 markdown 渲染区"放开** —— 正文区不再受五色相锁定，改由 `--md-*` 一组独立语义 token 单独治理（六层标题色 + 七档代码色 + 真下划线）；裸色值门禁与两端 token 同源不变。落地票 #244（spec #234 §5/§8），差异表见文末「后续修订（2026-11-04）」，肉眼验收见[色板预览](../notes/2026-11-04-markdown-palette-preview.md)。
- 后续修订（2026-11-07）：**约束 6 / 9 新增三处例外** —— 合集/项目文件夹卡的 flap 允许玻璃（内容面唯一例外）、消息中心 tab 颈部允许 `margin-left` 布局动画、三处动效改用弹簧（标题大纲飞行器 / 目录树沟槽圆点 / tab 颈部）。三档时长与单一缓动**继续服务 CSS 侧**。差异表见文末「后续修订（2026-11-07）」，来源会话见 [阅读页 UI grill 记录](../notes/2026-11-07-reading-ui-grill.md)。

## 背景与问题

ADR-0010 在 2026-10-30 落地（ticket #1–#13 已合并），取向是"图纸工程感"：五色相锁死、克莱因蓝 `#002FA7` 作主色、圆角归零、结构线一律 1px 克莱因蓝、动效只许 `transform`/`opacity`。落地一天后的实际观感与目标相反：

- **蓝色在三个量级上都过载**。暗色模式整页铺 `#001b63`、纸面 `#001f70`（大面积高饱和深蓝）；页脚与统计块是 `#002FA7` 满铺；而全站的 1px 分隔线、悬停底、阴影都是 `rgba(0, 47, 167, α)`，连正文墨色也是蓝黑 `#001b63` ——"每个黑色其实都是深蓝"。这是"伤眼"的直接来源。
- **圆角归零从未执行到底**。`pages/studio/settings.vue` 有 9 处 `rounded-[18px]`、`history.vue` 18px、`favorites.vue` `rounded-2xl`、`InboxChatPanel.vue` `rounded-lg`、`FavoriteFolderModal.vue` `rounded-[22px]`；品牌资产 `brand/jelly-flat.svg` 自己写着 `rx="4"/"3"/"1"`。门禁只检查 `tailwind.css` 与少数结构类，所以 CI 一直是绿的。
- **形状语义把颜色绑死在几何记号上**（三角=黄、方=橙、圆=蓝），蓝色一退役，这套语义整体失去锚点。
- **顶栏刻意非吸顶**（`AppHeader.vue:245` 注释："滚动时永远不遮挡正文"），而目标美学的骨架正是吸顶半透明导航。
- **两处颜色来源的硬伤**：16 个语义 token 把 `rgba(0, 47, 167, α)` 写死（`tokens.css:59-61,66,73` 与暗色对应行），不跟随 `--jelly-blue`，所以"改主色"不等于"改界面"；`tokens.css:156-179` 还留着 v1 的青绿过渡层（`--accent #0f766e`），无消费者。
- **门禁有盲区**：`utils/markdownTheme.ts` 里 573 行 CSS 字符串不被扫描；hex 正则的 `_` 转义让 `layouts/write.vue:6` 的 `#fefcf8` 溜过；阅读站缺后台那样的 `legacy-colors` 测试。
- **三个 token 根本没定义却在使用**：`--font-title`（`pages/index.vue:217,270`）、阅读站 `--font-mono`（`markdownTheme.ts:358`）、`--color-on-strong-soft` / `--color-on-strong-line`（`ArticleCover.vue`）。它们静默降级成继承值/透明。
- **md 写作页没有暗色开关且有第二层缺陷**：`layouts/write.vue` 不渲染 `AppHeader`（开关唯一落点在 `AppHeader.vue:176`），且第 6 行写死了亮色渐变与 `text-slate-900`，暗色下整页发白。
- **暗色 Logo 是白底方块**：`AppHeader.vue:118` 在暗色切到 `variant="white"` → `brand/logo-512-white.png`，该文件是 **24bppRgb 无 alpha 通道**、四角不透明纯白，在没有 `border-radius` 裁剪的 `<img>` 上就是一块白瓷砖。

这些问题的共同结构是：**ADR-0010 的约束是"编辑立场"，而立场被用户否定了**，不是修几个 bug 能补的。

## 决策

1. **色相集合改写（仍是五个锁定色相，不新增第六个）**：克莱因蓝 `#002FA7` 退役其"主色 / 铺底 / 墨色 / 结构线"角色，原始层替换为——奶油白 `#FDFBF6`、暖灰 `#F2EEE6`（含提亮派生 `#F5F3EE`）、珊瑚橙 `#FF6B4A`（填充）/ 派生暗橙 `#C2401F`（文字）、包豪斯黄 `#F5C518`（**仅**记号笔高亮底）、功能蓝 `#0066CC`（亮）/ `#2997FF`（暗）（**仅**文字链接与焦点环）。中性石墨阶由暖灰去饱和派生，属"同色相明度派生"而非新色相。首页首屏封面区的装饰色带是本条唯一的例外，见 [ADR-0013](./0013-editorial-hero-cover.md)。
2. **语义层 token 重建 + 消灭硬编码 alpha**：所有带透明度的颜色改为 `rgb(var(--*-rgb) / α)` 三元组派生（新增 `--jelly-coral-rgb`、`--graphite-rgb`、`--interactive-rgb` 等），删除 16 处写死的 `rgba(0, 47, 167, α)` 与删掉无消费者的过渡层（`tokens.css:156-179`）。
3. **层次反转与中性底**：亮色 = 底 `#F5F3EE` / 次底 `#F2EEE6` / 面 `#FDFBF6` / 抬升 `#FFFFFF`；墨 `#1C1B19`（暖石墨）、次级 `#625C52`、极淡 `#8A8378`；发丝线取墨色的三档透明度（`0.10 / 0.16 / 0.32`）。暗色 = 采用苹果实测中性暗阶 底 `#161617` / 次底 `#101011` / 面 `#1D1D1F` / 抬升 `#2C2C2E`；墨 `#F5F5F7`、次级 `#A1A1A6`、极淡 `#86868B`；线为 `#F5F5F7` 的 `0.08 / 0.14 / 0.28`。**暗色不采用暖黑**：暖黑压上白玻璃会发浊。
4. **圆角阶梯（取消圆角归零）**：`--radius-sm 8px` / `--radius-md 14px` / `--radius-lg 20px` / `--radius-xl 28px` / `--radius-pill 999px`。映射：控件与输入 8–14、卡片 20、大浮层与弹窗 28、胶囊与徽标 pill；嵌套层的子圆角 = 父圆角 − 内边距（同轴曲率，避免边缘高光错位）。现存 5 处圆角逃逸一并归token。
5. **液态玻璃只用于导航/控制层**：新增两档 `--glass-thin`（导航、胶囊、徽标：`blur 20px` + `saturate 180%` + 亮色 `rgb(255 255 255 / .72)` / 暗色 `rgb(22 22 23 / .72)`）与 `--glass-thick`（弹窗、抽屉、捏捏面板：`blur 28px` + `saturate 160%` + `alpha .80/.82`），配 1px 折射边缘带（`mask-composite: exclude` + `padding:1px` 的伪元素）、`feTurbulence` 噪点层（`opacity ≈ .055`，亮 `.055` / 暗 `.045`）与指针镜面高光（`--mx`/`--my` 变量 + 径向渐变，不改布局）。**正文、卡片、列表等内容面一律不透明实体面**；顶栏用滚动边缘效应（内容压到边缘时加强不透明度）而不是永久厚玻璃。
6. **顶栏改为吸顶玻璃导航**：`position: sticky`，高度压到 56–64px，滚动超阈值后加模糊与底部 1px 边缘高光，**滚动方向感知**（下滚隐藏、上滚出现）以保留"不永久遮挡正文"的原始动机；全站补 `scroll-padding-top` 保证锚点不被遮挡。
7. **动效档位 C（放宽 ADR-0010 的"档位 B"）**：保留三档时长 `120 / 200 / 320ms`、单一缓动 `cubic-bezier(.2,.8,.2,1)`、stagger `40ms`；新增**全站区块级滚动进场**（hero、列表条目、卡片、侧栏、区块容器），位移 `≤12px`（新 token `--motion-reveal-offset`）、只动 `transform`/`opacity`；视差上限 `10px`（新 token `--parallax-max`）；导航随滚动转玻璃。**正文 markdown 段落本身不做进场**（"区块"指布局级容器）。实现用已有依赖 `motion-v` 的 `useInView`/`useScroll`，沿用现有客户端挂载守卫。
8. **字体分工**：`--font-display: Sora` 用于字标、品牌记号与大标题（hero / 页面标题）；`--font-latin: Inter` 负责正文与 UI；中文继续 `Noto Sans SC`；**全站不使用衬线体**。补上缺失的 `--font-title`（取 `--font-display`）与阅读站 `--font-mono`，删除依赖未定义 token 的写法。
9. **两端同源 + 后台同步启用暗色**：两端各一份 `tokens.css` 保持同源，新增"两份逐字节一致"的同步门禁（不一致即 CI 红）；后台启用暗色开关，且**每个页面（含阅读站 `/studio/write`）都必须能看到开关**；后台密集信息面不上玻璃、以对比度为先（正文 4.5:1、非文字 3:1）。
10. **品牌资产例外 + 白边修复**：电光蓝果冻渲染图（`logo-256.png`）与 `jelly-flat.svg` 作为**品牌资产例外**保留自身颜色（同 App 图标），门禁豁免不变；暗色白边修复方案是**删除 `white` 变体与 `logoWhite` API**，两套主题统一使用透底资产（`logo-256.png` 已含 alpha），不再按主题切换 Logo。
11. **门禁扩展（与新立场同批交付）**：扫描目标扩到 `.ts` 中的 CSS 字符串；修掉 hex 正则 `_` 转义漏洞；阅读站补 `legacy-colors` 测试（禁 Tailwind 默认调色板类名）；mermaid 需要 `themeVariables` 从 token 取值（否则图表永远是第三方默认配色）。
12. **文档先行**：本条 ADR 与 `GLOSSARY.md` 词条先落，代码按"改造方案"的 4 批推进。

## 理由与已否决方案

高级感的来源不是"加特效"，而是**对比度纪律 + 材料感 + 克制的颜色**：苹果官网的页面本体几乎是黑白灰，颜色只出现在链接与极少数强调上，材料感只出现在导航/控制层。所以"退役蓝色主色"与"液态玻璃只给控制层"是同一个决策的两面——把颜色预算从大面积铺底挪到材料边缘与交互反馈上。

已否决的方案：

- **只换主色、保留圆角 0 与图纸语言**：会得到"染成灰色的同一个工程稿"。模板感来自形状 + 材料 + 构图的组合，ADR-0010 自己也记过这条教训（"只换皮会得到染成克莱因蓝的同一个模板"）。
- **只修暗色、亮色保留克莱因蓝**：亮色下全站蓝色 1px 线 + 蓝黑墨色同样过载，且与新玻璃材料冲突（蓝线压在模糊层上会变成脏青边）。
- **全站卡片都用玻璃**：`backdrop-filter` 会让元素成为 backdrop root，嵌套玻璃互相无法糊到页面、只会读成平灰；苹果自己的规则也要求"限制同屏效果数量"。内容面不透明是可用性与性能的共同结论。
- **纯 `#000` 暗色（OLED）**：白色填充在 `.14` 透明度压纯黑会变成灰板，玻璃读起来是"脏玻璃"而不是"材料"；苹果自己的暗色导航取 `rgb(22 22 23 / .8)`。
- **保留 3D 电光蓝渲染图并让它跟随色板**：渲染图无法用 CSS 变量改色，只能重新出图；且品牌角色像 App 图标一样自带颜色是行业惯例，强行中性化会丢掉"果冻盒"的辨识度。
- **后台不用暗色**：用户在会话中明确要求"每个页面都要有开关"，并把 `md 写作页缺开关`作为真实痛点提出。
- **引入 Playwright 截图比对做视觉回归**：成本高于本次收益；改用现有 `test/design/*` 结构测试 + 人工目视校准（见改造方案的验收清单）。
- **只做单档玻璃**：弹窗与大面板需要更厚的材料，单档会让浮层读起来"太薄、文字浮在背景上"。

## 代价 / 不做什么

- **暗色是中性灰而不是暖黑**：明暗两套在同一品牌里会有轻微色温断层（亮色偏暖、暗色中性）。缓解：亮色的暖底只在中性范围内（`#F5F3EE`），暗色的玻璃边缘带用暖白 `rgb(255 253 246 / α)` 把两头接上。
- **玻璃的性能与可读性是持续负债**：`backdrop-filter` 逐层重绘，移动端要降 `blur` 到 12px 并关闭指针高光；`prefers-reduced-transparency` 与 `@supports not` 要退化成不透明面。这些是长期维护成本，不是一次性改动。
- **全站滚动进场是本次最大的观感赌注**：用户选择了"全站所有区块"。缓解：只动 transform/opacity、位移 ≤12px、正文段落不动、强制 `prefers-reduced-motion`、单区块最多进场一次且不重复触发。
- **后台暗色要逐页面目视回归**：密集表格、表单、Tag、弹窗都要过一遍；Naive 的平行主题层（`frontend_admin/app/utils/theme.ts` 里 322 行硬编码 alpha 与 `shade()` 派生）必须随之重写，否则"token 改了、Naive 组件还是旧色"。
- **颜色迁移靠 token，不能逐处手改**：阅读站 42 行青绿族已由 ADR-0010 清过一次，本次残留风险集中在 16 处硬编码 alpha 与 `markdownTheme.ts` 的 573 行 CSS 字符串。
- **两份 `tokens.css` 是永久的同步义务**：加同步门禁是把它从"纪律"变成"门禁"，但仍是两份文件；抽共享包被否决（两端各有 workspace 与独立 lockfile，构建耦合风险高于收益）。
- **不做什么**：后台玻璃、e2e / 视觉截图比对、i18n、字体子集化极致优化、3D 品牌图形重绘、OG 图重做、顶部导航的多级菜单重设计。

## 当前生效约束

1. 五个锁定色相：奶油白、暖灰、珊瑚橙、包豪斯黄（仅高亮底）、功能蓝（仅链接与焦点环）；禁止新增第六个色相。**唯一例外**：首页首屏封面区的装饰色带可用黄/功能蓝/珊瑚，但不得引入第四色，也不得复用克莱因蓝（见 [ADR-0013](./0013-editorial-hero-cover.md)）。
2. 克莱因蓝不得再作为主色、铺底色、墨色或结构线出现；正文墨色与结构线一律中性石墨阶。
3. 所有带透明度的颜色必须由 `rgb(var(--*-rgb) / α)` 三元组派生，禁止写死的 `rgba(r,g,b,α)` 常量。
4. 组件只消费语义层 token；暗色是同一组语义 token 的第二份取值，由根类承载。
5. 圆角只用 `--radius-sm/md/lg/xl/pill` 五档；嵌套层子圆角 = 父圆角 − 内边距。
6. 液态玻璃只用于导航/控制层（导航、浮层、悬浮控件、胶囊）；正文、卡片、列表等内容面一律不透明；同屏可见玻璃层 ≤3。
7. 玻璃必须带 `@supports not` 与 `prefers-reduced-transparency` 的不透明降级；移动端 `blur` ≤12px 且关闭指针镜面高光。
8. 顶栏是吸顶玻璃导航 + 滚动方向感知（下滚隐藏、上滚出现），渲染站点页头的布局都要有 `scroll-padding-top`（创作页不渲染页头，因此不需要）。
9. 动效三档 `120 / 200 / 320ms` + 单一 `cubic-bezier(.2,.8,.2,1)` + stagger `40ms`；滚动进场位移 ≤12px、视差 ≤10px、只动 `transform`/`opacity`，正文段落不进场；强制尊重 `prefers-reduced-motion`。
10. 字体：Sora 用于字标与品牌记号 + 大标题（hero / 页面标题），Inter 负责正文与 UI，中文 Noto Sans SC；全站无衬线；不得使用未定义的字体 token。
11. 后台启用暗色，且每个页面的导航/工具条上都必须有可见的主题开关。读者站创作页 `/studio/write` 是例外形态：**它不渲染站点页头**（专心写的一屏，不要站点导航），开关落在创作页自己顶栏的最右端 —— 仍然满足"每页都能切"，只是落点不同。
12. 品牌资产例外：`brand/logo-256.png`、`brand/jelly-flat.svg`、`apple-touch-icon.png`、`og-image.png` 可自带品牌色并由门禁豁免；不得再按主题切换 Logo 变体（禁止重新引入 `logoWhite`）。
13. 门禁覆盖 `.vue`、`.css`、`.ts` 中的 CSS 字符串；hex 正则不得放过 `_` 转义；阅读站也要有 `legacy-colors` 测试。
14. 两端 `tokens.css` 必须逐字节一致，由同步门禁保证。

## 相关代码与文档

- 代码：[frontend/app/assets/css/tokens.css](../../frontend/app/assets/css/tokens.css)（原始层 `:29-50`、语义层 `:53-73`、暗色映射 `:99-123`、过渡层待删 `:156-179`）
- 代码：[frontend_admin/app/assets/css/tokens.css](../../frontend_admin/app/assets/css/tokens.css)（同源第二份，`nuxt.config.ts:188` 引用）
- 代码：[frontend/app/assets/css/tailwind.css](../../frontend/app/assets/css/tailwind.css)（组件类词汇表）；[frontend_admin/app/assets/css/admin.css](../../frontend_admin/app/assets/css/admin.css)
- 代码：[frontend/app/utils/readerTheme.ts](../../frontend/app/utils/readerTheme.ts) · [frontend_admin/app/utils/theme.ts](../../frontend_admin/app/utils/theme.ts)（Naive 主题的平行颜色源）
- 代码：[frontend/app/utils/markdownTheme.ts](../../frontend/app/utils/markdownTheme.ts)（573 行 CSS 字符串，门禁盲区）
- 代码：[frontend/app/components/layout/AppHeader.vue](../../frontend/app/components/layout/AppHeader.vue)（吸顶玻璃导航 + 滚动方向感知 + 主题开关的唯一落点；Logo 只保留透底一种形态）
- 代码：[frontend/app/components/studio/StudioWriteHeader.vue](../../frontend/app/components/studio/StudioWriteHeader.vue)（创作页自己顶栏；主题开关在它的最右端）
- 代码：[frontend/app/layouts/write.vue](../../frontend/app/layouts/write.vue)（创作页**不**渲染站点页头；背景与字色消费语义 token）
- 代码：[frontend/app/utils/brand.ts](../../frontend/app/utils/brand.ts)（`logoWhite` 待删；`BRAND_ASSETS`）
- 门禁：[scripts/check-design-tokens.mjs](../../scripts/check-design-tokens.mjs) · [scripts/design-token-baseline.json](../../scripts/design-token-baseline.json)
- 测试：[frontend/test/design/](../../frontend/test/design/) · [frontend_admin/test/unit/](../../frontend_admin/test/unit/)
- 文档：[ADR-0010](./0010-visual-system-and-motion-budget.md)（本 ADR 取代其色相/圆角/结构线/形状语义/动效档位条款）· [GLOSSARY · 语义色 token](../../GLOSSARY.md) · [Apple 液态玻璃解构](../notes/2026-10-30-apple-liquid-glass-deconstruction.md) · [中性玻璃改造方案](../notes/2026-10-31-neutral-glass-redesign-plan.md) · [ADR-0013](./0013-editorial-hero-cover.md)（首页首屏编辑部式封面的例外区）

## 后续修订（2026-11-01）

[ADR-0019](./0019-reader-rendering-and-cache-strategy.md)（已交付，落地票 #80–#87）取代本 ADR 的**两处字体条款**。按本目录的登记规则（"已交付的 ADR 不改结论，只追加后续修订"），正文原样保留，差异在此登记。

| 本 ADR 位置 | 原文口径 | 修订后 |
| --- | --- | --- |
| 决策 8 | 字体分工：Sora 用于字标/品牌记号/大标题，Inter 负责正文与 UI，**中文继续 Noto Sans SC** | 中文改为**系统字体栈**（`-apple-system` / PingFang SC / 微软雅黑 / 思源，末位保留 Noto 名称但不自托管）；Sora 与 Inter 保留自托管 |
| 约束 10 | 字体：Sora 字标与品牌记号 + 大标题，Inter 正文与 UI，**中文 Noto Sans SC**；全站无衬线 | 中文字体**不得以自托管 webfont 加载**；首屏字体预算 < 100 KB；Sora/Inter 的分工与"全站无衬线"不变 |

**原因**：生产实测首屏总下载 2.98 MB，其中字体 **2.41 MB**（Noto Sans SC 两个全量字重各约 1.1 MB），首页内容可见 1358 ms。详见 [读者站性能实测](../notes/2026-11-01-reader-site-performance-probe.md)。

**继续生效**：本 ADR 其余全部条款（五色相锁死、中性石墨阶、圆角阶梯、玻璃作用域与降级、动效三档、暗色、token 同源与门禁口径），以及字体分工里 Sora 与 Inter 的角色。

**落地时连带要改的**：[frontend/test/design/fonts.test.ts](../../frontend/test/design/fonts.test.ts)、[tokens.test.ts](../../frontend/test/design/tokens.test.ts)、[foundation-tokens.test.ts](../../frontend/test/design/foundation-tokens.test.ts)、[frontend/test/utils/readerTheme.test.ts](../../frontend/test/utils/readerTheme.test.ts) 里锁定"Noto 排在中文字体栈最前"的断言——它们锁的是旧立场，需按新口径改写，而不是删除断言。

## 后续修订（2026-11-04）

**正文 markdown 渲染区不再受本 ADR 的五色相锁定**（读者站正文色板与六层标题装置，落地票 #244，spec #234 §5/§8）。按本目录的登记规则（"已交付的 ADR 不改结论，只追加后续修订"），正文原样保留，差异在此登记。

| 本 ADR 位置 | 原文口径 | 修订后 |
| --- | --- | --- |
| 约束 1（五个锁定色相） | 五个锁定色相，禁止新增第六个色相 | **正文 markdown 渲染区例外**：正文区内可以有六层标题色与七档代码色，但它们只由 `--md-*` 一组 token 定义，且**不得溢出到组件/页面** |
| 约束 2（墨色一律中性石墨阶） | 正文墨色与结构线一律中性石墨阶 | 正文渲染区的字色由 `--md-*` 给出（H1 墨 `#1A1815`、加粗 `#0F0D0B` 等）；**页面上其余部分**仍守中性石墨阶 |
| 约束 3（三元组派生透明度） / 约束 4（只消费语义层） | 带透明度的颜色必须 `rgb(var(--*-rgb) / α)`；组件只消费语义层 | 正文色板是**同一套治理的第四个 token 组**：`tokens.css` 里定义、明暗各一份取值、组件里只能写 `var(--md-*)`、裸色值门禁照旧生效（hex 只出现在 `tokens.css`） |
| 决策 8（排版层） | 字体分工与字号阶梯 | 不变；正文色板只管**颜色**，字重/字距仍走 `--type-*` / `--font-*` |

**原因**：正文是"读文章"的主场。六层标题如果只靠字号区分，深层结构（H4–H6）几乎无法扫读；而五个锁定色相里没有一个能同时提供六个可当字色、且在明暗两态都达 WCAG AA 的色相。用户决定正文改用「方向 A：暖调编辑」色板：墨 / 赤陶 / 苔绿 / 靛青 / 梅紫 / 赭黄，六层各配一种装置（定标线 + 居中记号块 / 左侧 3px 色带 / 行首三角 / 约 40% 短下划线 / 上下双发丝线 / 大写字距 + 顶横线），层与层颜色和装置都不同。

**同时收口的两点**：

1. **下划线不再用黄底冒充**。此前 `++文字++`（`<ins>`）被实现成"重点上色"（包豪斯黄记号笔底 + `text-decoration: none`），与真正的下划线语义不符。现改为真 `text-decoration: underline` + 靛青线色（`--md-underline-line`）+ `text-underline-offset: 3px`，字色继承正文、悬停加粗确认"这是强调不是链接"。记号笔底（`==文字==` / `<mark>`）保留，两件事分开。
2. **删除线实际不生效的缺口一并修掉**：`markdown-it` 把 `~~文字~~` 产出的是 `<s>`，而主题只写了 `del`，等于删除线从来没被上色。现同时覆盖 `del` 与 `s`。

**一处刻意偏离色板**：代码注释 spec 写 `#7A756C`，它对正文纸面只有 4.43:1、对"降一档"的代码纸面只有 4.17:1，达不到 issue 验收里的 AA 硬要求。取同色相加深一档的 `#726D63`（对代码纸面 4.68:1），只动明度不动色相 —— **色板给形态、AA 给下限，可读性优先**。该偏离由 `frontend/test/design/markdown-palette.test.ts` 显式断言（含"与 spec 色通道差 ≤16 且更暗"两条约束），不是随手改色。

**继续生效**：本 ADR 其余全部条款 —— 五色相锁死（**正文渲染区除外**）、中性石墨阶（正文渲染区除外）、圆角阶梯、玻璃作用域与降级、动效三档、暗色机制、`tokens.css` 两端同源与裸色值门禁 —— 以及第 7 段 `--md-*` 组的"只供正文渲染区、明暗两份、组件只用 token"。

**验收与门禁**：

- 取值契约：[frontend/test/design/markdown-palette.test.ts](../../frontend/test/design/markdown-palette.test.ts)（色板逐项、明暗两份、全部文字/代码档 ≥4.5:1、代码纸面降一档）。
- 装置契约：[frontend/test/utils/markdownTheme.test.ts](../../frontend/test/utils/markdownTheme.test.ts)（六层各一套装置 + 四种内联 + 列表标记 + 代码七档）。
- 实际渲染对齐：[frontend/test/composables/markdownPaletteGallery.test.ts](../../frontend/test/composables/markdownPaletteGallery.test.ts)（把[预览稿](../../frontend/test/fixtures/markdown-palette-gallery.md)真渲染一遍，对齐 H1–H6 与 hljs 类名），肉眼验收步骤见[色板预览](../notes/2026-11-04-markdown-palette-preview.md)。

**相关代码**：[frontend/app/assets/css/tokens.css](../../frontend/app/assets/css/tokens.css)（第 7 段 `--md-*` 组）· [frontend/app/utils/markdownTheme.ts](../../frontend/app/utils/markdownTheme.ts)（`MARKDOWN_THEME_ROLES` 新增正文色板角色 + token 解析）· [frontend/app/utils/markdownThemeCss.ts](../../frontend/app/utils/markdownThemeCss.ts)（样式源；受「单文件 ≤ 800 行」约束从 `markdownTheme.ts` 外移）。

## 后续修订（2026-11-07）

**三处例外：内容面玻璃（文件夹 flap）、布局属性动画（tab 颈部）、弹簧动效（三处）**。按本目录的登记规则（"已交付的 ADR 不改结论，只追加后续修订"），正文原样保留，差异在此登记。来源是 2026-11-07 的 grill 会话（6 项阅读页/评论区/顶栏/合集卡片需求），决策全文见[阅读页 UI grill 记录](../notes/2026-11-07-reading-ui-grill.md)。

| 本 ADR 位置 | 原文口径 | 修订后 |
| --- | --- | --- |
| 约束 6（玻璃作用域） | 液态玻璃只用于导航/控制层；正文、卡片、列表等内容面一律不透明；同屏可见玻璃层 ≤3 | **合集/项目文件夹卡的 flap（前盖）是内容面上唯一的玻璃例外**：只有 flap 一层（背板与封面卡仍是不透明面），且必须带 `@supports not (backdrop-filter)` 与 `prefers-reduced-transparency` 的不透明降级、移动端 `blur ≤12px`（约束 7 照旧生效）。"同屏 ≤3"改为**非卡片玻璃层 ≤3**；卡片 flap 单独计数，允许常开（列表页可达 24 张） |
| 约束 9（动效三档） | 三档 `120 / 200 / 320ms` + 单一 `cubic-bezier(.2,.8,.2,1)` + stagger 40ms；只动 `transform`/`opacity` | 新增**弹簧例外，仅三处**：右栏标题大纲的飞行器位移与转向、目录树的沟槽圆点、消息中心 tab 条颈部。弹簧参数**集中在 `utils/motion.ts` 的 `SPRING_*` 预设**，组件禁止内联 `stiffness`/`damping` 字面量。**颈部允许 `margin-left`（布局属性）变化**，仅限该条 ≤5 个节点。三档时长与单一缓动继续服务 CSS 侧（评论展开/收起、路由过渡、hover 反馈），不因弹簧退役 |
| 约束 9（减少动态） | 强制尊重 `prefers-reduced-motion` | 口径不变且**加严**：弹簧在该偏好下必须 `jump()` 到终点（"归零"而不是"放慢"），弧线直接落位；弧线由"弹簧驱动 0→1 进度 + `useTransform` 派生偏移"实现，**不引入 `motion` 包**（本仓库未安装，`motion-v` 也不转出 `arc()`） |

**原因**：这两条都是用户在会话里**主动拍板**后偏离原立场（明确选择"flap 保留 `backdrop-filter` 且常开"与"三处都用弹簧"），不是实现时的顺手决定。若只留在代码里，下一个读者会按约束 6 / 9 把它们当成违规修掉——与 ADR-0010 那次"立场被否定但没人记录"的教训同型。

**一并加严的门禁（与实现同批交付）**：

1. 使用弹簧的组件在「减少动态」下必须落位为静止（禁止保留过渡）；
2. 组件里不得出现裸 `stiffness` / `damping` 数字，只能引用 `utils/motion.ts` 的 `SPRING_*`。

理由：[reduced-motion.test.ts](../../frontend/test/design/reduced-motion.test.ts) 只扫 CSS 的 `@keyframes` / `animation:`，**JS 弹簧完全在现有门禁视野之外**——这是本次新开的一个动效入口，不补断言等于让档位 C 静默退化。

**已知代价（接受并记录）**：24 张卡片同屏时每张 flap 都是一个 backdrop root，滚动有掉帧风险（第一顺位回退方案是"仅 hover 开 blur"）；颈部每帧触发该行重排；弧线形状由正弦近似自绘，与参考组件有细微视觉差异。

**继续生效**：本 ADR 其余全部条款——五色相锁死（正文渲染区除外）、中性石墨阶、圆角阶梯、玻璃作用域与降级（含约束 7 的两条降级与移动端 12px）、暗色机制、`tokens.css` 两端同源与裸色值门禁、字体条款（含 2026-11-01 与 2026-11-04 两次修订）。

**相关代码**：

- 弹簧预设：[frontend/app/utils/motion.ts](../../frontend/app/utils/motion.ts)（新增 `SPRING_*`；`MOTION_DURATIONS_MS` / `MOTION_EASE` 不改）
- 三处消费者：[frontend/app/components/article/ArticleTocAnchor.vue](../../frontend/app/components/article/ArticleTocAnchor.vue)（轨道与飞机）· [frontend/app/components/outline/OutlineTree.vue](../../frontend/app/components/outline/OutlineTree.vue)（沟槽圆点）· [frontend/app/components/inbox/InboxSubNav.vue](../../frontend/app/components/inbox/InboxSubNav.vue)（颈部）
- 玻璃例外落点：合集/项目列表页共用的文件夹卡组件（新建），消费方 [frontend/app/pages/collections/index.vue](../../frontend/app/pages/collections/index.vue) 与 [frontend/app/pages/projects/index.vue](../../frontend/app/pages/projects/index.vue)
- 现有门禁与测试：[frontend/test/design/motion-foundation.test.ts](../../frontend/test/design/motion-foundation.test.ts) · [frontend/test/design/reduced-motion.test.ts](../../frontend/test/design/reduced-motion.test.ts) · [scripts/check-design-tokens.mjs](../../scripts/check-design-tokens.mjs)

## 后续修订（2026-11-07 返工：按参考对齐四处视觉）

上表（同日早先那轮）登记的三处例外**继续生效**，但落点有变化，原因记在这里，免得下一个人按旧描述去找一个不存在的形状。

| 事项 | 早先那轮（同日） | 返工后（现行） |
| --- | --- | --- |
| 消息中心 tab 条的连接形状 | 接缝处画「颈部」：两条凹曲线 + 线性渐变 | **颈部整个删除**。用户明确要求去掉「与两边连接的样式」：激活块是独立圆角块，接缝里不画任何形状。`neckPath` / `neckGradientStops` / `FILL_ACTIVE` / `FILL_IDLE` / `NECK_VIEW_BOX` / `NECK_WAIST` / `NECK_NOMINAL_SPAN` 均已删除，`--neck-gap` 改名 `--seam-gap` |
| 约束 9 的布局动画例外 | 「tab 颈部允许 `margin-left` 变化」 | 例外**依然成立**，只是主体从"颈部"变成"接缝位移"：张开量仍由 `SPRING_NECK` 驱动 `margin-left`，仍是 ≤5 个节点的小子树、仍是全站唯一允许的布局动画。参考里 `SPRING_NECK` 因此**不退役** |
| 右栏标题大纲的当前项 | `.article-toc-link.is-active` 带一条 `border-left` 竖条 | **竖条去掉**：轨道是那一列唯一的视觉元素，当前项只靠加粗 + `--color-ink` 表达。节点对齐到行中心，文字固定离节点 14px |
| 合集/项目文件夹卡 | 参考比例的 321×270 舞台，封面占 65% | 用户要求「文件夹只比封面宽一点点」：舞台压到 **321×221**，封面占 **88%**，前盖从"整张前脸"收成一道前唇；卡片列宽收到 `16–18rem`、卡间距放大 |

**已知代价（更新）**：接缝位移每帧仍触发该行重排（范围不变，5 个节点）；文件夹 flap 的玻璃层与 24 张卡片同屏的掉帧风险不变；弧线仍用正弦近似。

