import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import JellyLauncher from "~/components/jelly/JellyLauncher.vue";
import type { JellyVisualState } from "~/composables/useJellyAssistant";

// 角色组件（JellyLauncher）的渲染冒烟。
//
// 分工是刻意的：**状态**归 useJellyAssistant（它有整组行为用例），这里只验证
// 外观组件把六态正确映射成 class / aria / 事件接线。所以 composable 被整个替换成
// 可控假实现 —— 不断言像素、不断言样式值，只断言「状态名 → 可观察的 DOM 契约」。
//
// 固定定位之后，组件**不再写任何位置**：根元素靠 CSS 钉在右下角，收起把手靠 CSS
// 钉在右边缘垂直中部。这里因此也不再有拖拽 / 落位 / 位置持久化的断言。
const apiRef = vi.hoisted(() => ({ current: null as unknown }));

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
    bubbleText: ref<string | null>(null),
    hintText: ref<string | null>(null),
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
    dismissBubble: vi.fn(),
    dismissHint: vi.fn(),
    startSession: vi.fn(),
    stopSession: vi.fn(),
  };

  // 收起 / 展开要带真实语义：组件要在状态翻转后换掉 DOM（本体 ↔ 把手），
  // 焦点交接与提示的用例只有让假实现真的翻状态才验得到。
  api.collapse.mockImplementation(() => {
    api.isCollapsed.value = true;
    api.state.value = "collapsed";
  });
  api.expand.mockImplementation(() => {
    api.isCollapsed.value = false;
    api.state.value = "idle";
  });

  return api;
}

type Api = ReturnType<typeof makeApi>;

function setup(options: { attachTo?: boolean } = {}): {
  api: Api;
  wrapper: ReturnType<typeof mount>;
  flushFrames: () => void;
} {
  const api = makeApi();
  apiRef.current = api;

  // Vue 的过渡收尾（收起 / 展开）靠两层 rAF；桩成可手动 flush 的队列，
  // 用例才能断言"动画结束后元素才被卸载"。
  const pendingFrames: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    pendingFrames.push(callback);
    return pendingFrames.length;
  });
  vi.stubGlobal("cancelAnimationFrame", () => {});

  const flushFrames = () => {
    while (pendingFrames.length > 0) {
      for (const callback of pendingFrames.splice(0)) callback(0);
    }
  };

  const wrapper = mount(JellyLauncher, {
    ...(options.attachTo ? { attachTo: document.body } : {}),
    // 收起 / 展开的可见位移靠真实 <Transition> 才会延迟卸载元素；VTU 默认把
    // transition 桩成同步替换，等于把"动画结束后才卸载"抹掉。
    global: { stubs: { transition: false } },
  });
  return { api, wrapper, flushFrames };
}

function launcherButton(wrapper: ReturnType<typeof mount>) {
  return wrapper.find("button.jelly-launcher__button");
}

