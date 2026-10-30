<script setup lang="ts">
import { onMounted, ref, watch } from "vue";

import {
  MARKDOWN_THEME_CSS,
  buildMarkdownThemeCss,
  buildMermaidConfig,
  type MarkdownThemeMode,
} from "~/utils/markdownTheme";

const props = withDefaults(
  defineProps<{
    html: string;
    /**
     * 可选的外部主题（兼容创作区旧的 12 套预览样式 / 私信面板）。
     * 正文阅读页不再传它：自有主题走 token，明暗切换不需要换 <link>。
     */
    themeHref?: string;
    extraStyleHrefs?: string[];
    articleClass?: string;
    /**
     * tokens.css 的源码（mermaid themeVariables 的取值来源）。
     *
     * 为什么要传进来：本组件的消费方横跨正文页 / 私信 / AI 面板 / 创作区预览，
     * 不该为了取 CSS 去 import 具体页面的资源路径，也不该 import uiStore 去猜
     * 当前明暗（那样就把一个纯展示组件和全局 store 焊死了）。缺 SSR 环境的调用方
     * 可以不传，此时 mermaid 不接 token 主题（退回 base 默认配色）。
     */
    tokensCss?: string;
    /** 明暗模式：与站点根类同源，决定 themeVariables 取哪一份取值。 */
    themeMode?: MarkdownThemeMode;
  }>(),
  {
    themeHref: "",
    extraStyleHrefs: () => [],
    articleClass: "markdown-body",
    tokensCss: "",
    themeMode: "light",
  },
);

// 主题 CSS 在模块级构建一次；shadow root 里注入的是字符串，不产生全局样式。
const MARKDOWN_THEME_STYLE = buildMarkdownThemeCss(MARKDOWN_THEME_CSS);

const hostRef = ref<HTMLDivElement | null>(null);

type MermaidRuntime = {
  initialize: (config: ReturnType<typeof buildMermaidConfig>) => void;
  render: (id: string, text: string) => Promise<{
    svg: string;
    bindFunctions?: (element: Element) => void;
  }>;
};

let shadowRootRef: ShadowRoot | null = null;
let themeStyleRef: HTMLStyleElement | null = null;
let themeLinkRef: HTMLLinkElement | null = null;
let articleRef: HTMLElement | null = null;
let extraStyleLinks: HTMLLinkElement[] = [];
let mermaidRef: MermaidRuntime | null = null;
/** 上一次 initialize 用的模式：与当前 props 不同就要重新 initialize（换肤要跟手）。 */
let mermaidModeRef: MarkdownThemeMode | null = null;
let renderTicket = 0;
const MERMAID_CACHE_LIMIT = 120;
const mermaidSvgCache = new Map<string, string>();

function hashMermaidDefinition(source: string) {
  let hash = 2166136261;
  for (let index = 0; index < source.length; index += 1) {
    hash ^= source.charCodeAt(index);
    hash += (hash << 1) + (hash << 4) + (hash << 7) + (hash << 8) + (hash << 24);
  }
  return `m${(hash >>> 0).toString(36)}`;
}

function setCachedMermaidSvg(key: string, svg: string) {
  if (mermaidSvgCache.has(key)) {
    mermaidSvgCache.delete(key);
  }
  mermaidSvgCache.set(key, svg);
  if (mermaidSvgCache.size <= MERMAID_CACHE_LIMIT) {
    return;
  }
  const oldestKey = mermaidSvgCache.keys().next().value;
  if (oldestKey) {
    mermaidSvgCache.delete(oldestKey);
  }
}

function ensureShadowRoot() {
  if (!hostRef.value || shadowRootRef) {
    return;
  }

  shadowRootRef = hostRef.value.attachShadow({ mode: "open" });

  const baseStyle = document.createElement("style");
  baseStyle.textContent = `
    :host {
      all: revert;
      display: block;
      box-sizing: border-box;
      width: 100%;
      min-height: 100%;
    }
    *, *::before, *::after {
      box-sizing: border-box;
    }
  `;

  // 自有 markdown 主题（token 驱动，明暗自动跟随站点根类，不换 <link>）。
  themeStyleRef = document.createElement("style");
  themeStyleRef.dataset.jellyMarkdown = "theme";
  themeStyleRef.textContent = MARKDOWN_THEME_STYLE;

  articleRef = document.createElement("article");
  articleRef.className = props.articleClass;

  shadowRootRef.append(baseStyle, themeStyleRef, articleRef);
}

function syncThemeHref() {
  ensureShadowRoot();
  if (!shadowRootRef || !articleRef) {
    return;
  }

  const href = props.themeHref?.trim() || "";
  if (!href) {
    themeLinkRef?.remove();
    themeLinkRef = null;
    return;
  }

  if (!themeLinkRef) {
    themeLinkRef = document.createElement("link");
    themeLinkRef.rel = "stylesheet";
    themeLinkRef.dataset.jellyMarkdown = "external-theme";
    shadowRootRef.insertBefore(themeLinkRef, articleRef);
  }

  if (themeLinkRef.getAttribute("href") !== href) {
    themeLinkRef.setAttribute("href", href);
  }
}

