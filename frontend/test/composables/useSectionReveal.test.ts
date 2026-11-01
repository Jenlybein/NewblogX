import { describe, expect, it } from "vitest";

import { useSectionReveal } from "~/composables/useSectionReveal";

/**
 * 区块滚动进场的接线（motion-v 的 useInView）。
 *
 * 这里测可观察的样式契约与降级：未进场时只改 opacity/translateY、进场一次后
 * 回到原位、减少动态时直接可见无过渡。IntersectionObserver 在测试环境是桩，
 * 因此不依赖真实滚动，只断言状态到样式的映射。
 */
const media = (matches: boolean) => ({
  matches,
  addEventListener: () => undefined,
  removeEventListener: () => undefined,
});

describe("useSectionReveal", () => {
  it("初始未进场：目标有 ref，样式是隐藏 + 位移（≤12px）", () => {
    const reveal = useSectionReveal({ motion: { matchMedia: () => media(false) } });

    expect(reveal.target.value).toBeNull();
    expect(reveal.revealed.value).toBe(false);
    expect(reveal.revealStyle.value).toMatchObject({ opacity: "0", transform: "translateY(12px)" });
  });

  it("reuse：挂载并进场后样式回到原位，且只有 transform/opacity/transition 三个键", () => {
    const reveal = useSectionReveal({ motion: { matchMedia: () => media(false) } });
    // 挂载完成才启用进场状态（挂载前强制 SSR 基线，见下面的一致性用例）。
    reveal.mounted.value = true;
    reveal.revealed.value = true;

    expect(Object.keys(reveal.revealStyle.value).sort()).toEqual(["opacity", "transform", "transition"]);
    expect(reveal.revealStyle.value).toMatchObject({ opacity: "1", transform: "none" });
  });

  it("减少动态：挂载前保持 SSR 基线（水合一致），挂载后直接可见无过渡（不做淡入）", () => {
    const reveal = useSectionReveal({ motion: { matchMedia: () => media(true) } });

    // 首次渲染（水合阶段）必须与 SSR 输出一致，否则首页会报
    // "Hydration completed but contains mismatches"（#88）。
    expect(reveal.revealStyle.value).toMatchObject({
      opacity: "0",
      transform: "translateY(12px)",
    });

    // 挂载完成后才启用真实偏好。
    reveal.mounted.value = true;
    expect(reveal.revealStyle.value).toEqual({
      opacity: "1",
      transform: "none",
      transition: "none",
    });
  });

  // #88 的核心护栏：SSR 与客户端首次渲染必须产出同一份样式。
  //
  // SSR 时没有 matchMedia（prefersReducedMotion 恒为 false），而客户端能读到
  // 真实偏好；若首次渲染直接用它，开了「减少动效」的用户就会拿到与 SSR
  // 不同的内联样式 → 水合不一致。
  it("水合一致性：SSR 与客户端首次渲染的样式相同（无论是否减少动效）", () => {
    const ssr = useSectionReveal({ motion: { matchMedia: () => null } });
    const clientReduced = useSectionReveal({ motion: { matchMedia: () => media(true) } });
    const clientNormal = useSectionReveal({ motion: { matchMedia: () => media(false) } });

    expect(clientReduced.revealStyle.value).toEqual(ssr.revealStyle.value);
    expect(clientNormal.revealStyle.value).toEqual(ssr.revealStyle.value);
  });

  it("位移可注入但钳在 12px 内", () => {
    const reveal = useSectionReveal({
      offset: 40,
      motion: { matchMedia: () => media(false) },
    });

    expect(reveal.revealStyle.value.transform).toBe("translateY(12px)");
  });
});
