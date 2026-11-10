<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, type CSSProperties } from "vue";
import { IconChevronLeft, IconChevronRight } from "@tabler/icons-vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import type { BannerItem } from "~/types/api";
import { bannerTitleText } from "~/utils/bannerTitle";

/** 默认停留时长：够看清一张图的构图，又不至于让人等得不耐烦。 */
const BANNER_AUTOPLAY_INTERVAL_MS = 6000;

const props = withDefaults(
  defineProps<{
    banners: BannerItem[];
    pending?: boolean;
    /** 首屏封面编排的媒体层视差（≤10px，来自 useParallax；只作用在底图上）。 */
    photoStyle?: CSSProperties;
    /** 自动轮播间隔（毫秒）；≤0 关闭自动轮播（测试与"静态展示"场景用）。 */
    autoPlayInterval?: number;
  }>(),
  { pending: false, photoStyle: () => ({}), autoPlayInterval: BANNER_AUTOPLAY_INTERVAL_MS },
);

const currentIndex = ref(0);

const currentBanner = computed(() => props.banners[currentIndex.value] ?? null);

/** 当前这张的标题（纯文本）。空标题不渲染文字区，不留一块空标题带。 */
const titleText = computed(() => bannerTitleText(currentBanner.value?.title));

/** ≥2 张才有控件；只有 1 张时圆点与箭头都没有可操作的对象。 */
const hasControls = computed(() => props.banners.length > 1);

/**
 * 有标题、但没有控件行。
 *
 * 这时渐黑区会短一整行控件的高度，标题就掉到面板最下沿（单张轮播的用户反馈）。
 * 模板据此挂 `is-single`，把控件行本该占的高度补进下内边距，让标题落在与
 * ≥2 张时**完全相同**的位置。没有标题时不补：那会变成一条空压暗带。
 */
const singleWithoutControls = computed(() => Boolean(titleText.value) && !hasControls.value);

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

/*
 * 自动轮播。
 *
 * 只在"真的能看"的条件下走：≥2 张、页面可见、系统没要求减少动态、间隔 > 0。
 * 指针停上去 / 键盘焦点进入 / 标签页切到后台就停表 —— 用户在读标题或操作控件时，
 * 图不该自己跑掉。
 *
 * 手动切换后重新计时（而不是"到点立刻跳"），否则刚点完下一张就会被打断。
 */
let timer: ReturnType<typeof setInterval> | null = null;

/** 是否允许自动轮播（纯判定，方便阅读与测试）。 */
function autoPlayEnabled(): boolean {
  return (
    props.autoPlayInterval > 0 &&
    props.banners.length > 1 &&
    typeof window !== "undefined" &&
    window.matchMedia?.("(prefers-reduced-motion: reduce)").matches !== true &&
    document.visibilityState === "visible"
  );
}

function stopAutoPlay() {
  if (timer !== null) {
    clearInterval(timer);
    timer = null;
  }
}

/** 从头开始计时一轮。 */
function restartAutoPlay() {
  stopAutoPlay();
  if (!autoPlayEnabled()) return;
  timer = setInterval(() => {
    switchBanner(1);
  }, props.autoPlayInterval);
}

function onVisibilityChange() {
  if (document.visibilityState === "visible") {
    restartAutoPlay();
  } else {
    stopAutoPlay();
  }
}

onMounted(() => {
  restartAutoPlay();
  document.addEventListener("visibilitychange", onVisibilityChange);
});

onBeforeUnmount(() => {
  stopAutoPlay();
  document.removeEventListener("visibilitychange", onVisibilityChange);
});

/** 指针进入 / 键盘聚焦：暂停。 */
function pauseAutoPlay() {
  stopAutoPlay();
}

/** 指针离开 / 焦点移出：重新开始一轮。 */
function resumeAutoPlay() {
  restartAutoPlay();
}

// 手动切换（点箭头 / 点圆点）后重新计时，避免"刚点完就被自动切走"。
watch(currentIndex, () => {
  if (timer !== null) restartAutoPlay();
});

// 张数变化（例如首屏数据到达 / 运营删了轮播）时重新评估。
watch(
  () => props.banners.length,
  () => {
    if (currentIndex.value >= props.banners.length) {
      currentIndex.value = 0;
    }
    restartAutoPlay();
  },
);
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
    @mouseenter="pauseAutoPlay"
    @mouseleave="resumeAutoPlay"
    @focusin="pauseAutoPlay"
    @focusout="resumeAutoPlay"
  >
    <template v-if="currentBanner">
      <!--
        底图与视差分属两层：视差（photoStyle 的 translateY）只在外层 .banner-photo，
        内层图片只负责铺满（object-fit: cover、固定居中）。封面不再消费取景参数（票 204）。
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
          @error="imageFailed = true"
        />
      </div>

      <!--
        底部渐黑区：上半完全透明、深墨只往底部过渡，用来承载控件与标题。
        控件是它的内容（一行靠底排版），不再另起一颗浮在图上、自成一块深色的玻璃胶囊。
        **高度由内容撑出来**，不是固定的面板占比：没有标题时就只留控件那一条，
        不会无端压暗半张图（那正是"大黑块"的来源）。
      -->
      <div
        class="banner-veil"
        :class="{
          'is-titleless': !titleText,
          'is-single': singleWithoutControls,
        }"
      >
        <div class="banner-veil__body">
          <!--
            标题是纯文本：用插值输出（Vue 自动转义），不接 v-html。
            空标题直接不渲染，避免在渐黑区里留一条空标题带。
          -->
          <p v-if="titleText" class="banner-title">{{ titleText }}</p>
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
   这一层只承担视差（photoStyle 的 translateY），图片本身固定居中铺满。 */
