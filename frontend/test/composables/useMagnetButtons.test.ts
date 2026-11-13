import { describe, expect, it } from "vitest";

import { magnetTarget } from "~/composables/useMagnetButtons";

/**
 * 磁吸位移的纯数学部分（接线与环境降级另有组件级验证）。
 *
 * 钉住三件事：
 *   1. 位移与指针偏移成正比（偏移 / strength）；
 *   2. 单轴永远不超过 maxOffset（防止按钮被拖出格子）；
 *   3. 关闭（strength ≤ 0）时恒为 0，供"减少动态"之外的降级路径使用。
 */
describe("magnetTarget：磁吸位移的边界", () => {
  it("位移 = 指针偏移 / strength（正向）", () => {
    expect(magnetTarget(12, 4, 8)).toBe(3);
    expect(magnetTarget(32, 4, 8)).toBe(8);
  });

  it("反方向对称，且同样受上限约束", () => {
    expect(magnetTarget(-12, 4, 8)).toBe(-3);
    expect(magnetTarget(-48, 4, 8)).toBe(-8);
  });

  it("strength ≤ 0 时恒为 0（不产生位移）", () => {
    expect(magnetTarget(30, 0, 8)).toBe(0);
    expect(magnetTarget(-30, -2, 8)).toBe(0);
  });
});
