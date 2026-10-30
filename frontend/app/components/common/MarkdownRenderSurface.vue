<script setup lang="ts">
import { computed, ref, watch } from "vue";
import StudioMarkdownShadowPreview from "~/components/studio/MarkdownShadowPreview.vue";
import { useArticleMarkdown } from "~/composables/useArticleMarkdown";
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";

const props = withDefaults(
  defineProps<{
    source?: string | null;
    /** 可选外部主题：阅读页不传（自有 token 主题），创作区/私信保留旧的多主题选择。 */
    themeHref?: string;
    extraStyleHrefs?: string[];
    articleClass?: string;
  }>(),
  {
    source: "",
    themeHref: "",
    extraStyleHrefs: () => [],
    articleClass: "markdown-body",
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

watch(
  headings,
  (value) => {
    emit("headings-change", value);
  },
  { immediate: true },
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
  />
</template>
