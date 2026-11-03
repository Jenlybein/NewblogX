/**
 * 正文 markdown 主题：明暗两份取值 + token 解析 + 样式字符串的包装器。
 *
 * 为什么需要这个模块：
 *   1. 正文此前由 Shadow DOM 注入通用 `github-markdown-css`（外加 highlight.js 的
 *      github.css），跟五色体系没关系；换肤只能换 `<link>`，切暗色会重新加载样式导致
 *      白块/跳版。这里改成**自有主题 + 语义 token**：shadow 内部的元素从宿主继承
 *      CSS 变量，站点根类切换明暗时颜色瞬间生效，不换链接。
 *   2. 主题取值可测。`resolveMarkdownTheme()` 把 tokens.css 里的 `var(--color-*)` /
 *      `var(--md-*)` 间接引用解析成真实色值，单测可以断言"明暗两套取值确实翻转"，
 *      也给 #12 的暗色收口提供同一份接口。
 *
 * 边界：本模块是纯函数（不 import Vue / Nuxt）。样式源本身在
 * `app/utils/markdownThemeCss.ts`（约 570 行，受「单文件 ≤ 800 行」约束外移）。
 */

export type MarkdownThemeMode = "light" | "dark";

/** 角色 → 语义 token 名。组件 CSS 只消费 token，这里只是给测试/#12 的稳定接口。 */
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
  link: "--color-link",
  highlight: "--color-highlight",
  danger: "--color-danger",
  dangerInk: "--color-danger-ink",
  dangerSoft: "--color-danger-soft",
  dangerLine: "--color-danger-line",
  onStrong: "--color-on-strong",
  onStrongMuted: "--color-on-strong-muted",

  /* ---- 正文色板（`--md-*`，ADR-0012 后续修订）----
   * 这一组**只服务正文渲染区**：六层标题 + 四种内联 + 列表标记 + 代码七档。
   * 它不受五色相锁定，但也只有 token 层一个来源 —— 组件/样式里不许出现裸色值。 */
  heading1: "--md-heading-h1",
  heading2: "--md-heading-h2",
  heading3: "--md-heading-h3",
  heading4: "--md-heading-h4",
  heading5: "--md-heading-h5",
  heading6: "--md-heading-h6",
  boldInk: "--md-bold-ink",
  italicInk: "--md-italic-ink",
  strikeInk: "--md-strike-ink",
  strikeLine: "--md-strike-line",
  underlineLine: "--md-underline-line",
  listOrderedMarker: "--md-list-ordered-marker",
  listUnorderedMarker: "--md-list-unordered-marker",
  codePaper: "--md-code-paper",
  codeInk: "--md-code-ink",
  codeKeyword: "--md-code-keyword",
  codeString: "--md-code-string",
  codeNumber: "--md-code-number",
  codeFunction: "--md-code-function",
  codeType: "--md-code-type",
  codeComment: "--md-code-comment",
  codeVariable: "--md-code-variable",
} as const;

export type MarkdownThemeRole = keyof typeof MARKDOWN_THEME_ROLES;

export interface TokenBlockPair {
  light: Record<string, string>;
  dark: Record<string, string>;
}

const DECLARATION_RE = /(--[a-z0-9-]+)\s*:\s*([^;]+);/giu;

/** 三元组派生写法：`rgb(var(--jelly-*-rgb) / α)`（#18 起语义层的统一写法）。 */
const TRIPLET_REFERENCE_RE = /^rgb\(\s*var\(\s*(--[a-z0-9-]+)\s*\)\s*\/\s*([\d.]+)\s*\)$/u;

