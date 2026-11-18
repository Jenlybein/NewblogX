import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import JellyLauncher from "~/components/jelly/JellyLauncher.vue";
import type { JellyVisualState } from "~/composables/useJellyAssistant";

// 角色组件（JellyLauncher）的渲染冒烟。
//
// 分工是刻意的：**状态**归 useJellyAssistant（它有 59 个行为用例），这里只验证
// 外观组件把七态正确映射成 class / aria / 事件接线。所以 composable 被整个替换成
// 可控假实现 —— 不断言像素、不断言样式值，只断言「状态名 → 可观察的 DOM 契约」。
const apiRef = vi.hoisted(() => ({ current: null as unknown }));

// 只用真实模块里的常量/钳制函数（组件要靠它们算视口边界），把 composable 本体换成可控假实现。
vi.mock("~/composables/useJellyAssistant", async (importOriginal) => {
  const actual = await importOriginal<typeof import("~/composables/useJellyAssistant")>();

  return { ...actual, useJellyAssistant: () => apiRef.current };
});

function makeApi() {
  const api = {
    state: ref<JellyVisualState>("idle"),
    isAvailable: ref(true),
    isOpen: ref(false),
    isCollapsed: ref(false),
    isChipPinned: ref(false),
    isReducedMotion: ref(false),
    shouldAnimate: ref(true),
    shouldRender: ref(true),
    breathDurationMs: ref(4_000),
    position: ref({ x: 100, y: 120 }),
    collapsedPosition: ref({ x: 980, y: 120 }),
    bubbleText: ref<string | null>(null),
    hintText: ref<string | null>(null),
    isDragged: ref(false),
    activate: vi.fn(),
    open: vi.fn(),
    close: vi.fn(),
    toggle: vi.fn(),
    collapse: vi.fn(),
    expand: vi.fn(),
    pinCollapseChip: vi.fn(),
    dismissCollapseChip: vi.fn(),
    pointerEnter: vi.fn(),
    pointerLeave: vi.fn(),
    pointerDown: vi.fn(),
    pointerUp: vi.fn(),
    beginDrag: vi.fn(),
    endDrag: vi.fn(),
    moveTo: vi.fn(),
    handleViewportResize: vi.fn(),
    dismissBubble: vi.fn(),
    dismissHint: vi.fn(),
    startSession: vi.fn(),
    stopSession: vi.fn(),
  };


  // 假实现要带真实语义：beginDrag 之后 isDragged 与 state 都要变，否则组件无法
  // 「拖完松手不当点击」/「拖拽期不挂过渡」——那正是这里要验的行为。
  api.beginDrag.mockImplementation(() => {
    api.isDragged.value = true;
    api.state.value = "dragging";
  });
  api.endDrag.mockImplementation(() => {
    api.isDragged.value = false;
    api.state.value = "idle";
  });

  return api;
}

type Api = ReturnType<typeof makeApi>;

function setup(): { api: Api; wrapper: ReturnType<typeof mount> } {
  const api = makeApi();
  apiRef.current = api;
  const wrapper = mount(JellyLauncher);
  return { api, wrapper };
}

function launcherButton(wrapper: ReturnType<typeof mount>) {
  return wrapper.find("button.jelly-launcher__button");
}

