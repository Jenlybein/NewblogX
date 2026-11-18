import type { VueWrapper } from "@vue/test-utils";
import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import CommentComposerSheet from "~/components/comment/CommentComposerSheet.vue";
import {
  sheetDragOffset,
  sheetReleaseVelocityPxPerMs,
  shouldDismissSheetDrag,
} from "~/utils/commentSheetDrag";

/**
 * 评论输入面板（决定清单 §4 第 19–22 条 + 票 #431）。
 *
 * 规格里有一半是"怎么关"，这些只能靠挂载后的点击/按键/指针断言，源码 grep 证不了：
 *   1. 点面板外（透明捕获层）收起；
 *   2. Esc 收起；
 *   3. 点右上 × 收起；
 *   4. 面板内**只有一个**提交按钮（取消 / 清空已退役），且没有登录提示；
 *   5. 提交把内容原样抛给页面（页面负责调接口 + 关面板）；
 *   6. #431：三条关闭路径都先播 120ms 退场、再等 `transitionend` 卸载；
 *   7. #431：把手下滑 1:1 跟手，过阈值（30% 高度或 0.11 px/ms）才关，否则弹回。
 *
 * 不断言像素：这里只断言"DOM 上还有没有面板""组件有没有 emit""位移是否跟手"，
 * 以及阈值判定函数本身。视觉取值归 tailwind.css 的 token 与设计门禁管。
 */

vi.mock("~/composables/useReaderLoginGate", () => ({
  useReaderLoginGate: () => async () => true,
}));

function mountSheet(props: Record<string, unknown> = {}) {
  return mount(CommentComposerSheet, { props: { show: true, ...props }, attachTo: document.body });
}

function panelOf(wrapper: VueWrapper) {
  return wrapper.get('[data-role="comment-sheet-panel"]');
}

/** 面板的内联 style 串（退场位、拖拽位移都写在 transform/opacity 上）。 */
function panelStyle(wrapper: VueWrapper): string {
  return panelOf(wrapper).attributes("style") ?? "";
}

/**
 * happy-dom 没有真实布局：面板高度由用例钉死，30% 阈值才有判定依据。
 * 生产里取 `getBoundingClientRect().height`。
 */
function stubPanelHeight(wrapper: VueWrapper, height: number) {
  const element = panelOf(wrapper).element as HTMLElement;
  vi.spyOn(element, "getBoundingClientRect").mockReturnValue({ height } as DOMRect);
}

/** 事件时间戳取 `performance.now()`（happy-dom 的 Event 构造器也是这么打的）。 */
function firePointer(target: Element, type: string, clientY: number, pointerId = 1) {
  target.dispatchEvent(
    new PointerEvent(type, {
      bubbles: true,
      cancelable: true,
      clientY,
      pointerId,
      pointerType: "touch",
      isPrimary: pointerId === 1,
    }),
  );
}

