import katex from "katex";
import type MarkdownIt from "markdown-it";

import { errorMessage } from "~/utils/errorMessage";

/**
 * 正文数学公式（`$…$` 行内、`$$…$$` 块级）。
 *
 * # 为什么自己写而不用 `markdown-it-katex`
 *
 * 那个包停在 2019 年，`dependencies` 里写死 `katex@^0.6.0` —— pnpm 会为此**另装一份
 * 0.6**，于是页面上出现了两件坏事：
 *
 * 1. 渲染用的是 0.6 的富文本结构（`.base textstyle uncramped` / `.mathit` /
 *    `fontsize-ensurer`），而站点加载的 CSS 是 0.16 的 —— 两代类名对不上，公式排版塌成
 *    一排散字；这就是"公式看起来像乱码"的真因；
 * 2. 0.6 不认识 `\tag`（正文里每个 `$$` 公式都带 `\tag{3.x}`），解析直接抛错，而那个包
 *    在 catch 里 **把 LaTeX 源码原样吐回页面** —— 读者看到的就是 `y = \sigma(\vec{w}^T
 *    \cdot \vec{x}) \quad \tag{3.1}` 这种源码。
 *
 * 这里不复刻它的分隔符判定，而是用 markdown-it 自己的 `ruler` 挂两个规则，渲染统一走
 * 站点依赖的 `katex@0.16`。解析失败时**不再吐源码**，而是给一段带 `math-error` 标记的
 * 降级文本：作者能看出"这里有问题"，读者也不会被 LaTeX 源码糊一脸。
 */

/** markdown-it 内部状态的最小结构：只声明本插件真正用到的字段。 */
interface InlineState {
  src: string;
  pos: number;
  posMax: number;
  pending: string;
  push(type: string, tag: string, nesting: number): MathToken;
}

interface BlockState {
  src: string;
  bMarks: number[];
  eMarks: number[];
  tShift: number[];
  blkIndent: number;
  line: number;
  push(type: string, tag: string, nesting: number): MathToken;
  getLines(begin: number, end: number, indent: number, keepLastLF: boolean): string;
}

interface MathToken {
  markup: string;
  content: string;
  /** 块级 token 必须置真，否则 markdown-it 会把它当行内内容包进 `<p>`。 */
  block?: boolean;
}

/** markdown-it 的规则集合（只在类型声明里补这一小块，不改第三方包）。 */
interface MarkdownItRulerHost {
  inline: { ruler: { after(name: string, rule: string, fn: MathRule): void } };
  block: { ruler: { after(name: string, rule: string, fn: BlockRule, options?: object): void } };
  renderer: { rules: Record<string, MathRenderer> };
}

type MathRule = (state: InlineState, silent: boolean) => boolean;
type BlockRule = (state: BlockState, start: number, end: number, silent: boolean) => boolean;
type MathRenderer = (tokens: Array<{ content: string }>, idx: number) => string;

const DOLLAR = 0x24;
const DIGIT_0 = 0x30;
const DIGIT_9 = 0x39;

function isSpace(code: number): boolean {
  return code === 0x20 || code === 0x09;
}

function isDigit(code: number): boolean {
  return code >= DIGIT_0 && code <= DIGIT_9;
}