function syncExtraStyleHrefs() {
  ensureShadowRoot();
  if (!shadowRootRef || !articleRef) {
    return;
  }

  for (const link of extraStyleLinks) {
    link.remove();
  }
  extraStyleLinks = [];

  for (const href of props.extraStyleHrefs) {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = href;
    shadowRootRef.insertBefore(link, articleRef);
    extraStyleLinks.push(link);
  }
}

function syncArticleClass() {
  ensureShadowRoot();
  if (!articleRef) {
    return;
  }
  articleRef.className = props.articleClass;
}

async function syncHtml() {
  ensureShadowRoot();
  if (!articleRef) {
    return;
  }

  const currentArticle = articleRef;
  const nextArticle = document.createElement("article");
  nextArticle.className = props.articleClass;
  nextArticle.innerHTML = props.html;

  const ticket = ++renderTicket;
  await renderMermaid(nextArticle, ticket);

  if (ticket !== renderTicket) {
    return;
  }

  currentArticle.replaceWith(nextArticle);
  articleRef = nextArticle;
}

/**
 * mermaid 的 initialize 配置。
 *
 * 没传 tokensCss（SSR / 拿不到 CSS 源的调用方）时不接 token 主题 —— 只保最低限度的
 * 安全配置，不假装染色。
 */
function mermaidConfig(): ReturnType<typeof buildMermaidConfig> {
  const base = { startOnLoad: false, securityLevel: "strict" };
  if (!props.tokensCss) {
    return base;
  }
  return buildMermaidConfig(props.tokensCss, props.themeMode);
}

async function ensureMermaidInstance() {
  if (typeof window === "undefined") {
    return null;
  }
  if (mermaidRef) {
    return mermaidRef;
  }

  const mermaidModule = await import("~/services/mermaid-runtime.mjs");
  const runtime = mermaidModule.default;
  // mermaid 的配色写死在 SVG 内联样式里，消费不了 var(--color-*)；所以这里是全站唯一
  // 需要"把 token 解析成真色值再喂给第三方"的接缝 —— 由 buildMermaidConfig 负责，
  // 明暗两套各自取第二份取值（跳过了它就是第三方默认浅紫，永远不跟随换肤）。
  runtime.initialize(mermaidConfig());
  mermaidRef = runtime;
  mermaidModeRef = props.themeMode;
  return runtime;
}

async function renderMermaid(targetArticle: HTMLElement, ticket: number) {
  const nodes = Array.from(targetArticle.querySelectorAll<HTMLElement>(".mermaid"));
  if (!nodes.length) {
    return;
  }

  const mermaid = await ensureMermaidInstance();
  if (!mermaid || ticket !== renderTicket) {
    return;
  }

  for (const [index, node] of nodes.entries()) {
    const definition = node.textContent?.trim();
    if (!definition) {
      continue;
    }

    const cacheKey = hashMermaidDefinition(definition);
    const cachedSvg = mermaidSvgCache.get(cacheKey);
    if (cachedSvg) {
      node.innerHTML = cachedSvg;
      continue;
    }

    try {
      const renderId = `studio-mermaid-${ticket}-${index}`;
      const { svg, bindFunctions } = await mermaid.render(renderId, definition);
      if (ticket !== renderTicket) {
        return;
      }
      node.innerHTML = svg;
      setCachedMermaidSvg(cacheKey, svg);
      bindFunctions?.(node);
    } catch {
      // Keep raw mermaid text as fallback if render fails.
    }
  }
}

function scrollToHeading(id: string) {
  if (!shadowRootRef || !id) {
    return false;
  }

  const heading = shadowRootRef.getElementById(id);
  if (!heading) {
    return false;
  }

  heading.scrollIntoView({
    behavior: "smooth",
    block: "start",
  });
  return true;
}

function getHeadingElement(id: string) {
  if (!shadowRootRef || !id) {
    return null;
  }
  return shadowRootRef.getElementById(id) as HTMLElement | null;
}

defineExpose({
  scrollToHeading,
  getHeadingElement,
});

onMounted(() => {
  ensureShadowRoot();
  syncThemeHref();
  syncExtraStyleHrefs();
  syncArticleClass();
  void syncHtml();
});

watch(
  () => props.themeHref,
  () => syncThemeHref(),
);

watch(
  () => props.extraStyleHrefs,
  () => syncExtraStyleHrefs(),
  { deep: true },
);

watch(
  () => props.articleClass,
  () => syncArticleClass(),
);

watch(
  () => props.html,
  () => {
    void syncHtml();
  },
);

// 明暗切换：mermaid 的配色已经烙进 SVG 内联样式，token 的第二份取值救不了它 ——
// 必须重新 initialize + 清掉旧缓存并重渲染，否则图表会一直停在上一个模式的配色上。
watch(
  () => props.themeMode,
  () => {
    if (!mermaidRef || !mermaidModeRef) {
      return;
    }
    if (mermaidModeRef === props.themeMode) {
      return;
    }

    mermaidRef.initialize(mermaidConfig());
    mermaidModeRef = props.themeMode;
    mermaidSvgCache.clear();
    void syncHtml();
  },
);
</script>

<template>
  <div ref="hostRef" />
</template>