describe("CommentComposerSheet：三条关闭途径", () => {
  it("点面板外（透明捕获层）收起", async () => {
    const wrapper = mountSheet();

    await wrapper.get('[data-role="comment-sheet-scrim"]').trigger("click");

    expect(wrapper.emitted("update:show")?.[0]).toEqual([false]);
    wrapper.unmount();
  });

  it("按 Esc 收起", async () => {
    const wrapper = mountSheet();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted("update:show")?.[0]).toEqual([false]);
    wrapper.unmount();
  });

  it("点右上 × 收起", async () => {
    const wrapper = mountSheet();

    await wrapper.get('[data-role="comment-sheet-close"]').trigger("click");

    expect(wrapper.emitted("update:show")?.[0]).toEqual([false]);
    wrapper.unmount();
  });

  it("收起时不渲染任何东西（面板与捕获层都不在）", () => {
    const wrapper = mount(CommentComposerSheet, { props: { show: false } });

    expect(wrapper.find('[data-role="comment-sheet"]').exists()).toBe(false);
    expect(wrapper.find('[data-role="comment-sheet-scrim"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("打开与收起都不锁 body 滚动（读者还要边看正文边写）", async () => {
    const wrapper = mountSheet();

    expect(document.body.style.overflow).toBe("");
    await wrapper.get('[data-role="comment-sheet-close"]').trigger("click");
    expect(document.body.style.overflow).toBe("");
    wrapper.unmount();
  });
});

describe("CommentComposerSheet：面板内部", () => {
  it("只有一个提交按钮，没有取消 / 清空，也没有登录提示", () => {
    const wrapper = mountSheet({ title: "回复 张三", submitLabel: "发送回复" });

    expect(wrapper.findAll('[data-role="comment-submit"]')).toHaveLength(1);
    expect(wrapper.text()).toContain("发送回复");
    expect(wrapper.text()).not.toContain("取消");
    wrapper.unmount();
  });

  it("面板带标题行（发表评论 / 回复 X）与抓手，语义在 dialog 上", () => {
    const wrapper = mountSheet({ title: "回复 张三" });
    const dialog = wrapper.get('[role="dialog"]');

    expect(dialog.attributes("aria-label")).toBe("回复 张三");
    expect(wrapper.find(".comment-sheet__grabber").exists()).toBe(true);
    wrapper.unmount();
  });

  it("提交把内容原样抛给页面（页面负责调接口与关面板）", async () => {
    const wrapper = mountSheet();

    await wrapper.get("textarea").setValue("  我的看法  ");
    await wrapper.get('[data-role="comment-submit"]').trigger("click");

    expect(wrapper.emitted("submit")?.[0]).toEqual(["我的看法"]);
    wrapper.unmount();
  });
});

describe("CommentComposerSheet：退场（先滑走再卸载，由 transitionend 驱动）", () => {
  it("点 × 后 emit 仍然同步，但面板还在、且已经站在退场位", async () => {
    const wrapper = mountSheet();

    await wrapper.get('[data-role="comment-sheet-close"]').trigger("click");

    // emit 必须立即发生：页面靠它同步收起 v-model:show，不能等动画。
    expect(wrapper.emitted("update:show")).toEqual([[false]]);
    expect(wrapper.find('[data-role="comment-sheet-panel"]').exists()).toBe(true);
    expect(panelStyle(wrapper)).toContain("translateY(100%)");
    wrapper.unmount();
  });

  it("transitionend 到达之前不卸载，到达之后面板与捕获层都消失", async () => {
    const wrapper = mountSheet();

    await wrapper.get('[data-role="comment-sheet-close"]').trigger("click");
    const panel = panelOf(wrapper);
    expect(panel.exists()).toBe(true);

    await panel.trigger("transitionend");
    await wrapper.vm.$nextTick();

    expect(wrapper.find('[data-role="comment-sheet"]').exists()).toBe(false);
    expect(wrapper.find('[data-role="comment-sheet-scrim"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("Esc 也把退场动画走完再卸载", async () => {
    const wrapper = mountSheet();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted("update:show")).toEqual([[false]]);
    expect(wrapper.find('[data-role="comment-sheet-panel"]').exists()).toBe(true);

    await panelOf(wrapper).trigger("transitionend");
    await wrapper.vm.$nextTick();
    expect(wrapper.find('[data-role="comment-sheet"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("点面板外也把退场动画走完再卸载", async () => {
    const wrapper = mountSheet();

    await wrapper.get('[data-role="comment-sheet-scrim"]').trigger("click");
    expect(wrapper.find('[data-role="comment-sheet-panel"]').exists()).toBe(true);

    await panelOf(wrapper).trigger("transitionend");
    await wrapper.vm.$nextTick();
    expect(wrapper.find('[data-role="comment-sheet"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("减少动态：没有退场，直接卸载（不等 transitionend）", async () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
    const wrapper = mountSheet();

    await wrapper.get('[data-role="comment-sheet-close"]').trigger("click");

    expect(wrapper.emitted("update:show")).toEqual([[false]]);
    expect(wrapper.find('[data-role="comment-sheet"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("退场途中重新打开：面板留下，随后的 transitionend 不会把它卸载", async () => {
    const wrapper = mountSheet();

    await wrapper.get('[data-role="comment-sheet-close"]').trigger("click");
    // 模拟父组件真的改了 v-model:show，然后立刻又打开。
    await wrapper.setProps({ show: false });
    await wrapper.setProps({ show: true });

    const panel = panelOf(wrapper);
    expect(panelStyle(wrapper)).not.toContain("translateY(100%)");

    await panel.trigger("transitionend");
    await wrapper.vm.$nextTick();
    expect(wrapper.find('[data-role="comment-sheet-panel"]').exists()).toBe(true);
    wrapper.unmount();
  });
});

describe("CommentComposerSheet：把手下滑关闭", () => {
  let clock = 0;

  beforeEach(() => {
    // Event.timeStamp 与组件里的 performance.now 都吃这个假时钟。
    clock = 0;
    vi.spyOn(performance, "now").mockImplementation(() => clock);
  });

  it("拖动期 1:1 跟手、没有过渡", async () => {
    const wrapper = mountSheet();
    stubPanelHeight(wrapper, 600);
    const handle = wrapper.get('[data-role="comment-sheet-handle"]').element;

    firePointer(handle, "pointerdown", 100);
    await wrapper.vm.$nextTick();
    expect(panelStyle(wrapper)).toContain("transition: none");

    firePointer(handle, "pointermove", 400);
    await wrapper.vm.$nextTick();
    expect(panelStyle(wrapper)).toContain("translateY(300px)");
    wrapper.unmount();
  });

  it("位移超过面板高度 30%：松手关闭，仍然等 transitionend 再卸载", async () => {
    const wrapper = mountSheet();
    stubPanelHeight(wrapper, 600);
    const handle = wrapper.get('[data-role="comment-sheet-handle"]').element;

    firePointer(handle, "pointerdown", 100);
    firePointer(handle, "pointermove", 400);
    clock = 600;
    firePointer(handle, "pointerup", 400);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted("update:show")).toEqual([[false]]);
    expect(wrapper.find('[data-role="comment-sheet-panel"]').exists()).toBe(true);
    expect(panelStyle(wrapper)).toContain("translateY(100%)");

    await panelOf(wrapper).trigger("transitionend");
    await wrapper.vm.$nextTick();
    expect(wrapper.find('[data-role="comment-sheet"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("位移不大但速度快（>0.11 px/ms）：同样关闭", async () => {
    const wrapper = mountSheet();
    stubPanelHeight(wrapper, 600);
    const handle = wrapper.get('[data-role="comment-sheet-handle"]').element;

    // 60px 只有 10% 高度，但 100ms 内甩完 → 0.6 px/ms。
    firePointer(handle, "pointerdown", 100);
    clock = 50;
    firePointer(handle, "pointermove", 160);
    clock = 100;
    firePointer(handle, "pointerup", 160);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted("update:show")).toEqual([[false]]);
    wrapper.unmount();
  });

  it("位移与速度都没过阈值：弹回原位（200ms），不关闭", async () => {
    const wrapper = mountSheet();
    stubPanelHeight(wrapper, 600);
    const handle = wrapper.get('[data-role="comment-sheet-handle"]').element;

    firePointer(handle, "pointerdown", 100);
    clock = 200;
    firePointer(handle, "pointermove", 160);
    clock = 1000;
    firePointer(handle, "pointerup", 160);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted("update:show")).toBeUndefined();
    expect(wrapper.find('[data-role="comment-sheet-panel"]').exists()).toBe(true);
    expect(panelStyle(wrapper)).toContain("translateY(0)");
    expect(panelStyle(wrapper)).toContain("var(--motion-base)");
    wrapper.unmount();
  });

  it("多指：第一根手指之后的触点被忽略，不会把面板拖走", async () => {
    const wrapper = mountSheet();
    stubPanelHeight(wrapper, 600);
    const handle = wrapper.get('[data-role="comment-sheet-handle"]').element;

    firePointer(handle, "pointerdown", 100, 1);
    firePointer(handle, "pointerdown", 500, 2);
    firePointer(handle, "pointermove", 900, 2);
    firePointer(handle, "pointermove", 500, 2);
    clock = 800;
    firePointer(handle, "pointerup", 100, 1);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted("update:show")).toBeUndefined();
    expect(wrapper.find('[data-role="comment-sheet-panel"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("向上拖动不产生负位移（面板不往上跑）", async () => {
    const wrapper = mountSheet();
    stubPanelHeight(wrapper, 600);
    const handle = wrapper.get('[data-role="comment-sheet-handle"]').element;

    firePointer(handle, "pointerdown", 300);
    firePointer(handle, "pointermove", 100);
    await wrapper.vm.$nextTick();
    expect(panelStyle(wrapper)).toContain("translateY(0px)");

    clock = 500;
    firePointer(handle, "pointerup", 100);
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted("update:show")).toBeUndefined();
    wrapper.unmount();
  });
});

describe("shouldDismissSheetDrag：位移 > 高度 30% 或最后 100ms 速度 > 0.11 px/ms", () => {
  /** 一次拖动的采样简写：`[时间, 位移]`。 */
  function samples(pairs: Array<[number, number]>) {
    return pairs.map(([atMs, offsetPx]) => ({ atMs, offsetPx }));
  }

  it("位移超过面板高度 30% 就关闭（不需要速度证据）", () => {
    expect(
      shouldDismissSheetDrag({ offsetPx: 181, panelHeightPx: 600, releaseAtMs: 5000, samples: [] }),
    ).toBe(true);
  });

  it("位移刚好等于 30% 不算（严格大于），且没有速度证据", () => {
    expect(
      shouldDismissSheetDrag({ offsetPx: 180, panelHeightPx: 600, releaseAtMs: 5000, samples: [] }),
    ).toBe(false);
  });

  it("位移不足但最后 100ms 甩得快就关闭", () => {
    // 慢拖 1 秒才到 100px，最后 100ms 又甩出 70px（0.7 px/ms）。
    expect(
      shouldDismissSheetDrag({
        offsetPx: 170,
        panelHeightPx: 600,
        releaseAtMs: 1100,
        samples: samples([[0, 0], [1000, 100], [1100, 170]]),
      }),
    ).toBe(true);
  });

  it("慢拖后急甩要关：全程平均速度很低，但松手瞬间在甩（评审判据）", () => {
    // 按下到松手跨 5 秒、全程平均只有 0.034 px/ms；最后 60ms 却甩出 100px。
    expect(
      shouldDismissSheetDrag({
        offsetPx: 170,
        panelHeightPx: 600,
        releaseAtMs: 5060,
        samples: samples([[0, 0], [2000, 40], [4000, 70], [5000, 70], [5060, 170]]),
      }),
    ).toBe(true);
  });

  it("快拖后停手不关：最后 100ms 没有位移，速度按 0 处理", () => {
    // 前 100ms 拖出 120px（本来很快），随后停手 700ms 再松开。
    expect(
      shouldDismissSheetDrag({
        offsetPx: 120,
        panelHeightPx: 600,
        releaseAtMs: 800,
        samples: samples([[0, 0], [100, 120], [800, 120]]),
      }),
    ).toBe(false);
  });

  it("速度刚好 0.11 px/ms 不算（严格大于）", () => {
    // 最后 100ms 内恰好 11px → 0.11 px/ms。
    expect(
      shouldDismissSheetDrag({
        offsetPx: 11,
        panelHeightPx: 600,
        releaseAtMs: 1000,
        samples: samples([[900, 0], [1000, 11]]),
      }),
    ).toBe(false);
  });

  it("面板高度未知时只按速度判定（不会因为高度 0 就一拖就关）", () => {
    expect(
      shouldDismissSheetDrag({ offsetPx: 100, panelHeightPx: 0, releaseAtMs: 5000, samples: [] }),
    ).toBe(false);
    expect(
      shouldDismissSheetDrag({
        offsetPx: 40,
        panelHeightPx: 0,
        releaseAtMs: 100,
        samples: samples([[0, 0], [100, 40]]),
      }),
    ).toBe(true);
  });

  it("窗口内没有采样时退回位移阈值（不猜速度、不除出 Infinity）", () => {
    expect(
      shouldDismissSheetDrag({ offsetPx: 40, panelHeightPx: 600, releaseAtMs: 900, samples: [] }),
    ).toBe(false);
    expect(
      shouldDismissSheetDrag({
        offsetPx: 40,
        panelHeightPx: 600,
        releaseAtMs: 0,
        samples: samples([[0, 0]]),
      }),
    ).toBe(false);
  });

  it("窗口外的旧采样不参与速度（只有松手前 100ms 内的才算）", () => {
    const velocity = sheetReleaseVelocityPxPerMs({
      offsetPx: 120,
      releaseAtMs: 800,
      samples: samples([[0, 0], [100, 120], [800, 120]]),
    });

    expect(velocity).toBe(0);
    expect(sheetReleaseVelocityPxPerMs({ offsetPx: 0, releaseAtMs: 0, samples: [] })).toBe(0);
  });
});

describe("sheetDragOffset：只跟向下的位移", () => {
  it("向下拖多远面板就下移多远", () => {
    expect(sheetDragOffset(100, 260)).toBe(160);
  });

  it("向上拖归零，不产生负位移", () => {
    expect(sheetDragOffset(300, 100)).toBe(0);
  });

  it("非法输入归零", () => {
    expect(sheetDragOffset(Number.NaN, 100)).toBe(0);
    expect(sheetDragOffset(100, Number.NaN)).toBe(0);
  });
});
