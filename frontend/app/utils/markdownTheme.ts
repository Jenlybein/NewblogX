/**
 * 正文 markdown 主题：明暗两份取值 + 可注入 shadow root 的样式字符串。
 *
 * 为什么需要这个模块：
 *   1. 正文此前由 Shadow DOM 注入通用 `github-markdown-css`（外加 highlight.js 的
 *      github.css），跟五色体系没关系；换肤只能换 `<link>`，切暗色会重新加载样式导致
 *      白块/跳版。这里改成**自有主题 + 语义 token**：shadow 内部的元素从宿主继承
 *      CSS 变量，站点根类切换明暗时颜色瞬间生效，不换链接。
 *   2. 主题取值可测。`resolveMarkdownTheme()` 把 tokens.css 里的 `var(--jelly-*)`
 *      间接引用解析成真实色值，单测可以断言"明暗两套取值确实翻转"，也给 #12 的暗色
 *      收口提供同一份接口。
 *
 * 边界：本模块是纯函数（不 import Vue / Nuxt），CSS 源在 `app/assets/css/markdown-theme.css`。
 */

export type MarkdownThemeMode = "light" | "dark";

/** 角色 → 语义 token 名。组件 CSS 只消费 `--color-*`，这里只是给测试/#12 的稳定接口。 */
export const MARKDOWN_THEME_ROLES = {
  body: "--color-ink",
  bodyMuted: "--color-ink-muted",
  paper: "--color-surface",
  paperRaised: "--color-surface-raised",
  subtle: "--color-bg-subtle",
  page: "--color-bg-page",
  line: "--color-line",
  lineStrong: "--color-line-strong",
  lineFaint: "--color-line-faint",
  accent: "--color-focus",
  highlight: "--color-highlight",
  danger: "--color-danger",
  dangerInk: "--color-danger-ink",
  dangerSoft: "--color-danger-soft",
  dangerLine: "--color-danger-line",
  focusSoft: "--color-focus-soft",
  onStrong: "--color-on-strong",
  onStrongMuted: "--color-on-strong-muted",
} as const;

export type MarkdownThemeRole = keyof typeof MARKDOWN_THEME_ROLES;

export interface TokenBlockPair {
  light: Record<string, string>;
  dark: Record<string, string>;
}

const DECLARATION_RE = /(--[a-z0-9-]+)\s*:\s*([^;]+);/giu;

function readDeclarations(cssText: string, selectorPattern: RegExp): Record<string, string> {
  const block = selectorPattern.exec(cssText);
  if (!block) {
    return {};
  }

  const declarations: Record<string, string> = {};
  for (const match of block[1]!.matchAll(DECLARATION_RE)) {
    declarations[match[1]!.trim()] = match[2]!.replace(/\s+/gu, " ").trim();
  }
  return declarations;
}

/**
 * 把 tokens.css 拆成明/暗两份取值表。
 *
 * 语义层在 `:root`（浅色）与 `.dark, .theme-dark`（暗色）各写一份；原始层
 * （`--jelly-*`）只在 `:root` 里，暗色取值通过 `var(--jelly-*)` 引用它 —— 解析时会回退。
 */