.banner-photo {
  /* 外扩给 ≤10px 的视差留余量，但**必须保持 16:9 配比**：纵向 12px、横向 12 × 16/9 ≈ 21.33px。
     四边等量外扩会把容器比例从 1.7778 改成 1.7397，于是 cover 后可见的原图区域
     会比 16:9 容器紧约 5%，封面的构图就随视差层偷偷变了。
     写成预计算数值而不是 calc() 里的乘法：`calc(长度 * 数字)` 在旧版 Safari 上会整条失效。 */
  position: absolute;
  inset: -12px -21.3333px;
  z-index: 0;
  display: block;
  overflow: hidden;
  pointer-events: auto;
}

/* 图片层：铺满底图层、固定居中。上传时已定形成 16:9（票 200），旧封面也按居中兜底（票 204）。 */
.banner-photo__img {
  display: block;
  height: 100%;
  width: 100%;
  object-fit: cover;
  object-position: center;
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
 * 底部渐黑（图层 1）：贴在面板底部，**高度由内容撑出来**。
 *
 * 曾经写死 `height: 44%`：那让"有没有标题"都按同一个高度压暗——运营没填标题时
 * 渐黑区里只剩一行控件，却仍压住近半张图，观感就是一块盖在轮播图上的大黑块。
 * 现在盒子高度 = padding-top（渐变过渡带）+ 内容，无标题时过渡带自动收窄。
 *
 * 渐变自全透明过渡到深墨，最深一档也留一点照片可见，不整幅压暗。
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
  height: auto;
  padding:
    clamp(2.25rem, 9vw, 3.5rem) clamp(0.5rem, 1.5vw, 1rem) clamp(0.4rem, 1vw, 0.75rem);
  background: linear-gradient(180deg, transparent 0%, var(--color-surface-inverse) 100%);
  background: linear-gradient(
    180deg,
    transparent 0%,
    color-mix(in srgb, var(--color-surface-inverse) 62%, transparent) 32%,
    color-mix(in srgb, var(--color-surface-inverse) 85%, transparent) 62%,
    color-mix(in srgb, var(--color-surface-inverse) 96%, transparent) 100%
  );
  pointer-events: none;
}

/* 没有标题：只剩一行控件，过渡带也跟着收窄，避免"空标题也压半张图"。 */
.banner-veil.is-titleless {
  padding-top: clamp(1.25rem, 5vw, 2rem);
}

/*
 * 有标题但没有控件行（只有 1 张轮播）。
 *
 * 渐黑区的高度由内容撑出来，少一行控件就短一行：标题因此掉到面板最下沿，
 * 看起来像"没排完"。这里把控件行本该占的高度补回下内边距，标题落在与 ≥2 张时
 * 完全相同的位置。
 *
 * 组成（与有控件时逐项对齐，改一处必须同时改这里）：
 *   下内边距 clamp(0.4rem, 1vw, 0.75rem)
 * + 控件行 2.75rem（`.banner-arrow` / `.banner-dot` 的 min-height: 44px）
 * + 标题与控件之间的间隙 clamp(0.25rem, 0.8vw, 0.75rem)（`.banner-veil__body` 的 gap）
 *
 * 为什么不是"始终渲染控件"：单张时圆点与箭头没有任何可操作的对象，
 * 摆在那里只是为了占位，会变成一组点了没反应的控件。
 */
.banner-veil.is-single {
  padding-bottom: calc(
    clamp(0.4rem, 1vw, 0.75rem) + 2.75rem + clamp(0.25rem, 0.8vw, 0.75rem)
  );
}

/* 无图时退回既有空态；它走正常文档流（封面文字仍在上方成立）。 */
.banner-empty {
  position: relative;
  width: 100%;
}

/* 渐黑区的内容列：标题在上、控件在下，共用同一块底部渐黑（不是两套浮层）。 */
.banner-veil__body {
  display: flex;
  width: 100%;
  flex-direction: column;
  gap: clamp(0.25rem, 0.8vw, 0.75rem);
}

/*
 * 轮播标题：只承载纯文本，靠底部渐黑区保证对比度。
 * 字号明确小于首屏主标题（clamp 上限 1.5rem），窄视口自动缩到 0.9rem，
 * 不与站点标识抢视觉权重；长标题最多两行，避免把渐黑区撑高压住图片。
 */
.banner-title {
  display: -webkit-box;
  overflow: hidden;
  margin: 0;
  padding: 0 clamp(0.5rem, 1.5vw, 1rem);
  color: var(--color-on-inverse);
  font-family: var(--font-title);
  font-size: clamp(0.9rem, 2.2vw, 1.5rem);
  font-weight: 600;
  line-height: 1.35;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
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