describe("JellyLauncher · 渲染", () => {
  it("默认渲染一个可聚焦的按钮，带 aria-label 与 aria-expanded", () => {
    const { wrapper } = setup();
    const button = launcherButton(wrapper);

    expect(button.exists()).toBe(true);
    expect(button.attributes("type")).toBe("button");
    expect(button.attributes("aria-label")).toContain("捏捏");
    expect(button.attributes("aria-expanded")).toBe("false");

    wrapper.unmount();
  });

  it.each<JellyVisualState>(["idle", "hover", "press", "open", "collapsed", "dragging", "disabled"])(
    "七态之一 %s 映射成对应的 class",
    async (state) => {
      const { api, wrapper } = setup();
      api.state.value = state;
      api.isCollapsed.value = state === "collapsed";
      await wrapper.vm.$nextTick();
      expect(wrapper.classes()).toContain(`jelly-launcher--${state}`);

      wrapper.unmount();
    },
  );

  it("面板打开时 aria-expanded 为 true，关闭为 false", async () => {
    const { api, wrapper } = setup();

    api.isOpen.value = true;
    await wrapper.vm.$nextTick();
    expect(launcherButton(wrapper).attributes("aria-expanded")).toBe("true");

    api.isOpen.value = false;
    await wrapper.vm.$nextTick();
    expect(launcherButton(wrapper).attributes("aria-expanded")).toBe("false");

    wrapper.unmount();
  });

  it("AI 关闭时 aria-label 说明当前不可用（不静默）", async () => {
    const { api, wrapper } = setup();

    api.isAvailable.value = false;
    api.state.value = "disabled";
    await wrapper.vm.$nextTick();

    expect(launcherButton(wrapper).attributes("aria-label")).toContain("关闭");

    wrapper.unmount();
  });

  it("减少动态时挂静态 class，且呼吸时长归零", async () => {
    const { api, wrapper } = setup();

    api.isReducedMotion.value = true;
    api.breathDurationMs.value = 0;
    await wrapper.vm.$nextTick();

    expect(wrapper.classes()).toContain("jelly-launcher--reduced");
    expect(wrapper.find(".jelly-launcher__body").attributes("style")).toContain("animation-duration: 0ms");

    wrapper.unmount();
  });

  it("创作区路由下不渲染（shouldRender=false 时整个角色消失）", async () => {
    const { api, wrapper } = setup();
    api.shouldRender.value = false;
    await wrapper.vm.$nextTick();

    expect(launcherButton(wrapper).exists()).toBe(false);

    wrapper.unmount();
  });

  it("挂载时启动会话（位置恢复 + 首次气泡），卸载时停止", () => {
    const { api, wrapper } = setup();

    expect(api.startSession).toHaveBeenCalledTimes(1);

    wrapper.unmount();
    expect(api.stopSession).toHaveBeenCalled();
  });

  it("布局传入共享实例时优先用它（#10 的 AI 面板必须与角色共享同一份开关）", () => {
    const shared = makeApi();
    const fallback = makeApi();
    apiRef.current = fallback;

    const wrapper = mount(JellyLauncher, { props: { assistant: shared } });

    expect(shared.startSession).toHaveBeenCalledTimes(1);
    expect(fallback.startSession).not.toHaveBeenCalled();

    wrapper.unmount();
    expect(shared.stopSession).toHaveBeenCalled();
  });
});

describe("JellyLauncher · 指针与拖拽", () => {
  it("悬停/离开翻译成 composable 的指针动作（判定挂在外层 cluster 上）", async () => {
    const { api, wrapper } = setup();
    const cluster = wrapper.find(".jelly-launcher__cluster");

    await cluster.trigger("pointerenter");
    expect(api.pointerEnter).toHaveBeenCalled();

    await cluster.trigger("pointerleave");
    expect(api.pointerLeave).toHaveBeenCalled();

    wrapper.unmount();
  });

  it("按下再松开（没拖动）= 先捏再触发", async () => {
    const { api, wrapper } = setup();
    const button = launcherButton(wrapper);
    const element = button.element;

    element.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, clientX: 120, clientY: 140 }));
    expect(api.pointerDown).toHaveBeenCalled();

    window.dispatchEvent(new MouseEvent("pointerup", { bubbles: true, clientX: 120, clientY: 140 }));
    expect(api.pointerUp).toHaveBeenCalled();
    expect(api.endDrag).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it("拖过阈值进入 dragging，并持续跟随；松手不触发点击、会持久化位置", async () => {
    const { api, wrapper } = setup();
    const element = launcherButton(wrapper).element;

    element.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, clientX: 120, clientY: 140 }));
    window.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 220, clientY: 240 }));

    expect(api.beginDrag).toHaveBeenCalled();
    expect(api.moveTo).toHaveBeenCalled();

    window.dispatchEvent(new MouseEvent("pointerup", { bubbles: true, clientX: 220, clientY: 240 }));
    expect(api.endDrag).toHaveBeenCalled();
    expect(api.activate).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it("微小抖动不算拖拽（否则一次点击会被判成拖动）", async () => {
    const { api, wrapper } = setup();
    const element = launcherButton(wrapper).element;

    element.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, clientX: 120, clientY: 140 }));
    window.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 122, clientY: 142 }));
    window.dispatchEvent(new MouseEvent("pointerup", { bubbles: true, clientX: 122, clientY: 142 }));

    expect(api.beginDrag).not.toHaveBeenCalled();

    wrapper.unmount();
  });
});

