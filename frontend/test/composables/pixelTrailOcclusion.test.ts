import { describe, expect, it } from "vitest";

import { hasVisibleInk, hitsRoundedBox } from "~/utils/occluders";

/**
 * 「尾迹碰到别的元素就隐身」的命中判定。
 *
 * 这是首屏留白的唯一判据：尾迹运动照旧，只是**画不画**由它决定。
 * 画布效果没法在 CI 里跑，所以把判定抽成纯函数在这里钉住：
 * 矩形内部命中、外拓 padding 也算命中、圆角外那一小块不算。
 */
describe("圆角外框命中判定", () => {
  const box = { x: 100, y: 100, w: 200, h: 120, radius: 16 };

  it("矩形内部命中", () => {
    expect(hitsRoundedBox(200, 160, box, 0)).toBe(true);
  });

  it("外拓范围内也算命中（尾迹提前隐身，不贴着边画）", () => {
    expect(hitsRoundedBox(200, 96, box, 8)).toBe(true);
    expect(hitsRoundedBox(96, 160, box, 8)).toBe(true);
  });

  it("超出外拓范围不命中", () => {
    expect(hitsRoundedBox(200, 88, box, 8)).toBe(false);
    // 右边沿是 x=300，外拓 8 之后到 308：320 已经在外面。
    expect(hitsRoundedBox(320, 160, box, 8)).toBe(false);
  });

  it("圆角外侧的角落不命中（圆角真的参与判断）", () => {
    // 外框左上角顶点：直角会命中，半径 16 的圆角不该命中。
    expect(hitsRoundedBox(101, 101, box, 0)).toBe(false);
    // 圆心 (116,116) 内侧一点：命中。
    expect(hitsRoundedBox(116, 116, box, 0)).toBe(true);
    expect(hitsRoundedBox(120, 120, box, 0)).toBe(true);
  });

  it("没有圆角时退化为普通矩形", () => {
    expect(hitsRoundedBox(101, 101, { ...box, radius: 0 }, 0)).toBe(true);
  });
});

/**
 * 「元素自己有墨」的判据。
 *
 * 这一条是下沿刻度的关键：数字是 <li>（叶子块，本来就会被遮），但那条发丝线是
 * <ul> 自己的 border-top —— 只看叶子块就会漏掉线，字压线看不见才叫脏。
 */
describe("元素是否自带可见的墨", () => {
  const style = (overrides: Partial<Record<string, string>> = {}) => ({
    borderTopWidth: "0px",
    borderRightWidth: "0px",
    borderBottomWidth: "0px",
    borderLeftWidth: "0px",
    backgroundColor: "rgba(0, 0, 0, 0)",
    boxShadow: "none",
    ...overrides,
  });

  it("纯布局容器（无描边 / 无底色 / 无阴影）不算", () => {
    expect(hasVisibleInk(style())).toBe(false);
  });

  it("只有一条上描边也算（下沿刻度就是这种）", () => {
    expect(hasVisibleInk(style({ borderTopWidth: "1px" }))).toBe(true);
  });

  it("只有底色也算（胶囊小签）", () => {
    expect(hasVisibleInk(style({ backgroundColor: "rgb(28, 27, 25)" }))).toBe(true);
  });

  it("只有阴影也算（浮起的卡）", () => {
    expect(hasVisibleInk(style({ boxShadow: "0 1px 2px black" }))).toBe(true);
  });
});
