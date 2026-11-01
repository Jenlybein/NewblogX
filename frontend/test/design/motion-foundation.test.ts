import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import nuxtConfig from "../../nuxt.config";
import { MOTION_DURATIONS_MS, MOTION_EASE_CSS, MOTION_STAGGER_MS } from "~/utils/motion";

/**
 * 动效地基的结构断言。
 *
 * 分成三块：
 *   1. 动效 token 落在既有的 tokens.css 体系里（不另发明一套变量）；
 *   2. CSS 侧的「减少动态」全局降级与路由过渡（只动 opacity/transform、out-in 不抖布局）；
 *   3. motion-v 接线（模块注册 + SSR 兜底）与后台包体不新增库。
 */
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
const tailwindCss = readFileSync(join(projectDir, "app/assets/css/tailwind.css"), "utf8");
const appVue = readFileSync(join(projectDir, "app/app.vue"), "utf8");
const homeSidebar = readFileSync(join(projectDir, "app/components/home/HomeSidebar.vue"), "utf8");
const adminPackage = JSON.parse(readFileSync(join(projectDir, "../frontend_admin/package.json"), "utf8")) as {
  dependencies?: Record<string, string>;
  devDependencies?: Record<string, string>;
};

/** 取出某个媒体查询块内的内容（够用即可，不做完整 CSS 解析）。 */
function mediaBlock(css: string, query: string): string {
  const start = css.indexOf(`@media (${query})`);
  expect(start).toBeGreaterThan(-1);
  // 从媒体查询的左花括号起，按括号配对找到块尾。
  const open = css.indexOf("{", start);
  let depth = 0;
  for (let index = open; index < css.length; index += 1) {
    if (css[index] === "{") depth += 1;
    if (css[index] === "}") {
      depth -= 1;
      if (depth === 0) return css.slice(open + 1, index);
    }
  }
  throw new Error("媒体查询块没有闭合");
}

/** 取页面过渡相关规则里声明的属性名（去重）。 */
function pageTransitionProperties(): string[] {
  const properties = new Set<string>();
  for (const match of tailwindCss.matchAll(/\.page-(?:enter|leave)[a-z-]*[^\n{]*\{([^}]*)\}/gu)) {
    for (const declaration of match[1].split(";")) {
      const colon = declaration.indexOf(":");
      if (colon < 0) continue;
      properties.add(declaration.slice(0, colon).trim());
    }
  }
  return [...properties].sort();
}

describe("动效 token 进既有 token 体系（不另发明一套变量）", () => {
  // 注意这里是**现存**的两档 + 缓动，不是最初那四档：`--motion-slow` 与
  // `--motion-stagger` 全仓 var() 引用数为 0，已按 spec A2 从两份 tokens.css
  // 删除（stagger 的 delay 由 motion.ts 的 staggerDelayMs 直接算毫秒，不经 CSS
  // 变量）。删的依据不是"看着没用"，而是确认为零引用。
  it("三档时长 + 单一缓动 + stagger 节奏都在 tokens.css 里", () => {
    expect(tokensCss).toContain("--motion-fast: 120ms");
    expect(tokensCss).toContain("--motion-base: 200ms");
    expect(tokensCss).toContain("--motion-ease: cubic-bezier(0.2, 0.8, 0.2, 1)");
  });

  it("无消费者的 --motion-slow / --motion-stagger 已删除（留额度与实际不一致）", () => {
    expect(tokensCss).not.toContain("--motion-slow");
    expect(tokensCss).not.toContain("--motion-stagger");
  });

  it("动效 token 与颜色/排版 token 在同一条定义链上（同一个 :root）", () => {
    const rootBlock = tokensCss.slice(tokensCss.indexOf(":root {"), tokensCss.indexOf("\n}", tokensCss.indexOf(":root {")));

    expect(rootBlock).toContain("--motion-base");
    expect(rootBlock).toContain("--font-sans");
    expect(rootBlock).toContain("--color-ink");
  });

  // MOTION_DURATIONS_MS 保留 slow=320 一档（motion.ts 的公共预算常量，某些 JS 分支
  // 仍在用），但**没有对应的 CSS 变量** —— CSS 侧现存只有 fast / base 两档。
  it("CSS 变量与 app/utils/motion.ts 是同一份预算（不是两套写死的数字）", () => {
    expect(tokensCss).toContain(`--motion-fast: ${MOTION_DURATIONS_MS.fast}ms`);
    expect(tokensCss).toContain(`--motion-base: ${MOTION_DURATIONS_MS.base}ms`);
    expect(tokensCss).toContain(`--motion-ease: ${MOTION_EASE_CSS}`);
    // stagger 的 delay 由 motion.ts 直接算毫秒（staggerDelayMs / staggerItemMotion），
    // 不经 CSS 变量 —— 所以这里断言的是"两边同一个 40ms 数"，而不是"有个 CSS 变量"。
    expect(MOTION_STAGGER_MS).toBe(40);
  });
});

