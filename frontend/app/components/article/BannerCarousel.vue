<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, type CSSProperties } from "vue";
import {
  IconChevronLeft,
  IconChevronRight,
  IconPlayerPause,
  IconPlayerPlay,
} from "@tabler/icons-vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { useMotionPreference } from "~/composables/useMotionPreference";
import type { BannerItem } from "~/types/api";
import { bannerTitleText } from "~/utils/bannerTitle";
import { MOTION_EXPRESSIVE_MS } from "~/utils/motion";

/** 默认停留时长：够看清一张图的构图，又不至于让人等得不耐烦。 */
const BANNER_AUTOPLAY_INTERVAL_MS = 6000;

/**
 * 换图交叉淡入时长（票 432）：160ms。
 *
 * 它不是三档界面时长之一，所以登记在 `utils/motion.ts` 的表达型预设表里，
 * 组件只引用名字；样式层通过根节点的 `--banner-crossfade` 变量消费 ——
 * 组件里不写裸毫秒（ADR-0011 决策 7 要求动效预算只有一个来源）。
 */
const CROSSFADE_MS = MOTION_EXPRESSIVE_MS.bannerCrossfade;

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

/**
 * 媒体层的一帧。
 *
 * 换图时旧帧不立刻消失：它留在画面上淡出（`leaving`），新帧直接落在它下面，
 * 叠起来就是一次交叉淡入。`key` 是稳定渲染键 —— 同一张图切回来也算新的一帧。
 */
interface BannerFrame {
  key: number;
  banner: BannerItem;
  /** 该帧对应的轮播序号（alt 文案用）。 */
  index: number;
  leaving: boolean;
  failed: boolean;
}

const frames = ref<BannerFrame[]>([]);
let frameSeq = 0;
/** 正在淡出的帧 → 卸载它的定时器；卸载、反向切换或更新数据前统一清掉。 */
const leavingFrameTimers = new Map<number, ReturnType<typeof setTimeout>>();

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

/* ------------------------------------------------------------------------------------
 * 换图帧与交叉淡入（票 432）
 * ---------------------------------------------------------------------------------- */

/**
 * 系统「减少动态」偏好：统一走 `useMotionPreference()`（票 437 评审）。
 *
 * 以前这里自建了一份 `readReducedMotion()` + matchMedia 监听 + `reducedMotion` ref，
 * 与 AppFooter / useJellyAssistant 各持一套；现在只用仓库唯一来源，SSR 没有
 * matchMedia 时按"不减少动态"兜底、客户端由 composable 自己接管 change 监听。
 */
const { prefersReducedMotion } = useMotionPreference();

/** 一帧对应哪张轮播图：数据刷新后据此判断画面要不要重建。 */
function bannerKeyOf(banner: BannerItem | null | undefined): string {
  if (!banner) return "";
  return `${banner.id ?? ""}|${banner.cover ?? ""}|${banner.href ?? ""}`;
}

function createFrame(index: number): BannerFrame | null {
  const banner = props.banners[index];
  if (!banner) return null;
  frameSeq += 1;
  return { key: frameSeq, banner, index, leaving: false, failed: false };
}

function clearLeavingTimers() {
  for (const timer of leavingFrameTimers.values()) clearTimeout(timer);
  leavingFrameTimers.clear();
}

/** 卸载一帧（淡出结束 / 被更晚的切换挤掉 / 减少动态）。 */
function dropFrame(key: number) {
  const timer = leavingFrameTimers.get(key);
  if (timer !== undefined) {
    clearTimeout(timer);
    leavingFrameTimers.delete(key);
  }
  frames.value = frames.value.filter((frame) => frame.key !== key);
}

/** 硬切到当前帧（数据变化 / 减少动态）：不摆任何出场帧。 */
function resetFrames() {
  clearLeavingTimers();
  const frame = createFrame(currentIndex.value);
  frames.value = frame ? [frame] : [];
}

/**
 * 切到某一帧：新帧放在数组最前（先绘制、叠在旧帧下面），旧帧留在画面上淡出。
 * 两帧叠起来就是一次 A→B 的交叉淡入；减少动态则直接换，不留过渡帧。
 */
