<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { NButton } from "naive-ui";
import BannerCarousel from "~/components/article/BannerCarousel.vue";
import HomeFeedSection from "~/components/home/HomeFeedSection.vue";
import HomePinnedSection from "~/components/home/HomePinnedSection.vue";
import HomeSidebar from "~/components/home/HomeSidebar.vue";
import { useHomePageSources } from "~/composables/useHomePageSources";
import { useParallax } from "~/composables/useParallax";
import { useSectionReveal } from "~/composables/useSectionReveal";
import { BRAND, brandDescription, brandFullName } from "~/utils/brand";
import { MOTION_STAGGER_MS } from "~/utils/motion";

const siteStore = useSiteStore();

// 首页三个数据源（轮播 / 热门置顶 / 最新文章）在 composable 里**同时发起**，
// 并按渲染环境决定等谁：服务端等齐（SSR 的 HTML 里要有内容），
// 客户端跳转一个都不等（先出骨架），首屏 hydration 只等不在 payload 里的分页首屏。
const { bannerData, bannerPending, hotTopData, latestPager, latestRequestError } =
  await useHomePageSources();


useSeoMeta({
  title: siteStore.seo?.site_title || brandFullName(),
  description: brandDescription(siteStore.seo?.description),
  keywords: siteStore.seo?.keywords || "前端, Nuxt, Vue, OpenAPI",
  ogTitle: siteStore.seo?.site_title || brandFullName(),
  ogDescription: brandDescription(siteStore.seo?.description),
});

const banners = computed(() => bannerData.value?.list || []);
const hotTopArticles = computed(() => hotTopData.value?.list || []);
const latestArticles = computed(() => latestPager.currentItems.value);
const latestPending = computed(() => latestPager.pending.value);
const latestPage = computed(() => latestPager.currentPage.value);
const latestHasPrevious = computed(() => latestPager.hasPreviousPage.value);
const latestHasNext = computed(() => latestPager.hasNextPage.value);
const siteBootstrapPending = computed(() => !siteStore.fetched);
const latestFailed = computed(() => Boolean(latestRequestError.value));

// 首屏封面的分层动效（ADR-0013 决策 9，全部归口 #23 的同一份预算）：
//   · 装饰层（色带 / 网格刻度）走 useSectionReveal：只动 transform/opacity，位移 ≤12px；
//   · 线条场自己从右端延展入场（见 heroLines / linesArmed）；
//   · 文字层（h1/记号）**不做入场动画** —— 关键内容必须常显（详见模板里的注释）；
//   · 记号挂一条 useParallax：≤10px，reduced-motion 下归零。
// 首屏不自己写关键帧动画，也不对布局属性做过渡。
const { target: coverRevealTarget, revealStyle } = useSectionReveal();
const { parallaxStyle } = useParallax();

/**
 * 右栏线条场（纯装饰，aria-hidden）。
 *
 * 三条几何约定（前两条来自用户，第三条是我加的审美判断）：
 * 1. **右端贴右**：全部用 `right` 定位（0–1.5% 的小抖动），右端点不许离开最右边太远；
 * 2. **中间最长、上下渐短**：`width` 以垂直中位为峰值向两端递减；
 * 3. **粗细混排 + 间距不均匀**：只用 1px 细线会像表格网格，所以掺 4–12px 的板块；
 *    `top` 不按等差排布，而是成簇（有的两条挨着 2–3%，有的隔 12–14%），
 *    这样才像参考图里那种"排版事故感"的层次，而不是一堆刻度线。
 *
 * `tone` 只允许 ink / blue / coral / yellow：ink 是反白线，后三者是首屏锁定的色相。
 */
