<script setup lang="ts">
import { defineAsyncComponent, ref } from "vue";

import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";

// 写作页的预览栏。
//
// 从 pages/studio/write.vue 拆出来（M6 ⑧）。它管两件事：
// 渲染 Markdown、把渲染出的标题锚点上报给页面（页面再喂给目录）。
//
// scrollToHeading 通过 defineExpose 暴露：点目录时页面拿组件实例调它，
// 而不是让页面知道预览区内部是怎么滚动的。

const WriteMarkdownRenderer = defineAsyncComponent(() => import("~/components/common/MarkdownRenderSurface.vue"));

const props = defineProps<{
  /** 是否显示正文为空时的占位。 */
  empty: boolean;
  /** 防抖后的正文（避免每次按键都重新渲染）。 */
  source: string;
  /** 当前主题的 CSS 地址。 */
  themeHref: string;
  /** 附加样式（katex / 代码高亮）。 */
  extraStyleHrefs: string[];
  /** 主题展示名（右上角提示）。 */
  themeLabel: string;
}>();

const emit = defineEmits<{ headingsChange: [headings: ArticleHeadingAnchor[]] }>();

const rendererRef = ref<{ scrollToHeading: (id: string) => boolean } | null>(null);

// scrollToHeading 供页面（目录点击）调用。
function scrollToHeading(id: string) {
  return rendererRef.value?.scrollToHeading(id) ?? false;
}

defineExpose({ scrollToHeading });
</script>

<template>
  <aside class="write-page__pane write-page__pane--preview">
    <div v-if="!props.empty" class="write-page__preview-shell">
      <div class="write-page__theme-indicator">预览样式：{{ props.themeLabel }}</div>
      <WriteMarkdownRenderer
        ref="rendererRef"
        class="write-page__markdown"
        :source="props.source"
        :theme-href="props.themeHref"
        :extra-style-hrefs="props.extraStyleHrefs"
        article-class="markdown-body"
        @headings-change="emit('headingsChange', $event)" />
    </div>

    <div v-else class="write-page__preview-empty">
      <p>预览区</p>
      <span>先输入标题或正文，这里会实时显示正式渲染结果。</span>
    </div>
  </aside>
</template>
