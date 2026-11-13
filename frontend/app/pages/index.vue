<script setup lang="ts">
import { computed } from "vue";
import { NButton } from "naive-ui";
import BannerCarousel from "~/components/article/BannerCarousel.vue";
import BrandGlassHeroTitle from "~/components/brand/GlassHeroTitle.vue";
import HomeFeedSection from "~/components/home/HomeFeedSection.vue";
import HomeInkFluid from "~/components/home/HomeInkFluid.vue";
import HomePinnedSection from "~/components/home/HomePinnedSection.vue";
import HomeSidebar from "~/components/home/HomeSidebar.vue";
import { useHomePageSources } from "~/composables/useHomePageSources";
import { useParallax } from "~/composables/useParallax";
import { useSectionReveal } from "~/composables/useSectionReveal";
import { BRAND, brandDescription, brandFullName } from "~/utils/brand";
import { webSiteJsonLd } from "~/utils/seo";

const siteStore = useSiteStore();

// 首页三个数据源（轮播 / 热门置顶 / 最新文章）在 composable 里**同时发起**，
// 并按渲染环境决定等谁：服务端等齐（SSR 的 HTML 里要有内容），
// 客户端跳转一个都不等（先出骨架），首屏 hydration 只等不在 payload 里的分页首屏。
const { bannerData, bannerPending, hotTopData, sidebarCardsData, latestPager, latestRequestError } =
  await useHomePageSources();


useSeoMeta({
  // 站点标题回到品牌常量（票 192：site_info.title 已删除）。分享卡片与浏览器
  // 标题共用同一个兜底，保证 og:title 不为空；描述与关键词仍可由后台覆盖。
  title: brandFullName(),
  description: brandDescription(siteStore.seo?.description),
  keywords: siteStore.seo?.keywords || "前端, Nuxt, Vue, OpenAPI",
  ogTitle: brandFullName(),
  ogDescription: brandDescription(siteStore.seo?.description),
});

// 票 314 交付 6：站点声明 + 站内搜索（WebSite + SearchAction）。首页是 SSR 白名单里
// 唯一的另一个页面（ADR-0016 决策 1），所以这条 JSON-LD 一定出现在爬虫拿到的 HTML 里。
// SearchAction 的参数名必须是搜索页**真的读**的那一个（`/search?key=`）——写错不报错，
// 只会让"在搜索结果里再搜一次"跳到一页空结果。
const siteUrl = String(useRuntimeConfig().public.siteUrl || "");
useHead({
  script: [
    {
      key: "seo-website-jsonld",
      type: "application/ld+json",
      innerHTML: JSON.stringify(webSiteJsonLd({ siteName: brandFullName(), siteUrl })),
    },
  ],
});

const banners = computed(() => bannerData.value?.list || []);
const hotTopArticles = computed(() => hotTopData.value?.list || []);
// 主栏已经露出的文章 ID：右栏热门文章卡据此去重，同一篇文章不在两处出现。
const pinnedArticleIds = computed(() =>
  hotTopArticles.value.map((article) => String(article.id || "")).filter((id) => id.length > 0),
);
// 右栏卡片（票 111）：顺序/标题/种类/启用都由后端配置决定，这里只透传。
const sidebarCards = computed(() => sidebarCardsData.value?.list || []);
const latestArticles = computed(() => latestPager.currentItems.value);
const latestPending = computed(() => latestPager.pending.value);
const latestPage = computed(() => latestPager.currentPage.value);
const latestHasPrevious = computed(() => latestPager.hasPreviousPage.value);
const latestHasNext = computed(() => latestPager.hasNextPage.value);
const siteBootstrapPending = computed(() => !siteStore.fetched);
const latestFailed = computed(() => Boolean(latestRequestError.value));