const RAW_HERO_LINES = [
  { tone: "ink", style: { top: "2%", right: "0.5%", width: "30%", height: "1px" } },
  { tone: "ink", style: { top: "6%", right: "0%", width: "58%", height: "6px" } },
  { tone: "ink", style: { top: "8.5%", right: "1.5%", width: "22%", height: "1px" } },
  { tone: "blue", style: { top: "19%", right: "0%", width: "70%", height: "5px" } },
  { tone: "ink", style: { top: "22%", right: "0.5%", width: "34%", height: "1px" } },
  { tone: "ink", style: { top: "35%", right: "0%", width: "74%", height: "2px" } },
  { tone: "ink", style: { top: "37.5%", right: "1%", width: "62%", height: "10px" } },
  { tone: "yellow", style: { top: "41%", right: "0%", width: "76%", height: "12px" } },
  { tone: "ink", style: { top: "55%", right: "0.5%", width: "58%", height: "1px" } },
  { tone: "coral", style: { top: "58%", right: "1.5%", width: "44%", height: "5px" } },
  { tone: "ink", style: { top: "72%", right: "0%", width: "30%", height: "8px" } },
  { tone: "blue", style: { top: "74.5%", right: "0.5%", width: "17%", height: "1px" } },
  { tone: "ink", style: { top: "88%", right: "0%", width: "24%", height: "1px" } },
  { tone: "ink", style: { top: "91%", right: "1%", width: "12%", height: "4px" } },
  { tone: "ink", style: { top: "94.5%", right: "0%", width: "9%", height: "1px" } },
] as const;

/**
 * 入场延迟：**不均匀**，一律取 `MOTION_STAGGER_MS` 的整数倍。
 *
 * 之前用 `|index - 中线| × 40ms` 算等差延迟，跑起来像节拍器（可预测 = 廉价）。
 * 现在改成手写的一组跳数：中段那几条仍然先出（0–40ms），两端晚到（240–320ms），
 * 但相邻两条的间隔在 0–320ms 之间乱跳 —— 落位有先后、没有节奏感，才像"随手排的版"。
 */
const HERO_LINE_DELAY_STEPS = [5, 3, 1, 7, 6, 1, 8, 0, 8, 7, 3, 5, 7, 8, 6] as const;

const heroLines = RAW_HERO_LINES.map((line, index) => ({
  ...line,
  delay: `${(HERO_LINE_DELAY_STEPS[index] ?? 0) * MOTION_STAGGER_MS}ms`,
}));

/**
 * 线条的"从右端长出来"：先收起初态（is-armed），下一帧再置终态（is-drawn）。
 *
 * 为什么不纯靠 CSS 动画关键帧：全站约定装饰不得自写关键帧（有设计测试锁住），
 * 入场一律用 transition + 状态类。arm 与 draw 分两帧，是为了让浏览器先绘制 scaleX(0)，
 * 否则首帧就是终态、transition 不会跑起来。
 *
 * 无 JS / 减少动态时都不会 arm：线条直接是终态可见（装饰不该因为脚本没跑就消失）。
 */
const linesArmed = ref(false);
const linesDrawn = ref(false);

onMounted(() => {
  if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    return;
  }

  linesArmed.value = true;
  requestAnimationFrame(() => {
    linesDrawn.value = true;
  });
});

async function handlePreviousLatestPage() {
  await latestPager.goToPreviousPage();
}

async function handleNextLatestPage() {
  await latestPager.goToNextPage();
}

async function handleRetryLatest() {
  await latestPager.refreshCurrentPage();
}
</script>

