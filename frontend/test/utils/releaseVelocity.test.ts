import { describe, expect, it } from "vitest";

import {
  JELLY_RELEASE_VELOCITY_WINDOW_MS,
  releaseVelocity,
  SHEET_RELEASE_VELOCITY_WINDOW_MS,
} from "~/utils/releaseVelocity";

/**
 * 松手瞬时速度的唯一实现（票 423）。
 *
 * 捏捏落位与评论面板下滑关闭共用这里；两个窗口各登记一条命名常量，
 * 测试按"最后 80 / 100ms"分别钉住边界。单位是 px/ms —— 捏捏自己换算成 px/s。
 */

function sample(x: number, y: number, t: number) {
  return { x, y, t };
}

describe("releaseVelocity：窗口内最早一点 → 松手点", () => {
  it("取窗口内最早采样到松手点的位移差（px/ms）", () => {
    const release = releaseVelocity(
      [sample(0, 0, 900), sample(0, 10, 950), sample(0, 40, 1_000)],
      1_000,
      100,
    );

    // 窗口 [900, 1000)：最早是 900 → 100ms 走了 40px。
    expect(release.x).toBe(0);
    expect(release.y).toBeCloseTo(0.4);
  });

  it("窗口外的旧采样不参与（慢拖后停住再松手不会被平均成'甩出去'）", () => {
    expect(releaseVelocity([sample(0, 0, 0), sample(0, 120, 800)], 800, 100)).toEqual({
      x: 0,
      y: 0,
    });
  });

  it("两个轴各自求差（捏捏按 x/y 分轴把初速交给弹簧）", () => {
    const release = releaseVelocity([sample(10, 20, 960), sample(30, 0, 1_000)], 1_000, 80);

    expect(release.x).toBeCloseTo(20 / 40);
    expect(release.y).toBeCloseTo(-20 / 40);
  });

  it("没有速度证据时返回 0：少于两点、没有窗口内采样、时间差 ≤0、非法输入", () => {
    expect(releaseVelocity([sample(0, 40, 1_000)], 1_000, 100)).toEqual({ x: 0, y: 0 });
    expect(releaseVelocity([], 1_000, 100)).toEqual({ x: 0, y: 0 });
    expect(releaseVelocity([sample(0, 0, 500), sample(0, 40, 1_000)], 1_000, 100)).toEqual({
      x: 0,
      y: 0,
    });
    expect(releaseVelocity([sample(0, 0, 1_000), sample(0, 40, 1_000)], 1_000, 100)).toEqual({
      x: 0,
      y: 0,
    });
    expect(releaseVelocity([sample(Number.NaN, 0, 900), sample(0, 40, 1_000)], 1_000, 100)).toEqual(
      { x: 0, y: 0 },
    );
    expect(releaseVelocity([sample(0, 0, 900), sample(0, 40, 1_000)], 1_000, Number.NaN)).toEqual({
      x: 0,
      y: 0,
    });
  });

  it("两个消费者用各自登记的窗口（80ms / 100ms）", () => {
    expect(JELLY_RELEASE_VELOCITY_WINDOW_MS).toBe(80);
    expect(SHEET_RELEASE_VELOCITY_WINDOW_MS).toBe(100);

    // 起点 910：80ms 窗口从 920 起算，910 落在窗外 → 没有证据。
    expect(releaseVelocity([sample(0, 0, 910), sample(0, 40, 1_000)], 1_000, 80)).toEqual({
      x: 0,
      y: 0,
    });
    // 同一个输入在 100ms 窗口里有效：90ms 走了 40px。
    expect(
      releaseVelocity([sample(0, 0, 910), sample(0, 40, 1_000)], 1_000, 100).y,
    ).toBeCloseTo(40 / 90);
  });
});
