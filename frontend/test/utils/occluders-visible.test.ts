import { afterEach, describe, expect, it, vi } from "vitest";

import { collectOccluders, visibleOccluderRect } from "~/utils/occluders";

/**
 * 「装饰的墙 = 元素**可见**的外框」。
 *
 * 判据原来直接吃 `getBoundingClientRect()`（布局盒）。轮播图的图片层为了留视差余量
 * 外扩了约 41px，却被面板的 `overflow: clip` + 20px 圆角裁掉 —— 按布局盒算，指针墨流
 * 会在卡片四周空出一圈黑边（用户反馈「与轮播图的碰撞范围太大了」）。
 *
 * 这里把「布局盒 → 可见盒」的求交钉住：裁剪祖先真的切到元素时，墙缩到裁剪边，
 * 圆角也跟着取裁剪祖先的半径；元素完整落在祖先里时，仍用它自己的半径。
 */

interface FakeRect {
  left: number;
  top: number;
  width: number;
  height: number;
}

interface FakeStyle {
  overflowX?: string;
  overflowY?: string;
  borderWidth?: number;
  radius?: number;
  ink?: boolean;
}

const realGetComputedStyle = window.getComputedStyle.bind(window);
let cleanup: Array<() => void> = [];

function rectOf({ left, top, width, height }: FakeRect): DOMRect {
  return {
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    x: left,
    y: top,
    toJSON: () => ({ left, top, width, height }),
  } as DOMRect;
}

/**
 * 装一台假舞台：`boxes` 给每个元素装假布局盒，`styles` 给它装假计算样式。
 * 返回 host（宿主）与按 `data-test` 取元素的小工具。
 */
function stage(boxes: Record<string, FakeRect>, styles: Record<string, FakeStyle> = {}) {
  const host = document.createElement("div");
  host.innerHTML = Object.keys(boxes)
    .map((key) => `<div data-test="${key}"></div>`)
    .join("");
  // 让 `<data-test>` 之间按需要嵌套：调用方用 `nest` 再挂父子关系。
  document.body.append(host);

  const byKey = (key: string) => {
    const el = host.querySelector<HTMLElement>(`[data-test="${key}"]`);
    if (!el) throw new Error(`stage 缺少节点 ${key}`);
    return el;
  };

  for (const [key, box] of Object.entries(boxes)) {
    byKey(key).getBoundingClientRect = () => rectOf(box);
  }

  const spy = vi.spyOn(window, "getComputedStyle").mockImplementation((el) => {
    const key = (el as HTMLElement).dataset?.test;
    const fake = key ? styles[key] : undefined;
    if (!fake) return realGetComputedStyle(el as Element);
    const border = fake.borderWidth ?? 0;
    return {
      overflowX: fake.overflowX ?? "visible",
      overflowY: fake.overflowY ?? "visible",
      borderTopWidth: `${border}px`,
      borderRightWidth: `${border}px`,
      borderBottomWidth: `${border}px`,
      borderLeftWidth: `${border}px`,
      borderTopLeftRadius: `${fake.radius ?? 0}px`,
      display: "block",
      visibility: "visible",
      opacity: "1",
      backgroundColor: fake.ink ? "rgb(28, 27, 25)" : "rgba(0, 0, 0, 0)",
      boxShadow: "none",
      textContent: "",
    } as unknown as CSSStyleDeclaration;
  });

  cleanup.push(() => {
    spy.mockRestore();
    host.remove();
  });

  return { host, byKey, nest: (parent: string, child: string) => byKey(parent).append(byKey(child)) };
}

afterEach(() => {
  for (const fn of cleanup) fn();
  cleanup = [];
});