<template>
  <div class="page-stack" data-page="home">
    <!--
      首屏封面（ADR-0013）：全站唯一的编辑部式例外区。
      色带 / 网格刻度 / 方括号与箭头记号 / 胶囊小签只允许在这里出现（有设计测试锁住）。
      图层自下而上：底图（BannerCarousel 的媒体层）→ 底部渐黑 → 色带 → 网格刻度 →
      超大 h1 → 记号 → 渐黑区里的轮播控件。
    -->
    <section ref="coverRevealTarget" class="home-cover" data-home-hero>
      <!-- 2 色带：黄 / 功能蓝 / 珊瑚，仅首屏可作装饰（3 条，窄屏只留 1 条） -->
      <div class="home-cover__bands motion-reveal" :style="revealStyle" aria-hidden="true">
        <span class="home-cover__band home-cover__band--yellow" />
        <span class="home-cover__band home-cover__band--blue" />
        <span class="home-cover__band home-cover__band--coral" />
      </div>

      <!-- 3 网格线与下沿刻度：≥1024px 才显示（窄屏隐藏） -->
      <div class="home-cover__grid motion-reveal" :style="revealStyle" aria-hidden="true">
        <span class="home-cover__grid-line home-cover__grid-line--a" />
        <span class="home-cover__grid-line home-cover__grid-line--b" />
        <span class="home-cover__grid-line home-cover__grid-line--c" />
      </div>

      <!-- 4 超大 h1 在上；轮播是它下方偏右的面板，右栏线条场会压过面板 -->
      <div class="home-cover__inner">
        <!-- 关键内容不进入场动画：错峰入场的初态是 opacity:0（SSR 会内联），
             标题一旦被包进去，动画没跑起来就等于整站首页没有 h1（减少动态的用户必现）。 -->
        <div class="home-cover__copy">
          <div class="eyebrow home-cover__eyebrow">{{ BRAND.name.en }} · {{ BRAND.name.zh }}</div>
          <h1 class="home-cover__title">
            <span class="home-cover__title-name">{{ BRAND.name.en.toUpperCase() }}</span>
            <span class="home-cover__title-bracket">[{{ BRAND.name.zh }}]</span>
          </h1>
        </div>

        <div class="home-cover__panel" :class="{ 'home-cover__panel--empty': !banners.length }">
          <!-- 0 底图 + 1 底部渐黑 + 控件：轮播作为大字下方的面板（≥2 张才出现控件） -->
          <BannerCarousel
            class="home-cover__media"
            :banners="banners"
            :pending="bannerPending"
            :photo-style="parallaxStyle"
          />
        </div>

        <!-- 右栏线条场：从右端"长出来"（scaleX + transform-origin: right），中线先出、两端后出 -->
        <div
          class="home-cover__lines"
          :class="{ 'is-armed': linesArmed, 'is-drawn': linesDrawn }"
          aria-hidden="true"
        >
          <span
            v-for="(line, index) in heroLines"
            :key="index"
            class="home-cover__line"
            :class="`home-cover__line--${line.tone}`"
            :style="[line.style, { transitionDelay: line.delay }]"
          />
        </div>

        <!-- 记号与刻度同样不进入场动画：它们是 aria-hidden 的装饰，但也没必要承担"可能不可见"的风险 -->
        <div class="home-cover__aside">
          <ul class="home-cover__marks" aria-hidden="true">
            <li class="home-cover__mark home-cover__arrow motion-parallax" :style="parallaxStyle">↗</li>
            <li class="home-cover__mark home-cover__capsule">VOL. 01</li>
            <li class="home-cover__mark home-cover__bracket">]</li>
          </ul>
          <ul class="home-cover__ticks" aria-hidden="true">
            <li v-for="tick in ['000', '025', '050', '075', '100']" :key="tick" class="home-cover__tick">
              {{ tick }}
            </li>
          </ul>
        </div>
      </div>
    </section>

    <div class="hero-grid">
      <div class="page-stack">
        <!-- 热门置顶运营位（ADR-0017）：无数据 / 加载中 / 失败都整块不渲染，组件内部控制。 -->
        <HomePinnedSection :articles="hotTopArticles" />

        <HomeFeedSection
          :articles="latestArticles"
          :pending="latestPending"
          :failed="latestFailed"
          @retry="handleRetryLatest"
        >
          <template #footer>
            <div
              v-if="latestPage > 1 || latestHasNext"
              class="mt-6 flex flex-wrap items-center justify-end gap-3 border-t border-line-faint pt-3 text-sm text-ink-muted"
              data-role="feed-pager"
            >
              <div class="flex items-center gap-3 blueprint-actions">
                <NButton quaternary :disabled="!latestHasPrevious" @click="handlePreviousLatestPage">上一页</NButton>
                <NButton
                  type="primary"
                  ghost
                  :disabled="!latestHasNext"
                  :loading="latestPending"
                  @click="handleNextLatestPage"
                >
                  下一页
                </NButton>
              </div>
            </div>
          </template>
        </HomeFeedSection>
      </div>

      <HomeSidebar
        :pending="siteBootstrapPending || bannerPending"
        :runtime-config="siteStore.runtimeConfig"
        :articles="latestArticles"
        sticky
      />
    </div>
  </div>
