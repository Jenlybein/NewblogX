import MarkdownIt from "#markdown-it";
import markdownItKatex from "markdown-it-katex";
import markdownItIns from "#markdown-it-ins";
import hljs from "highlight.js/lib/core";
import bash from "highlight.js/lib/languages/bash";
import css from "highlight.js/lib/languages/css";
import diff from "highlight.js/lib/languages/diff";
import go from "highlight.js/lib/languages/go";
import javascript from "highlight.js/lib/languages/javascript";
import json from "highlight.js/lib/languages/json";
import markdownLanguage from "highlight.js/lib/languages/markdown";
import plaintext from "highlight.js/lib/languages/plaintext";
import python from "highlight.js/lib/languages/python";
import sql from "highlight.js/lib/languages/sql";
import typescript from "highlight.js/lib/languages/typescript";
import xml from "highlight.js/lib/languages/xml";
import yaml from "highlight.js/lib/languages/yaml";
import { computed, toRef, toValue } from "vue";
import type { MaybeRefOrGetter } from "vue";
import DOMPurify from "isomorphic-dompurify";

import { splitNoticeSegments } from "~/utils/markdownNotice";
import type { NoticeSegment } from "~/utils/markdownNotice";

export interface ArticleHeadingAnchor {
  id: string;
  href: string;
  title: string;
  level: number;
}

interface RenderedArticleMarkdown {
  html: string;
  headings: ArticleHeadingAnchor[];
}

const registeredLanguages = [
  ["bash", bash],
  ["sh", bash],
  ["shell", bash],
  ["css", css],
  ["diff", diff],
  ["go", go],
  ["javascript", javascript],
  ["js", javascript],
  ["json", json],
  ["markdown", markdownLanguage],
  ["md", markdownLanguage],
  ["plaintext", plaintext],
  ["text", plaintext],
  ["python", python],
  ["py", python],
  ["sql", sql],
  ["typescript", typescript],
  ["ts", typescript],
  ["html", xml],
  ["xml", xml],
  ["vue", xml],
  ["yaml", yaml],
  ["yml", yaml],
] as const;

for (const [name, language] of registeredLanguages) {
  hljs.registerLanguage(name, language);
}

