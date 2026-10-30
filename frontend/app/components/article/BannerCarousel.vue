<script setup lang="ts">
import { computed, ref } from "vue";
import { IconChevronLeft, IconChevronRight } from "@tabler/icons-vue";
import { NButton } from "naive-ui";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import type { BannerItem } from "~/types/api";

const props = withDefaults(
  defineProps<{
    banners: BannerItem[];
    pending?: boolean;
  }>(),
  { pending: false },
);

const currentIndex = ref(0);

const currentBanner = computed(() => props.banners[currentIndex.value] ?? null);

function switchBanner(step: number) {
  if (!props.banners.length) return;
  currentIndex.value = (currentIndex.value + step + props.banners.length) % props.banners.length;
}

function goToBanner(index: number) {
  if (!props.banners.length) return;
  currentIndex.value = index;
}
</script>

<template>
  <div class="banner-shell" data-home-section="banner">
    <div v-if="currentBanner" class="banner-stage">
      <NuxtLink v-if="currentBanner.href" :to="currentBanner.href" class="banner-link">
        <img :src="currentBanner.cover" :alt="`首页轮播图 ${currentIndex + 1}`" />
      </NuxtLink>
      <img v-else :src="currentBanner.cover" :alt="`首页轮播图 ${currentIndex + 1}`" />

      <div v-if="banners.length > 1" class="banner-nav is-prev">
        <NButton secondary :aria-label="`上一张轮播图（第 ${currentIndex + 1} 张）`" @click="switchBanner(-1)">
          <template #icon>
            <IconChevronLeft :size="18" aria-hidden="true" />
          </template>
        </NButton>
      </div>
      <div v-if="banners.length > 1" class="banner-nav is-next">
        <NButton secondary :aria-label="`下一张轮播图（第 ${currentIndex + 1} 张）`" @click="switchBanner(1)">
          <template #icon>
            <IconChevronRight :size="18" aria-hidden="true" />
          </template>
        </NButton>
      </div>

      <div v-if="banners.length > 1" class="banner-dots">
        <button
          v-for="(banner, index) in banners"
          :key="banner.id || index"
          type="button"
          class="banner-dot"
          :class="{ 'is-active': index === currentIndex }"
          :aria-label="`切换到第 ${index + 1} 张轮播图`"
          :aria-current="index === currentIndex ? 'true' : undefined"
          @click="goToBanner(index)"
        >
          <span aria-hidden="true" />
        </button>
      </div>
    </div>

    <ProfileEmptyState
      v-else
      size="sm"
      :pending="pending"
      title="暂无轮播内容"
      description="运营还没有配置首页轮播，下面的置顶与最新文章照常可看。"
      :hints="['轮播图由管理员后台上传后随站点配置即时生效。']"
    />
  </div>
</template>

<style scoped>
/* Banner 是图纸里的一格：1px 结构线、方角、不加阴影；图片自身承载颜色。 */
.banner-shell {
  border: 1px solid var(--color-line);
  background: var(--color-surface);
  padding: 0.75rem;
}

.banner-stage {
  position: relative;
  overflow: hidden;
  border: 1px solid var(--color-line-faint);
  background: var(--color-bg-subtle);
  aspect-ratio: 1.8 / 1;
}

.banner-stage img {
  display: block;
  height: 100%;
  width: 100%;
  object-fit: cover;
}

.banner-nav {
  position: absolute;
  inset-block: 0;
  display: flex;
  align-items: center;
}

.banner-nav.is-prev {
  left: 0.75rem;
}

.banner-nav.is-next {
  right: 0.75rem;
}

.banner-dots {
  position: absolute;
  inset-inline: 0;
  bottom: 0.5rem;
  display: flex;
  justify-content: center;
  gap: 0.25rem;
}

/* 触控区 44px，视觉点用内部 span 表达 —— 不靠肉眼看不见的小按钮。 */
.banner-dot {
  display: inline-flex;
  min-height: 44px;
  min-width: 44px;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: 0;
  padding: 0;
  touch-action: manipulation;
}

.banner-dot span {
  display: block;
  height: 8px;
  width: 8px;
  border: 1px solid var(--color-line-strong);
  background: var(--color-surface);
  transition: width var(--motion-fast) var(--motion-ease);
}

.banner-dot.is-active span {
  width: 28px;
  border-color: var(--color-focus);
  background: var(--color-focus);
}

.banner-dot:focus-visible span {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

@media (prefers-reduced-motion: reduce) {
  .banner-dot span {
    transition: none;
  }
}
</style>
