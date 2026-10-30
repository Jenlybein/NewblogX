import { describe, expect, it, vi } from "vitest";

import { useStaggerEntrance } from "~/composables/useStaggerEntrance";

/**
 * 列表 stagger 入场：这是首个真实用例（首页热门标签）的接线层。
 * 后续票（#5–#9）直接复用它，所以这里钉住"索引 → props"的契约。
 */
function fakeReducedMotion(matches: boolean) {
  return {
    matchMedia: () => ({
      matches,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }),
  };
}

describe("useStaggerEntrance", () => {
  it("按索引给出错峰入场 props（opacity + y，duration 0.2s，delay 40ms×index）", () => {
    const { entranceProps } = useStaggerEntrance(fakeReducedMotion(false));

    expect(entranceProps(0)).toEqual({
      initial: { opacity: 0, y: 8 },
      animate: { opacity: 1, y: 0 },
      transition: { duration: 0.2, delay: 0, ease: [0.2, 0.8, 0.2, 1] },
    });
    expect(entranceProps(2).transition.delay).toBe(0.08);
  });

  it("减少动态时：延迟与时长归零（条目一次性出现，不逐条错峰）", () => {
    const { prefersReducedMotion, entranceProps } = useStaggerEntrance(fakeReducedMotion(true));

    expect(prefersReducedMotion.value).toBe(true);
    expect(entranceProps(2).transition).toEqual({ duration: 0, delay: 0, ease: [0.2, 0.8, 0.2, 1] });
  });
});
