import { describe, expect, it } from "vitest";

import {
  MOTION_JUMP_TRANSITION,
  SPRING_DOT,
  SPRING_NECK,
  SPRING_PRESETS,
  SPRING_TRAVEL,
  SPRING_TURN,
  resolveSpringMotion,
  type SpringPreset,
} from "~/utils/motion";

/**
 * 弹簧预设与「要么弹簧、要么立即落位」的解析（ADR-0012 后续修订 2026-11-07，
 * grill 记录 §三 3.8 / 3.9）。
 *
 * 这里测的是可观察的**取值约定**与**解析结果的行为**，不是动画怎么写：
 *   1. 四组预设齐全、参数集中在动效预算里、组件改不动它们；
 *   2. 解析助手在「减少动态」下给的是**静止**（时长恰为 0），不是慢速弹簧 ——
 *      这是本次唯一真正的新语义，也是 `motion-foundation` / `motion-tier-c` 之外
 *      第二套预算的落位口径。
 *
 * 参数值里 travel / turn / neck 是 grill §3.8 登记过的数字（140/26/0.6、
 * 260/30、200/28/1），dot 未登记，只约束它的**手感性质**（近临界阻尼、不回弹），
 * 免得把"调手感"变成改测试。
 */

/** 阻尼比 ζ = damping / (2√(k·m))：1 左右意味着"落到位就不再晃"。 */
function dampingRatio(preset: SpringPreset): number {
  return preset.damping / (2 * Math.sqrt(preset.stiffness * preset.mass));
}

describe("弹簧预设：四组、集中一处、组件改不动", () => {
  it("四组预设齐全（飞行器位移 / 转向 / 目录树圆点 / 分段条颈部）", () => {
    expect(Object.keys(SPRING_PRESETS).sort()).toEqual(["dot", "neck", "travel", "turn"]);
  });

  it("分组名字与预设对象一一对应（不靠调用点各自持有一份副本）", () => {
    expect(SPRING_PRESETS.travel).toBe(SPRING_TRAVEL);
    expect(SPRING_PRESETS.turn).toBe(SPRING_TURN);
    expect(SPRING_PRESETS.dot).toBe(SPRING_DOT);
    expect(SPRING_PRESETS.neck).toBe(SPRING_NECK);
  });

  it("travel / turn / neck 的取值就是登记过的那三组（grill §3.8）", () => {
    expect(SPRING_TRAVEL).toEqual({ stiffness: 140, damping: 26, mass: 0.6 });
    expect(SPRING_TURN).toEqual({ stiffness: 260, damping: 30, mass: 1 });
    expect(SPRING_NECK).toEqual({ stiffness: 200, damping: 28, mass: 1 });
  });

  it("dot 是近临界阻尼：落到位后不回弹（不是「永远在晃」的软弹簧）", () => {
    const ratio = dampingRatio(SPRING_DOT);

    expect(ratio).toBeGreaterThanOrEqual(0.85);
    expect(ratio).toBeLessThanOrEqual(1.15);
  });

  it("四组参数两两不同（四组不是同一个数字抄四遍）", () => {
    const signatures = Object.values(SPRING_PRESETS).map(
      ({ stiffness, damping, mass }) => `${stiffness}/${damping}/${mass}`,
    );

    expect(new Set(signatures).size).toBe(signatures.length);
  });

  it("每组参数都是正的有限数（0 / NaN / 负数不会被当成「更软」）", () => {
    for (const [name, preset] of Object.entries(SPRING_PRESETS)) {
      for (const [key, value] of Object.entries(preset)) {
        expect(Number.isFinite(value), `${name}.${key}`).toBe(true);
        expect(value, `${name}.${key}`).toBeGreaterThan(0);
      }
    }
  });

  it("预算是冻结的：组件拿到预设也改不动全站手感", () => {
    expect(Object.isFrozen(SPRING_PRESETS)).toBe(true);
    for (const [name, preset] of Object.entries(SPRING_PRESETS)) {
      expect(Object.isFrozen(preset), name).toBe(true);
    }
  });
});

describe("resolveSpringMotion：要么弹簧，要么立即落位", () => {
  it("不减少动态时就是该预设的弹簧（参数原样带过去）", () => {
    expect(resolveSpringMotion(SPRING_TRAVEL, false)).toEqual({
      type: "spring",
      stiffness: 140,
      damping: 26,
      mass: 0.6,
    });
    expect(resolveSpringMotion(SPRING_NECK, false)).toEqual({
      type: "spring",
      stiffness: 200,
      damping: 28,
      mass: 1,
    });
  });

  it("解析结果是副本：组件就地改它不会污染预算", () => {
    const resolved = resolveSpringMotion(SPRING_DOT, false);

    expect(resolved).not.toBe(SPRING_DOT);
    expect(resolved).toEqual({
      type: "spring",
      stiffness: SPRING_DOT.stiffness,
      damping: SPRING_DOT.damping,
      mass: SPRING_DOT.mass,
    });
  });

  it("减少动态时落到静止：时长恰为 0、且不再带弹簧参数（不是「慢速弹簧」）", () => {
    const resolved = resolveSpringMotion(SPRING_DOT, true);

    expect(resolved).toEqual({ type: "tween", duration: 0, delay: 0 });
    expect(resolved).not.toHaveProperty("stiffness");
    expect(resolved).not.toHaveProperty("damping");
    expect(resolved).not.toHaveProperty("mass");
  });

  it("四组预设共用同一条落位规则（谁也不能开小差）", () => {
    for (const [name, preset] of Object.entries(SPRING_PRESETS)) {
      expect(resolveSpringMotion(preset, true), name).toEqual(MOTION_JUMP_TRANSITION);
      expect(resolveSpringMotion(preset, false), name).toHaveProperty("type", "spring");
    }
  });

  it("落位常量是冻结的：没人能把它改成「放慢」", () => {
    expect(Object.isFrozen(MOTION_JUMP_TRANSITION)).toBe(true);
    expect(MOTION_JUMP_TRANSITION).toEqual({ type: "tween", duration: 0, delay: 0 });
  });
});
