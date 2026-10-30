# ADR-0012：中性玻璃视觉系统（取代 ADR-0010 的色相、圆角、结构线与动效档位）

- 状态：已接受
- 日期：2026-10-31
- 来源：2026-10-31 的 grilling 会话（用户反馈："当前主题不好看，蓝色作为主色调太伤眼"；目标：apple.com 式高级感美学 + 液态玻璃 + 圆角）。spec 已发布为 [issue #17](https://github.com/Jenlybein/NewblogX/issues/17)（`ready-for-agent`），落地分票待开；视觉解构见 [Apple 液态玻璃解构](../notes/2026-10-30-apple-liquid-glass-deconstruction.md)，改造方案见 [中性玻璃改造方案](../notes/2026-10-31-neutral-glass-redesign-plan.md)

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
8. 顶栏是吸顶玻璃导航 + 滚动方向感知（下滚隐藏、上滚出现），全站有 `scroll-padding-top`。
9. 动效三档 `120 / 200 / 320ms` + 单一 `cubic-bezier(.2,.8,.2,1)` + stagger `40ms`；滚动进场位移 ≤12px、视差 ≤10px、只动 `transform`/`opacity`，正文段落不进场；强制尊重 `prefers-reduced-motion`。
10. 字体：Sora 用于字标与品牌记号 + 大标题（hero / 页面标题），Inter 负责正文与 UI，中文 Noto Sans SC；全站无衬线；不得使用未定义的字体 token。
11. 后台启用暗色，且每个页面的导航/工具条上都必须有可见的主题开关（阅读站 `/studio/write` 必须补上）。
12. 品牌资产例外：`brand/logo-256.png`、`brand/jelly-flat.svg`、`apple-touch-icon.png`、`og-image.png` 可自带品牌色并由门禁豁免；不得再按主题切换 Logo 变体（禁止重新引入 `logoWhite`）。
13. 门禁覆盖 `.vue`、`.css`、`.ts` 中的 CSS 字符串；hex 正则不得放过 `_` 转义；阅读站也要有 `legacy-colors` 测试。
14. 两端 `tokens.css` 必须逐字节一致，由同步门禁保证。

## 相关代码与文档

- 代码：[frontend/app/assets/css/tokens.css](../../frontend/app/assets/css/tokens.css)（原始层 `:29-50`、语义层 `:53-73`、暗色映射 `:99-123`、过渡层待删 `:156-179`）
- 代码：[frontend_admin/app/assets/css/tokens.css](../../frontend_admin/app/assets/css/tokens.css)（同源第二份，`nuxt.config.ts:188` 引用）
- 代码：[frontend/app/assets/css/tailwind.css](../../frontend/app/assets/css/tailwind.css)（组件类词汇表）；[frontend_admin/app/assets/css/admin.css](../../frontend_admin/app/assets/css/admin.css)
- 代码：[frontend/app/utils/readerTheme.ts](../../frontend/app/utils/readerTheme.ts) · [frontend_admin/app/utils/theme.ts](../../frontend_admin/app/utils/theme.ts)（Naive 主题的平行颜色源）
- 代码：[frontend/app/utils/markdownTheme.ts](../../frontend/app/utils/markdownTheme.ts)（573 行 CSS 字符串，门禁盲区）
- 代码：[frontend/app/components/layout/AppHeader.vue](../../frontend/app/components/layout/AppHeader.vue)（`:118` 主题切换 Logo、`:245` 非吸顶注释、`:284-290` 黄三角）
- 代码：[frontend/app/layouts/write.vue](../../frontend/app/layouts/write.vue)（`:6` 写死亮色渐变，无 `AppHeader`/无开关）
- 代码：[frontend/app/utils/brand.ts](../../frontend/app/utils/brand.ts)（`logoWhite` 待删；`BRAND_ASSETS`）
- 门禁：[scripts/check-design-tokens.mjs](../../scripts/check-design-tokens.mjs) · [scripts/design-token-baseline.json](../../scripts/design-token-baseline.json)
- 测试：[frontend/test/design/](../../frontend/test/design/) · [frontend_admin/test/unit/](../../frontend_admin/test/unit/)
- 文档：[ADR-0010](./0010-visual-system-and-motion-budget.md)（本 ADR 取代其色相/圆角/结构线/形状语义/动效档位条款）· [GLOSSARY · 语义色 token](../../GLOSSARY.md) · [Apple 液态玻璃解构](../notes/2026-10-30-apple-liquid-glass-deconstruction.md) · [中性玻璃改造方案](../notes/2026-10-31-neutral-glass-redesign-plan.md) · [ADR-0013](./0013-editorial-hero-cover.md)（首页首屏编辑部式封面的例外区）