</template>

<style scoped>
/*
 * 首屏封面样式集中在同一个文件里（ADR-0013 约束 1/3）。
 *
 * 满幅实现：width:100vw + margin-inline:calc(50% - 50vw) 让封面脱离 page-shell 的
 * max-width/px 内边距；1px 的视口溢出不在这里自裁，而是由布局层（site-layout）
 * 的 overflow-x: clip 兜住，避免桌面上多出一条横向滚动条。
 * 没有卡片边框、没有纸面底色：图像缺失时坐在深墨兜底面上（与文章封面兜底同一口径），
 * 所以反白文字在任何情况下都满足对比度。
 */
.home-cover {
  /* 封面的左右内边距：线条场要抵到视口右沿，所以抽成变量给它取负值用。 */
  --cover-pad-x: clamp(1rem, 5vw, 4rem);
  position: relative;
  isolation: isolate;
  display: flex;
  flex-direction: column;
  justify-content: center;
  width: 100vw;
  max-width: 100vw;
  min-height: clamp(380px, 62vh, 620px);
  margin-inline: calc(50% - 50vw);
  overflow: clip;
  background: var(--color-surface-inverse);
  color: var(--color-on-inverse);
  padding: clamp(1.5rem, 4vw, 3rem) var(--cover-pad-x);
}

/* ---- 图层 2：色带（仅首屏可出现的装饰；黄 / 功能蓝 / 珊瑚三色，不得第四色） ---- */
.home-cover__bands {
  position: absolute;
  inset: 0;
  z-index: 2;
  overflow: hidden;
  pointer-events: none;
}

.home-cover__band {
  position: absolute;
  display: block;
  height: clamp(8px, 1.1vw, 14px);
}

.home-cover__band--yellow {
  top: 0;
  left: 0;
  width: 100%;
  background: var(--jelly-yellow);
}

.home-cover__band--blue {
  top: 52%;
  right: 0;
  width: 28%;
  background: var(--jelly-interactive);
}

.home-cover__band--coral {
  bottom: 0;
  left: 0;
  width: 26%;
  background: var(--jelly-coral);
}

/* ---- 图层 3：网格线与下沿刻度（≥1024px 才显示） ---- */
.home-cover__grid {
  position: absolute;
  inset: 0;
  z-index: 3;
  display: none;
  pointer-events: none;
  background-image: linear-gradient(to right, var(--color-line-faint) 1px, transparent 1px);
  background-size: 25% 100%;
}

.home-cover__grid-line {
  position: absolute;
  display: block;
  background: var(--color-line-faint);
}

.home-cover__grid-line--a {
  inset-block: 0;
  left: 0;
  width: 1px;
}

.home-cover__grid-line--b {
  inset-block: 0;
  right: 0;
  width: 1px;
}

.home-cover__grid-line--c {
  inset-inline: 0;
  bottom: 18%;
  height: 1px;
}

/* ---- 图层 4/5：超大 h1 在上，轮播面板在它下方（略偏右），右栏线条场与刻度 ---- */
.home-cover__inner {
  position: relative;
  z-index: 4;
  display: grid;
  grid-template-columns: minmax(0, 52%) minmax(0, 1fr);
  grid-template-rows: auto auto;
  align-items: end;
  gap: clamp(1rem, 2.5vw, 2.5rem);
  width: 100%;
}

/* 左栏：轮播面板，位于大字下方并整体右移一点（margin 让它不贴左沿）。
   它同时是 .banner-stage（absolute inset: 0）的定位上下文 —— 给 relative + 固定比例，
   内部底图 / 罩层 / 玻璃控件就自然填满面板。
   比例锁死 16:9（ADR-0015 决策 3）：所有断点同一比例，不用高度档 ——
   固定高度会随屏宽变化，同一张图在手机和显示器上就被裁成两种形状。 */