describe("CSS 侧的「减少动态」全局降级", () => {
  const reduced = mediaBlock(tokensCss, "prefers-reduced-motion: reduce");

  it("现存动效 token 全部归零（静态，而不是慢慢动）", () => {
    expect(reduced).toContain("--motion-fast: 0ms");
    expect(reduced).toContain("--motion-base: 0ms");
    expect(reduced).toContain("--motion-reveal-offset: 0px");
    expect(reduced).toContain("--parallax-max: 0px");
  });

  it("减少动态块里不再有已删除的旧 token", () => {
    expect(reduced).not.toContain("--motion-slow");
    expect(reduced).not.toContain("--motion-stagger");
  });

  it("全局兜底：把动画/过渡压成一次性，滚动行为不再平滑", () => {
    expect(reduced).toContain("transition-duration: 0.01ms !important");
    expect(reduced).toContain("animation-duration: 0.01ms !important");
    expect(reduced).toContain("animation-iteration-count: 1 !important");
    expect(reduced).toContain("scroll-behavior: auto !important");
  });
});

describe("路由切换过渡", () => {
  it("pageTransition 用 out-in：同时只有一页在文档流里，过渡结束不抖布局", () => {
    expect(nuxtConfig.app?.pageTransition).toEqual({ name: "page", mode: "out-in" });
  });

  it("过渡只动 opacity / transform（不碰会引起重排的属性）", () => {
    expect(pageTransitionProperties()).toEqual(["opacity", "transform", "transition"]);
  });

  it("过渡时长/缓动取自 token（单一来源，不写死毫秒）", () => {
    const block = tailwindCss.slice(tailwindCss.indexOf(".page-enter-active"));

    // #87 收尾：out-in 的离场是切页可见的固定开销。实测把过渡关掉后切页可见从
    // ~300ms 降到 ~125ms，其中 ~170ms 是这段过渡；按 ADR-0012 的三档预算从 base 档
    // （200ms）下调到 fast 档（120ms）。这里断言的是"改用了哪一档 token"，不是删断言：
    // 若改成写死毫秒或换回别的档位，用例会红；理由与回退方式见 ADR-0019「实施记录」。
    expect(block).toContain("var(--motion-fast)");
    expect(block).toContain("var(--motion-ease)");
    expect(block).not.toContain("var(--motion-base)");
  });

  it("减少动态下路由过渡直接关掉", () => {
    const reduced = mediaBlock(tailwindCss, "prefers-reduced-motion: reduce");

    expect(reduced).toContain(".page-enter-active");
    expect(reduced).toContain("transition: none");
  });
});

describe("motion-v 接线", () => {
  it("以 Nuxt 模块注册（组件与 composable 自动可用，不用逐处 import）", () => {
    expect(nuxtConfig.modules).toContain("motion-v/nuxt");
  });

  it("vite.ssr.noExternal 含 motion-v：生产构建的 SSR 兜底", () => {
    const noExternal = nuxtConfig.vite?.ssr?.noExternal ?? [];

    expect(noExternal).toContain("motion-v");
  });

  it("全局 MotionConfig 打开 reducedMotion=user（库级尊重系统设置，退化为一次性淡入）", () => {
    expect(appVue).toContain("MotionConfig");
    expect(appVue).toContain("reduced-motion");
  });

  it("首个真实用例：首页热门标签用 entranceProps 做 stagger 入场", () => {
    expect(homeSidebar).toContain("entranceProps(");
    expect(homeSidebar).toContain("<Motion");
  });

  it("后台包体不新增动效库依赖", () => {
    const allAdminDependencies = {
      ...(adminPackage.dependencies ?? {}),
      ...(adminPackage.devDependencies ?? {}),
    };

    expect(Object.keys(allAdminDependencies).filter((name) => /motion/i.test(name))).toEqual([]);
  });
});
