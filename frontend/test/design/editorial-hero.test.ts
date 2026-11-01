import { readFileSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

// blockAfter / collectFiles / SOURCES 的遍历在多个 design 测试里重复，已收进 helpers。
import { blockAfter, collectFiles, extensionFilter } from "../helpers/designFiles";

/**
 * 首屏编辑部式封面（ADR-0013）的静态设计契约。
 *
 * 首屏是全站唯一的"编辑部式例外区"：满幅色带、超大 h1、网格刻度、方括号/箭头记号
 * 只允许长在这一个文件里。例外区最危险的退化是"被复制到列表页"，所以这里的断言
 * 一半在验证版式本身，一半在验证**边界**（记号类名与三色色带不出首屏）。
 *
 * 首屏上的行为断言（轮播控件 ≥2 张、空态）落在组件测试里（BannerCarousel.test.ts），
 * 这里只锁结构关系与删除项，不重复测组件行为。
 */
const projectDir = process.cwd();
const appDir = join(projectDir, "app");
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

const SOURCES = collectFiles(appDir, extensionFilter("vue", "ts", "css")).map((absolute) => ({
  file: relative(projectDir, absolute).replace(/\\/gu, "/"),
  text: readFileSync(absolute, "utf8"),
}));

const indexVue = read("app/pages/index.vue");
const bannerVue = read("app/components/article/BannerCarousel.vue");
const headerVue = read("app/components/layout/AppHeader.vue");
const footerVue = read("app/components/layout/AppFooter.vue");
const layoutVue = read("app/layouts/default.vue");
const brandTs = read("app/utils/brand.ts");
const homeTs = read("app/utils/home.ts");

/** 首屏 `<section class="home-cover">` 的模板片段（用来判断层级是否都在封面内）。 */
function coverTemplate(source: string): string {
  const classAt = source.indexOf('class="home-cover"');
  expect(classAt, "找不到首屏封面 section").toBeGreaterThan(-1);
  const start = source.lastIndexOf("<section", classAt);
  expect(start, "找不到首屏封面 section").toBeGreaterThan(-1);
  const end = source.indexOf("</section>", start);
  expect(end, "首屏封面 section 没有闭合").toBeGreaterThan(-1);
  return source.slice(start, end);
}

describe("满幅：脱离 page-shell 的水平内边距", () => {
  it("用 100vw + 负边距 bleed 铺满视口，且外层有 overflow-x: clip 防横向滚动条", () => {
    const cover = blockAfter(indexVue, ".home-cover {");

    expect(cover).toContain("width: 100vw");
    expect(cover).toContain("margin-inline: calc(50% - 50vw)");
    expect(cover).toContain("overflow: clip");
    // 出血铺满后，外层必须在视口级容器上兜住横向溢出，否则 100vw 会带出横向滚动条。
    expect(layoutVue).toContain("overflow-x: clip");
  });

  it("高度 clamp(380px, 62vh, 620px)，且没有卡片边框与纸面底色", () => {
    const cover = blockAfter(indexVue, ".home-cover {");

    expect(cover).toContain("min-height: clamp(380px, 62vh, 620px)");
    expect(cover).not.toMatch(/\bborder\s*:/u);
    expect(cover).not.toContain("var(--color-surface)");
  });

  it("封面直接坐在深墨兜底面上（无图也能读；有图时被底图层覆盖）", () => {
    const cover = blockAfter(indexVue, ".home-cover {");

    expect(cover).toContain("var(--color-surface-inverse)");
    expect(cover).toContain("var(--color-on-inverse)");
  });
});

describe("分层：底图 → 罩层 → 色带 → 网格刻度 → h1 → 记号 → 控件", () => {
  it("底图层来自运营上传的首张轮播 cover，铺满面板", () => {
    expect(bannerVue).toContain('class="banner-photo"');
    expect(bannerVue).toMatch(/<img[\s\S]*:src="currentBanner\.cover"/u);
    const photo = blockAfter(bannerVue, ".banner-photo__img {");
    expect(photo).toContain("height: 100%");
    expect(photo).toContain("width: 100%");
    // object-fit 由取景纯函数产出（framingStyle），CSS 不再重复定义。
    // 取景算术在 coverFraming（轮播与文章封面的取景窗共用它），bannerFraming 只做契约字段适配。
    expect(photo).not.toContain("object-fit");
    expect(read("app/utils/coverFraming.ts")).toContain('objectFit: "cover"');
  });

  it("罩层是中性石墨渐变，保证反白文字的对比度 ≥4.5:1", () => {
    const scrim = blockAfter(bannerVue, ".banner-scrim {");

    expect(scrim).toContain("linear-gradient");
    expect(scrim).toContain("var(--color-surface-inverse)");
    // 最浅的一档也压到至少 70%，白色字压任意底图的对比度才够 4.5:1。
    expect(scrim).toContain("70%");
    expect(scrim).toContain("95%");
  });

  it("2–3 条横向色带：黄 / 功能蓝 / 珊瑚，且只消费三个锁定色相", () => {
    const cover = coverTemplate(indexVue);

    expect(cover).toContain("home-cover__band--yellow");
    expect(cover).toContain("home-cover__band--blue");
    expect(cover).toContain("home-cover__band--coral");
    expect(blockAfter(indexVue, ".home-cover__band--yellow {")).toContain("var(--jelly-yellow)");
    expect(blockAfter(indexVue, ".home-cover__band--blue {")).toContain("var(--jelly-interactive)");
    expect(blockAfter(indexVue, ".home-cover__band--coral {")).toContain("var(--jelly-coral)");
  });

  it("网格线走发丝线 token，下沿刻度用 --font-mono", () => {
    const grid = blockAfter(indexVue, ".home-cover__grid {");

    expect(grid).toContain("var(--color-line-faint)");
    const ticks = blockAfter(indexVue, ".home-cover__ticks {");
    expect(ticks).toContain("var(--font-mono)");
    expect(coverTemplate(indexVue)).toContain("home-cover__tick");
  });

  it("超大 h1 之上还有纯装饰（aria-hidden）的箭头 / 方括号 / 胶囊小签", () => {
    const cover = coverTemplate(indexVue);

    for (const cls of ["home-cover__arrow", "home-cover__bracket", "home-cover__capsule"]) {
      expect(cover, cls).toContain(cls);
    }
    const marks = cover.slice(cover.indexOf("home-cover__marks"), cover.indexOf("home-cover__marks") + 400);
    expect(marks).toContain('aria-hidden="true"');
  });
});

describe("首页唯一的 h1 = JELLYBIN + [果冻盒]", () => {
  it("首页页面源码只有一个 h1，且内容是站名 + 方括号中文副标", () => {
    const h1Count = (indexVue.match(/<h1\b/gu) ?? []).length;

    expect(h1Count).toBe(1);
    const h1 = indexVue.slice(indexVue.indexOf("<h1"), indexVue.indexOf("</h1>"));
    expect(h1).toContain("BRAND.name.en.toUpperCase()");
    expect(h1).toContain("[{{ BRAND.name.zh }}]");
    // 原先的小字「JellyBin · 果冻盒」降为普通文本（不再抢 h1）。
    expect(h1).not.toContain("BRAND.name.en }} · {{ BRAND.name.zh");
  });

  it("页头 / 页脚 / 首页其它区块都没有第二个 h1", () => {
    for (const [file, source] of [
      ["app/components/layout/AppHeader.vue", headerVue],
      ["app/components/layout/AppFooter.vue", footerVue],
      ["app/components/home/HomeFeedSection.vue", read("app/components/home/HomeFeedSection.vue")],
      ["app/components/home/HomePinnedSection.vue", read("app/components/home/HomePinnedSection.vue")],
      ["app/components/home/HomeSidebar.vue", read("app/components/home/HomeSidebar.vue")],
    ] as const) {
      expect(source.match(/<h1\b/gu) ?? [], file).toHaveLength(0);
    }
  });

  it("h1 字号 clamp(2rem, 9vw, 6rem)、字距 -0.04em、700，字体走 --font-title", () => {
    const title = blockAfter(indexVue, ".home-cover__title {");

    expect(title).toContain("font-family: var(--font-title)");
    expect(title).toContain("font-size: clamp(2rem, 9vw, 6rem)");
    expect(title).toContain("letter-spacing: -0.04em");
    expect(title).toMatch(/font-weight:\s*(?:700|var\(--type-weight-title\))/u);
  });
});

describe("删除项：主旨句 / 三格事实 / 首屏卡片容器", () => {
  it("brand.ts 不再有 tagline 字段", () => {
    expect(brandTs).not.toMatch(/tagline\s*:/u);
  });

  it("页头不再有 projectAbstract / site-header__abstract", () => {
    expect(headerVue).not.toContain("projectAbstract");
    expect(headerVue).not.toContain("site-header__abstract");
  });

  it("页脚不再有 tagline 行", () => {
    expect(footerVue).not.toContain("site-footer__tagline");
    expect(footerVue).not.toContain("BRAND.tagline");
  });

  it("homeHeroFacts / HomeHeroFact 与它的单测一起退场", () => {
    expect(homeTs).not.toContain("homeHeroFacts");
    expect(homeTs).not.toContain("HomeHeroFact");
    expect(read("test/utils/home.test.ts")).not.toContain("homeHeroFacts");
  });

  it("全站源码里没有主旨句与品牌 tagline 的残留引用（grep 级）", () => {
    const offenders = SOURCES.filter(({ text }) =>
      /BRAND\.tagline|一个会呼吸的个人博客|homeHeroFacts|HomeHeroFact/u.test(text),
    ).map(({ file }) => file);

    expect(offenders).toEqual([]);
  });

  it("首屏卡片容器（三格事实条 / .home-hero 样式）已删除", () => {
    // `data-home-hero` 是保留的埋点标记，不在此列；这里锁的是"卡片容器"本身。
    expect(indexVue).not.toContain("home-hero__");
    expect(indexVue).not.toMatch(/\.home-hero\b|class="[^"]*\bhome-hero\b/u);
  });
});