.home-cover__panel {
  position: relative;
  z-index: 2;
  grid-column: 1;
  grid-row: 2;
  margin-inline-start: clamp(1.25rem, 4vw, 4rem);
  display: flex;
  align-items: flex-end;
  justify-content: flex-start;
  width: 100%;
  /* 固定 16:9：宽度由左栏决定，高度跟着算出来 —— 任何屏宽下比例都不变，
     轮播图用 object-fit: cover 裁切，不会被拉扁或拉长。 */
  aspect-ratio: 16 / 9;
  padding: clamp(0.75rem, 1.4vw, 1.25rem);
  overflow: clip;
  border-radius: var(--radius-lg);
  /* 面板是"屏幕"而不是空白框：给一层极淡的反白染色，让它自己成立；
     不支持 color-mix 时退化为纯反白面（不依赖它也能看出边界）。 */
  background: var(--color-surface-inverse);
  background: color-mix(in srgb, var(--color-on-inverse) 6%, var(--color-surface-inverse));
}

/* 线条场：铺满整个内层，因此 left < 44% 的线条会**压到轮播面板上**（用户要的凌乱感）。
   z-index 高于面板、低于跨栏标题与记号 —— 装饰不许盖住可读文本。 */
.home-cover__lines {
  position: absolute;
  inset: 0 calc(-1 * var(--cover-pad-x)) 0 0;
  z-index: 3;
  display: none;
  overflow: clip;
  pointer-events: none;
}

.home-cover__line {
  position: absolute;
  display: block;
  /* 入场：从**右端**长出来（缩放原点贴右），只动 transform/opacity，
     时长/缓动全站同一份预算；交错延迟由数据按"中线→两端"算好，走内联 transition-delay。 */
  --line-opacity: 1;
  transform-origin: right center;
  border-radius: var(--radius-pill);
  transition:
    transform var(--motion-base) var(--motion-ease),
    opacity var(--motion-base) var(--motion-ease);
}

/* 只有 JS 就位后才收起初态：无 JS / 减少动态时线条直接是终态（装饰不该消失）。 */
.home-cover__lines.is-armed:not(.is-drawn) .home-cover__line {
  opacity: 0;
  transform: scaleX(0);
}

.home-cover__lines.is-drawn .home-cover__line {
  opacity: var(--line-opacity);
  transform: scaleX(1);
}

/* 反白细线用 on-inverse + 透明度，避免为装饰再造一个新色。
   透明度走 --line-opacity，这样"收起 / 展开"两个状态不会把调子覆盖掉。 */
.home-cover__line--ink {
  --line-opacity: 0.5;
  background: var(--color-on-inverse);
}

.home-cover__line--blue {
  background: var(--jelly-interactive);
}

.home-cover__line--coral {
  background: var(--jelly-coral);
}

.home-cover__line--yellow {
  background: var(--jelly-yellow);
}

/* 超大 h1：占满第一行（大字压在最上面，权重最高）。 */
.home-cover__copy {
  position: relative;
  z-index: 4;
  grid-column: 1 / -1;
  grid-row: 1;
  min-width: 0;
}

/* 11px 小字从"页头/首屏 h1"降为普通文本：颜色跟着反白层走（不再用纸面次级墨）。 */
.home-cover__eyebrow {
  color: var(--color-on-inverse-muted);
}

.home-cover__title {
  font-family: var(--font-title);
  font-size: clamp(2rem, 9vw, 6rem);
  font-weight: var(--type-weight-title);
  line-height: 0.92;
  letter-spacing: -0.04em;
  color: var(--color-on-inverse);
  text-wrap: balance;
}

.home-cover__title-name {
  display: inline-block;
}

.home-cover__title-bracket {
  display: inline-block;
  margin-inline-start: 0.5em;
  font-size: clamp(0.95rem, 2.4vw, 1.75rem);
  font-weight: 400;
  letter-spacing: 0;
  color: var(--color-on-inverse-muted);
}

/* 面板里的空态不要"卡片"：留边框+纸面底会和面板边框叠成双重容器（用户说的"很怪"）。
   改成像封面左下角的一行字幕 —— 透明底、左对齐、贴底，线条压上来也不挡字。 */
.home-cover__panel :deep(.empty-state) {
  min-height: 0;
  align-items: flex-start;
  justify-content: flex-end;
  gap: 0.4rem;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  text-align: start;
}

