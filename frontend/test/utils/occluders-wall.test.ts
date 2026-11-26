import { afterEach, describe, expect, it, vi } from "vitest";

import { collectOccluders } from "~/utils/occluders";

/**
 * 显式墙 `data-ink-wall`（票 441）。
 *
 * 背景：首屏轮播面板是**设计上必须挡墨**的实心面，但自动判定里有三条启发式会把它
 * 踢出墙：面积上限（面板面积随视口比例变化）、压字让位（面板里压着轮播标题）、
 * 零透明度（进场瞬间）。任何一条命中，墨就从面板里穿过去 —— 观感是"轮播图挡不住墨"，
 * 而且只在某些窗口尺寸/时机复现。所以这类元素用标记声明，不再交给启发式猜。
 *
 * 这些用例只钉判据本身：真实的 DOM/CSS 由设计门禁与线上快照验证。
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
  opacity?: string;
  display?: string;
  visibility?: string;
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

/** 假舞台：每个 `data-test="key"` 元素配一个假布局盒 + 假计算样式。 */
function stage(boxes: Record<string, FakeRect>, styles: Record<string, FakeStyle> = {}) {
  const host = document.createElement("div");
  host.innerHTML = Object.keys(boxes)
    .map((key) => `<div data-test="${key}"></div>`)
    .join("");
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
      display: fake.display ?? "block",
      visibility: fake.visibility ?? "visible",
      opacity: fake.opacity ?? "1",
      backgroundColor: fake.ink ? "rgb(28, 27, 25)" : "rgba(0, 0, 0, 0)",
      boxShadow: "none",
    } as unknown as CSSStyleDeclaration;
  });

  cleanup.push(() => {
    spy.mockRestore();
    host.remove();
  });

  return { host, byKey };
}

afterEach(() => {
  for (const fn of cleanup) fn();
  cleanup = [];
});

describe("data-ink-wall：显式墙跳过所有启发式", () => {
  it("面积超过上限的墙依然进墙（否则大视口下轮播面板会被踢出）", () => {
    const { host, byKey } = stage(
      {
        hero: { left: 0, top: 0, width: 1200, height: 600 },
        panel: { left: 100, top: 200, width: 900, height: 350 },
      },
      { hero: { ink: true }, panel: { ink: true } },
    );
    byKey("hero").append(byKey("panel"));
    byKey("panel").setAttribute("data-ink-wall", "");

    // 900×350 = 315k，宿主 1200×600 = 720k → 43.75%（上限 0.6，本来没问题）；
    // 把上限调到 0.2 模拟"面板在大视口里占比过高"的场景。
    const capped = collectOccluders(host, null, { minSize: 6, maxAreaRatio: 0.2 });
    const forced = collectOccluders(host, null, { minSize: 6, maxAreaRatio: 0.2, wallSelector: "[data-ink-wall]" });

    expect(capped.some((box) => Math.abs(box.w - 900) < 1 && Math.abs(box.h - 350) < 1)).toBe(false);
    expect(forced.some((box) => Math.abs(box.w - 900) < 1 && Math.abs(box.h - 350) < 1)).toBe(true);
  });

  it("被标题字框压住的墙依然是墙（不让位给字）", () => {
    const { host, byKey } = stage(
      {
        panel: { left: 100, top: 200, width: 300, height: 200 },
        title: { left: 110, top: 300, width: 280, height: 80 },
      },
      { panel: { ink: true }, title: {} },
    );
    byKey("panel").append(byKey("title"));
    byKey("panel").setAttribute("data-ink-wall", "");
    byKey("title").textContent = "很长的轮播标题";

    const withoutForced = collectOccluders(host, null, { minSize: 6 });
    const withForced = collectOccluders(host, null, { minSize: 6, wallSelector: "[data-ink-wall]" });
    const panelBox = (boxes: ReturnType<typeof collectOccluders>) =>
      boxes.some((box) => Math.abs(box.w - 300) < 1 && Math.abs(box.h - 200) < 1);

    // 字框覆盖 280×80 / (300×200) = 37% > 15%：不声明时面板被当成"压字的装饰"。
    expect(panelBox(withoutForced)).toBe(false);
    expect(panelBox(withForced)).toBe(true);
  });

  it("零透明度下依然是墙（进场瞬间也不该让墨穿过去）", () => {
    const { host, byKey } = stage(
      {
        hero: { left: 0, top: 0, width: 800, height: 500 },
        panel: { left: 40, top: 120, width: 400, height: 225 },
      },
      { hero: { ink: true }, panel: { ink: true, opacity: "0" } },
    );
    byKey("hero").append(byKey("panel"));
    byKey("panel").setAttribute("data-ink-wall", "");

    const boxes = collectOccluders(host, null, { minSize: 6, wallSelector: "[data-ink-wall]" });

    expect(boxes.some((box) => Math.abs(box.w - 400) < 1 && Math.abs(box.h - 225) < 1)).toBe(true);
  });

  it("display: none / visibility: hidden 的墙不算（看不见的东西不该挡墨）", () => {
    const { host, byKey } = stage(
      {
        hero: { left: 0, top: 0, width: 800, height: 500 },
        panel: { left: 40, top: 120, width: 400, height: 225 },
      },
      { hero: { ink: true }, panel: { ink: true, display: "none" } },
    );
    byKey("hero").append(byKey("panel"));
    byKey("panel").setAttribute("data-ink-wall", "");

    const boxes = collectOccluders(host, null, { minSize: 6, wallSelector: "[data-ink-wall]" });

    expect(boxes.some((box) => Math.abs(box.w - 400) < 1)).toBe(false);
  });

  it("data-ink-pass 优先：同时声明 pass 与 wall 的元素，墨穿过去", () => {
    const { host, byKey } = stage(
      {
        hero: { left: 0, top: 0, width: 800, height: 500 },
        panel: { left: 40, top: 120, width: 400, height: 225 },
      },
      { hero: { ink: true }, panel: { ink: true } },
    );
    byKey("hero").append(byKey("panel"));
    byKey("panel").setAttribute("data-ink-wall", "");
    byKey("panel").setAttribute("data-ink-pass", "");

    const boxes = collectOccluders(host, null, {
      minSize: 6,
      skipSelector: "[data-ink-pass]",
      wallSelector: "[data-ink-wall]",
    });

    expect(boxes.some((box) => Math.abs(box.w - 400) < 1)).toBe(false);
  });

  it("没声明的元素仍然受启发式约束（显式墙是出口，不是取消判据）", () => {
    const { host, byKey } = stage(
      {
        hero: { left: 0, top: 0, width: 800, height: 500 },
        plain: { left: 40, top: 120, width: 400, height: 225 },
      },
      { hero: { ink: true }, plain: { ink: true, opacity: "0" } },
    );
    byKey("hero").append(byKey("plain"));

    const boxes = collectOccluders(host, null, { minSize: 6, wallSelector: "[data-ink-wall]" });

    expect(boxes.some((box) => Math.abs(box.w - 400) < 1)).toBe(false);
  });
});
