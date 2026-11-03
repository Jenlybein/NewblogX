# 正文 markdown 色板（方向 A 暖调编辑）：预览与肉眼验收

- 票：[#244](https://github.com/Jenlybein/NewblogX/issues/244)（spec [#234](https://github.com/Jenlybein/NewblogX/issues/234) §5 / §8）
- 治理：[ADR-0012「后续修订（2026-11-04）」](../adr/0012-neutral-glass-visual-system.md) —— 正文渲染区**不再受五族色相锁定**，色板由 `--md-*` 一组独立语义 token 治理
- 预览稿：[`frontend/test/fixtures/markdown-palette-gallery.md`](../../frontend/test/fixtures/markdown-palette-gallery.md)

## 怎么肉眼验收（两条路，任选）

### A. 站内预览（推荐，和真实渲染同一份样式）

1. 本地起读者站（见仓库根 [`README.md`](../../README.md)）：`cd frontend; pnpm dev`
2. 登录后打开 `/studio/write`，把预览稿**整段**粘进正文编辑器。
3. 切到「预览」页签 —— 预览走 `MarkdownShadowPreview`，与阅读页正文是**同一份** `MARKDOWN_THEME_CSS`，所以这里看到的就是读者看到的。
4. 顶栏主题开关切到暗色，再逐项核对一遍。

### B. 阅读页对稿

1. 用同一段内容发一篇草稿/测试文章，打开 `/article/{id}`。
2. 同样在明暗两态下对照下面这张检查表。

## 检查表（逐条打勾）

| # | 看什么 | 合格线 |
| --- | --- | --- |
| 1 | 六层标题 | H1 居中 + 定标线 + 居中记号块（墨）· H2 左侧 3px 色带（赤陶）· H3 行首三角（苔绿）· H4 约 40% 短下划线（靛青）· H5 上下双发丝线（梅紫）· H6 大写字距 + 顶横线（赭黄）。**颜色与装置都不能重复** |
| 2 | 加粗 | 深墨 `#0F0D0B` + 字重 650（比正文明显更黑，但仍是墨不是彩） |
| 3 | 斜体 | 暖灰 `#5F5A52` + **真斜体**（中文没有斜体字形时会被浏览器倾斜，属预期） |
| 4 | 删除线 | 字色暖灰 `#6E6961`、线色赤陶 `#B2442A`、线宽 1.5px；线要压在字上而不是变成另一种色块 |
| 5 | 下划线 | **真下划线**：靛青线 `#35597D`、字色与正文相同、与文字拉开 3px；悬停时加粗。**不许再是黄底**（黄底只留给 `==记号笔==`） |
| 6 | 列表 | 无序圆点苔绿、有序数字赤陶；和正文墨色一眼可分 |
| 7 | 代码 | 代码块底色比正文纸面**降一档**（亮色更暗、暗色更深）；七档 token 可辨（关键字赤陶 / 字符串苔绿 / 数字靛青 / 函数梅紫 / 类型赭黄 / 注释暖灰 / 变量深灰） |
| 8 | 暗色 | 同一套装置，颜色换成暗面第二份取值；没有白块、没有看不清的标题 |

## 自动化兜底（不用肉眼也能卡住的大部分回归）

| 测试 | 卡住什么 |
| --- | --- |
| [`frontend/test/design/markdown-palette.test.ts`](../../frontend/test/design/markdown-palette.test.ts) | 色板取值与 spec 逐项一致、明暗两份齐全、**全部文字档与代码档 ≥ 4.5:1**、代码纸面降一档 |
| [`frontend/test/utils/markdownTheme.test.ts`](../../frontend/test/utils/markdownTheme.test.ts) | 六层装置 + 四种内联 + 列表标记 + 代码七档的 CSS 契约（含"下划线不许再是黄底"的反向断言） |
| [`frontend/test/composables/markdownPaletteGallery.test.ts`](../../frontend/test/composables/markdownPaletteGallery.test.ts) | 把上面的预览稿**真渲染**一遍，确认 H1–H6 与 hljs 类名真的产出（防止"CSS 有装置、渲染器不出这个标签"） |
| `node scripts/check-design-tokens.mjs` | 组件/主题里不得出现裸色值（hex 只允许在 `tokens.css`） |
| `node scripts/check-token-parity.mjs` | 两端 `tokens.css` 逐字节一致 |

## 色板速查（`tokens.css` 第 7 段）

| 角色 | 亮色 | 暗色 | token |
| --- | --- | --- | --- |
| H1 墨 | `#1A1815` | `#F2EFE8` | `--md-heading-h1` |
| H2 赤陶 | `#B2442A` | `#E08063` | `--md-heading-h2` |
| H3 苔绿 | `#4E6B3C` | `#9DBC85` | `--md-heading-h3` |
| H4 靛青 | `#35597D` | `#8FB2DD` | `--md-heading-h4` |
| H5 梅紫 | `#6B4A78` | `#C2A0CC` | `--md-heading-h5` |
| H6 赭黄 | `#8A6524` | `#D9B269` | `--md-heading-h6` |
| 加粗 | `#0F0D0B`（字重 650） | `#FAF7F1` | `--md-bold-ink` / `--md-bold-weight` |
| 斜体 | `#5F5A52` | `#B9B2A7` | `--md-italic-ink` |
| 删除线（字 / 线） | `#6E6961` / `#B2442A`（1.5px） | `#A8A297` / `#DD7A5E` | `--md-strike-ink` / `--md-strike-line` / `--md-strike-width` |
| 下划线线色 | `#35597D`（offset 3px） | `#8FB2DD` | `--md-underline-line` / `--md-underline-offset` |
| 有序 / 无序标记 | `#B2442A` / `#4E6B3C` | `#E08063` / `#9DBC85` | `--md-list-ordered-marker` / `--md-list-unordered-marker` |
| 代码纸面 | `#F7F4ED` | `#131315` | `--md-code-paper` |
| 代码正文 | `#1A1815` | `#EDE9E1` | `--md-code-ink` |
| 代码 关键字 / 字符串 / 数字 | `#B2442A` / `#4E6B3C` / `#35597D` | `#E08063` / `#9DBC85` / `#8FB2DD` | `--md-code-keyword` / `--md-code-string` / `--md-code-number` |
| 代码 函数 / 类型 / 变量 | `#6B4A78` / `#8A6524` / `#2A2724` | `#C2A0CC` / `#D9B269` / `#E7E3DA` | `--md-code-function` / `--md-code-type` / `--md-code-variable` |
| 代码 注释 | `#726D63` | `#948E84` | `--md-code-comment` |

**一处刻意偏离**：代码注释 spec 给 `#7A756C`，但它对正文纸面 4.43:1、对代码纸面 4.17:1，达不到 issue 验收的 AA。实际取同色相加深一档的 `#726D63`（对代码纸面 4.68:1），只动明度不动色相 —— 由 `markdown-palette.test.ts` 显式断言（与 spec 色通道差 ≤16 且更暗）。详见 ADR-0012 的「后续修订（2026-11-04）」。