.home-cover__panel :deep(.empty-state__title) {
  margin: 0;
  color: var(--color-on-inverse);
}

.home-cover__panel :deep(.empty-state__body),
.home-cover__panel :deep(.empty-state__hints) {
  max-width: 30rem;
  color: var(--color-on-inverse-muted);
}

/* 没有轮播图时，面板要压在装饰线条之上：线条压着图片是"凌乱美"，
   压着空态文案就是可读性问题。 */
.home-cover__panel--empty {
  z-index: 4;
}

/* 空态时媒体层不再撑满面板，否则它会吃掉 align-items: flex-end，字幕会飘到顶上去。 */
.home-cover__panel--empty .home-cover__media {
  height: auto;
}
.home-cover__aside {
  display: flex;
  position: relative;
  z-index: 4;
  grid-column: 2;
  grid-row: 2;
  min-width: 0;
  flex-direction: column;
  align-items: flex-end;
  justify-content: flex-end;
  gap: 1.25rem;
}

.home-cover__marks {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.home-cover__mark {
  color: var(--color-on-inverse);
  line-height: 1;
}

.home-cover__arrow {
  font-size: clamp(1.5rem, 3vw, 2.5rem);
}

.home-cover__bracket {
  font-family: var(--font-title);
  font-size: clamp(2rem, 5vw, 4rem);
  color: var(--color-on-inverse-muted);
}

/* 黑底白字胶囊小签（纯装饰，aria-hidden） */
.home-cover__capsule {
  padding: 0.35rem 0.75rem;
  border: 1px solid var(--color-on-inverse-muted);
  border-radius: var(--radius-pill);
  background: var(--color-surface-inverse);
  color: var(--color-on-inverse);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.12em;
}

.home-cover__ticks {
  display: flex;
  width: 100%;
  justify-content: space-between;
  margin: 0;
  padding: 0.5rem 0 0;
  border-top: 1px solid var(--color-on-inverse-muted);
  list-style: none;
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.08em;
  color: var(--color-on-inverse-muted);
}

.home-cover__tick {
  min-width: 0;
}

/* ---- 图层 0/1/6：轮播媒体层（底图 + 底部渐黑 + 控件）由 BannerCarousel 渲染 ----
   面板模式下它是普通块级元素，铺满 .home-cover__panel（面板已给它定位上下文）。 */
.home-cover__media {
  position: relative;
  display: block;
  height: 100%;
  width: 100%;
}

@media (min-width: 1024px) {
  .home-cover__grid,
  .home-cover__lines {
    display: block;
  }
}

@media (max-width: 767px) {
  .home-cover__inner {
    grid-template-columns: minmax(0, 1fr);
    align-items: start;
  }

  /* 窄屏：顺序仍是大字 → 面板 → 记号；面板铺满整行、不再右移。 */
  .home-cover__copy {
    grid-column: 1;
    grid-row: 1;
  }

  /* 窄屏：顺序仍是大字 → 面板 → 记号；面板铺满整行、不再右移。
     比例不在这里改 —— 16:9 是所有断点同一份定义（ADR-0015）。 */
  .home-cover__panel {
    grid-column: 1;
    grid-row: 2;
    margin-inline-start: 0;
  }

  .home-cover__title {
    font-size: clamp(2rem, 9vw, 3rem);
  }

  .home-cover__aside {
    grid-column: 1;
    grid-row: 3;
    align-items: flex-start;
  }

  .home-cover__grid {
    display: none;
  }

  .home-cover__lines {
    display: none;
  }

  .home-cover__ticks {
    display: none;
  }

  .home-cover__bracket {
    display: none;
  }

  .home-cover__arrow {
    display: none;
  }

  .home-cover__band--blue {
    display: none;
  }

  .home-cover__band--coral {
    display: none;
  }
}

/* 减少动态：线条不做延展，直接静态到位（JS 那边也不会 arm，这里是兜底）。 */
@media (prefers-reduced-motion: reduce) {
  .home-cover__lines .home-cover__line {
    transition: none;
  }

  .home-cover__lines.is-armed .home-cover__line {
    opacity: var(--line-opacity);
    transform: none;
  }
}
</style>
