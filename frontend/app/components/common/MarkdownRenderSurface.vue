<script setup lang="ts">
import { computed, ref, watch } from "vue";
import StudioMarkdownShadowPreview from "~/components/studio/MarkdownShadowPreview.vue";
import { useArticleMarkdown } from "~/composables/useArticleMarkdown";
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { themeRootClass } from "~/utils/themeClass";
import type { MarkdownThemeMode } from "~/utils/markdownTheme";
import tokensCssSource from "~/assets/css/tokens.css?raw";

const props = withDefaults(
  defineProps<{
    source?: string | null;
    /** 可选外部主题：阅读页不传（自有 token 主题），创作区/私信保留旧的多主题选择。 */
    themeHref?: string;
    extraStyleHrefs?: string[];
    articleClass?: string;
    /**
     * mermaid 图表配色的取值来源（tokens.css 源码）。
     *
     * 缺省值让下游（阅读页 / 私信 / AI 面板 / 创作区预览）不必各自引 CSS；
     * 测试与 SSR 仍可传自己的桩字符串。
     */
    tokensCss?: string;
    /** 明确指定明暗；不传则跟随站点根类（`.theme-dark`）。 */
    themeMode?: MarkdownThemeMode;
  }>(),
  {
    source: "",
    themeHref: "",
    extraStyleHrefs: () => [],
    articleClass: "markdown-body",
    tokensCss: tokensCssSource,
    themeMode: undefined,
  },
);

const emit = defineEmits<{
  "headings-change": [value: ArticleHeadingAnchor[]];
}>();

const previewRef = ref<{
  scrollToHeading: (id: string) => boolean;
  getHeadingElement: (id: string) => HTMLElement | null;
} | null>(null);

const { renderedHtml, headings } = useArticleMarkdown(computed(() => props.source || ""));

/**
 * 明暗跟随站点根类 —— 这也是 tokens.css 自己切换第二份取值的同一个类。
 *
 * 为什么不读 uiStore：本组件横跨阅读页 / 私信 / AI 面板 / 创作区预览四处，引 store
 * 会把一个纯展示组件焊死在 Pinia 上（连它的单测都得先造 store）。根类是两边共同的
 * 真相，SSR 下 document 不存在时兜回浅色（此时本来也不渲染 mermaid）。
 */
function detectThemeMode(): MarkdownThemeMode {
  if (props.themeMode) {
    return props.themeMode;
  }
  if (typeof document === "undefined") {
    return "light";
  }
  return document.documentElement.classList.contains(themeRootClass("dark")) ? "dark" : "light";
}

/** 站点换肤时跟着翻：这里变了，子组件的 watch 会重新 initialize mermaid。 */
const mermaidThemeMode = ref<MarkdownThemeMode>(detectThemeMode());

watch(headings, (value) => emit("headings-change", value), { immediate: true });

watch(
  () => props.themeMode,
  (mode) => {
    mermaidThemeMode.value = mode ?? detectThemeMode();
  },
);

function scrollToHeading(id: string) {
  return previewRef.value?.scrollToHeading(id) ?? false;
}

function getHeadingElement(id: string) {
  return previewRef.value?.getHeadingElement(id) ?? null;
}

defineExpose({
  scrollToHeading,
  getHeadingElement,
});
</script>

<template>
  <StudioMarkdownShadowPreview
    ref="previewRef"
    :html="renderedHtml"
    :theme-href="themeHref"
    :extra-style-hrefs="extraStyleHrefs"
    :article-class="articleClass"
    :tokens-css="tokensCss"
    :theme-mode="mermaidThemeMode"
  />
</template>
