import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import JellyLauncher from "~/components/jelly/JellyLauncher.vue";
import type { JellyVisualState } from "~/composables/useJellyAssistant";

// 角色组件（JellyLauncher）的渲染冒烟。
//
// 分工是刻意的：**状态**归 useJellyAssistant（它有 33 个行为用例），这里只验证
// 外观组件把六态正确映射成 class / aria / 事件接线。所以 composable 被整个替换成
// 可控假实现 —— 不断言像素、不断言样式值，只断言「状态名 → 可观察的 DOM 契约」。
const apiRef = vi.hoisted(() => ({ current: null as unknown }));

vi.mock("~/composables/useJellyAssistant", () => ({
  useJellyAssistant: () => apiRef.current,
}));

function makeApi() {
  const api = {
    state: ref<JellyVisualState>("idle"),
    isAvailable: ref(true),
    isOpen: ref(false),
    isReducedMotion: ref(false),
    shouldAnimate: ref(true),
    shouldRender: ref(true),
    breathDurationMs: ref(4_000),
    position: ref({ x: 100, y: 120 }),
    bubbleText: ref<string | null>(null),
    hintText: ref<string | null>(null),
    isDragged: ref(false),
    activate: vi.fn(),
    open: vi.fn(),
    close: vi.fn(),
    toggle: vi.fn(),
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

  // 假实现要带真实语义：beginDrag 之后 isDragged 才是 true，否则组件无法
  // 「拖完松手不当点击」——那正是这里要验的行为。
  api.beginDrag.mockImplementation(() => {
    api.isDragged.value = true;
  });
  api.endDrag.mockImplementation(() => {
    api.isDragged.value = false;
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

  it.each<JellyVisualState>(["idle", "hover", "press", "open", "dragging", "disabled"])(
    "六态之一 %s 映射成对应的 class",
    async (state) => {
      const { api, wrapper } = setup();
      api.state.value = state;
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
});

describe("JellyLauncher · 指针与拖拽", () => {
  it("悬停/离开翻译成 composable 的指针动作", async () => {
    const { api, wrapper } = setup();

    await launcherButton(wrapper).trigger("pointerenter");
    expect(api.pointerEnter).toHaveBeenCalled();

    await launcherButton(wrapper).trigger("pointerleave");
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