describe("JellyLauncher · 渲染", () => {
  it("默认渲染一个可聚焦的按钮，aria-label 中性（不自称捏捏）并带 aria-expanded", () => {
    const { wrapper } = setup();
    const button = launcherButton(wrapper);

    expect(button.exists()).toBe(true);
    expect(button.attributes("type")).toBe("button");
    expect(button.attributes("aria-label")).toContain("AI 助手");
    expect(button.attributes("aria-label")).not.toContain("捏捏");
    expect(button.attributes("aria-expanded")).toBe("false");

    wrapper.unmount();
  });

  it.each<JellyVisualState>(["idle", "hover", "press", "open", "collapsed", "disabled"])(
    "六态之一 %s 映射成对应的 class",
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

  it("挂载时启动会话，卸载时停止", () => {
    const { api, wrapper } = setup();

    expect(api.startSession).toHaveBeenCalledTimes(1);

    wrapper.unmount();
    expect(api.stopSession).toHaveBeenCalled();
  });

  it("布局传入共享实例时优先用它（票 10 的 AI 面板必须与角色共享同一份开关）", () => {
    const shared = makeApi();
    const fallback = makeApi();
    apiRef.current = fallback;

    const wrapper = mount(JellyLauncher, { props: { assistant: shared } });

    expect(shared.startSession).toHaveBeenCalledTimes(1);
    expect(fallback.startSession).not.toHaveBeenCalled();

    wrapper.unmount();
    expect(shared.stopSession).toHaveBeenCalled();
  });

  it("根元素不再写位置：固定定位全交给 CSS，inline style 只有几何变量", () => {
    const { wrapper } = setup();
    const style = wrapper.find(".jelly-launcher").attributes("style") ?? "";

    expect(style).toContain("--jelly-size: 56px");
    expect(style).toContain("--jelly-margin: 12px");
    expect(style).toContain("--jelly-handle-hit: 44px");
    // 旧口径的左上角坐标 / 安全区让位副本彻底退场。
    expect(style).not.toContain("translate3d");
    expect(style).not.toContain("--jelly-y");
    expect(style).not.toContain("transition");

    wrapper.unmount();
  });
});

describe("JellyLauncher · 指针与按压", () => {
  it("悬停/离开翻译成 composable 的指针动作（判定挂在外层 cluster 上）", async () => {
    const { api, wrapper } = setup();
    const cluster = wrapper.find(".jelly-launcher__cluster");

    await cluster.trigger("pointerenter");
    expect(api.pointerEnter).toHaveBeenCalled();

    await cluster.trigger("pointerleave");
    expect(api.pointerLeave).toHaveBeenCalled();

    wrapper.unmount();
  });

  it("按下再松开（没拖动）= 先捏再触发：只把事件翻译成按压与松手", async () => {
    const { api, wrapper } = setup();
    const element = launcherButton(wrapper).element;

    element.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }));
    expect(api.pointerDown).toHaveBeenCalled();

    element.dispatchEvent(new MouseEvent("pointerup", { bubbles: true }));
    expect(api.pointerUp).toHaveBeenCalled();

    wrapper.unmount();
  });

  it("拖动退场：指针移动既不写位置、也不触发任何拖拽动作", async () => {
    const { api, wrapper } = setup();
    const element = launcherButton(wrapper).element;
    const styleBefore = wrapper.find(".jelly-launcher").attributes("style");

    element.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }));
    element.dispatchEvent(new MouseEvent("pointermove", { bubbles: true, clientX: 220, clientY: 240 }));

    expect(api.pointerDown).toHaveBeenCalled();
    expect(wrapper.find(".jelly-launcher").attributes("style")).toBe(styleBefore);
    expect(api).not.toHaveProperty("beginDrag");
    expect(api).not.toHaveProperty("moveTo");

    wrapper.unmount();
  });

  it("粗指针长按 500ms 唤出收起 chip；提前松手则取消", async () => {
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
    vi.useFakeTimers();

    const hold = setup();
    launcherButton(hold.wrapper).element.dispatchEvent(
      new MouseEvent("pointerdown", { bubbles: true }),
    );
    vi.advanceTimersByTime(500);
    expect(hold.api.pinCollapseChip).toHaveBeenCalledTimes(1);
    hold.wrapper.unmount();

    const tap = setup();
    launcherButton(tap.wrapper).element.dispatchEvent(
      new MouseEvent("pointerdown", { bubbles: true }),
    );
    vi.advanceTimersByTime(300);
    launcherButton(tap.wrapper).element.dispatchEvent(
      new MouseEvent("pointerup", { bubbles: true }),
    );
    vi.advanceTimersByTime(500);
    expect(tap.api.pinCollapseChip).not.toHaveBeenCalled();
    tap.wrapper.unmount();
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

  it("状态提示以 role=status 呈现（读屏能读，不弹出失败面板）", async () => {
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

describe("JellyLauncher · 收进窗口右边缘（抽屉核心）", () => {
  function collapseChip(wrapper: ReturnType<typeof mount>) {
    return wrapper.find("button.jelly-collapse-chip");
  }

  it("收起态渲染贴右边缘的把手而不是捏捏本体（收起动画结束后本体才卸载）", async () => {
    const { api, wrapper, flushFrames } = setup();

    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    await wrapper.vm.$nextTick();

    // 收起是可见位移：刚翻状态时本体还在（带 leave 过渡），把手已经叠上来。
    expect(wrapper.find("button.jelly-launcher__button").exists()).toBe(true);
    flushFrames();
    await wrapper.vm.$nextTick();

    expect(wrapper.classes()).toContain("jelly-launcher--collapsed");
    expect(wrapper.find("button.jelly-launcher__button").exists()).toBe(false);

    const handle = wrapper.find("button.jelly-launcher__handle");
    expect(handle.exists()).toBe(true);
    expect(handle.attributes("type")).toBe("button");
    expect(handle.attributes("aria-label")).toBe("展开 AI 助手");

    wrapper.unmount();
  });

  it("把手由 CSS 钉在右边缘垂直中部：组件不写 translate3d 位置", async () => {
    const { api, wrapper, flushFrames } = setup();
    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    await wrapper.vm.$nextTick();
    flushFrames();
    await wrapper.vm.$nextTick();

    const rootStyle = wrapper.find(".jelly-launcher").attributes("style") ?? "";
    expect(rootStyle).not.toContain("translate3d");
    expect(wrapper.find("button.jelly-launcher__handle").exists()).toBe(true);

    wrapper.unmount();
  });

  it("点把手调用 expand()（回到固定右下角）", async () => {
    const { api, wrapper, flushFrames } = setup();
    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    await wrapper.vm.$nextTick();
    flushFrames();
    await wrapper.vm.$nextTick();

    await wrapper.find("button.jelly-launcher__handle").trigger("click");

    expect(api.expand).toHaveBeenCalledTimes(1);
    expect(api.collapse).not.toHaveBeenCalled();

    wrapper.unmount();
  });

  it("收起态的把手是原生按钮：可 Tab 到，回车展开，焦点交还给本体（票 426）", async () => {
    const { api, wrapper, flushFrames } = setup({ attachTo: true });
    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    await wrapper.vm.$nextTick();
    flushFrames();
    await wrapper.vm.$nextTick();

    const handle = wrapper.find("button.jelly-launcher__handle");
    expect(handle.element.tagName).toBe("BUTTON");
    // 没有被 tabindex 踢出 Tab 序列 —— 收起来之后键盘路径仍然完整。
    expect(handle.attributes("tabindex")).toBeUndefined();

    (handle.element as HTMLButtonElement).focus();
    expect(document.activeElement).toBe(handle.element);

    await handle.trigger("keydown", { key: "Enter" });
    await wrapper.vm.$nextTick();
    await wrapper.vm.$nextTick();

    expect(api.expand).toHaveBeenCalledTimes(1);
    // 把手已经被本体替换：焦点要跟着回去，不能掉到 body。
    expect(document.activeElement).toBe(launcherButton(wrapper).element);

    wrapper.unmount();
  });

  it("收起期间不渲染气泡，新短句提示仍然走 role=status", async () => {
    const { api, wrapper, flushFrames } = setup();
    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    api.bubbleText.value = "捏我一下，问我点什么吧。";
    api.hintText.value = "我先躲起来";
    await wrapper.vm.$nextTick();
    flushFrames();
    await wrapper.vm.$nextTick();

    expect(wrapper.find("button.jelly-bubble").exists()).toBe(false);

    const hint = wrapper.find(".jelly-hint");
    expect(hint.exists()).toBe(true);
    expect(hint.attributes("role")).toBe("status");
    expect(hint.text()).toContain("我先躲起来");
    expect(hint.text()).not.toContain("捏捏");

    wrapper.unmount();
  });

  it("悬停捏捏时渲染右上角的收起 chip（aria-label 中性）", async () => {
    const { api, wrapper } = setup();
    api.state.value = "hover";
    await wrapper.vm.$nextTick();

    const chip = collapseChip(wrapper);
    expect(chip.exists()).toBe(true);
    expect(chip.attributes("type")).toBe("button");
    expect(chip.attributes("aria-label")).toBe("收起 AI 助手");

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

  it("捏捏真的拿到焦点（focus 冒泡到 cluster）就出现 chip，不需要先悬停（票 426）", async () => {
    const { wrapper } = setup({ attachTo: true });

    launcherButton(wrapper).element.focus();
    await wrapper.vm.$nextTick();

    expect(collapseChip(wrapper).exists()).toBe(true);

    launcherButton(wrapper).element.blur();
    await wrapper.vm.$nextTick();
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

  it("键盘触发收起后焦点交给把手；指针点击不抢焦点（票 423）", async () => {
    const keyboard = setup({ attachTo: true });
    keyboard.api.state.value = "hover";
    await keyboard.wrapper.vm.$nextTick();
    const chip = collapseChip(keyboard.wrapper);
    (chip.element as HTMLButtonElement).focus();
    // 键盘合成的 click 没有指针来源：detail === 0。
    chip.element.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 0 }));
    await keyboard.wrapper.vm.$nextTick();

    expect(keyboard.api.collapse).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(
      keyboard.wrapper.find("button.jelly-launcher__handle").element,
    );
    keyboard.wrapper.unmount();

    const pointer = setup({ attachTo: true });
    pointer.api.state.value = "hover";
    await pointer.wrapper.vm.$nextTick();
    const pointerChip = collapseChip(pointer.wrapper);
    pointerChip.element.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1 }));
    await pointer.wrapper.vm.$nextTick();

    expect(pointer.api.collapse).toHaveBeenCalledTimes(1);
    // 指针用户点完收起后不该被抢焦点（焦点随 chip 卸载离开即可）。
    expect(document.activeElement).not.toBe(
      pointer.wrapper.find("button.jelly-launcher__handle").element,
    );
    pointer.wrapper.unmount();
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

  it("焦点在 chip 上按 Esc：chip 收起且焦点回到捏捏，不掉到 body（票 426）", async () => {
    const { api, wrapper } = setup({ attachTo: true });
    api.state.value = "hover";
    await wrapper.vm.$nextTick();

    const chip = collapseChip(wrapper);
    expect(chip.exists()).toBe(true);
    (chip.element as HTMLButtonElement).focus();
    expect(document.activeElement).toBe(chip.element);

    await chip.trigger("keydown", { key: "Escape" });
    await wrapper.vm.$nextTick();

    expect(api.dismissCollapseChip).toHaveBeenCalledTimes(1);
    expect(collapseChip(wrapper).exists()).toBe(false);
    expect(document.activeElement).toBe(launcherButton(wrapper).element);

    wrapper.unmount();
  });

  it("收起态下把手只负责展开：pointerdown 不进入按压 / 拖拽动作", async () => {
    const { api, wrapper, flushFrames } = setup();
    api.isCollapsed.value = true;
    api.state.value = "collapsed";
    await wrapper.vm.$nextTick();
    flushFrames();
    await wrapper.vm.$nextTick();

    const handle = wrapper.find("button.jelly-launcher__handle").element;
    handle.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }));

    expect(api.pointerDown).not.toHaveBeenCalled();
    expect(api).not.toHaveProperty("beginDrag");
    expect(api).not.toHaveProperty("moveTo");

    wrapper.unmount();
  });
});