function escapeHtml(raw: string) {
  return raw
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

const markdown = new MarkdownIt({
  breaks: true,
  linkify: true,
  // 必须开 HTML：正文里的内联 SVG 是原始 HTML，关着就只会被转义成源码文本。
  //
  // 放行原始 HTML 等于把注入面交给渲染层，所以下面**必须**过一遍消毒
  // （sanitizeArticleHtml），两者是配套的一对，不能只改一边。
  html: true,
  highlight(code, language) {
    if (language && hljs.getLanguage(language)) {
      const highlighted = hljs.highlight(code, { language, ignoreIllegals: true }).value;
      return `<pre><code class="hljs language-${language}">${highlighted}</code></pre>`;
    }

    const highlighted = hljs.highlightAuto(code).value;
    return `<pre><code class="hljs">${highlighted}</code></pre>`;
  },
});
markdown.use(markdownItKatex);
markdown.use(markdownItIns);
const markdownRuntime = markdown as MarkdownIt & {
  parse: (source: string, env: Record<string, unknown>) => Array<{
    type: string;
    tag: string;
    info?: string;
    content: string;
    attrSet: (name: string, value: string) => void;
  }>;
  renderer: {
    render: (tokens: unknown[], options: unknown, env: Record<string, unknown>) => string;
    rules: {
      fence?: (
        tokens: Array<{ info?: string; content: string }>,
        idx: number,
        options: unknown,
        env: unknown,
        self: { renderToken: (tokens: unknown[], idx: number, options: unknown) => string },
      ) => string;
    };
  };
  options: unknown;
};

const defaultFenceRenderer = markdownRuntime.renderer.rules.fence?.bind(markdownRuntime.renderer.rules);markdownRuntime.renderer.rules.fence = (tokens, idx, options, env, self) => {
  const token = tokens[idx];
  if (!token) {
    return "";
  }
  const language = token?.info?.trim().split(/\s+/u)[0]?.toLowerCase();
  if (language === "mermaid") {
    return `<div class="mermaid">${escapeHtml(token.content || "")}</div>\n`;
  }
  if (defaultFenceRenderer) {
    return defaultFenceRenderer(tokens, idx, options, env, self);
  }
  return self.renderToken(tokens as unknown[], idx, options);
};

// SVG 里刻意禁止的面。
//
// 开了 html:true 之后正文里的原始 HTML 会原样进入渲染结果，所以必须消毒。
// DOMPurify 默认已拦掉所有 on* 事件属性与 javascript: 协议，但它的 SVG
// 允许清单里**包含**特效与装载面，这些要显式禁掉：
//   - foreignObject：能装任意 HTML（等于绕过整个 HTML 白名单）
//   - animate* / filter / mask / clipPath：动画与滤镜，是混淆攻击的常见载体
//   - image / symbol / switch / textPath / metadata：能让 SVG 引用外部资源
const FORBIDDEN_SVG_TAGS = [
  "foreignObject",
  "animate",
  "animateMotion",
  "animateTransform",
  "filter",
  "mask",
  "clipPath",
  "image",
  "symbol",
  "switch",
  "textPath",
  "metadata",
];

/**
 * 消毒正文 HTML。
 *
 * # 为什么有这一层
 *
 * markdown-it 开 `html: true` 后会把正文里的原始 HTML 原样输出。正文是
 * "作者可写、所有读者都会执行"的位置，不消毒等于把存储型 XSS 留给读者。
 *
 * 用 DOMPurify 的 html+svg 档位：它自带一份经过维护的完整允许清单，
 * 比手写全清单更不容易漏；我们只在它之上**收窄**（禁掉 SVG 特效与装载面）。
 * 这也符合 ADR-0021「严格白名单，只放行绘图元素」的口径。
 *
 * SSR 与浏览器都要跑，所以用 isomorphic 版本（Node 侧自带 DOM 实现）。
 */
function sanitizeArticleHtml(html: string): string {
  return DOMPurify.sanitize(html, {
    USE_PROFILES: { html: true, svg: true, svgFilters: false },
    FORBID_TAGS: FORBIDDEN_SVG_TAGS,
  });
}

function createHeadingId(title: string, serial: number, usedIds: Map<string, number>) {  const normalized = title
    .toLowerCase()
    .trim()
    .replace(/[^\p{L}\p{N}\u4e00-\u9fa5]+/gu, "-")
    .replace(/^-+|-+$/g, "");

  const base = normalized || `section-${serial}`;
  const currentCount = usedIds.get(base) || 0;
  usedIds.set(base, currentCount + 1);
  return currentCount === 0 ? base : `${base}-${currentCount + 1}`;
}

interface RenderState {
  headings: ArticleHeadingAnchor[];
  usedIds: Map<string, number>;
  headingSerial: number;
}

/** 给标题 token 补 id 并登记目录项（h1~h4）。 */
function collectHeadings(tokens: ReturnType<typeof markdownRuntime.parse>, state: RenderState) {
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (!token || token.type !== "heading_open") {
      continue;
    }

    const inlineToken = tokens[index + 1];
    if (!inlineToken || inlineToken.type !== "inline") {
      continue;
    }

    state.headingSerial += 1;
    const level = Number(token.tag.slice(1));
    const title = inlineToken.content.trim() || `章节 ${state.headingSerial}`;
    const id = createHeadingId(title, state.headingSerial, state.usedIds);
    token.attrSet("id", id);

    if (level >= 1 && level <= 4) {
      state.headings.push({
        id,
        href: `#${id}`,
        title,
        level,
      });
    }
  }
}

function renderSegment(segment: NoticeSegment, state: RenderState): string {
  const tokens = markdownRuntime.parse(segment.source, {});
  collectHeadings(tokens, state);
  const rendered = markdownRuntime.renderer.render(tokens, markdownRuntime.options, {});

  if (segment.type === "markdown") {
    return rendered;
  }

  const title = segment.title
    ? `<p class="markdown-notice__title">${escapeHtml(segment.title)}</p>`
    : "";

  return `<div class="markdown-notice markdown-notice--${segment.tone}">${title}${rendered}</div>\n`;
}

/**
 * 把 markdown 源渲染成正文 HTML（导出供单测直接断言，不必挂载页面）。
 *
 * 提示块走 `splitNoticeSegments` 切分后逐段渲染；标题锚点在两条路径上都收集，
 * 保证提示块里的标题也能进目录。
 */
export function renderArticleMarkdown(source: string): RenderedArticleMarkdown {
  if (!source.trim()) {
    return {
      html: "<p>暂无正文内容。</p>",
      headings: [],
    };
  }

  const normalizedSource = source.replace(/<u>([\s\S]*?)<\/u>/gi, "++$1++");
  const state: RenderState = {
    headings: [],
    usedIds: new Map<string, number>(),
    headingSerial: 0,
  };

  const html = splitNoticeSegments(normalizedSource)
    .map((segment) => renderSegment(segment, state))
    .join("");

  return {
    // 所有渲染路径（含提示块、mermaid、katex）都经过同一次消毒，
    // 避免某条分支漏掉。
    html: sanitizeArticleHtml(html),
    headings: state.headings,
  };
}

export function useArticleMarkdown(source: MaybeRefOrGetter<string | null | undefined>) {
  const sourceRef = toRef(source);

  const parsed = computed(() => renderArticleMarkdown(toValue(sourceRef) || ""));

  return {
    renderedHtml: computed(() => parsed.value.html),
    headings: computed(() => parsed.value.headings),
  };
}
