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
  html: false,
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

const defaultFenceRenderer = markdownRuntime.renderer.rules.fence?.bind(markdownRuntime.renderer.rules);
markdownRuntime.renderer.rules.fence = (tokens, idx, options, env, self) => {
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

function createHeadingId(title: string, serial: number, usedIds: Map<string, number>) {
  const normalized = title
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
    html,
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