describe("visibleOccluderRect：布局盒 → 可见盒", () => {
  it("被 overflow: clip + 圆角的卡片裁掉时，墙缩到卡片边、半径取卡片的", () => {
    const { byKey, nest } = stage(
      {
        card: { left: 100, top: 200, width: 672, height: 378 },
        // 图片层布局盒比卡片大一圈（视差余量）：左右各 41px、上下各 41px 附近
        img: { left: 59, top: 159, width: 795, height: 464 },
      },
      {
        card: { overflowX: "clip", overflowY: "clip", radius: 20, ink: true },
        img: { radius: 0 },
      },
    );
    nest("card", "img");

    const visible = visibleOccluderRect(byKey("img"), null);
    expect(visible).not.toBeNull();
    expect(visible).toMatchObject({ x: 100, y: 200, w: 672, h: 378, radius: 20 });
  });

  it("元素完整落在裁剪祖先里时，保留它自己的半径（不继承卡片的）", () => {
    const { byKey, nest } = stage(
      {
        card: { left: 100, top: 200, width: 672, height: 378 },
        chip: { left: 120, top: 220, width: 80, height: 32 },
      },
      {
        card: { overflowX: "clip", overflowY: "clip", radius: 20, ink: true },
        chip: { radius: 16, ink: true },
      },
    );
    nest("card", "chip");

    const visible = visibleOccluderRect(byKey("chip"), null);
    expect(visible).toMatchObject({ x: 120, y: 220, w: 80, h: 32, radius: 16 });
  });

  it("多层裁剪取交集（外层先切、内层再切）", () => {
    const { byKey, nest } = stage(
      {
        outer: { left: 0, top: 0, width: 800, height: 600 },
        inner: { left: 100, top: 100, width: 400, height: 300 },
        bleed: { left: 50, top: 50, width: 600, height: 500 },
      },
      {
        outer: { overflowX: "hidden", overflowY: "hidden", radius: 24 },
        inner: { overflowX: "clip", overflowY: "clip", radius: 10 },
        bleed: { radius: 0 },
      },
    );
    nest("outer", "inner");
    nest("inner", "bleed");

    const visible = visibleOccluderRect(byKey("bleed"), null);
    // 内层最后裁到 100..500 / 100..400，半径取内层的 10。
    expect(visible).toMatchObject({ x: 100, y: 100, w: 400, h: 300, radius: 10 });
  });

  it("被完全裁没的元素返回 null", () => {
    const { byKey, nest } = stage(
      {
        card: { left: 100, top: 200, width: 100, height: 100 },
        ghost: { left: 300, top: 400, width: 50, height: 50 },
      },
      { card: { overflowX: "clip", overflowY: "clip", radius: 12 } },
    );
    nest("card", "ghost");
    expect(visibleOccluderRect(byKey("ghost"), null)).toBeNull();
  });

  it("祖先都不裁剪时等同于布局盒", () => {
    const { byKey } = stage(
      { plain: { left: 10, top: 20, width: 30, height: 40 } },
      { plain: { radius: 8 } },
    );
    expect(visibleOccluderRect(byKey("plain"), null)).toMatchObject({
      x: 10,
      y: 20,
      w: 30,
      h: 40,
      radius: 8,
    });
  });
});

describe("collectOccluders：墙用可见盒", () => {
  it("卡片里的图片层不再按外扩的布局盒算墙（用户看到的黑边就是它）", () => {
    const { host } = stage(
      {
        cover: { left: 0, top: 0, width: 1920, height: 600 },
        card: { left: 100, top: 200, width: 672, height: 378 },
        img: { left: 59, top: 159, width: 795, height: 464 },
      },
      {
        cover: { radius: 0 },
        card: { overflowX: "clip", overflowY: "clip", radius: 20, ink: true },
        img: { radius: 0 },
      },
    );
    // 组装成 cover > card > img 的层级
    const cover = host.querySelector<HTMLElement>('[data-test="cover"]')!;
    const card = host.querySelector<HTMLElement>('[data-test="card"]')!;
    const img = host.querySelector<HTMLElement>('[data-test="img"]')!;
    cover.append(card);
    card.append(img);

    const boxes = collectOccluders(cover, null, { minSize: 6 });
    const cardBox = boxes.find((box) => Math.abs(box.w - 672) < 1 && Math.abs(box.h - 378) < 1);
    expect(cardBox).toBeTruthy();
    // 没有 795×464 那种"比卡片大一圈"的墙
    expect(boxes.some((box) => Math.abs(box.w - 795) < 1)).toBe(false);
    // 图片层折到卡片上，圆角也变成卡片的
    expect(boxes.filter((box) => Math.abs(box.w - 672) < 1).every((box) => box.radius === 20)).toBe(true);
  });
});