function showFrame(index: number) {
  const frame = createFrame(index);
  if (!frame) {
    resetFrames();
    return;
  }

  if (prefersReducedMotion.value) {
    clearLeavingTimers();
    frames.value = [frame];
    return;
  }

  for (const previous of frames.value) {
    previous.leaving = true;
    leavingFrameTimers.set(
      previous.key,
      setTimeout(() => dropFrame(previous.key), CROSSFADE_MS),
    );
  }

  frames.value = [frame, ...frames.value];
  // 极快连点时最多留两帧：更早的出场帧直接卸载，避免底图叠成一摞。
  for (const extra of frames.value.slice(2)) dropFrame(extra.key);
}

/** 交叉淡入时长交给样式层：组件里只有 `var(--banner-crossfade)`，没有裸毫秒。 */
const crossfadeStyle = computed<CSSProperties>(() => ({
  "--banner-crossfade": `${CROSSFADE_MS}ms`,
}));

function switchBanner(step: number) {
  if (!props.banners.length) return;
  currentIndex.value = (currentIndex.value + step + props.banners.length) % props.banners.length;
}

function goToBanner(index: number) {
  if (!props.banners.length) return;
  currentIndex.value = index;
}

/* ------------------------------------------------------------------------------------
 * 自动轮播
 *
 * 只在"真的能看"的条件下走：≥2 张、页面可见、间隔 > 0、读者没有按暂停。
 * 「减少动态」不是在这里判的：它决定 `userPaused` 的**初始值**（默认暂停），并让
 * 播放按钮保持禁用 —— spec 432 要求该偏好下不启动自动播放，点击不能重新开表
 * （票 423）。指针停上去 / 键盘焦点进入 / 标签页切到后台就停表 —— 用户在读标题或
 * 操作控件时，图不该自己跑掉。
 *
 * 手动切换后重新计时（而不是"到点立刻跳"），否则刚点完下一张就会被打断。
 * 读者按下暂停后，这些"被动暂停"不再能把它偷偷恢复：`userPaused` 只有再点一次才翻。
 * ---------------------------------------------------------------------------------- */
let timer: ReturnType<typeof setInterval> | null = null;
/**
 * 是否"被暂停"。
 *
 * 初始 true（暂停）而不是 false：SSR 没有 `prefersReducedMotion`，客户端首帧又可能
 * 是 true —— 初始值若为 false，按钮文案会在水合时跳变。统一按"暂停"进入，挂载后
 * 再按真实偏好决定要不要默认播放（`onMounted`）；「减少动态」下就一直保持暂停，
 * 只有读者显式点「继续轮播」才动。
 */
const userPaused = ref(true);