function escapeHtml(raw: string): string {
  return raw
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/**
 * 把一段 LaTeX 渲染成 KaTeX 的 HTML。
 *
 * `strict: "ignore"`：正文里有中文 `\text{}`、`\ ` 之类的宽松写法，0.16 默认会往
 * 服务端日志刷几百条警告；这里只要结果正确，不评审写法。
 *
 * 失败时降级成 `.math-error`：保留源码文本（读者至少能看懂式子），但带上虚线标记与
 * `title`，让"这里有语法错误"变得可诊断 —— 之前的做法是把源码当正文吐出来，谁也看不出
 * 是该改文章还是该改渲染器。
 */
export function renderMath(latex: string, displayMode: boolean): string {
  try {
    return katex.renderToString(latex, {
      displayMode,
      throwOnError: true,
      strict: "ignore",
      trust: false,
    });
  } catch (error) {
    const message = errorMessage(error, "公式解析失败");
    const tone = displayMode ? " math-error--display" : "";
    return `<span class="math-error${tone}" role="note" title="${escapeHtml(message)}">${escapeHtml(
      latex,
    )}</span>`;
  }
}

/** 行内 `$…$`。 */
function mathInline(state: InlineState, silent: boolean): boolean {
  if (state.src.charCodeAt(state.pos) !== DOLLAR) return false;

  // 开括号：后面不能是空白（`$ x$` 不算公式），也不能是另一个 `$`（那是块级 `$$`）。
  const openNext = state.pos + 1 <= state.posMax ? state.src.charCodeAt(state.pos + 1) : -1;
  if (openNext === DOLLAR || openNext === -1 || isSpace(openNext)) return false;

  const start = state.pos + 1;
  let search = start;
    let close = -1;

  while (search < state.posMax) {
    const found = state.src.indexOf("$", search);
    if (found === -1) break;
    // `\$` 是字面的美元符号，跳过。
    if (found > 0 && state.src[found - 1] === "\\") {
      search = found + 1;
      continue;
    }
    const prev = found > 0 ? state.src.charCodeAt(found - 1) : -1;
    const after = found + 1 <= state.posMax ? state.src.charCodeAt(found + 1) : -1;
    // 闭括号：前面不能是空白，紧跟数字也不行（否则 `$5 和 $10` 会被当成公式）。
    if (!isSpace(prev) && !isDigit(after)) {
      close = found;
      break;
    }
    search = found + 1;
  }

  if (close === -1) return false;
  const content = state.src.slice(start, close);
  if (!content.trim()) return false;

  if (!silent) {
    const token = state.push("math_inline", "math", 0);
    token.markup = "$";
    token.content = content;
  }
  state.pos = close + 1;
  return true;
}

/** 块级 `$$…$$`：单行形式与多行形式都支持。 */
function mathBlock(state: BlockState, start: number, end: number, silent: boolean): boolean {
  const lineStart = (state.bMarks[start] ?? 0) + (state.tShift[start] ?? 0);
  const lineEnd = state.eMarks[start] ?? 0;
  if (lineStart + 2 > lineEnd) return false;
  if (state.src.slice(lineStart, lineStart + 2) !== "$$") return false;

  const opened = state.src.slice(lineStart + 2, lineEnd).trim();
  let content: string;
  let nextLine: number;

  if (opened.endsWith("$$")) {
    // `$$ x = 1 $$` 单行写法
    content = opened.slice(0, -2).trim();
    nextLine = start + 1;
  } else {
    // 多行：往下找到以 `$$` 收尾的那一行
    let closing = -1;
    let closingStart = 0;
    for (let line = start + 1; line < end; line += 1) {
      const from = (state.bMarks[line] ?? 0) + (state.tShift[line] ?? 0);
      const to = state.eMarks[line] ?? 0;
      const text = state.src.slice(from, to);
      if (!text.trimEnd().endsWith("$$")) continue;
      closing = line;
      closingStart = from + text.trimEnd().length - 2;
      break;
    }
    if (closing === -1) return false;

    const head = opened ? `${opened}\n` : "";
    const body = state.getLines(start + 1, closing, state.tShift[start] ?? 0, true);
    const closingLineStart = (state.bMarks[closing] ?? 0) + (state.tShift[closing] ?? 0);
    const tail = state.src.slice(closingLineStart, closingStart).trim();
    content = `${head}${body}${tail}`;
    nextLine = closing + 1;
  }

  if (!content.trim()) return false;
  if (silent) return true;

  const token = state.push("math_block", "math", 0);
  token.block = true;
  token.markup = "$$";
  token.content = content;
  state.line = nextLine;
  return true;
}

/** 注册到 markdown-it 实例上。 */
export function markdownMath(md: MarkdownIt): void {
  const host = md as unknown as MarkdownItRulerHost;

  host.inline.ruler.after("escape", "math_inline", mathInline);
  host.block.ruler.after("blockquote", "math_block", mathBlock, {
    alt: ["paragraph", "reference", "blockquote", "list"],
  });

  host.renderer.rules.math_inline = (tokens, idx) => renderMath(tokens[idx]?.content ?? "", false);
  host.renderer.rules.math_block = (tokens, idx) => `${renderMath(tokens[idx]?.content ?? "", true)}\n`;
}

/** `$$` 分隔行最多允许的跨度：再长就当成"漏了闭括号"，别一口气吞掉半篇文章。 */
const MAX_BLOCK_SPAN = 60;

/** 行首的容器前缀（引用 `>` 与缩进，可重复）：`>    $$` → 前缀 `>    `。 */
const CONTAINER_PREFIX = /^(?:\s*>\s*)*\s*/u;

/**
 * 把 `$$…$$` 归一成**单行**写法，再交给 markdown-it。
 *
 * 真实正文里常见两种写坏的分隔：`$$` 开在引用 / 列表里（`>    $$`），闭的却在行首
 * （`$$`）——按 Markdown 的容器语义，闭括号那一行**不属于**引用，于是引用里没有闭
 * 括号，后面的裸 `$$` 会去配对下一个公式，把中间整段正文当成公式内容吞掉：渲染结果
 * 就是一堆 `KaTeX parse error … got '#'`（连标题都被吃进去了）。
 *
 * 这里不改写作习惯，只按"分隔行成对出现"的直觉把它们配对：取出两行之间的内容、
 * 去掉每行的引用/缩进前缀、拼成一行 `$$ … $$`。KaTeX 本来就接受单行写法，
 * `\begin{aligned} a &= b \\ c &= d \end{aligned}` 也在同一行里成立。
 */
export function normalizeMathBlocks(source: string): string {
  const lines = source.split("\n");
  const out: string[] = [];

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i] ?? "";
    if (line.replace(CONTAINER_PREFIX, "").trim() !== "$$") {
      out.push(line);
      continue;
    }

    // 找到下一个分隔行；找不到（或跨度过大）就原样留下，交给 markdown-it 当普通文本。
    let closing = -1;
    for (let j = i + 1; j < lines.length && j - i <= MAX_BLOCK_SPAN; j += 1) {
      if ((lines[j] ?? "").replace(CONTAINER_PREFIX, "").trim() === "$$") {
        closing = j;
        break;
      }
    }
    if (closing === -1) {
      out.push(line);
      continue;
    }

    const prefix = line.match(CONTAINER_PREFIX)?.[0] ?? "";
    const body = lines
      .slice(i + 1, closing)
      .map((inner) => (inner ?? "").replace(/^(?:\s*>\s*)+/u, "").trim())
      .filter((inner) => inner.length > 0)
      .join(" ");
    if (!body) {
      // 空公式：原样留着，交给 KaTeX 报错。
      out.push(line, ...lines.slice(i + 1, closing));
    } else {
      out.push(`${prefix}$$ ${body} $$`);
    }
    i = closing;
  }

  return out.join("\n");
}