/** 三元组基底的真实取值：形如 `0 47 167`（空格分隔的 RGB 三通道）。 */
const TRIPLET_CHANNELS_RE = /^(\d{1,3})\s+(\d{1,3})\s+(\d{1,3})$/u;

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

  // #18 起带透明度的语义色由 `--jelly-*-rgb` 三元组派生。对外仍返回"真实色值"
  // （rgba 字符串），这样对比度计算与 mermaid themeVariables 不必认识 var()。
  // 注意这不是写死常量：通道值来自 token 层，改色相时这里自动跟着变。
  const triplet = TRIPLET_REFERENCE_RE.exec(raw);
  if (triplet) {
    const base = resolveTokenValue(blocks, mode, triplet[1]!, [...seen, name]);
    const channels = TRIPLET_CHANNELS_RE.exec(base);
    if (!channels) {
      throw new Error(`token ${triplet[1]} 不是 "R G B" 三元组基底：${base}`);
    }
    return `rgba(${channels[1]}, ${channels[2]}, ${channels[3]}, ${triplet[2]})`;
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

/**
 * mermaid themeVariables 的角色 → 语义 token 名。
 *
 * mermaid 把配色写死在自己的 themeVariables 里（theme: "base" 下不传就是第三方默认
 * 浅紫），而它渲染出的 SVG 用的是内联样式 —— 既消费不了 var(--color-*)，也躲开了
 * token 漂移门禁。所以这里从 tokens.css 的语义层把真实色值解析出来再喂给它：每个
 * 变量都由 token 派生，改语义色时图表自动跟随，明暗两套也各拿第二份取值。
 *
 * 约束：这里只写"角色 → token 名"的映射，不出现任何颜色字面量。
 */
export const MERMAID_THEME_ROLES = {
  background: "--color-surface",
  primaryColor: "--color-bg-subtle",
  primaryTextColor: "--color-ink",
  primaryBorderColor: "--color-line-strong",
  secondaryColor: "--color-surface-raised",
  secondaryTextColor: "--color-ink",
  secondaryBorderColor: "--color-line",
  tertiaryColor: "--color-bg-page",
  tertiaryTextColor: "--color-ink",
  tertiaryBorderColor: "--color-line-faint",
  lineColor: "--color-line",
  textColor: "--color-ink",
  mainBkg: "--color-surface",
  nodeBorder: "--color-line-strong",
  clusterBkg: "--color-bg-subtle",
  clusterBorder: "--color-line",
  titleColor: "--color-ink",
  edgeLabelBackground: "--color-surface-raised",
} as const;

export type MermaidThemeVariable = keyof typeof MERMAID_THEME_ROLES;

/**
 * 解析出 mermaid 在某模式下需要的全部 themeVariables（真实色值，明暗两套）。
 *
 * 与 resolveMarkdownTheme 共用同一份 resolveTokenValue —— 这是"图表配色 = 语义 token
 * 的第二张面孔"能成立的原因：token 改一处，正文与图表一起变。
 */
export function resolveMermaidTheme(
  cssText: string,
  mode: MarkdownThemeMode,
): Record<MermaidThemeVariable, string> {
  const blocks = parseTokenBlocks(cssText);
  const resolved = {} as Record<MermaidThemeVariable, string>;

  for (const [variable, tokenName] of Object.entries(MERMAID_THEME_ROLES)) {
    resolved[variable as MermaidThemeVariable] = resolveTokenValue(blocks, mode, tokenName);
  }

  return resolved;
}

/**
 * mermaid initialize 的配置。
 *
 * `theme` 与 `themeVariables` 是可选的：拿不到 token 源（SSR、或调用方没传 CSS 源）
 * 时只保最低限度的安全配置、**不假装染色**——所以这两个字段不能是必填，否则调用点
 * 只能靠塞空对象来满足类型，反而把"没染"伪装成"染过了"。
 * 有 token 源时必须两份都给（见 buildMermaidConfig）：`theme: "base"` 是
 * themeVariables 生效的前提。
 */
export interface MermaidRuntimeConfig {
  startOnLoad: boolean;
  securityLevel: string;
  theme?: string;
  themeVariables?: Record<MermaidThemeVariable, string>;
}

/**
 * 组装 mermaid 的 initialize 配置。
 *
 * 为什么单独出这个函数：传参点只有一处，但"必须带 themeVariables"是 ADR-0012 决策
 * #11 的硬要求 —— 抽成函数后单测可以直接断言这份契约，不需要挂载组件。
 */
export function buildMermaidConfig(
  cssText: string,
  mode: MarkdownThemeMode,
): MermaidRuntimeConfig {
  return {
    startOnLoad: false,
    securityLevel: "strict",
    theme: "base",
    themeVariables: resolveMermaidTheme(cssText, mode),
  };
}

export const MARKDOWN_THEME_CSS_HEADER = `/* markdown 主题（JellyBin 自有）：由 app/utils/markdownTheme.ts 构建，注入 shadow root。
 * 颜色只消费语义 token（纸面/结构线 --color-*，正文色板 --md-*），明暗切换靠 token
 * 的第二份取值自动生效。 */`;

/**
 * 把主题 CSS 源包成可直接塞进 `<style>` 的字符串。
 *
 * 注入点只有 Shadow DOM（见 MarkdownShadowPreview.vue），样式不泄漏到页面其他部分。
 */
export function buildMarkdownThemeCss(baseCss: string): string {
  return `${MARKDOWN_THEME_CSS_HEADER}\n${baseCss.trim()}\n`;
}