/** 是否允许自动轮播（纯判定，方便阅读与测试）。 */
function autoPlayEnabled(): boolean {
  return (
    props.autoPlayInterval > 0 &&
    props.banners.length > 1 &&
    !userPaused.value &&
    typeof window !== "undefined" &&
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

/** 播放/暂停按钮的文案随状态变化；减少动态下说明"自动轮播已按系统偏好关闭"。 */
const playbackDisabled = computed(() => prefersReducedMotion.value);
const playbackLabel = computed(() => {
  if (prefersReducedMotion.value) return "已按系统偏好关闭自动轮播";
  return userPaused.value ? "继续轮播" : "暂停轮播";
});

/**
 * 播放/暂停控件**常显**（spec 432：控件行必须提供播放/暂停切换；票 437 评审）。
 *
 * 「减少动态」下按钮保持可见但**禁用**（`disabled` + `aria-disabled`）：spec 432 要求
 * 该偏好下仍不启动自动播放，读者再点「继续轮播」也不能把定时器开起来。禁用比隐藏好：
 * 控件行结构不变、读者能看到"为什么没动"，而不是一个凭空消失的按钮。
 */
const hasPlaybackControl = computed(
  () => props.autoPlayInterval > 0 && props.banners.length > 1,
);

function toggleAutoPlay() {
  // 防御：禁用态在浏览器里不会派发 click，但键盘/程序化触发仍要挡住。
  if (prefersReducedMotion.value) return;
  userPaused.value = !userPaused.value;
  if (userPaused.value) stopAutoPlay();
  else restartAutoPlay();
}

/* ------------------------------------------------------------------------------------
 * 预载下一张（票 432）
 *
 * 用 `new Image()` 提前拉下一张封面，让换图时新图已经在缓存里。
 * 调度放在空闲回调里（没有 requestIdleCallback 的环境退到下一帧），
 * 不与首屏正在加载的这张最大图抢带宽；不引新依赖。
 * ---------------------------------------------------------------------------------- */
const preloadedCovers = new Set<string>();
let cancelPreload: (() => void) | null = null;

function schedulePreload() {
  if (typeof window === "undefined" || props.banners.length < 2) return;

  const nextCover = props.banners[(currentIndex.value + 1) % props.banners.length]?.cover;
  if (!nextCover || preloadedCovers.has(nextCover) || cancelPreload) return;

  const runPreload = () => {
    cancelPreload = null;
    // 排队期间可能已经换图：按当时的下一张取，而不是排队那一刻的。
    const cover = props.banners[(currentIndex.value + 1) % props.banners.length]?.cover;
    if (!cover || preloadedCovers.has(cover) || typeof Image === "undefined") return;
    preloadedCovers.add(cover);
    const image = new Image();
    image.decoding = "async";
    image.src = cover;
  };

  if (typeof window.requestIdleCallback === "function") {
    const handle = window.requestIdleCallback(runPreload);
    cancelPreload = () => window.cancelIdleCallback(handle);
  } else {
    const handle = window.requestAnimationFrame(runPreload);
    cancelPreload = () => window.cancelAnimationFrame(handle);
  }
}

/**
 * 系统偏好切到「减少动态」时立刻停表、并把它记成"已暂停"：此前是自动播放着的，
 * 不能无视读者的偏好继续跑。反过来（reduce 关掉）不自动恢复：让读者自己决定。
 */
watch(prefersReducedMotion, (reduced) => {
  if (!reduced) return;
  userPaused.value = true;
  stopAutoPlay();
});

/* ------------------------------------------------------------------------------------
 * 生命周期与数据 / 索引联动
 * ---------------------------------------------------------------------------------- */
resetFrames();

// 换图：更新媒体层帧，并重新计时（避免"刚点完就被自动切走"）。
watch(currentIndex, (index) => {
  showFrame(index);
  if (timer !== null) restartAutoPlay();
  schedulePreload();
});

/** 轮播数据指纹：同一份数据重复计算不算变化，避免父组件重渲染就重建画面。 */
const bannersFingerprint = computed(() => props.banners.map(bannerKeyOf).join("||"));

watch(bannersFingerprint, () => {
  const outOfRange = currentIndex.value >= props.banners.length;
  if (outOfRange) {
    currentIndex.value = 0;
  }
  if (!outOfRange) {
    // 当前帧对应的轮播项变了（首屏数据到达 / 运营改了图）：重建，避免渲染旧封面。
    if (bannerKeyOf(frames.value[0]?.banner) !== bannerKeyOf(props.banners[currentIndex.value])) {
      resetFrames();
    }
  } else if (!props.banners.length) {
    resetFrames();
  }
  restartAutoPlay();
  schedulePreload();
});

onMounted(() => {
  // 水合安全：`userPaused` 初始恒为 true（SSR 与客户端首帧一致，按钮文案不会跳变），
  // 客户端的默认播放在这里再打开；「减少动态」下保持暂停，由读者显式点「继续轮播」。
  // 此时 `prefersReducedMotion` 已经由 useMotionPreference 在客户端读到了真实值。
  if (!prefersReducedMotion.value) userPaused.value = false;
  restartAutoPlay();
  document.addEventListener("visibilitychange", onVisibilityChange);
  schedulePreload();
});

onBeforeUnmount(() => {
  stopAutoPlay();
  clearLeavingTimers();
  cancelPreload?.();
  cancelPreload = null;
  document.removeEventListener("visibilitychange", onVisibilityChange);
});

/** 指针进入 / 键盘聚焦：暂停（被动暂停，不改 `userPaused`）。 */
function pauseAutoPlay() {
  stopAutoPlay();
}

/** 指针离开 / 焦点移出：重新开始一轮（读者显式暂停时不会恢复）。 */
function resumeAutoPlay() {
  restartAutoPlay();
}
</script>

<template>
  <!--
    轮播已并入首屏封面（ADR-0011 决策 8，GLOSSARY「首屏封面」）：
    有图时它是封面最底一层的「媒体舞台」（底图 + 底部渐黑 + 控件），
    无图时退回既有空态组件（ProfileEmptyState），不渲染任何控件。
    ≥2 张才出现控件（1 张 / 0 张都不该有可点的箭头与圆点）。
  -->
  <div
    class="banner-carousel"
    :class="currentBanner ? 'banner-stage' : 'banner-empty'"
    :style="crossfadeStyle"
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

        换图时这里会短暂同时存在两帧（票 432）：新帧在最前、先绘制（叠在下面），
        旧帧带 `is-leaving` 淡出 —— 两帧叠起来就是交叉淡入，不是硬切。
      -->
      <template v-for="frame in frames" :key="frame.key">
        <NuxtLink
          v-if="frame.banner.href"
          :to="frame.banner.href"
          class="banner-photo"
          :class="{ 'is-failed': frame.failed, 'is-leaving': frame.leaving }"
          :style="photoStyle"
        >
          <img
            class="banner-photo__img"
            :src="frame.banner.cover"
            :alt="`首页轮播图 ${frame.index + 1}`"
            @error="frame.failed = true"
          />
        </NuxtLink>
        <div
          v-else
          class="banner-photo"
          :class="{ 'is-failed': frame.failed, 'is-leaving': frame.leaving }"
          :style="photoStyle"
        >
          <img
            class="banner-photo__img"
            :src="frame.banner.cover"
            :alt="`首页轮播图 ${frame.index + 1}`"
            @error="frame.failed = true"
          />
        </div>
      </template>

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
          <p v-if="titleText" :key="`caption-${currentIndex}`" class="banner-title">{{ titleText }}</p>
          <div v-if="banners.length > 1" class="banner-controls" role="group" aria-label="轮播控件">
            <!-- 左半边：显式的播放/暂停 + 圆点；箭头仍在右端。整行还是渐黑区里的内容。 -->
            <div class="banner-left">
              <button
                v-if="hasPlaybackControl"
                type="button"
                class="banner-playback"
                :aria-label="playbackLabel"
                :aria-disabled="playbackDisabled ? 'true' : undefined"
                :disabled="playbackDisabled"
                @click="toggleAutoPlay"
              >
                <span :key="userPaused ? 'play' : 'pause'" class="banner-playback__icon">
                  <IconPlayerPause v-if="!userPaused" :size="18" aria-hidden="true" />
                  <IconPlayerPlay v-else :size="18" aria-hidden="true" />
                </span>
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
                  <span class="banner-dot__ring" aria-hidden="true" />
                  <span class="banner-dot__pill" aria-hidden="true" />
                </button>
              </div>
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
 * 换图交叉淡入（票 432）：出场帧留在画面上淡出，新帧已叠在它下面 —— 两帧一起
 * 读出来就是一次 A→B 的交叉，而不是硬切。
 *
 * 只动 opacity；用 transition 而不是 keyframes：过渡中途反向切换时由新值接管，
 * 不会把一整段动画重播一遍。时长来自根节点的 `--banner-crossfade`（motion.ts 的
 * 命名预设），减少动态下直接切（见文件末尾的媒体查询）。
 */
.banner-photo.is-leaving {
  opacity: 0;
  transition: opacity var(--banner-crossfade) var(--motion-ease);
  pointer-events: none;
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
  font-family: var(--font-sans);
  font-size: clamp(0.9rem, 2.2vw, 1.5rem);
  font-weight: 600;
  letter-spacing: var(--type-tracking-sm);
  line-height: var(--type-leading-sm);
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

/* 控件行左侧：显式播放/暂停 + 圆点，靠左成一组；箭头仍靠右。 */
.banner-left {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

/*
 * 播放/暂停（票 432）：读者能明确让轮播停下来，不必靠悬停"碰巧"暂停。
 * 命中区 44px（与圆点/箭头同一档），图形走反白 token，仍在渐黑区这一行里。
 */
.banner-playback {
  display: inline-flex;
  min-height: 44px;
  min-width: 44px;
  align-items: center;
  justify-content: center;
  border: 0;
  background: transparent;
  color: var(--color-on-inverse);
  cursor: pointer;
  touch-action: manipulation;
}

/* 减少动态（票 423）：按钮可见但禁用 —— 控件行不变，读者能看到"已按系统偏好关闭"，
 * 但不再有任何"点一下就能启动自动轮播"的路径。 */
.banner-playback:disabled {
  color: var(--color-on-inverse-muted);
  cursor: default;
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

/*
 * 圆点用**两层**表示两种状态，而不是让同一个点把 width 从 8px 补间到 28px：
 * width 是布局属性（ADR 只登记了两处布局例外），每换一张图都会让控件行重排一次。
 * 现在两层都只动 opacity / transform：未选中是一枚 8px 空心圈，选中的是 28px 实心
 * pill（`scaleX` 从 0.3 长到 1，合成属性，不触发布局）。
 */
.banner-dot {
  position: relative;
}

.banner-dot__ring {
  display: block;
  height: 8px;
  width: 8px;
  border: 1px solid var(--color-on-inverse-muted);
  border-radius: var(--radius-pill);
  background: transparent;
  transition: opacity var(--motion-fast) var(--motion-ease);
}

.banner-dot__pill {
  position: absolute;
  top: 50%;
  left: 50%;
  display: block;
  height: 8px;
  width: 28px;
  translate: -50% -50%;
  transform: scaleX(0.3);
  border-radius: var(--radius-pill);
  background: var(--color-on-inverse);
  opacity: 0;
  transition:
    opacity var(--motion-fast) var(--motion-ease),
    transform var(--motion-fast) var(--motion-ease);
}

/* 当前项：圈隐去、pill 长开 —— 与未选中的空心点一眼可分。 */
.banner-dot.is-active .banner-dot__ring {
  opacity: 0;
}

.banner-dot.is-active .banner-dot__pill {
  transform: scaleX(1);
  opacity: 1;
}

/* 标题随图切换淡入（票 441 加法）：图片是 160ms 交叉淡入，标题若硬切，一次换图
 * 只完成了一半。key 跟着 currentIndex 重挂，动画从 0 起跑；减少动态下直接切。 */
.banner-title {
  animation: banner-caption-in var(--banner-crossfade) var(--motion-ease);
}

@keyframes banner-caption-in {
  from {
    opacity: 0;
  }

  to {
    opacity: 1;
  }
}

/* 播放/暂停图标：状态切换时重挂，120ms 淡入 + 轻微放大，让"停了"有回应。 */
.banner-playback__icon {
  display: inline-flex;
  animation: banner-icon-in var(--motion-fast) var(--motion-ease);
}

@keyframes banner-icon-in {
  from {
    opacity: 0;
    transform: scale(0.9);
  }

  to {
    opacity: 1;
    transform: none;
  }
}

/* 按下态：与箭头 / 圆点同一档（这个键此前完全没有按下反馈）。 */
.banner-playback {
  transition: transform var(--motion-fast) var(--motion-ease);
}

.banner-playback:active:not(:disabled) {
  transform: scale(0.97);
}

/*
 * 按下态（票 427）：与 tailwind.css 的按钮词表同一档 —— 按下缩 .97、松手弹回。
 * 这两个是 scoped 类，全局再声明同名类会撞 `global-class-collision` 门禁，所以规则
 * 落在这里。基态登记 transform 过渡（圆点自身的 width 过渡留在内部 span 上）。
 */
.banner-arrow,
.banner-dot {
  transition: transform var(--motion-fast) var(--motion-ease);
}

.banner-arrow:active,
.banner-dot:active {
  transform: scale(0.97);
}

/* 键盘焦点：功能蓝描边压一圈深墨垫环 —— 任意底图上都 ≥3:1，焦点环不会被照片吃掉。 */
.banner-arrow:focus-visible,
.banner-playback:focus-visible,
.banner-dot:focus-visible span {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
  box-shadow: 0 0 0 4px var(--color-surface-inverse);
}

@media (prefers-reduced-motion: reduce) {
  .banner-dot__ring,
  .banner-dot__pill {
    transition: none;
  }

  /* 标题与图标是"内容交换"的桥，reduce 下直接切（不做淡入）。 */
  .banner-title,
  .banner-playback__icon {
    animation: none;
  }

  /* 减少动态：换图直接切，出场帧不再淡出（JS 侧也不再安排出场帧）。 */
  .banner-photo.is-leaving {
    transition: none;
  }

  /* 按下态（票 427）：减少动态下不缩放，改走反白描边/底色 —— 反馈仍在，只是不位移。
   * 底色是语义 token（反白 16% 档），不在组件里用 color-mix 混裸 alpha。 */
  .banner-arrow:active {
    transform: none;
    background-color: var(--color-on-inverse-line-strong);
  }

  .banner-dot:active,
  .banner-playback:active:not(:disabled) {
    transform: none;
  }

  .banner-dot:active .banner-dot__ring {
    border-color: var(--color-on-inverse);
    background-color: var(--color-on-inverse-muted);
  }

  /* 播放键在 reduce 下不可用（自动轮播已按系统偏好关闭），但仍保留底色反馈。 */
  .banner-playback:active:not(:disabled) {
    background-color: var(--color-on-inverse-line-strong);
  }
}
</style>
