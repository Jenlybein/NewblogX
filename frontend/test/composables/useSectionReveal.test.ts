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

  it("reuse：进场后样式回到原位，且只有 transform/opacity/transition 三个键", () => {
    const reveal = useSectionReveal({ motion: { matchMedia: () => media(false) } });
    reveal.revealed.value = true;

    expect(Object.keys(reveal.revealStyle.value).sort()).toEqual(["opacity", "transform", "transition"]);
    expect(reveal.revealStyle.value).toMatchObject({ opacity: "1", transform: "none" });
  });

  it("减少动态：即使未进场也直接可见、无过渡（不做淡入）", () => {
    const reveal = useSectionReveal({ motion: { matchMedia: () => media(true) } });

    expect(reveal.revealStyle.value).toEqual({
      opacity: "1",
      transform: "none",
      transition: "none",
    });
  });

  it("位移可注入但钳在 12px 内", () => {
    const reveal = useSectionReveal({
      offset: 40,
      motion: { matchMedia: () => media(false) },
    });

    expect(reveal.revealStyle.value.transform).toBe("translateY(12px)");
  });
});
