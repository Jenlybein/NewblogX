import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, blockAfterContaining, stripComments } from "../helpers/designFiles";

/**
 * 首页动效整治（票 442）：把这一批**不可从代码里推论、只能靠约定维持**的判断钉住。
 *
 * 每条都对应一个曾经的真实问题，改回旧写法会红：
 *   1. 抽屉开有动画、关是硬切（`visibility` / scrim 的 `display` 不在过渡里）；
 *   2. 整张信息流卡（上千像素宽）按下缩到 .97，文字跟着放大缩小；
 *   3. 同一行按钮里只有一枚有按下反馈；
 *   4. 果冻的 hover / press 形变没有 reduce 分支、也没 `(hover: hover)` 门控；
 *   5. 首屏光晕用内建 `ease-out` + 320ms，是全站唯一的第二套曲线；
 *   6. 气泡只有进场没有出场（`v-if` 卸载）；而收起 chip 反过来**不该**加过渡；
 *   7. 像素尾迹在绘制帧里跑整页 DOM 扫描；
 *   8. 减少动态的墨流静帧一次性同步跑 140 步、冻住主线程。
 */

const projectDir = process.cwd();

function read(relativePath: string): string {
  return readFileSync(join(projectDir, relativePath), "utf8");
}

function scopedStyle(source: string): string {
  const match = /<style[^>]*scoped[^>]*>([\s\S]*?)<\/style>/u.exec(source);
  expect(match, "找不到 <style scoped>").not.toBeNull();
  return stripComments(match![1] ?? "");
}

describe("移动端右栏抽屉：开与关都要看得见", () => {
  const tailwind = stripComments(read("app/assets/css/tailwind.css"));
  /** 抽屉相关规则全部落在 `max-width: 1023px` 这一段里。 */
  const drawerCss = blockAfterContaining(
    tailwind,
    "@media (max-width: 1023px)",
    ".sidebar-drawer-scrim",
  );

  it("收起方向：位移走 base 档，visibility 延迟到位移结束才生效", () => {
    expect(drawerCss, "位移走 base 档，不是按钮级的 fast 档").toMatch(
      /translate var\(--motion-base\) var\(--motion-ease\)/u,
    );
    expect(drawerCss, "visibility 必须在位移结束后再收起").toMatch(
      /visibility 0s linear var\(--motion-base\)/u,
    );
  });

  it("展开方向：立刻可见，不等那 200ms", () => {
    expect(drawerCss).toContain("visibility: visible");
    expect(drawerCss).toMatch(/visibility 0s linear 0s/u);
  });

  it("遮罩常驻 + opacity 过渡，只在展开时吃指针", () => {
    const scrimStart = drawerCss.indexOf(".sidebar-drawer-scrim {");
    expect(scrimStart, "找不到抽屉遮罩规则").toBeGreaterThan(-1);
    const scrim = drawerCss.slice(scrimStart, drawerCss.indexOf("}", scrimStart));

    expect(scrim, "display 切不出过渡，遮罩必须常驻").toContain("display: block");
    expect(scrim).toContain("opacity: 0");
    expect(scrim).toContain("pointer-events: none");
    expect(scrim).toMatch(/opacity var\(--motion-base\) var\(--motion-ease\)/u);

    const openStart = drawerCss.indexOf(":checked ~ .sidebar-drawer-scrim {");
    expect(drawerCss.slice(openStart, drawerCss.indexOf("}", openStart))).toContain(
      "pointer-events: auto",
    );
  });
});