// 首屏封面的分层动效（ADR-0011 决策 9，全部归口 #23 的同一份预算）：
//   · 装饰层（色带）走 useSectionReveal：只动 transform/opacity，位移 ≤12px；
//   · 文字层（h1/记号）**不做入场动画** —— 关键内容必须常显（详见模板里的注释）；
//   · 记号挂一条 useParallax：≤10px，reduced-motion 下归零；
//   · 指针墨流是独立的 RAF（composables/useInkFluid），reduce 下只画一帧静帧。
// 首屏不自己写关键帧动画，也不对布局属性做过渡。
// 2026-11-03 后续修订：右栏线条场（15 条横条 + 两段式入场）已退役，
// 改为「标题下发丝基线」的编辑部网格语言（见 ADR-0011 后续修订）。
const { target: coverRevealTarget, revealStyle } = useSectionReveal();
const { parallaxStyle } = useParallax();

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
      首屏封面（ADR-0011）：全站唯一的编辑部式例外区。
      色带 / 下沿刻度 / 方括号与箭头记号 / 胶囊小签 / 发丝基线 / 指针墨流
       只允许在这里出现（有设计测试锁住），装饰层都 aria-hidden。
      图层自下而上：底图（BannerCarousel 的媒体层）→ 指针墨流 → 色带 →
      超大 h1 → 记号与下沿刻度 → 底部渐黑区里的轮播控件。
       2026-11-03 后续修订：右栏线条场退役，改为编辑部网格语言。
       2026-11-10 后续修订：主题相关的细网格与四角裁切发丝线退场，换成指针墨流。
    -->
    <section ref="coverRevealTarget" class="home-cover" data-home-hero>
      <!-- 2 指针墨流：鼠标划过时在深墨底上晕开的流体墨迹，纯装饰、不吃点击 -->
      <HomeInkFluid
        class="home-cover__ink"
        aria-hidden="true"
        :intensity="0.95"
        :tint="0.5"
        :force="0.7"
      />

      <!-- 3 色带：黄 / 功能蓝 / 珊瑚，仅首屏可作装饰（3 条，窄屏只留 1 条） -->
      <div class="home-cover__bands motion-reveal" :style="revealStyle" aria-hidden="true">
        <span class="home-cover__band home-cover__band--yellow" />
        <span class="home-cover__band home-cover__band--blue" />
        <span class="home-cover__band home-cover__band--coral" />
      </div>

      <!-- 4 超大 h1 在上；轮播是它下方偏右的面板，右栏是记号与下沿刻度 -->
      <div class="home-cover__inner">
        <!-- 关键内容不进入场动画：错峰入场的初态是 opacity:0（SSR 会内联），
             标题一旦被包进去，动画没跑起来就等于整站首页没有 h1（减少动态的用户必现）。 -->
        <div class="home-cover__copy">
          <div class="eyebrow home-cover__eyebrow">{{ BRAND.name.en }} · {{ BRAND.name.zh }}</div>
          <h1 class="home-cover__title">
            <!-- 大字换成"碎玻璃"呈现（见 components/brand/GlassHeroTitle.vue）：
                 九块裁切碎片叠印出整字，指针视差 + 点击碎裂。
                 字标用衬线斜体（--font-wordmark，全站唯一例外），保留句点。
                 中文名不再挂在字标后面（由上方 eyebrow 承担）。
                 字标是首屏最上层：装饰色带 / 记号 / 轮播面板都在它下面。 -->
            <BrandGlassHeroTitle :text="BRAND.name.en + '.'" />
          </h1>
        </div>

        <!-- 标题下发丝基线：只跨左栏（与轮播面板同宽），落在标题与面板的栅格间隙中线。
             纯装饰、aria-hidden、不吃点击，属于 2026-11-03 的编辑部网格语言。 -->
        <span class="home-cover__rule" aria-hidden="true" />

        <div class="home-cover__panel" :class="{ 'home-cover__panel--empty': !banners.length }">
          <!-- 0 底图 + 1 底部渐黑 + 控件：轮播作为大字下方的面板（≥2 张才出现控件） -->
          <BannerCarousel
            class="home-cover__media"
            :banners="banners"
            :pending="bannerPending"
            :photo-style="parallaxStyle"
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

    <!-- 右栏没有启用卡片时整块不渲染，同时把栅格收成单列，避免右侧留下空列。 -->
    <div class="hero-grid" :class="{ 'lg:grid-cols-1': !sidebarCards.length }">
      <div class="page-stack">
        <!-- 热门置顶运营位（ADR-0015）：无数据 / 加载中 / 失败都整块不渲染，组件内部控制。 -->
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
        :sidebar-cards="sidebarCards"
        :pinned-article-ids="pinnedArticleIds"
        sticky
      />
    </div>
  </div>
</template>

<style scoped>
/*
 * 首屏封面样式集中在同一个文件里（ADR-0011 约束 1/3）。
 *
 * 满幅实现：width:100vw + margin-inline:calc(50% - 50vw) 让封面脱离 page-shell 的
 * max-width/px 内边距；1px 的视口溢出不在这里自裁，而是由布局层（site-layout）
 * 的 overflow-x: clip 兜住，避免桌面上多出一条横向滚动条。
 * 没有卡片边框、没有纸面底色：图像缺失时坐在深墨兜底面上（与文章封面兜底同一口径），
 * 所以反白文字在任何情况下都满足对比度。
 */
