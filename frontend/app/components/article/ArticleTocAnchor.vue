<script setup lang="ts">
import { nextTick, ref, watch } from "vue";
import { NAnchor, NAnchorLink } from "naive-ui";
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { formatProgressPercent, tocIndentLevel } from "~/utils/toc";

const props = defineProps<{
  headings: ArticleHeadingAnchor[];
  activeHeadingId: string;
  progressPercent: number;
}>();

const emit = defineEmits<{
  jump: [id: string];
}>();

const tocBodyRef = ref<HTMLElement | null>(null);

function handleJump(id: string) {
  emit("jump", id);
}

watch(
  () => props.activeHeadingId,
  async (nextId) => {
    if (!nextId) {
      return;
    }

    await nextTick();
    const currentLink = tocBodyRef.value?.querySelector<HTMLElement>(`[data-heading-id="${nextId}"]`);
    currentLink?.scrollIntoView({
      block: "nearest",
      behavior: "smooth",
    });
  },
  { flush: "post" },
);
</script>

<template>
  <section v-if="headings.length" class="article-paper article-toc">
    <div class="article-toc__head">
      <p class="article-eyebrow"><span class="article-eyebrow__mark" aria-hidden="true" />Contents</p>
      <h2 class="article-toc__title">目录</h2>
      <span class="article-toc__progress">{{ formatProgressPercent(progressPercent) }}</span>
    </div>

    <div
      class="article-reading-progress"
      role="progressbar"
      aria-label="阅读进度"
      :aria-valuemin="0"
      :aria-valuemax="100"
      :aria-valuenow="Math.round(progressPercent)"
    >
      <span class="article-reading-progress__fill" :style="{ width: `${progressPercent}%` }" />
    </div>

    <nav ref="tocBodyRef" class="article-toc__body" aria-label="文章目录">
      <NAnchor :bound="96" :show-rail="false" type="block">
        <NAnchorLink
          v-for="heading in headings"
          :key="heading.id"
          :href="heading.href"
          :title="heading.title"
          :data-heading-id="heading.id"
          :class="[
            'article-toc-link',
            `article-toc-link--indent-${tocIndentLevel(heading.level)}`,
            { 'is-active': activeHeadingId === heading.id },
          ]"
          @click.prevent="handleJump(heading.id)"
        />
      </NAnchor>
    </nav>
  </section>
</template>
