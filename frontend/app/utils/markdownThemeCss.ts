/**
 * 正文 markdown 主题的样式源（字符串常量，注入 Shadow DOM）。
 *
 * 为什么单独一个模块：仓库口径是「单文件 ≤ 800 行」（eslint max-lines，error），
 * 超了就把东西外移而不是把上限调高；这份样式源本身约 570 行。
 *
 * 为什么是字符串而不是 .css 文件：这个样式只注入 Shadow DOM，不能进全局样式表；
 * 而 Vitest 默认不处理 CSS 导入（?raw / ?inline 都会返回空串），.css 文件会让
 * 「组件挂载后 style 里到底有没有主题」这类冒烟测试永远看不到内容。放成字符串后，
 * 单测与运行时读到的是同一份 CSS。
 *
 * 颜色只消费 tokens.css 的两层语义 token：纸面/结构线走 --color-*，正文色板
 * （六层标题、四种内联、列表标记、代码七档）走 --md-*；明暗切换靠 token 的
 * 第二份取值自动生效。由 app/utils/markdownTheme.ts#buildMarkdownThemeCss 包成
 * 可注入的字符串。
 */
export const MARKDOWN_THEME_CSS = `/* ============================================================================
 * 正文 markdown 主题（JellyBin 自有）：注入 Shadow DOM，样式不泄漏到页面其他部分。
 *
 * 取代原来的 github-markdown-css + highlight.js github.css：
 *   - 颜色只消费 \`app/assets/css/tokens.css\` 的两层语义 token：
 *     纸面 / 结构线 / 链接 / 高亮底走 \`--color-*\`；**正文色板**（六层标题、四种内联、
 *     列表标记、代码七档）走一组独立的 \`--md-*\`（ADR-0012「后续修订」：正文渲染区
 *     不受五色相锁定）。明暗切换靠同一组 token 的第二份取值自动生效 —— 不用换
 *     <link>，不闪白、不跳版；
 * 版式照参考主题 graphite-island：正文 16px / 行高 1.85，标题六档各一种记号
 *   （H1 定标线 / H2 色带 / H3 三角 / H4 短下划线 / H5 双发丝线 / H6 顶横线），
 *   代码块是另一层纸面，引用块用强调面；字体族 / 字重 / 字距仍走 --font-* / --type-*；
 *   - 本文件由 \`app/utils/markdownTheme.ts#buildMarkdownThemeCss\` 包成可注入的字符串。
 *
 * 约束：颜色只有一个来源。这里**不允许出现裸 hex / rgba**（漂移门禁同款规则），
 * 也不允许 @keyframes / 滚动编排；动作只到 hover/focus 的颜色过渡。
 * ========================================================================== */

.markdown-body {
  color: var(--color-ink);
  background: transparent;
  font-family: var(--font-sans);
  /* 正文节奏取自参考主题 graphite-island：16px / 行高 1.85。
   * 这是版式常量（不是色值），所以不走 --type-*；行长仍由页面容器
   * .article-body 的 `--type-measure` 决定，shadow 里不另设宽度。 */
  font-size: 16px;
  line-height: 1.85;
  text-align: start;
  overflow-wrap: anywhere;
  -webkit-text-size-adjust: 100%;
}

.markdown-body > :first-child {
  margin-top: 0;
}

.markdown-body > :last-child {
  margin-bottom: 0;
}

/* ---- 六层标题装置：每层的颜色与记号都不同（正文色板 \`--md-*\`）----
 * H1 墨 / 定标线 + 居中记号块 · H2 赤陶 / 左侧 3px 色带 · H3 苔绿 / 行首三角 ·
 * H4 靛青 / 约 40% 短下划线 · H5 梅紫 / 上下双发丝线 · H6 赭黄 / 大写字距 + 顶横线。
 *
 * 颜色与装置必须成对出现：只换颜色会让六层"深浅不同但看起来一样"，只换装置又会被
 * 当成普通分隔线。每层字色与装置都取同一枚 \`--md-heading-hN\`，换色板时装置跟着走，
 * 不会出现"标题变色了、色带还是旧色"。
 *
 * 为什么不再用 --color-*：ADR-0012 的「后续修订」明确**正文渲染区不受五色相锁定** ——
 * 五色里没有六个可当字色的色相，六层标题就无法靠颜色区分层级。正文色板单独一组
 * \`--md-*\`（见 tokens.css 第 7 段），纸面/结构线这类与正文色板无关的仍然走 --color-*。 */
.markdown-body h1,
.markdown-body h2,
.markdown-body h3,
.markdown-body h4,
.markdown-body h5,
.markdown-body h6 {
  font-family: var(--font-sans);
  font-weight: var(--type-weight-title);
  line-height: var(--type-leading-title);
}

/* H1 墨：居中大标题 + 下沿定标线 + 线上压一块居中记号块。 */
.markdown-body h1 {
  margin: 1.35em 0 1.25em;
  padding-bottom: 0.95em;
  border-bottom: 1px solid var(--md-heading-h1);
  color: var(--md-heading-h1);
  font-size: 2.15em;
  line-height: 1.28;
  letter-spacing: 0.03em;
  text-align: center;
}

.markdown-body h1::after {
  content: "";
  display: block;
  width: 44px;
  height: 3px;
  margin: 0.65em auto 0;
  background: var(--md-heading-h1);
}

/* H2 赤陶：左侧 3px 色带，标题贴着色带。 */
.markdown-body h2 {
  margin: 2.05em 0 0.95em;
  padding: 0.55em 0 0.6em 1.1em;
  border-left: 3px solid var(--md-heading-h2);
  color: var(--md-heading-h2);
  font-size: 1.58em;
  line-height: 1.35;
}

/* H3 苔绿：行首三角，其余靠留白。 */
.markdown-body h3 {
  position: relative;
  margin: 1.75em 0 0.7em;
  padding: 0 0 0 1.05em;
  color: var(--md-heading-h3);
  font-size: 1.33em;
  line-height: 1.4;
}

.markdown-body h3::before {
  content: "";
  position: absolute;
  top: 0.45em;
  left: 0;
  width: 0.6em;
  height: 0.6em;
  background: var(--md-heading-h3);
  clip-path: polygon(50% 0, 100% 100%, 0 100%);
}

/* H4 靛青：短下划线只占标题宽度的约 40%。 */
.markdown-body h4 {
  margin: 1.5em 0 0.6em;
  padding-bottom: 0.5em;
  color: var(--md-heading-h4);
  font-size: 1.18em;
  line-height: 1.35;
}

.markdown-body h4::after {
  content: "";
  display: block;
  width: 40%;
  height: 3px;
  margin-top: 0.45em;
  background: var(--md-heading-h4);
}

/* H5 梅紫：上下双发丝线，把标题夹在中间。 */
.markdown-body h5 {
  margin: 1.35em 0 0.6em;
  padding: 0.45em 0;
  border-top: 1px solid var(--md-heading-h5);
  border-bottom: 1px solid var(--md-heading-h5);
  color: var(--md-heading-h5);
  font-size: 1em;
  font-weight: 600;
  letter-spacing: var(--type-tracking-dense);
}

/* H6 赭黄：制图式小型大写标签 + 顶横线，靠开字距而不是字号喊叫。 */
.markdown-body h6 {
  margin: 1.25em 0 0.35em;
  padding-top: 0.4em;
  border-top: 1px solid var(--md-heading-h6);
  color: var(--md-heading-h6);
  font-size: 0.8em;
  text-transform: uppercase;
  letter-spacing: 0.18em;
}

/* ---- 正文块 ---- */
.markdown-body p {
  margin: 0 0 1.15em;
}

/* ---- 四种内联形态：颜色与形态都必须互不混淆 ---- */
.markdown-body strong {
  color: var(--md-bold-ink);
  font-weight: var(--md-bold-weight);
}

.markdown-body em {
  color: var(--md-italic-ink);
  font-style: italic;
}

/* 删除线：markdown-it 把 \`~~文字~~\` 产出 \`<s>\`，正文里手写的 HTML 可能是 \`<del>\`；
 * 两种标签都要覆盖，否则"主题有删除线样式但实际渲染没上色"。 */
.markdown-body del,
.markdown-body s {
  color: var(--md-strike-ink);
  text-decoration: line-through;
  text-decoration-color: var(--md-strike-line);
  text-decoration-thickness: var(--md-strike-width);
}

/* 下划线（markdown-it-ins 的 ++文字++ 与 <u>）：**真下划线**，不再是黄底冒充。
 * 字色继承正文（不另起一种颜色），线色单独取靛青，并与文字拉开 3px；
 * 悬停加粗用来确认"这是强调不是链接"。 */
.markdown-body ins {
  background: none;
  color: inherit;
  text-decoration: underline;
  text-decoration-color: var(--md-underline-line);
  text-decoration-thickness: 1px;
  text-underline-offset: var(--md-underline-offset);
  transition: font-weight var(--motion-fast) var(--motion-ease);
}

.markdown-body ins:hover {
  font-weight: var(--md-bold-weight);
}

/* 记号笔底（==文字==）：与下划线是两回事 —— 黄只作底、永不做字色。 */
.markdown-body mark {
  background: var(--color-highlight);
  color: var(--color-ink);
}

/* ---- 链接：无下划线，靠 1px 底边和色相认领"可点" ----
 * 参考主题的形式（底色细线 + 悬停反白），颜色仍只取 --color-link / --color-ink：
 * 暗色的功能蓝由 token 的第二份取值给出，对比度由 design 测试钉住。 */
.markdown-body a {
  padding: 0 1px;
  border-bottom: 1px solid var(--color-link);
  color: var(--color-link);
  text-decoration: none;
  transition:
    color var(--motion-fast) var(--motion-ease),
    border-color var(--motion-fast) var(--motion-ease);
}

.markdown-body a:hover {
  border-bottom-color: var(--color-ink);
  color: var(--color-ink);
}

.markdown-body a:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

.markdown-body hr {
  margin: 2.4em 0;
  border: 0;
  border-top: 1px solid var(--color-line);
}

/* ---- 列表 ---- */
.markdown-body ul,
.markdown-body ol {
  margin: 0 0 1.15em;
  padding-left: 1.6em;
}

.markdown-body li {
  margin: 0.35em 0;
}

.markdown-body li > ul,
.markdown-body li > ol {
  margin: 0.35em 0 0.35em;
}

/* 列表标记与正文有区分：无序圆点苔绿、有序数字赤陶（都取 --md-* 色板）。
 * 分开写两条规则：\`::marker\` 只能拿到最近一层列表的标记，混写会让有序/无序
 * 变成同一个颜色。 */
.markdown-body ul > li::marker {
  color: var(--md-list-unordered-marker);
}

.markdown-body ol > li::marker {
  color: var(--md-list-ordered-marker);
}

/* ---- 引用：强调面 ---- 
 * 参考主题里引言是一块实心的暖砂"强调面"，不是只换一条左侧结构线。
 * 这里用包豪斯黄的记号笔底（--color-highlight，黄只作底）当强调面，
 * 文字回到正文墨色：彩底上的次级墨会掉对比度。 */
.markdown-body blockquote {
  margin: 1.5em 0;
  padding: 0.9em 1.15em;
  border: 1px solid var(--color-line);
  border-left: 3px solid var(--color-line-strong);
  border-radius: var(--radius-sm);
  background: var(--color-highlight);
  color: var(--color-ink);
}

.markdown-body blockquote > :last-child {
  margin-bottom: 0;
}

.markdown-body blockquote strong {
  color: var(--color-ink);
}

/* 强调面上的行内代码换回纸面：彩底再压彩贴片会脏。 */
.markdown-body blockquote code {
  background: var(--color-surface);
  border-color: var(--color-line);
}

/* ---- 提示块（::: tip / note / warning / danger） ---- */
.markdown-body .markdown-notice {
  margin: 1.5em 0;
  padding: 1em 1.25em;
  border: 1px solid var(--color-line);
  border-left-width: 4px;
  background: var(--color-bg-subtle);
  color: var(--color-ink);
}

.markdown-body .markdown-notice > :last-child {
  margin-bottom: 0;
}

.markdown-body .markdown-notice__title {
  margin: 0 0 0.45em;
  font-size: 0.92em;
  font-weight: 700;
  letter-spacing: var(--type-tracking-dense);
}

.markdown-body .markdown-notice--tip {
  border-left-color: var(--color-line-strong);
  background: var(--color-bg-subtle);
}

.markdown-body .markdown-notice--note {
  border-left-color: var(--color-line-strong);
  background: var(--color-bg-subtle);
}

.markdown-body .markdown-notice--warning {
  border-left-color: var(--color-line);
  background: var(--color-highlight);
}

.markdown-body .markdown-notice--danger {
  border-color: var(--color-danger-line);
  border-left-color: var(--color-danger);
  background: var(--color-danger-soft);
}

.markdown-body .markdown-notice--danger .markdown-notice__title {
  color: var(--color-danger-ink);
}

/* ---- 代码：行内是贴片，代码块是"另一张纸"（比正文纸面降一档 + 边线 + 平投影） ----
 * 代码纸面走 --md-code-paper：亮色比正文纸面暗一档、暗色比正文纸面再深一档，两态
 * 都是"同一场景里的第二层纸"而不是换一种颜色。投影只用 --color-shadow。 */
.markdown-body code {
  padding: 0.15em 0.45em;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-sm);
  background: var(--color-surface-raised);
  color: var(--md-code-ink);
  font-family: var(--font-mono);
  font-size: 0.9em;
  line-height: 1.6;
}

.markdown-body pre {
  margin: 1.5em 0;
  padding: 1rem 1.15rem;
  overflow-x: auto;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--md-code-paper);
  box-shadow: 0 3px 0 0 var(--color-shadow);
  color: var(--md-code-ink);
  font-family: var(--font-mono);
  font-size: 0.92em;
  tab-size: 4;
}

.markdown-body pre code {
  display: block;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: var(--md-code-ink);
  font-size: 14px;
  line-height: 1.75;
  white-space: pre;
}

.markdown-body .hljs {
  color: var(--md-code-ink);
  background: transparent;
}

/* 七档高亮 token：注释 / 关键字 / 字符串 / 数字 / 函数 / 类型 / 变量。
 * 分档原则照"读代码时先看到什么"：关键字与函数是骨架，字符串/数字/类型是数据，
 * 注释最轻。颜色只来自 --md-code-*，换色板时七档一起走。 */
.markdown-body .hljs-comment,
.markdown-body .hljs-quote {
  color: var(--md-code-comment);
  font-style: italic;
}

.markdown-body .hljs-keyword,
.markdown-body .hljs-selector-tag,
.markdown-body .hljs-literal {
  color: var(--md-code-keyword);
  font-weight: 700;
}

.markdown-body .hljs-string,
.markdown-body .hljs-attr,
.markdown-body .hljs-template-tag {
  color: var(--md-code-string);
}

.markdown-body .hljs-number,
.markdown-body .hljs-attr-value,
.markdown-body .hljs-link,
.markdown-body .hljs-regexp {
  color: var(--md-code-number);
  font-weight: 600;
}

.markdown-body .hljs-function,
.markdown-body .hljs-title.function_,
.markdown-body .hljs-title,
.markdown-body .hljs-section,
.markdown-body .hljs-selector-id,
.markdown-body .hljs-selector-class {
  color: var(--md-code-function);
  font-weight: 600;
}

/* 类型排在函数之后：\`.hljs-title.class_\` 同时命中 \`.hljs-title\`，靠更高的
 * 选择器特异性（两个类）取到"类型"那一档，而不是掉进函数色。 */
.markdown-body .hljs-type,
.markdown-body .hljs-built_in,
.markdown-body .hljs-title.class_,
.markdown-body .hljs-class .hljs-title {
  color: var(--md-code-type);
  font-weight: 600;
}

.markdown-body .hljs-variable,
.markdown-body .hljs-params,
.markdown-body .hljs-attribute,
.markdown-body .hljs-template-variable {
  color: var(--md-code-variable);
}

.markdown-body .hljs-deletion {
  background: var(--color-danger-soft);
  color: var(--color-danger-ink);
}

.markdown-body .hljs-addition {
  background: var(--color-highlight);
  color: var(--color-ink);
}

.markdown-body .hljs-emphasis {
  font-style: italic;
}

.markdown-body .hljs-strong {
  font-weight: 700;
}

/* ---- 表格：发丝线（1px line-faint）+ 暖灰表头 + 极淡斑马纹；窄屏横向滚动 ----
 * 参考主题的表格是"发丝线框"，这里把原来的 2px 表头线收成 1px。 */
.markdown-body table {
  display: block;
  width: 100%;
  margin: 1.5em 0;
  overflow-x: auto;
  border-collapse: collapse;
  font-size: 0.94em;
}

.markdown-body thead {
  border-bottom: 1px solid var(--color-line-strong);
}

.markdown-body th,
.markdown-body td {
  padding: 0.5em 0.75em;
  border: 1px solid var(--color-line-faint);
  text-align: start;
  vertical-align: top;
}

.markdown-body th {
  background: var(--color-bg-subtle);
  color: var(--color-ink);
  font-weight: 700;
  letter-spacing: var(--type-tracking-dense);
}

.markdown-body tbody tr:nth-child(even) {
  background: var(--color-surface-raised);
}

/* ---- 数学公式：katex 自带布局样式，主题只负责颜色与横向溢出 ---- */
.markdown-body .katex {
  color: var(--color-ink);
  font-size: 1.05em;
}

.markdown-body .katex-display {
  margin: 1.5em 0;
  padding: 0.25em 0;
  overflow-x: auto;
  overflow-y: hidden;
}

.markdown-body .katex-display > .katex {
  color: var(--color-ink);
}

/* ---- 图片 / mermaid / 细节块 ---- */
.markdown-body img {
  max-width: 100%;
  height: auto;
  border: 1px solid var(--color-line-faint);
}

.markdown-body figure {
  margin: 1.5em 0;
}

.markdown-body figcaption {
  margin-top: 0.5em;
  color: var(--color-ink-muted);
  font-size: 0.85em;
}

/* ---- mermaid 图表 ----
 * 配色不在这里给：mermaid 把颜色写死进 SVG 的内联样式，消费不了 var(--color-*)，
 * 所以由 resolveMermaidTheme() 把语义 token 解析成真实色值、经 themeVariables
 * 在 initialize 时喂给它（明暗两套各自的第二份取值）。这里只负责版式与横向溢出。
 *
 * .mermaid[data-processed] 是 mermaid 渲染完的样子；未渲染时（渲染失败降级为裸
 * 定义文本）套代码块外观，避免中间态跳版。 */
.markdown-body .mermaid {
  display: block;
  margin: 1.5em 0;
  overflow-x: auto;
  color: var(--color-ink);
  font-family: var(--font-sans);
}

.markdown-body .mermaid svg {
  display: block;
  max-width: 100%;
  height: auto;
}

.markdown-body .mermaid:not([data-processed]) {
  padding: 1rem 1.15rem;
  border: 1px solid var(--color-line);
  background: var(--color-bg-subtle);
  color: var(--color-ink);
  font-family: var(--font-mono);
  font-size: 14px;
  line-height: 1.65;
}

.markdown-body details {
  margin: 1.5em 0;
  padding: 0.75em 1em;
  border: 1px solid var(--color-line-faint);
  background: var(--color-surface-raised);
}

.markdown-body summary {
  cursor: pointer;
  font-weight: 600;
}

/* ---- 移动端：字号阶梯已全部是 em（随 16px 正文缩放），这里只管代码块的
 * 内边距与字号，以及表格不撑破容器 ---- */
@media (max-width: 640px) {
  .markdown-body pre {
    padding: 0.9rem;
  }

  .markdown-body pre code {
    font-size: 13px;
  }
}
`;