describe("轮播并入首屏", () => {
  it("轮播只出现在首屏封面内，首屏下方不再有独立轮播卡", () => {
    const renders = (indexVue.match(/<BannerCarousel\b/gu) ?? []).length;

    expect(renders).toBe(1);
    expect(coverTemplate(indexVue)).toContain("<BannerCarousel");
    // 首屏之后紧接的是置顶区块，而不是轮播卡。
    const afterCover = indexVue.slice(indexVue.indexOf("</section>"), indexVue.indexOf("</template>"));
    expect(afterCover).toContain("HomePinnedSection");
    expect(afterCover).not.toContain("BannerCarousel");
  });

  it("无图时版式仍成立：封面老是渲染，色带与标题不依赖 banners", () => {
    const openTag = indexVue.slice(
      indexVue.indexOf('class="home-cover"'),
      indexVue.indexOf(">", indexVue.indexOf('class="home-cover"')),
    );

    expect(openTag).not.toContain("v-if");
    expect(coverTemplate(indexVue)).not.toMatch(/v-if=["'][^"']*banners/u);
  });

  it("空态沿用既有空态组件（ProfileEmptyState），控件只在 ≥2 张时渲染", () => {
    expect(bannerVue).toContain("ProfileEmptyState");
    expect(bannerVue).toContain("banners.length > 1");
    expect(bannerVue).not.toContain("banner-shell");
  });
});

describe("窄屏 <768px：色带 ≤1 条、刻度/记号隐藏、标题收敛、纵向堆叠", () => {
  const narrow = () => blockAfter(indexVue, "@media (max-width: 767px)");

  it("只留一条色带（蓝/珊瑚在窄屏隐藏）", () => {
    const block = narrow();

    expect(block).toContain(".home-cover__band--blue");
    expect(block).toContain(".home-cover__band--coral");
    expect(block).toMatch(/\.home-cover__band--(?:blue|coral)[^{}]*\{[^}]*display:\s*none/u);
  });

  it("刻度、方括号与箭头全部隐藏", () => {
    const block = narrow();

    for (const selector of [".home-cover__ticks", ".home-cover__bracket", ".home-cover__arrow"]) {
      expect(block, selector).toContain(selector);
    }
    expect(block).toMatch(/display:\s*none/u);
  });

  it("标题字号按 clamp 收敛到 clamp(2rem, 9vw, 3rem)", () => {
    expect(narrow()).toContain("clamp(2rem, 9vw, 3rem)");
  });

  it("两栏改为纵向堆叠", () => {
    expect(narrow()).toContain("grid-template-columns: minmax(0, 1fr)");
  });
});

describe("动效：分层进场 + 视差只动 transform/opacity，reduced-motion 归零", () => {
  it("首屏接 #23 的 useSectionReveal / useParallax（装饰层才进场）", () => {
    expect(indexVue).toContain("useSectionReveal");
    expect(indexVue).toContain("useParallax");
    expect(indexVue).toContain("revealStyle");
    expect(indexVue).toContain("parallaxStyle");
    expect(indexVue).toContain("motion-reveal");
  });

  it("关键内容不进入场动画：h1 与记号不得被 entranceProps 包住（SSR 会内联 opacity:0）", () => {
    // entranceProps 的初态是 opacity:0，靠客户端动画才显示；h1 一旦被包住，
    // 动画没跑起来（减少动态 / 脚本异常 / 截图工具）就等于首页没有标题。
    expect(indexVue).not.toContain("entranceProps");
    expect(indexVue).not.toContain("useStaggerEntrance");
    expect(indexVue).not.toContain("motion-stagger-item");
    expect(indexVue).toMatch(/<h1 class="home-cover__title">/u);
  });

  it("视差只挂在图像层与装饰记号上（≤10px 由 composable 钳位）", () => {
    expect(indexVue).toContain("motion-parallax");
    expect(indexVue).toMatch(/:style="parallaxStyle"/u);
    // 视差由首屏页编排、经 photoStyle 传给媒体层的外层容器；
    // 取景的缩放/焦点落在内层图片上 —— 两个 transform 分属不同层（ADR-0015 决策 6）。
    expect(indexVue).toContain(':photo-style="parallaxStyle"');
    expect(bannerVue).toMatch(/class="banner-photo"[\s\S]*?:style="photoStyle"/u);
    expect(bannerVue).toMatch(/class="banner-photo__img"[\s\S]*?:style="framingStyle"/u);
    expect(bannerVue).toContain("bannerFramingStyle");
  });

  it("首屏不自己写关键帧动画（归零由 #3 的动效预算统一负责）", () => {
    expect(indexVue).not.toContain("@keyframes");
    expect(indexVue).not.toMatch(/animation(?:-name)?\s*:/u);
    // 过渡只允许时长/缓动类属性，不允许布局属性参与动画。
    expect(indexVue).not.toMatch(/transition:\s*(?:width|height|top|left|margin|padding)/u);
  });
});

describe("例外边界：色带 / 刻度 / 记号类名只在首屏封面内", () => {
  it("home-cover__* 只出现在 index.vue 一个文件", () => {
    const offenders = SOURCES.filter(
      ({ file, text }) => file !== "app/pages/index.vue" && text.includes("home-cover__"),
    ).map(({ file }) => file);

    expect(offenders).toEqual([]);
  });

  it("三色色带的原始色相只被首屏消费（不得外溢到列表页）", () => {
    const offenders = SOURCES.filter(
      ({ file, text }) =>
        file !== "app/pages/index.vue" &&
        file !== "app/assets/css/tokens.css" &&
        /var\(--jelly-(?:yellow|interactive|coral)\)/u.test(text),
    ).map(({ file }) => file);

    expect(offenders).toEqual([]);
  });

  it("轮播控件走薄玻璃胶囊（--glass-thin + pill）", () => {
    expect(bannerVue).toContain("glass-thin");
    const controls = blockAfter(bannerVue, ".banner-controls {");

    expect(controls).toContain("var(--radius-pill)");
  });
});

describe("首屏版式复盘：窄高轮播面板 + 右栏线条场", () => {
  it("删掉那句补充说明与两个入口按钮（顶栏已有入口）", () => {
    expect(indexVue).not.toContain("搜索文章");
    expect(indexVue).not.toContain("开始创作");
    expect(indexVue).not.toContain("home-cover__lede");
    expect(indexVue).not.toContain("home-cover__actions");
  });

  it("轮播面板在大字下方并整体右移；大字仍然排在它前面（不能把 h1 挤到看不见）", () => {
    const copyIndex = indexVue.indexOf('class="home-cover__copy"');
    const panelIndex = indexVue.indexOf('class="home-cover__panel"');
    const carouselIndex = indexVue.indexOf("<BannerCarousel");

    expect(copyIndex).toBeGreaterThan(-1);
    expect(panelIndex).toBeGreaterThan(copyIndex);
    expect(carouselIndex).toBeGreaterThan(panelIndex);

    const panel = blockAfter(indexVue, ".home-cover__panel {");
    expect(panel).toContain("position: relative");
    expect(panel).toContain("grid-row: 2");
    expect(panel).toContain("margin-inline-start");
    expect(panel).toContain("var(--radius-lg)");
  });

  it("面板固定 16:9（所有断点同一比例）并占左栏过半宽度", () => {
    const panel = blockAfter(indexVue, ".home-cover__panel {");
    const columns = /grid-template-columns:\s*minmax\(0,\s*(\d+)%\)/u.exec(
      blockAfter(indexVue, ".home-cover__inner {"),
    )?.[1];

    expect(panel).toContain("aspect-ratio: 16 / 9");
    expect(panel).toContain("width: 100%");
    // 不再用高度档（固定高度会随屏宽变化，破坏"同一张图同一形状"），也不被 max-height 封顶。
    expect(panel).not.toContain("height:");
    expect(panel).not.toContain("max-height");
    expect(Number(columns)).toBeGreaterThanOrEqual(50);
    // 窄屏也不换比例（比例在基础规则里定义一次，断点只调栅格位置）。
    const narrowPanel = blockAfter(blockAfter(indexVue, "@media (max-width: 767px)"), ".home-cover__panel {");
    expect(narrowPanel).not.toContain("aspect-ratio");
    expect(narrowPanel).not.toContain("height:");
  });

  it("面板要像屏幕而不是空白框：淡染色 + 降级底色，缺图时也看得出容器", () => {
    const panel = blockAfter(indexVue, ".home-cover__panel {");

    // 第一行是不支持 color-mix 时的降级值，第二行才是染色版本。
    expect(panel).toContain("background: var(--color-surface-inverse)");
    expect(panel).toMatch(/background:\s*color-mix\(in srgb, var\(--color-on-inverse\)/u);
  });

  it("缺图时面板压在线条之上（线条不许盖住空态文案）", () => {
    expect(indexVue).toMatch(/home-cover__panel--empty/u);
    const emptyOverride = blockAfter(indexVue, ".home-cover__panel--empty {");
    const panel = blockAfter(indexVue, ".home-cover__panel {");

    const zOf = (block: string) => Number(/z-index:\s*(\d+)/u.exec(block)?.[1] ?? "0");
    expect(zOf(emptyOverride)).toBeGreaterThan(zOf(panel));
  });

  it("线条的右端点抵到视口右沿（负外边距抵消封面左右内边距）", () => {
    const lines = blockAfter(indexVue, ".home-cover__lines {");

    expect(lines).toContain("calc(-1 * var(--cover-pad-x))");
    expect(indexVue).toContain("--cover-pad-x: clamp(");
  });

  it("线条场压过面板（z-index 高于面板、低于标题），且只在宽屏出现", () => {
    const lines = blockAfter(indexVue, ".home-cover__lines {");
    const panel = blockAfter(indexVue, ".home-cover__panel {");
    const copy = blockAfter(indexVue, ".home-cover__copy {");
    const zOf = (block: string) => Number(/z-index:\s*(\d+)/u.exec(block)?.[1] ?? "0");

    expect(lines).toContain("position: absolute");
    expect(lines).toContain("pointer-events: none");
    expect(zOf(lines)).toBeGreaterThan(zOf(panel));
    expect(zOf(copy)).toBeGreaterThan(zOf(lines));
    expect(indexVue).toMatch(/@media \(max-width: 767px\)[\s\S]*\.home-cover__lines \{\s*display: none;/u);
  });

  it("线条只用四种调子（反白 / 功能蓝 / 珊瑚 / 高亮黄），数量够密", () => {
    const tones = [...indexVue.matchAll(/tone: "([a-z]+)"/gu)].map((match) => match[1]);

    expect(tones.length).toBeGreaterThanOrEqual(12);
    expect([...new Set(tones)].sort()).toEqual(["blue", "coral", "ink", "yellow"]);
  });

  it("线条右端点贴着最右边（用 right 定位，抖动 ≤2%）", () => {
    const rights = [...indexVue.matchAll(/right: "([0-9.]+)%"/gu)].map((match) => Number(match[1]));

    expect(rights.length).toBeGreaterThanOrEqual(12);
    for (const right of rights) {
      expect(right).toBeLessThanOrEqual(2);
    }
    // 右端定位后不再用 left 决定终点。
    expect(indexVue).not.toMatch(/left: "\d+%",\s*width:/u);
  });

  it("长度分布：中间最长、上下渐短且不规则", () => {
    const lines = [...indexVue.matchAll(/top: "([\d.]+)%"[^}]*width: "([\d.]+)%"/gu)].map(
      (match) => ({
        top: Number(match[1]),
        width: Number(match[2]),
      }),
    );

    expect(lines.length).toBeGreaterThanOrEqual(12);

    const widest = lines.reduce((max, line) => (line.width > max.width ? line : max));
    // 峰值落在垂直中段（而不是贴顶或贴底）。
    expect(widest.top).toBeGreaterThanOrEqual(38);
    expect(widest.top).toBeLessThanOrEqual(62);
    // 中段最长的那条要越过左栏面板的右沿（44%），才谈得上"压住轮播"。
    expect(widest.width).toBeGreaterThan(100 - 44);
    // 两端明显更短（不足峰值的一半）。
    expect(lines[0]!.width).toBeLessThan(widest.width / 2);
    expect(lines.at(-1)!.width).toBeLessThan(widest.width / 2);
    // 不规则：允许抖动，所以相邻两条的差不必相等（只要求整体趋势）。
    const widths = lines.map((line) => line.width);
    expect(widths).not.toEqual([...widths].sort((a, b) => b - a));
  });
});

describe("线条入场：从右端延展（只动 transform/opacity，不走 @keyframes）", () => {
  const lineRule = blockAfter(indexVue, ".home-cover__line {");

  it("缩放原点贴右：视觉上是从最右边长出来", () => {
    expect(lineRule).toContain("transform-origin: right center");
  });

  it("用 scaleX(0) → scaleX(1) 的过渡，而不是改 width", () => {
    expect(indexVue).toContain("transform: scaleX(0)");
    expect(indexVue).toContain("transform: scaleX(1)");
    // 布局属性不许过渡（也不许用来做展开）。
    expect(indexVue).not.toMatch(/transition:[^;]*\bwidth/u);
    expect(indexVue).not.toMatch(/transform:\s*scaleY/u);
  });

  it("状态类两段式（is-armed → is-drawn），保留无 JS 兜底", () => {
    expect(indexVue).toContain("'is-armed': linesArmed");
    expect(indexVue).toContain("'is-drawn': linesDrawn");
    expect(indexVue).toMatch(/is-armed:not\(\.is-drawn\)/u);
    // 只有 arm 成功后才收起初态，所以脚本没跑时线条仍是终态可见。
    expect(indexVue).toMatch(/onMounted\([\s\S]*prefers-reduced-motion/u);
    expect(indexVue).toContain("requestAnimationFrame");
  });

  it("入场延迟不规则：手写跳数、不是等差，但仍是同一份 40ms 量子的整数倍", () => {
    const steps = /HERO_LINE_DELAY_STEPS = \[([\d,\s]+)\]/u.exec(indexVue)?.[1];

    expect(steps, "延迟必须来自一组显式跳数").toBeDefined();
    const values = steps!.split(",").map((part) => Number(part.trim()));

    expect(values.length).toBeGreaterThanOrEqual(14);
    // 等差会被读成节拍器：相邻差值不能恒定。
    const gaps = values.slice(1).map((value, index) => value - values[index]!);
    expect(new Set(gaps).size).toBeGreaterThan(2);
    // 中段线条最早落位（用户要的"先结构后细节"），两端更晚。
    expect(Math.min(...values)).toBeLessThanOrEqual(1);
    expect(Math.max(...values)).toBeGreaterThanOrEqual(7);
    // 延迟仍是 40ms 量子的整数倍，且逐条内联。
    expect(indexVue).toMatch(/HERO_LINE_DELAY_STEPS\[index\][^`]*\* MOTION_STAGGER_MS/u);
    expect(indexVue).toMatch(/transitionDelay: line\.delay/u);
  });

  it("时长与缓动走全站同一份预算（--motion-base + --motion-ease）", () => {
    expect(lineRule).toContain("var(--motion-base)");
    expect(lineRule).toContain("var(--motion-ease)");
  });

  it("减少动态下不延展、直接静态到位", () => {
    const reduced = /@media \(prefers-reduced-motion: reduce\) \{([\s\S]*?)\n\}/u.exec(indexVue)?.[1] ?? "";

    expect(reduced).toContain(".home-cover__line");
    expect(reduced).toContain("transition: none");
  });
});
