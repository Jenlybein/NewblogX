<script setup lang="ts">
import { computed, ref, type CSSProperties } from "vue";
import { IconChevronLeft, IconChevronRight } from "@tabler/icons-vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import type { BannerItem } from "~/types/api";
import { bannerFramingStyle } from "~/utils/bannerFraming";

const props = withDefaults(
  defineProps<{
    banners: BannerItem[];
    pending?: boolean;
    /** 首屏封面编排的媒体层视差（≤10px，来自 useParallax；只作用在底图上）。 */
    photoStyle?: CSSProperties;
  }>(),
  { pending: false, photoStyle: () => ({}) },
);

const currentIndex = ref(0);

const currentBanner = computed(() => props.banners[currentIndex.value] ?? null);

/** 取景（焦点 + 缩放）→ 图片层样式；历史图（无 crop）走居中、缩放 1。 */
const framingStyle = computed(() => bannerFramingStyle(currentBanner.value?.crop));

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
  <!--
    轮播已并入首屏封面（ADR-0013 决策 8，GLOSSARY「首屏封面」）：
    有图时它是封面最底一层的「媒体舞台」（底图 + 罩层 + 玻璃胶囊控件），
    无图时退回既有空态组件（ProfileEmptyState），不渲染任何控件。
    ≥2 张才出现控件（1 张 / 0 张都不该有可点的箭头与圆点）。
  -->
  <div
    class="banner-carousel"
    :class="currentBanner ? 'banner-stage' : 'banner-empty'"
    data-home-section="banner"
  >
    <template v-if="currentBanner">
      <!--
        取景与视差分属两层（ADR-0015 决策 6）：两者都动 transform，不能落在同一个元素上。
        外层 .banner-photo 只承担首屏视差（photoStyle）；内层图片只承担取景的缩放与焦点。
      -->
      <NuxtLink
        v-if="currentBanner.href"
        :to="currentBanner.href"
        class="banner-photo"
        :style="photoStyle"
      >
        <img
          class="banner-photo__img"
          :src="currentBanner.cover"
          :alt="`首页轮播图 ${currentIndex + 1}`"
          :style="framingStyle"
        />
      </NuxtLink>
      <div v-else class="banner-photo" :style="photoStyle">
        <img
          class="banner-photo__img"
          :src="currentBanner.cover"
          :alt="`首页轮播图 ${currentIndex + 1}`"
          :style="framingStyle"
        />
      </div>

      <span class="banner-scrim" aria-hidden="true" />

      <div v-if="banners.length > 1" class="banner-controls glass-thin" role="group" aria-label="轮播控件">
        <button
          type="button"
          class="banner-arrow is-prev"
          :aria-label="`上一张轮播图（第 ${currentIndex + 1} 张）`"
          @click="switchBanner(-1)"
        >
          <IconChevronLeft :size="18" aria-hidden="true" />
        </button>

        <div class="banner-dots">
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

        <button
          type="button"
          class="banner-arrow is-next"
          :aria-label="`下一张轮播图（第 ${currentIndex + 1} 张）`"
          @click="switchBanner(1)"
        >
          <IconChevronRight :size="18" aria-hidden="true" />
        </button>
      </div>
    </template>

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
/* 媒体舞台：铺满首屏封面（不设 z-index 的 auto 定位，控件的 z-index 才能回到封面那一层）。 */
.banner-stage {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

/* 底图层（图层 0）：运营上传的首张轮播 cover。
   这一层只承担视差（photoStyle 的 translateY），取景的缩放/焦点在内层图片上。 */
.banner-photo {
  /* 外扩给 ≤10px 的视差留余量，但**必须保持 16:9 配比**：纵向 12px、横向 12 × 16/9 ≈ 21.33px。
     四边等量外扩会把容器比例从 1.7778 改成 1.7397，于是 cover 后可见的原图区域
     与后台取景框预览差约 5%（线上更紧），"预览＝线上"就只剩近似。
     写成预计算数值而不是 calc() 里的乘法：`calc(长度 * 数字)` 在旧版 Safari 上会整条失效。 */
  position: absolute;
  inset: -12px -21.3333px;
  z-index: 0;
  display: block;
  overflow: hidden;
  pointer-events: auto;
}

/* 取景层：object-fit / object-position / transform-origin / scale 全部来自 bannerFramingStyle()。 */
.banner-photo__img {
  display: block;
  height: 100%;
  width: 100%;
}

/*
 * 罩层（图层 1）：中性石墨渐变，最浅一档压到 70%、底部到 95%，
 * 让任意底图上的反白文字都 ≥4.5:1（ADR-0013 规格「罩层保证文字可读」）。
 * 第一行是降级值（不支持 color-mix 的浏览器退化为不透明深墨面：图没了，字仍然可读）。
 */
.banner-scrim {
  position: absolute;
  inset: 0;
  z-index: 1;
  background: var(--color-surface-inverse);
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--color-surface-inverse) 70%, transparent) 0%,
    color-mix(in srgb, var(--color-surface-inverse) 95%, transparent) 100%
  );
  pointer-events: none;
}

/* 无图时退回既有空态；它走正常文档流（封面文字仍在上方成立）。 */
.banner-empty {
  position: relative;
  width: 100%;
}

/* 图层 6：玻璃胶囊控件（箭头 + 圆点在同一层薄玻璃里，1 张 / 0 张不渲染）。 */
.banner-controls {
  position: absolute;
  left: 50%;
  bottom: clamp(0.75rem, 3vw, 1.5rem);
  z-index: 6;
  display: flex;
  align-items: center;
  gap: 0.125rem;
  padding: 0.125rem 0.375rem;
  border-radius: var(--radius-pill);
  transform: translateX(-50%);
  pointer-events: auto;
}

.banner-arrow {
  display: inline-flex;
  min-height: 44px;
  min-width: 44px;
  align-items: center;
  justify-content: center;
  border: 0;
  background: transparent;
  color: var(--color-ink);
  cursor: pointer;
  touch-action: manipulation;
}

.banner-dots {
  display: flex;
  align-items: center;
  gap: 0;
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
  cursor: pointer;
  touch-action: manipulation;
}

.banner-dot span {
  display: block;
  height: 8px;
  width: 8px;
  border: 1px solid var(--color-line-strong);
  border-radius: var(--radius-pill);
  background: var(--color-surface);
  transition: width var(--motion-fast) var(--motion-ease);
}

.banner-dot.is-active span {
  width: 28px;
  border-color: var(--color-ink);
  background: var(--color-ink);
}

.banner-arrow:focus-visible,
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