describe("JellyLauncher · 键盘", () => {
  it("回车触发（先捏再开），空格按下/松开同样触发", async () => {
    const { api, wrapper } = setup();
    const button = launcherButton(wrapper);

    await button.trigger("keydown", { key: "Enter" });
    expect(api.pointerDown).toHaveBeenCalled();
    expect(api.activate).toHaveBeenCalled();

    await button.trigger("keydown", { key: " " });
    expect(api.pointerDown).toHaveBeenCalledTimes(2);

    await button.trigger("keyup", { key: " " });
    expect(api.pointerUp).toHaveBeenCalled();

    wrapper.unmount();
  });
});

describe("JellyLauncher · 气泡与提示", () => {
  it("气泡文案渲染出来，点击即关", async () => {
    const { api, wrapper } = setup();

    api.bubbleText.value = "捏我一下，问我点什么吧。";
    await wrapper.vm.$nextTick();

    const bubble = wrapper.find("button.jelly-bubble");
    expect(bubble.exists()).toBe(true);
    expect(bubble.text()).toContain("捏我一下");

    await bubble.trigger("click");
    expect(api.dismissBubble).toHaveBeenCalled();

    wrapper.unmount();
  });

  it("不可用提示以 role=status 呈现（读屏能读，不弹出失败面板）", async () => {
    const { api, wrapper } = setup();

    api.hintText.value = "AI 还开着小差";
    await wrapper.vm.$nextTick();

    const hint = wrapper.find(".jelly-hint");
    expect(hint.exists()).toBe(true);
    expect(hint.attributes("role")).toBe("status");
    expect(hint.text()).toContain("AI 还开着小差");

    wrapper.unmount();
  });
});

/** 一帧的名义时长（毫秒）：用例自己推进 rAF，不依赖真实时间。 */
const FRAME_MS = 16;

/**
 * 可手动推进的动画帧队列。
 *
 * 落位是逐帧直写的（不是 CSS 过渡），所以"动画有没有动、动完有没有停"只能从帧调度上看：
 * 把 requestAnimationFrame 换成"先存起来、由用例决定何时走一帧"，断言就完全不依赖真实时间。
 */
function stubAnimationFrames() {
  let nextId = 0;
  let timestamp = 0;
  const pending = new Map<number, FrameRequestCallback>();

  vi.stubGlobal(
    "requestAnimationFrame",
    vi.fn((callback: FrameRequestCallback) => {
      nextId += 1;
      pending.set(nextId, callback);
      return nextId;
    }),
  );
  vi.stubGlobal(
    "cancelAnimationFrame",
    vi.fn((id: number) => {
      pending.delete(id);
    }),
  );

  return {
    pending: () => pending.size,
    /** 走一帧：执行当前挂起的回调并推进名义时间。 */
    step() {
      timestamp += FRAME_MS;
      const callbacks = [...pending.values()];
      pending.clear();
      for (const callback of callbacks) callback(timestamp);
    },
  };
}

/** 根元素当前的实际位移（位置 + 落位偏移）；解析不了就判红，别让断言静默失效。 */
function rootOffset(wrapper: ReturnType<typeof mount>): { x: number; y: number } {
  const style = wrapper.find(".jelly-launcher").attributes("style") ?? "";
  const match = style.match(/translate3d\((-?[\d.]+)px,\s*(-?[\d.]+)px,\s*0\)/u);
  if (!match) throw new Error(`根元素没有 translate3d：${style}`);

  return { x: Number(match[1]), y: Number(match[2]) };
}

/** 把角色从 (120,140) 拖到 (220,240) 并在同一位置松手（相对角色左上角偏移 20px）。 */
function dragAndRelease(wrapper: ReturnType<typeof mount>) {
  const element = launcherButton(wrapper).element;
  element.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, clientX: 120, clientY: 140 }));
  window.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 170, clientY: 190 }));
  window.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 220, clientY: 240 }));
  window.dispatchEvent(new MouseEvent("pointerup", { bubbles: true, clientX: 220, clientY: 240 }));
}

