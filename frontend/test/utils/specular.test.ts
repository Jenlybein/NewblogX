import { describe, expect, it } from "vitest";

import { specularPercentage } from "~/utils/specular";

/**
 * 指针镜面高光的坐标换算（纯函数）。
 *
 * 为什么值得测：`--mx/--my` 是"百分比的圆心"，算错一位就会把高光甩到元素外；
 * 同时这段会跑在 pointermove 的热路径上，必须有除零兜底，且不做布局读取之外的副作用。
 */
const rect = { left: 10, top: 20, width: 200, height: 100 };

describe("specularPercentage", () => {
  it("元素中心 → 50% / 50%", () => {
    expect(specularPercentage(110, 70, rect)).toEqual({ mx: 50, my: 50 });
  });

  it("左上角 → 0% / 0%（角度按矩形反推，不做任何截断的假设）", () => {
    expect(specularPercentage(10, 20, rect)).toEqual({ mx: 0, my: 0 });
  });

  it("右下角 → 100% / 100%", () => {
    expect(specularPercentage(210, 120, rect)).toEqual({ mx: 100, my: 100 });
  });

  it("指针跑出元素时钳到 0–100（CSS 变量不能出现负数或越界百分比）", () => {
    expect(specularPercentage(-40, -30, rect)).toEqual({ mx: 0, my: 0 });
    expect(specularPercentage(400, 900, rect)).toEqual({ mx: 100, my: 100 });
  });

  it("尺寸为 0（未挂载 / display:none）时退回中心，不产生 NaN", () => {
    expect(specularPercentage(30, 30, { left: 0, top: 0, width: 0, height: 0 })).toEqual({
      mx: 50,
      my: 50,
    });
  });

  it("百分比保留两位小数（避免把长浮点写进 CSS 变量）", () => {
    expect(specularPercentage(11, 20, { left: 10, top: 20, width: 3, height: 7 })).toEqual({
      mx: 33.33,
      my: 0,
    });
  });

  it("NaN 输入按居中兜底", () => {
    expect(specularPercentage(Number.NaN, Number.NaN, rect)).toEqual({ mx: 50, my: 50 });
  });
});