describe("大面积可点面：下沉而不是缩放", () => {
  const tailwind = stripComments(read("app/assets/css/tailwind.css"));

  it("`.press-sink` 按下只落 1px，字体不跟着缩放", () => {
    const sink = blockAfter(tailwind, "&.press-sink:active {");

    expect(sink).toContain("transform: translateY(1px)");
    expect(sink).not.toMatch(/scale\(/u);
  });

  it("减少动态下下沉也归零，改走底色/描边（反馈不整段失效）", () => {
    const reduce = blockAfterContaining(
      tailwind,
      "@media (prefers-reduced-motion: reduce)",
      "&.press-sink:active",
    );

    expect(reduce).toContain("transform: none");
    expect(reduce).toContain("background-color: var(--color-bg-subtle)");
  });

  it("信息流条目卡走 press-sink（1100px 宽的卡不该整体缩放）", () => {
    expect(read("app/components/article/ArticleFeedItem.vue")).toContain(
      'class="feed-entry press-sink"',
    );
  });
});

describe("页头动作区：一排按钮同一档按下反馈", () => {
  const headerSource = read("app/components/layout/AppHeader.vue");
  const header = scopedStyle(headerSource);

  it(".site-action 有按下缩放，且过渡只登记 token", () => {
    const base = blockAfter(header, ".site-action {");

    expect(base).toContain("transform var(--motion-fast) var(--motion-ease)");
    expect(blockAfter(header, ".site-action:active {")).toContain("transform: scale(0.97)");
  });

  it("减少动态下位移归零但补回底色/描边（此前这一排唯一反馈被删掉）", () => {
    const reduce = blockAfterContaining(
      header,
      "@media (prefers-reduced-motion: reduce)",
      ".site-action:active",
    );

    expect(reduce).toContain("transform: none");
    expect(reduce).toContain("background-color: var(--color-bg-subtle)");
  });

  it("消息面板走 <Transition>，进场/出场同一条 transition（不再只有进场）", () => {
    expect(headerSource).toContain('<Transition name="site-pop">');
    expect(headerSource).not.toContain("@keyframes site-messages-reveal");

    const enter = blockAfter(header, ".site-pop-enter-active,");
    expect(enter).toContain("transition:");
    expect(enter).toContain("opacity var(--motion-fast) var(--motion-ease)");
    expect(header).toContain(".site-pop-leave-to");
  });
});

describe("果冻：形变要门控、要能被减少动态关掉", () => {
  const jellySource = read("app/components/jelly/JellyLauncher.vue");
  const jelly = scopedStyle(jellySource);

  it("悬停挤压只在 (hover: hover) and (pointer: fine) 上生效", () => {
    // 两处悬停形变各在自己那个媒体查询里（角色本体 / 收起把手）。
    expect(jelly).toMatch(
      /@media \(hover: hover\) and \(pointer: fine\) \{\s*\.jelly-launcher--hover \.jelly-launcher__button \{/u,
    );
    expect(jelly).toMatch(
      /@media \(hover: hover\) and \(pointer: fine\) \{\s*\.jelly-launcher__handle:hover \.jelly-launcher__handle-capsule \{/u,
    );
  });

  it("reduce 下的覆盖排在六态之后，并把形变归零", () => {
    const pressIndex = jelly.indexOf(".jelly-launcher--press .jelly-launcher__button {");
    const reduceMatch = /@media \(prefers-reduced-motion: reduce\) \{\s*\.jelly-launcher--hover \.jelly-launcher__button,\s*\.jelly-launcher--press \.jelly-launcher__button \{\s*transform: none;/u.exec(
      jelly,
    );

    expect(reduceMatch, "reduce 覆盖必须存在（形变归零）").not.toBeNull();
    expect(reduceMatch!.index, "reduce 覆盖必须排在六态之后（同特异性靠后者生效）").toBeGreaterThan(
      pressIndex,
    );
  });

  it("气泡走 <Transition>（进场/出场都在），收起 chip 刻意不加过渡", () => {
    expect(jellySource).toContain('<Transition name="jelly-soft">');
    expect(jelly).toContain(".jelly-soft-leave-to");
    expect(jellySource).not.toContain("@keyframes jelly-bubble-in");

    const chipStart = jellySource.indexOf('class="jelly-collapse-chip"');
    const chipBefore = jellySource.slice(Math.max(0, chipStart - 400), chipStart);
    expect(chipBefore, "chip 是焦点目标，离场延迟会漂移焦点与读屏顺序").not.toContain(
      '<Transition name="jelly-soft">',
    );
  });
});

describe("首屏光晕：全站唯一曲线、UI 预算内、blur 不超 20px", () => {
  const hero = scopedStyle(read("app/components/brand/GlassHeroTitle.vue"));

  it("halo 用 var(--motion-ease) + base 档，不再出现内建 ease-out", () => {
    const halo = blockAfter(hero, ".glass-title__halo {");

    expect(halo).toContain("var(--motion-ease)");
    expect(halo).not.toMatch(/\bease-out\b/u);
    expect(halo).not.toContain("--motion-slow");
  });

  it("过渡期的 blur 低于 20px（Safari 上 20px 很贵）", () => {
    const halo = blockAfter(hero, ".glass-title__halo {");
    const blur = /filter:\s*blur\((\d+)px\)/u.exec(halo);

    expect(blur, "halo 的 filter: blur 找不到").not.toBeNull();
    expect(Number(blur![1])).toBeLessThan(20);
  });

  it("will-change 只在交互期间挂（静态首屏不该有九个常驻合成层）", () => {
    expect(hero).toContain(".glass-title.is-live .glass-title__shard");
    const shard = blockAfter(hero, ".glass-title__shard {");
    expect(shard).not.toMatch(/will-change/u);
  });

  it("reduce 下保留一次纯淡光（只动 opacity）", () => {
    const reduce = hero.slice(hero.lastIndexOf("@media (prefers-reduced-motion: reduce)"));

    expect(reduce).toContain("scale: 1");
    expect(reduce).toMatch(/transition:\s*opacity var\(--motion-base\)/u);
  });
});

describe("首屏标题的平滑按 dt 归一化（高刷屏不该快一倍）", () => {
  const composable = read("app/composables/useGlassShardTitle.ts");

  it("有 smoothingAlpha 且 lerp 经过它换算", () => {
    expect(composable).toContain("function smoothingAlpha(");
    expect(composable).toMatch(/Math\.pow\(1 - base, frames\)/u);
    expect(composable).toContain("smoothingAlpha(POINTER_LERP, time)");
    expect(composable).toContain("smoothingAlpha(SPRING_LERP, time)");
  });

  it("reduce 下点击不位移但保留淡光", () => {
    expect(composable).toContain("REDUCED_FLASH_MS");
    const burst = composable.slice(composable.indexOf("function burst()"));
    // reduce 分支：给一次纯淡光后**提前返回**（不跑乱码、不做位移）。
    expect(burst).toMatch(
      /if \(prefersReducedMotion\.value\) \{[\s\S]{0,400}toggleHalo\(true\);[\s\S]{0,200}return;/u,
    );
  });
});

describe("像素尾迹：几何扫描不在绘制帧里", () => {
  const trail = read("app/composables/usePixelTrail.ts");

  it("draw() 里不再有按时间触发的整页遮挡扫描", () => {
    const draw = trail.slice(
      trail.indexOf("function draw("),
      trail.indexOf("function refreshOccludersIfStale("),
    );

    expect(draw).not.toMatch(/occluderScanAt/u);
    expect(draw).not.toContain("refreshOccluders()");
  });

  it("改为在指针/可见性时机按去抖判断", () => {
    expect(trail).toContain("function refreshOccludersIfStale()");
    const kick = trail.slice(trail.indexOf("function kick("));
    expect(kick.slice(0, 400)).toContain("refreshOccludersIfStale()");
  });

  it("ctx.font 按字号缓存，不逐粒子重设", () => {
    expect(trail).toContain("if (fontPx !== lastFontPx) {");
  });

  it("ResizeObserver 去抖（拖窗口时不再每次回调重建画布/渐变/外框）", () => {
    const start = trail.slice(trail.indexOf("new ResizeObserver("));

    expect(start.slice(0, 300)).toContain("setTimeout(");
    expect(start.slice(0, 300)).toContain("120");
  });
});

describe("墨流静帧：分帧烘焙，不冻住主线程", () => {
  const ink = read("app/composables/useInkFluid.ts");

  it("still() 每帧只跑一小步，而不是同步跑完 140 步", () => {
    const still = ink.slice(ink.indexOf("const still = ()"), ink.indexOf("try {\n    resize();"));

    expect(still).toContain("requestAnimationFrame(bake)");
    expect(still, "同步 for 循环一次性跑完 140 步会冻结几百毫秒").not.toMatch(
      /for \(let i = 0; i < 140/u,
    );
    expect(still).toContain("stepIndex + 10");
  });

  it("重入保护：连续 resize 只烘最后一次的几何", () => {
    expect(ink).toContain("let stillToken = 0");
    expect(ink).toMatch(/if \(token !== stillToken\) return/u);
  });

  it("启动时先比几何签名再决定要不要重烘遮罩", () => {
    expect(ink).toContain("const refreshMaskIfGeometryChanged = ()");
    expect(ink).toMatch(/if \(key !== maskSizeKey\) refreshMask\(\)/u);
  });
});
