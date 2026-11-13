import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const root = resolve(__dirname, "../..");
const shaders = readFileSync(resolve(root, "app/utils/inkFluid.ts"), "utf8");
const composable = readFileSync(resolve(root, "app/composables/useInkFluid.ts"), "utf8");

/**
 * 墨流的"元素当墙"能力。
 *
 * 背景：墨流没有碰撞体，元素只是画布上方的 DOM，所以墨会直接穿过卡片和大标题，
 * 或者从元素里"渗"出来 —— 看着像碰到元素还会往回弹。
 * 现在把元素外框烘成遮罩，流场按**实心墙**处理（和画布边缘同一套边界条件）。
 *
 * 这几条只是静态门禁：遮罩一旦从某个 pass 上掉下来，观感就退回旧样子，
 * 而这种退化在快照里看不出来。
 */
describe("墨流的元素遮罩", () => {
  it("压力 / 散度 / 梯度三个 pass 都读遮罩（墙要真的参与求解）", () => {
    expect(shaders).toMatch(/divergence: `[\s\S]*?uMask/);
    expect(shaders).toMatch(/pressure: `[\s\S]*?uMask/);
    expect(shaders).toMatch(/gradient: `[\s\S]*?uMask/);
  });

  it("墙里不留速度、不留染料", () => {
    // 速度：gradient pass 用遮罩把速度清零。
    expect(shaders).toMatch(/v \* \(1\.0 - step\(0\.5, texture\(uMask, vUv\)\.r\)\)/);
    // 染料：单独一个 mask pass 把墙内的墨抹掉。
    expect(shaders).toMatch(/mask: `[\s\S]*?uMask/);
    expect(composable).toMatch(/programs\.mask/);
  });

  it("遮罩来自统一的元素收集器，而不是墨流自己再写一套筛选", () => {
    expect(composable).toContain('from "~/utils/occluders"');
    expect(composable).toContain("collectOccluders(");
  });

  it("压着字的装饰让位给字（否则墨永远碰不到字）", () => {
    const occluders = readFileSync(resolve(root, "app/utils/occluders.ts"), "utf8");
    // 装饰块被字框覆盖 15% 以上就不再是墙；标题的字层用 data-glass-text 标记，永远算字。
    expect(occluders).toContain("isTextBackdrop");
    expect(occluders).toMatch(/covered \/ area > 0\.15/);
    expect(occluders).toContain('data-glass-text');
    // 纯文字块必须取逐字框，而不是块级元素撑满整行的外框。
    expect(occluders).toContain("textRunBoxes");
    expect(occluders).toMatch(/setStart\(node, i\)/);
  });
  it("遮罩节流刷新，并在销毁时释放纹理", () => {
    expect(composable).toMatch(/now - maskScanAt > 600/);
    expect(composable).toMatch(/gl\.deleteTexture\(maskTex\)/);
  });
});