.home-cover {
  /* 封面的左右内边距。 */
  --cover-pad-x: clamp(1rem, 5vw, 4rem);
  /* 右下角整组（记号 + 下沿刻度）的刚体位移：右 56 / 下 40，
     抵到边界前留 8px 呼吸，避免 overflow: clip 切掉数字字形下缘（G8 定案）。 */
  --cover-aside-x: 56px;
  --cover-aside-y: 40px;
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

/* ---- 图层 1：指针墨流（画布铺满封面，压在色带、轮播面板与内容之下） ---- */
.home-cover__ink {
  position: absolute;
  inset: 0;
  z-index: 1;
  pointer-events: none;
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

/* ---- 图层 4/5：超大 h1 在上，轮播面板在它下方（略偏右），右栏记号与下沿刻度 ---- */
.home-cover__inner {
  position: relative;
  z-index: 4;
  display: grid;
  grid-template-columns: minmax(0, 52%) minmax(0, 1fr);
  grid-template-rows: auto auto;
  align-items: end;
  gap: clamp(1rem, 2.5vw, 2.5rem);
  width: 100%;
  /* 首屏内容给一个宽度上限：轮播面板固定 16:9，宽度跟着窗口一路长高，
     超宽屏上首屏能被顶到 800px 以上，底图被放大到超过原图分辨率就发糊。
     限住这一层，标题、面板、基线一起收敛，比例不变，只是不再无限长大。 */
  max-width: 1440px;
  margin-inline: auto;
}

/* 左栏：轮播面板，位于大字下方并整体右移一点（margin 让它不贴左沿）。
   它同时是 .banner-stage（absolute inset: 0）的定位上下文 —— 给 relative + 固定比例，
   内部底图 / 罩层 / 玻璃控件就自然填满面板。
   比例锁死 16:9（ADR-0013 决策 3）：所有断点同一比例，不用高度档 ——
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

/* ---- 2026-11-03 编辑部网格语言：标题下发丝基线 ----
    取代已退役的右栏线条场。基线只用中性反白色，
    黄 / 功能蓝 / 珊瑚三色仍然只属于色带（ADR-0011 约束 1、ADR-0011 约束 2）。 */

/* 基线只跨左栏（与轮播面板同宽、左缘对齐），落在标题与面板的栅格间隙中线。
   用 grid 定位而不是 JS 量坐标：它是 inner 里的一个 1px 网格项，
   align-self:end 贴在第一行底部，再 translateY 半个行距进到间隙里。 */
.home-cover__rule {
  grid-column: 1;
  grid-row: 1;
  align-self: end;
  width: 100%;
  height: 1px;
  margin-inline-start: clamp(1.25rem, 4vw, 4rem);
  transform: translateY(calc(clamp(1rem, 2.5vw, 2.5rem) / 2));
  background: var(--color-on-inverse);
  opacity: 0.38;
  pointer-events: none;
}

/* 超大 h1：占满第一行（大字压在最上层，权重最高）。
   z-index 6 高于右侧记号组（4）与轮播面板（2）：字标不能被任何装饰压住。 */
.home-cover__copy {
  position: relative;
  z-index: 6;
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

/* 2026-11-13：字标后面的中文方括号 `[果冻盒]` 退场（中文名由上方 eyebrow 承担），
   字标独占一行，不再与副标共享基线。 */

/* 面板里的空态不要"卡片"：留边框+纸面底会和面板边框叠成双重容器（用户说的"很怪"）。
   改成像封面左下角的一行字幕 —— 透明底、左对齐、贴底。 */
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

/* 没有轮播图时，面板压在装饰层之上，空态文案不被任何装饰盖住。
   基线等装饰只落在空隙与四角，不压可读内容。 */
.home-cover__panel--empty {
  z-index: 4;
}

/* 空态时媒体层回到普通文档流：否则绝对定位会让它脱离流，把字幕压到顶上。 */
.home-cover__panel--empty .home-cover__media {
  position: static;
  inset: auto;
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
  /* 记号与刻度是纯装饰（aria-hidden），必须点击穿透：
      它们落在轮播圆点/箭头附近，不穿透就点不动。
      `elementFromPoint` 实测过：箭头中心命中的曾是 `li.home-cover__tick`。
      整组一起往右下刚体位移（G8 定案），留 8px 呼吸不被 overflow: clip 切掉字形。 */
  transform: translate(var(--cover-aside-x), var(--cover-aside-y));
  pointer-events: none;
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
   媒体层必须铺满面板的**边框盒**（连同面板自己的 padding），而不是内容盒：
   底图按 16:9 铺满面板、超出部分由面板的 overflow: clip 裁掉，底部渐黑也才
   真正压到面板底边。写成 normal flow + height:100% 时它只占内容盒，图片向下
   外扩的那 12px 露在渐黑区之外，底部就会出现一条没被罩住的亮带。 */
.home-cover__media {
  position: absolute;
  inset: calc(-1 * clamp(0.75rem, 1.4vw, 1.25rem));
  display: block;
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
     比例不在这里改 —— 16:9 是所有断点同一份定义（ADR-0013）。 */
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
    /* 窄屏改成纵向堆叠后，右下角整组的刚体位移会把记号推出视口：归零。 */
    transform: none;
  }

  .home-cover__rule {
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

/* 减少动态：封面装饰层不做进场，基线位移是静态定位而非动画。 */
@media (prefers-reduced-motion: reduce) {
  .home-cover__rule {
    transition: none;
  }
}
</style>
