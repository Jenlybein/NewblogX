import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

// blockAfter 在多个 design 测试里逐字节相同，已收进 test/helpers/designFiles。
import { blockAfter } from "../helpers/designFiles";

import { MOTION_REVEAL_OFFSET_PX, PARALLAX_MAX_PX } from "~/utils/motion";

/**
 * 吸顶玻璃导航 + 滚动进场 + 锚点补偿的设计断言（ADR-0012 决策 6–7 与约束 8–9）。
 *
 * 这里锁的是"文件之间必须成立的关系"：页头真的 sticky、三套布局都给锚点留了
 * scroll-padding、区块进场只出现在布局级容器而不是正文段落、视差有真实消费者。
 */
const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");
const tailwindCss = read("app/assets/css/tailwind.css");
const appVue = read("app/app.vue");
const header = read("app/components/layout/AppHeader.vue");

// 只有渲染站点页头的布局需要为它留 scroll-padding：创作页（write）刻意不渲染页头，
// 它的开关在自己顶栏里，所以不在这个清单里。
const LAYOUTS = ["app/layouts/default.vue", "app/layouts/studio.vue"];

describe("吸顶玻璃导航", () => {
  it("页头改成 sticky，且贴顶（不再是非吸顶的相对定位）", () => {
    const block = blockAfter(header, ".site-header {");

    expect(block).toContain("position: sticky");
    expect(block).toContain("top: 0");
  });

  it("高度压在 56–64px（滚动后不永久遮挡正文的原始动机还在）", () => {
    const tokensBlock = blockAfter(tailwindCss, ":root {");
    const heightMatch = tokensBlock.match(/--site-header-height:\s*([0-9.]+)rem/u);

    expect(heightMatch, "--site-header-height 必须由 tailwind.css 定义").toBeDefined();
    const px = Number(heightMatch![1]) * 16;
    expect(px).toBeGreaterThanOrEqual(56);
    expect(px).toBeLessThanOrEqual(64);
    expect(blockAfter(header, ".site-header {")).toContain("min-height: var(--site-header-height)");
  });

  it("滚动超阈值才转玻璃，并加底部 1px 边缘高光", () => {
    expect(header).toContain("glass-thin");
    expect(header).toContain("is-scrolled");

    const scrolledBlock = blockAfter(header, ".site-header.is-scrolled {");
    expect(scrolledBlock).toContain("box-shadow");
    expect(scrolledBlock).toContain("var(--glass-rim)");
  });

  it("下滚隐藏 / 上滚出现（位移只走 transform）", () => {
    const hiddenBlock = blockAfter(header, ".site-header.is-hidden {");

    expect(hiddenBlock).toContain("translateY(-100%)");
    expect(hiddenBlock).toContain("transition");
  });

  it("页头是滚动与方向的唯一消费者（不自己读 scrollY，走 composable）", () => {
    expect(header).toContain("useScrollChrome");
    expect(header).not.toContain("window.addEventListener");
  });

  it("指针镜面高光走 usePointerSpecular（移动端 / 减少动态由 composable 关闭）", () => {
    expect(header).toContain("usePointerSpecular");
    expect(header).toContain("@pointermove");
  });
});

describe("锚点补偿：全站 scroll-padding-top", () => {
  it("全局 html 收起 scroll-padding-top（页头高度 + 呼吸余量）", () => {
    const htmlBlock = blockAfter(tailwindCss, "html {");

    expect(htmlBlock).toContain("scroll-padding-top");
    expect(htmlBlock).toContain("var(--site-header-height)");
  });

  it("渲染站点页头的两套布局都声明 scroll-padding-top（页内滚动容器同样不被页头盖住）", () => {
    for (const layout of LAYOUTS) {
      expect(read(layout), layout).toContain("scroll-padding-top");
    }
  });
});

describe("创作页顶栏贴顶吸附（这一页没有站点页头）", () => {
  const writeHeader = read("app/components/studio/StudioWriteHeader.vue");

  it("吸顶位置是 top: 0，不再给站点页头留 60px 空隙", () => {
    const block = blockAfter(writeHeader, ".write-page__header {");

    expect(block).toContain("position: sticky");
    expect(block).toContain("top: 0");
    expect(block).not.toContain("--site-header-height");
  });

  it("创作页任何样式都不再按站点页头高度做补偿", () => {
    for (const file of [
      "app/components/studio/StudioWriteHeader.vue",
      "app/layouts/write.vue",
      "app/assets/css/studio-write.css",
    ]) {
      expect(read(file), file).not.toContain("--site-header-height");
    }
  });
});

describe("动效档位 C：区块滚动进场", () => {
  const revealConsumers = [
    "app/components/home/HomeFeedSection.vue",
    "app/components/home/HomePinnedSection.vue",
    "app/components/home/HomeSidebar.vue",
    "app/pages/article/[id].vue",
  ];

  it("布局级区块接 useSectionReveal（motion-v 的 useInView）", () => {
    for (const file of revealConsumers) {
      const source = read(file);
      expect(source, file).toContain("useSectionReveal");
      expect(source, file).toContain("motion-reveal");
    }
  });

  it("进场只进一次：useInView 关闭重复触发", () => {
    const composable = read("app/composables/useSectionReveal.ts");

    expect(composable).toContain("useInView");
    expect(composable).toContain("once: true");
  });

  it("正文段落不进场（markdown / 列表条目 / 评论正文一律无 reveal）", () => {
    for (const file of [
      "app/components/common/MarkdownRenderSurface.vue",
      "app/components/article/ArticleFeedItem.vue",
      "app/components/comment/CommentThread.vue",
      "app/components/studio/MarkdownShadowPreview.vue",
    ]) {
      const source = read(file);
      expect(source, file).not.toContain("useSectionReveal");
      expect(source, file).not.toContain("motion-reveal");
    }
  });

  it("无 JS 兜底：noscript 把 reveal 元素直接置为可见", () => {
    expect(appVue).toContain(".motion-reveal{opacity:1!important;transform:none!important}");
  });

  it("减少动态下整段归零：CSS 直接可见 + transition: none", () => {
    const reduced = blockAfter(tailwindCss, "@media (prefers-reduced-motion: reduce)", true);

    expect(reduced).toContain(".motion-reveal");
    expect(reduced).toContain("transition: none");
    expect(reduced).toContain(".motion-parallax");
  });
});

describe("动效档位 C：视差 ≤10px", () => {
  it("视差预算与位移预算来自同一份 motion.ts", () => {
    expect(MOTION_REVEAL_OFFSET_PX).toBe(12);
    expect(PARALLAX_MAX_PX).toBe(10);
  });

  it("视差由 motion-v 的 useScroll 驱动，并钳在预算内", () => {
    const composable = read("app/composables/useParallax.ts");

    expect(composable).toContain("useScroll");
    expect(composable).toContain("clampParallaxOffset");
  });

  it("有真实消费者（不是只写了个 composable）", () => {
    const articlePage = read("app/pages/article/[id].vue");

    expect(articlePage).toContain("useParallax");
    expect(articlePage).toContain("parallaxStyle");
  });
});
