<script setup lang="ts">
import { nextTick, ref, watch } from "vue";
import { NAnchor, NAnchorLink } from "naive-ui";
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";

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
  <section v-if="headings.length" class="surface-card p-5 md:p-6">
    <div class="eyebrow">Contents</div>
    <div class="mt-2 flex items-center justify-between gap-3">
      <h2 class="section-title text-[18px] md:text-[20px]">目录</h2>
      <div class="glass-badge">{{ Math.round(progressPercent) }}%</div>
    </div>

    <div class="mt-4">
      <div class="article-reading-progress">
        <div class="article-reading-progress__fill" :style="{ width: `${progressPercent}%` }" />
      </div>
    </div>

    <div ref="tocBodyRef" class="mt-4 article-toc-anchor">
      <NAnchor :bound="96" :show-rail="false" type="block">
        <NAnchorLink
          v-for="heading in headings"
          :key="heading.id"
          :href="heading.href"
          :title="heading.title"
          :data-heading-id="heading.id"
          :class="{
            'article-toc-link': true,
            'is-active': activeHeadingId === heading.id,
            'ml-3': heading.level === 3,
            'ml-6': heading.level >= 4,
          }"
          @click.prevent="handleJump(heading.id)"
        />
      </NAnchor>
    </div>
  </section>
</template>
