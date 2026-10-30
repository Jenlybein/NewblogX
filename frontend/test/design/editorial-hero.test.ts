import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

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

function walk(dir: string, files: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      if (["node_modules", ".nuxt", ".output", "coverage"].includes(name)) continue;
      walk(full, files);
      continue;
    }
    if (/\.(?:vue|ts|css)$/u.test(name)) files.push(full);
  }
  return files;
}

const SOURCES = walk(appDir).map((absolute) => ({
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

/** 取出某个选择器之后的第一个 `{...}` 块内容（够用即可，不做完整 CSS 解析）。 */
function blockAfter(source: string, marker: string, last = false): string {
  const start = last ? source.lastIndexOf(marker) : source.indexOf(marker);
  expect(start, `找不到 ${marker}`).toBeGreaterThan(-1);
  const open = source.indexOf("{", start);
  let depth = 0;
  for (let index = open; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    if (source[index] === "}") {
      depth -= 1;
      if (depth === 0) return source.slice(open + 1, index);
    }
  }
  throw new Error(`${marker} 的块没有闭合`);
}

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
  it("底图层来自运营上传的首张轮播 cover，object-fit: cover", () => {
    expect(bannerVue).toContain('class="banner-photo"');
    expect(bannerVue).toMatch(/<img[\s\S]*:src="currentBanner\.cover"/u);
    const photo = blockAfter(bannerVue, ".banner-photo__img {");
    expect(photo).toContain("object-fit: cover");
    expect(photo).toContain("height: 100%");
    expect(photo).toContain("width: 100%");
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
  it("首屏接 #23 的 useSectionReveal / useParallax，文字层走 useStaggerEntrance", () => {
    expect(indexVue).toContain("useSectionReveal");
    expect(indexVue).toContain("useParallax");
    expect(indexVue).toContain("useStaggerEntrance");
    expect(indexVue).toContain("revealStyle");
    expect(indexVue).toContain("parallaxStyle");
    expect(indexVue).toContain("motion-reveal");
    expect(indexVue).toContain("motion-stagger-item");
  });

  it("视差只挂在图像层与装饰记号上（≤10px 由 composable 钳位）", () => {
    expect(indexVue).toContain("motion-parallax");
    expect(indexVue).toMatch(/:style="parallaxStyle"/u);
    // 底图的视差由首屏页编排、经 photoStyle 传给媒体层，只落在图片元素上。
    expect(indexVue).toContain(':photo-style="parallaxStyle"');
    expect(bannerVue).toMatch(/<img[\s\S]*:style="photoStyle"/u);
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
