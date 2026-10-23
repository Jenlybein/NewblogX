<script setup lang="ts">
import { onMounted, ref, watch } from "vue";

const props = withDefaults(
  defineProps<{
    html: string;
    themeHref: string;
    extraStyleHrefs?: string[];
    articleClass?: string;
  }>(),
  {
    extraStyleHrefs: () => [],
    articleClass: "markdown-body",
  },
);

const hostRef = ref<HTMLDivElement | null>(null);

type MermaidRuntime = {
  initialize: (config: { startOnLoad: boolean; securityLevel: string }) => void;
  render: (id: string, text: string) => Promise<{
    svg: string;
    bindFunctions?: (element: Element) => void;
  }>;
};

let shadowRootRef: ShadowRoot | null = null;
let themeLinkRef: HTMLLinkElement | null = null;
let articleRef: HTMLElement | null = null;
let extraStyleLinks: HTMLLinkElement[] = [];
let mermaidRef: MermaidRuntime | null = null;
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

  themeLinkRef = document.createElement("link");
  themeLinkRef.rel = "stylesheet";

  articleRef = document.createElement("article");
  articleRef.className = props.articleClass;

  shadowRootRef.append(baseStyle, themeLinkRef, articleRef);
}

function syncThemeHref() {
  ensureShadowRoot();
  if (!themeLinkRef) {
    return;
  }
  themeLinkRef.href = props.themeHref;
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

async function ensureMermaidInstance() {
  if (typeof window === "undefined") {
    return null;
  }
  if (mermaidRef) {
    return mermaidRef;
  }

  const mermaidModule = await import("~/services/mermaid-runtime.mjs");
  const runtime = mermaidModule.default;
  runtime.initialize({
    startOnLoad: false,
    securityLevel: "strict",
  });
  mermaidRef = runtime;
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
</script>

<template>
  <div ref="hostRef" />
</template>