describe("JellyLauncher · 收进窗口右边缘（抽屉核心）", () => {
  /** `(pointer: coarse)` 长按唤出只在触摸设备上生效，用例按需把这一档打开。 */
  function stubCoarsePointer() {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === "(pointer: coarse)",
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
  }

  function collapseChip(wrapper: ReturnType<typeof mount>) {
    return wrapper.find("button.jelly-collapse-chip");
  }

  it("收起态渲染贴右边缘的把手而不是捏捏本体（把手 aria-label 为「展开捏捏」）", async () => {
    const { api, wrapper } = setup();

    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    await wrapper.vm.$nextTick();

    expect(wrapper.classes()).toContain("jelly-launcher--collapsed");
    expect(wrapper.find("button.jelly-launcher__button").exists()).toBe(false);

    const handle = wrapper.find("button.jelly-launcher__handle");
    expect(handle.exists()).toBe(true);
    expect(handle.attributes("type")).toBe("button");
    expect(handle.attributes("aria-label")).toBe("展开捏捏");

    // 位置来自 composable 的派生坐标（贴右边缘），组件不自己算。
    expect(wrapper.find(".jelly-launcher").attributes("style")).toContain(
      `translate3d(${api.collapsedPosition.value.x}px, ${api.collapsedPosition.value.y}px, 0)`,
    );

    wrapper.unmount();
  });

  it("点把手调用 expand()（回到收起前的自由位置）", async () => {
    const { api, wrapper } = setup();
    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    await wrapper.vm.$nextTick();

    await wrapper.find("button.jelly-launcher__handle").trigger("click");

    expect(api.expand).toHaveBeenCalledTimes(1);
    expect(api.collapse).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it("收起期间不渲染气泡，状态提示仍然走 role=status", async () => {
    const { api, wrapper } = setup();
    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    api.bubbleText.value = "捏我一下，问我点什么吧。";
    api.hintText.value = "捏捏收进右边缘了，点把手叫它出来";
    await wrapper.vm.$nextTick();

    expect(wrapper.find("button.jelly-bubble").exists()).toBe(false);

    const hint = wrapper.find(".jelly-hint");
    expect(hint.exists()).toBe(true);
    expect(hint.attributes("role")).toBe("status");
    expect(hint.text()).toContain("收进右边缘");

    wrapper.unmount();
  });

  it("悬停捏捏时渲染右上角的收起 chip", async () => {
    const { api, wrapper } = setup();
    api.state.value = "hover";
    await wrapper.vm.$nextTick();

    const chip = collapseChip(wrapper);
    expect(chip.exists()).toBe(true);
    expect(chip.attributes("type")).toBe("button");
    expect(chip.attributes("aria-label")).toBe("收起捏捏");

    wrapper.unmount();
  });

  it("键盘焦点落在捏捏（cluster focus-within）时也渲染 chip，离开焦点后消失", async () => {
    const { wrapper } = setup();
    const cluster = wrapper.find(".jelly-launcher__cluster");

    await cluster.trigger("focusin");
    expect(collapseChip(wrapper).exists()).toBe(true);

    await cluster.trigger("focusout");
    expect(collapseChip(wrapper).exists()).toBe(false);

    wrapper.unmount();
  });

  it("长按唤出（isChipPinned）时 chip 常显，直到被 dismiss", async () => {
    const { api, wrapper } = setup();

    api.isChipPinned.value = true;
    await wrapper.vm.$nextTick();
    expect(collapseChip(wrapper).exists()).toBe(true);

    api.isChipPinned.value = false;
    await wrapper.vm.$nextTick();
    expect(collapseChip(wrapper).exists()).toBe(false);

    wrapper.unmount();
  });

  it("面板开着或已经收起时 chip 不出现（此时捏捏是面板把手 / 只剩边缘把手）", async () => {
    const { api, wrapper } = setup();

    api.state.value = "hover";
    api.isOpen.value = true;
    await wrapper.vm.$nextTick();
    expect(collapseChip(wrapper).exists()).toBe(false);

    api.isOpen.value = false;
    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    await wrapper.vm.$nextTick();
    expect(collapseChip(wrapper).exists()).toBe(false);

    wrapper.unmount();
  });

  it("chip 在 Tab 序列里紧跟捏捏，点击调用 collapse()", async () => {
    const { api, wrapper } = setup();
    api.state.value = "hover";
    await wrapper.vm.$nextTick();

    const buttons = wrapper.findAll(".jelly-launcher__cluster > button");
    expect(buttons[0].classes()).toContain("jelly-launcher__button");
    expect(buttons[1].classes()).toContain("jelly-collapse-chip");

    await buttons[1].trigger("click");
    expect(api.collapse).toHaveBeenCalledTimes(1);

    wrapper.unmount();
  });

  it("按 Esc 收掉 chip（调用 dismissCollapseChip，焦点留在捏捏上）", async () => {
    const { api, wrapper } = setup();
    api.state.value = "hover";
    await wrapper.vm.$nextTick();

    await launcherButton(wrapper).trigger("keydown", { key: "Escape" });

    expect(api.dismissCollapseChip).toHaveBeenCalledTimes(1);
    expect(api.collapse).not.toHaveBeenCalled();
    // 悬停条件还在，但 Esc 之后 chip 不能立刻再弹回来（指针离开 / 重新进入才复位）。
    expect(collapseChip(wrapper).exists()).toBe(false);

    wrapper.unmount();
  });

  it("收起态下 pointerdown 不接受拖拽（收起是显式动作，展开是把手的事）", async () => {
    const { api, wrapper } = setup();
    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    await wrapper.vm.$nextTick();

    const handle = wrapper.find("button.jelly-launcher__handle").element;
    handle.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, clientX: 1_000, clientY: 300 }));
    window.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 800, clientY: 300 }));

    expect(api.pointerDown).not.toHaveBeenCalled();
    expect(api.beginDrag).not.toHaveBeenCalled();
    expect(api.moveTo).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it("粗指针长按 500ms 唤出 chip；位移超过拖拽阈值则取消这次长按", async () => {
    stubCoarsePointer();
    vi.useFakeTimers();
    const { api, wrapper } = setup();

    launcherButton(wrapper).element.dispatchEvent(
      new MouseEvent("pointerdown", { bubbles: true, clientX: 120, clientY: 140 }),
    );
    vi.advanceTimersByTime(500);
    expect(api.pinCollapseChip).toHaveBeenCalledTimes(1);

    wrapper.unmount();

    // 第二条：按下后开始拖拽（位移 > 6px），长按计时必须被取消。
    const { api: draggedApi, wrapper: draggedWrapper } = setup();
    launcherButton(draggedWrapper).element.dispatchEvent(
      new MouseEvent("pointerdown", { bubbles: true, clientX: 120, clientY: 140 }),
    );
    window.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 220, clientY: 240 }));
    vi.advanceTimersByTime(1_000);

    expect(draggedApi.pinCollapseChip).not.toHaveBeenCalled();

    draggedWrapper.unmount();
  });

  it("收起 / 展开只动 transform 与 opacity 的 transition（可中途反向，不用 keyframes）", async () => {
    const { wrapper } = setup();

    const style = wrapper.find(".jelly-launcher").attributes("style") ?? "";
    expect(style).toContain("transition: transform var(--motion-base)");
    expect(style).toContain("opacity var(--motion-base)");
    expect(style).not.toContain("animation");

    wrapper.unmount();
  });
});