export function parseTokenBlocks(cssText: string): TokenBlockPair {
  return {
    light: readDeclarations(cssText, /:root\s*\{([\s\S]*?)\n\s*\}/u),
    dark: readDeclarations(cssText, /(?:\.dark|\.theme-dark)[^\n{]*\{([\s\S]*?)\n\s*\}/u),
  };
}

/**
 * 解析一个自定义属性的最终值（支持 `var(--x)` 递归引用）。
 *
 * @throws 未知 token / 循环引用 —— 静默回落到默认色正是这次重构要消灭的问题。
 */
export function resolveTokenValue(
  blocks: TokenBlockPair,
  mode: MarkdownThemeMode,
  name: string,
  seen: string[] = [],
): string {
  if (seen.includes(name)) {
    throw new Error(`token 循环引用：${[...seen, name].join(" -> ")}`);
  }

  const raw = blocks[mode][name] ?? blocks.light[name];
  if (!raw) {
    throw new Error(`未知 token：${name}`);
  }

  const reference = /^var\(\s*(--[a-z0-9-]+)\s*\)$/u.exec(raw);
  if (reference) {
    return resolveTokenValue(blocks, mode, reference[1]!, [...seen, name]);
  }

  return raw;
}

/** 解析出 markdown 主题在指定模式下的全部真实色值（明暗两套）。 */
export function resolveMarkdownTheme(
  cssText: string,
  mode: MarkdownThemeMode,
): Record<MarkdownThemeRole, string> {
  const blocks = parseTokenBlocks(cssText);
  const resolved = {} as Record<MarkdownThemeRole, string>;

  for (const [role, tokenName] of Object.entries(MARKDOWN_THEME_ROLES)) {
    resolved[role as MarkdownThemeRole] = resolveTokenValue(blocks, mode, tokenName);
  }

  return resolved;
}

export const MARKDOWN_THEME_CSS_HEADER = `/* markdown 主题（JellyBin 自有）：由 app/utils/markdownTheme.ts 构建，注入 shadow root。
 * 颜色只消费语义 token（--color-*），明暗切换靠 token 的第二份取值自动生效。 */`;

/**
 * 把主题 CSS 源包成可直接塞进 `<style>` 的字符串。
 *
 * 注入点只有 Shadow DOM（见 MarkdownShadowPreview.vue），样式不泄漏到页面其他部分。
 */
export function buildMarkdownThemeCss(baseCss: string): string {
  return `${MARKDOWN_THEME_CSS_HEADER}\n${baseCss.trim()}\n`;
}

/**
 * 主题 CSS 源：字符串常量而不是 .css 文件。
 *
 * 原因很实际：这个样式只会注入 Shadow DOM，不能进全局样式表；而 Vitest 默认
 * 不处理 CSS 导入（`?raw` / `?inline` 都会返回空串），.css 文件会让"组件挂载后
 * style 里到底有没有主题"这类冒烟测试永远看不到内容。放成字符串后，单测与运行时
 * 读到的是同一份 CSS。
 */
export const MARKDOWN_THEME_CSS = `/* ============================================================================
 * 正文 markdown 主题（JellyBin 自有）：注入 Shadow DOM，样式不泄漏到页面其他部分。
 *
 * 取代原来的 github-markdown-css + highlight.js github.css：
 *   - 颜色只消费 \`app/assets/css/tokens.css\` 的语义层（--color-*），
 *     明暗切换靠同一组 token 的第二份取值自动生效 —— 不用换 <link>，不闪白、不跳版；
 *   - 排版只消费 --font-* / --type-*；
 *   - 本文件由 \`app/utils/markdownTheme.ts#buildMarkdownThemeCss\` 包成可注入的字符串。
 *
 * 约束：颜色只有一个来源。这里**不允许出现裸 hex / rgba**（漂移门禁同款规则），
 * 也不允许 @keyframes / 滚动编排；动作只到 hover/focus 的颜色过渡。
 * ========================================================================== */

.markdown-body {
  color: var(--color-ink);
  background: transparent;
  font-family: var(--font-sans);
  font-size: 17px;
  line-height: var(--type-leading-prose);
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

/* ---- 标题：h2 下方 1px 结构线；h3 用包豪斯黄做记号笔侧标 ---- */
.markdown-body h1,
.markdown-body h2,
.markdown-body h3,
.markdown-body h4,
.markdown-body h5,
.markdown-body h6 {
  margin: 2.2em 0 0.7em;
  color: var(--color-ink);
  font-family: var(--font-sans);
  font-weight: var(--type-weight-title);
  line-height: var(--type-leading-title);
  letter-spacing: var(--type-tracking-title);
}

.markdown-body h1 {
  font-size: 1.9rem;
}

.markdown-body h2 {
  padding-bottom: 0.35em;
  border-bottom: 1px solid var(--color-line);
  font-size: 1.55rem;
}

.markdown-body h3 {
  padding-left: 0.7em;
  border-left: 3px solid var(--color-highlight);
  font-size: 1.28rem;
}

.markdown-body h4 {
  font-size: 1.1rem;
}

.markdown-body h5,
.markdown-body h6 {
  font-size: 1rem;
  color: var(--color-ink-muted);
}

/* ---- 正文块 ---- */
.markdown-body p {
  margin: 0 0 1.15em;
}

.markdown-body strong {
  color: var(--color-ink);
  font-weight: 700;
}

.markdown-body em {
  font-style: italic;
}

.markdown-body del {
  color: var(--color-ink-muted);
  text-decoration: line-through;
  text-decoration-color: var(--color-danger-ink);
}

/* 下划线（markdown-it-ins 的 ++文字++）：包豪斯黄只做底，文字仍是墨色 */
.markdown-body ins {
  background: var(--color-highlight);
  color: var(--color-ink);
  text-decoration: none;
}

.markdown-body mark {
  background: var(--color-highlight);
  color: var(--color-ink);
}

.markdown-body a {
  color: var(--color-focus);
  text-decoration: underline;
  text-decoration-thickness: 1px;
  text-underline-offset: 0.2em;
  transition:
    color var(--motion-fast) var(--motion-ease),
    text-decoration-color var(--motion-fast) var(--motion-ease);
}

.markdown-body a:hover {
  color: var(--color-danger-ink);
  text-decoration-thickness: 2px;
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

.markdown-body li::marker {
  color: var(--color-ink-muted);
}

/* ---- 引用：1px 蓝色结构线 + 次级纸面 ---- */
.markdown-body blockquote {
  margin: 1.5em 0;
  padding: 0.75em 1.1em;
  border-left: 4px solid var(--color-line-strong);
  background: var(--color-bg-subtle);
  color: var(--color-ink-muted);
}

.markdown-body blockquote > :last-child {
  margin-bottom: 0;
}

.markdown-body blockquote strong {
  color: var(--color-ink);
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
  border-left-color: var(--color-focus);
  background: var(--color-focus-soft);
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

/* ---- 代码：行内 / 代码块 / highlight.js token ---- */
.markdown-body code {
  padding: 0.15em 0.4em;
  border: 1px solid var(--color-line-faint);
  background: var(--color-bg-subtle);
  color: var(--color-ink);
  font-family: var(--font-mono);
  font-size: 0.9em;
  line-height: 1.6;
}

.markdown-body pre {
  margin: 1.5em 0;
  padding: 1rem 1.15rem;
  overflow-x: auto;
  border: 1px solid var(--color-line);
  background: var(--color-bg-subtle);
  tab-size: 4;
}

.markdown-body pre code {
  display: block;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-ink);
  font-size: 14px;
  line-height: 1.65;
  white-space: pre;
}

.markdown-body .hljs {
  color: var(--color-ink);
  background: transparent;
}

.markdown-body .hljs-comment,
.markdown-body .hljs-quote {
  color: var(--color-ink-muted);
  font-style: italic;
}

.markdown-body .hljs-keyword,
.markdown-body .hljs-selector-tag,
.markdown-body .hljs-literal,
.markdown-body .hljs-built_in,
.markdown-body .hljs-symbol {
  color: var(--color-danger-ink);
  font-weight: 700;
}

.markdown-body .hljs-string,
.markdown-body .hljs-attr,
.markdown-body .hljs-template-tag {
  color: var(--color-danger-ink);
}

.markdown-body .hljs-number,
.markdown-body .hljs-attr-value,
.markdown-body .hljs-link,
.markdown-body .hljs-regexp {
  color: var(--color-focus);
  font-weight: 600;
}

.markdown-body .hljs-title,
.markdown-body .hljs-title.class_,
.markdown-body .hljs-function .hljs-title,
.markdown-body .hljs-section,
.markdown-body .hljs-selector-id,
.markdown-body .hljs-selector-class {
  color: var(--color-focus);
  font-weight: 600;
}

.markdown-body .hljs-variable,
.markdown-body .hljs-params,
.markdown-body .hljs-attribute,
.markdown-body .hljs-template-variable {
  color: var(--color-ink-muted);
}

.markdown-body .hljs-meta,
.markdown-body .hljs-doctag {
  color: var(--color-ink-muted);
  font-weight: 600;
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

/* ---- 表格：1px 结构线 + 暖灰表头 + 极淡斑马纹；窄屏横向滚动 ---- */
.markdown-body table {
  display: block;
  width: 100%;
  margin: 1.5em 0;
  overflow-x: auto;
  border-collapse: collapse;
  font-size: 0.94em;
}

.markdown-body thead {
  border-bottom: 2px solid var(--color-line);
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

.markdown-body .mermaid {
  margin: 1.5em 0;
  overflow-x: auto;
}

.markdown-body .mermaid svg {
  max-width: 100%;
  height: auto;
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

/* ---- 移动端：正文略小、代码块仍可横滚，表格不撑破容器 ---- */
@media (max-width: 640px) {
  .markdown-body {
    font-size: 16px;
  }

  .markdown-body h1 {
    font-size: 1.6rem;
  }

  .markdown-body h2 {
    font-size: 1.35rem;
  }

  .markdown-body h3 {
    font-size: 1.18rem;
  }

  .markdown-body pre {
    padding: 0.9rem;
  }

  .markdown-body pre code {
    font-size: 13px;
  }
}
`;
