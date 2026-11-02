<script setup lang="ts">
import { computed, ref, watch, type CSSProperties } from "vue";
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

/** 当前这张图加载失败：只把底图层切到失败态，切换下一张时复位。 */
const imageFailed = ref(false);

watch(currentBanner, () => {
  imageFailed.value = false;
});

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
    有图时它是封面最底一层的「媒体舞台」（底图 + 底部渐黑 + 控件），
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
        :class="{ 'is-failed': imageFailed }"
        :style="photoStyle"
      >
        <img
          class="banner-photo__img"
          :src="currentBanner.cover"
          :alt="`首页轮播图 ${currentIndex + 1}`"
          :style="framingStyle"
          @error="imageFailed = true"
        />
      </NuxtLink>
      <div
        v-else
        class="banner-photo"
        :class="{ 'is-failed': imageFailed }"
        :style="photoStyle"
      >
        <img
          class="banner-photo__img"
          :src="currentBanner.cover"
          :alt="`首页轮播图 ${currentIndex + 1}`"
          :style="framingStyle"
          @error="imageFailed = true"
        />
      </div>

      <!--
        底部渐黑区：上半完全透明、深墨只往底部过渡，用来承载控件与标题。
        控件是它的内容（一行靠底排版），不再另起一颗浮在图上、自成一块深色的玻璃胶囊。
      -->
      <div class="banner-veil">
        <div v-if="banners.length > 1" class="banner-controls" role="group" aria-label="轮播控件">
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

          <div class="banner-arrows">
            <button
              type="button"
              class="banner-arrow is-prev"
              :aria-label="`上一张轮播图（第 ${currentIndex + 1} 张）`"
              @click="switchBanner(-1)"
            >
              <IconChevronLeft :size="18" aria-hidden="true" />
            </button>
            <button
              type="button"
              class="banner-arrow is-next"
              :aria-label="`下一张轮播图（第 ${currentIndex + 1} 张）`"
              @click="switchBanner(1)"
            >
              <IconChevronRight :size="18" aria-hidden="true" />
            </button>
          </div>
        </div>
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
 * 图片加载失败：不往面板上补一块纯黑底。只在底图层加一层很淡的反白渐变，
 * 让"缺图"读成刻意的留白；面板自己的深墨染色仍是最底层的兜底。
 * 失败的图本身隐掉（只保留 alt 语义），避免浏览器画出来的碎图标。
 */
.banner-photo.is-failed {
  background-image: linear-gradient(
    135deg,
    color-mix(in srgb, var(--color-on-inverse) 14%, transparent) 0%,
    transparent 60%
  );
}

.banner-photo.is-failed .banner-photo__img {
  opacity: 0;
}

/*
 * 底部渐黑（图层 1）：只覆盖面板下半（高度有界），上半张图完全露出。
 * 渐变自全透明过渡到深墨，最深一档也留一点照片可见，不再整幅压暗。
 * 第一行是不支持 color-mix 时的降级：仍是"透明→深墨"的渐变，不会退化成
 * 100% 不透明的纯黑面。渐变最弱一档的透明度要保证反白内容叠在纯白底图上
 * 仍 ≥4.5:1（由设计测试按真实 token 取值算出来，见 banner-veil 专项）。
 */
.banner-veil {
  position: absolute;
  inset: auto 0 0;
  z-index: 1;
  display: flex;
  align-items: flex-end;
  height: 44%;
  padding:
    clamp(1rem, 4vw, 2.5rem) clamp(0.5rem, 1.5vw, 1rem) clamp(0.4rem, 1vw, 0.75rem);
  background: linear-gradient(180deg, transparent 0%, var(--color-surface-inverse) 100%);
  background: linear-gradient(
    180deg,
    transparent 0%,
    color-mix(in srgb, var(--color-surface-inverse) 62%, transparent) 55%,
    color-mix(in srgb, var(--color-surface-inverse) 96%, transparent) 100%
  );
  pointer-events: none;
}

/* 无图时退回既有空态；它走正常文档流（封面文字仍在上方成立）。 */
.banner-empty {
  position: relative;
  width: 100%;
}

/* 控件住进渐黑区：一行内容（圆点靠左、箭头靠右），不再是一颗自带材质与底色的胶囊。 */
.banner-controls {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  pointer-events: auto;
}

.banner-dots,
.banner-arrows {
  display: flex;
  align-items: center;
  gap: 0.125rem;
}

.banner-arrow {
  display: inline-flex;
  min-height: 44px;
  min-width: 44px;
  align-items: center;
  justify-content: center;
  border: 0;
  background: transparent;
  /* 渐黑区是深墨面：图形走反白 token，纸面墨色在这里读不出来。 */
  color: var(--color-on-inverse);
  cursor: pointer;
  touch-action: manipulation;
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
  border: 1px solid var(--color-on-inverse-muted);
  border-radius: var(--radius-pill);
  background: transparent;
  transition: width var(--motion-fast) var(--motion-ease);
}

/* 当前项：实心反白 + 拉长的 pill，与未选中的空心点一眼可分。 */
.banner-dot.is-active span {
  width: 28px;
  border-color: var(--color-on-inverse);
  background: var(--color-on-inverse);
}

/* 键盘焦点：功能蓝描边压一圈深墨垫环 —— 任意底图上都 ≥3:1，焦点环不会被照片吃掉。 */
.banner-arrow:focus-visible,
.banner-dot:focus-visible span {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
  box-shadow: 0 0 0 4px var(--color-surface-inverse);
}

@media (prefers-reduced-motion: reduce) {
  .banner-dot span {
    transition: none;
  }
}
</style>