describe("JellyLauncher · 拖拽跟手与松手弹簧落位", () => {
  it("拖拽期位置直写、根元素不参与过渡（指针与角色 1:1）", async () => {
    const { api, wrapper } = setup();
    const element = launcherButton(wrapper).element;

    element.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, clientX: 120, clientY: 140 }));
    window.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 220, clientY: 240 }));
    await wrapper.vm.$nextTick();

    // 位置要么直写（拖拽）、要么逐帧弹簧（松手落位），CSS 过渡一律不参与。
    expect(wrapper.find(".jelly-launcher").attributes("style")).toContain("transition: none");

    // 按下点在角色左上角右侧 20px：指针走 100px，位置也走 100px，中间没有补间。
    expect(api.beginDrag).toHaveBeenCalled();
    expect(api.moveTo).toHaveBeenNthCalledWith(1, { x: 200, y: 220 });

    window.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 260, clientY: 300 }));
    expect(api.moveTo).toHaveBeenNthCalledWith(2, { x: 240, y: 280 });

    wrapper.unmount();
  });

  it("松手后按登记弹簧把余劲走完：先顺甩动方向滑一小段，再回到松手处", async () => {
    const frames = stubAnimationFrames();
    const { wrapper } = setup();

    dragAndRelease(wrapper);

    // 松手立刻进入落位（帧循环启动），角色不再硬停在原地。
    expect(frames.pending()).toBe(1);
    const released = rootOffset(wrapper);

    frames.step();
    await wrapper.vm.$nextTick();
    const firstFrame = rootOffset(wrapper);
    expect(firstFrame.x).toBeGreaterThan(released.x);
    expect(firstFrame.y).toBeGreaterThan(released.y);

    // 落位会自己停：不留下永远在跑的帧循环，而且最终回到松手处（不漂移）。
    let used = 1;
    while (frames.pending() > 0 && used < 120) {
      frames.step();
      await wrapper.vm.$nextTick();
      used += 1;
    }

    expect(frames.pending()).toBe(0);
    expect(used).toBeLessThan(120);
    expect(rootOffset(wrapper)).toEqual(released);

    wrapper.unmount();
  });

  it("视口边界即时钳制：贴着右下角向外甩也不会越界", async () => {
    const frames = stubAnimationFrames();
    const { api, wrapper } = setup();

    // 1024×768 视口里右下角的合法位置就是 (956, 700)。
    api.position.value = { x: 956, y: 700 };
    await wrapper.vm.$nextTick();

    const element = launcherButton(wrapper).element;
    element.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, clientX: 976, clientY: 720 }));
    window.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 1_060, clientY: 800 }));
    window.dispatchEvent(new MouseEvent("pointerup", { bubbles: true, clientX: 1_060, clientY: 800 }));

    expect(frames.pending()).toBe(1);

    let guard = 0;
    while (frames.pending() > 0 && guard < 120) {
      frames.step();
      await wrapper.vm.$nextTick();
      const offset = rootOffset(wrapper);
      expect(offset.x).toBeLessThanOrEqual(956);
      expect(offset.y).toBeLessThanOrEqual(700);
      guard += 1;
    }

    expect(rootOffset(wrapper)).toEqual({ x: 956, y: 700 });

    wrapper.unmount();
  });

  it("减少动态时松手直接落位：走既有弹簧解析的落位档，不启动弹簧帧循环", async () => {
    const frames = stubAnimationFrames();
    const { api, wrapper } = setup();

    api.isReducedMotion.value = true;
    await wrapper.vm.$nextTick();

    dragAndRelease(wrapper);

    expect(frames.pending()).toBe(0);
    expect(rootOffset(wrapper)).toEqual({ x: 100, y: 120 });

    wrapper.unmount();
  });

  it("没有拖拽的点击不启动落位（只有「拖着甩出去」才需要收余劲）", async () => {
    const frames = stubAnimationFrames();
    const { wrapper } = setup();
    const element = launcherButton(wrapper).element;

    element.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true, clientX: 120, clientY: 140 }));
    window.dispatchEvent(new MouseEvent("pointerup", { bubbles: true, clientX: 121, clientY: 141 }));

    expect(frames.pending()).toBe(0);

    wrapper.unmount();
  });

  it("落位途中再次按下：立刻接管，不留残余动画帧", async () => {
    const frames = stubAnimationFrames();
    const { wrapper } = setup();

    dragAndRelease(wrapper);
    frames.step();
    await wrapper.vm.$nextTick();
    expect(rootOffset(wrapper)).not.toEqual({ x: 100, y: 120 });

    launcherButton(wrapper).element.dispatchEvent(
      new MouseEvent("pointerdown", { bubbles: true, clientX: 500, clientY: 500 }),
    );
    await wrapper.vm.$nextTick();

    expect(frames.pending()).toBe(0);
    expect(rootOffset(wrapper)).toEqual({ x: 100, y: 120 });

    wrapper.unmount();
  });

  it("卸载时停掉落位帧循环（不留下一直在跑的动画）", () => {
    const frames = stubAnimationFrames();
    const { wrapper } = setup();

    dragAndRelease(wrapper);
    expect(frames.pending()).toBe(1);

    wrapper.unmount();
    expect(frames.pending()).toBe(0);
  });
});
