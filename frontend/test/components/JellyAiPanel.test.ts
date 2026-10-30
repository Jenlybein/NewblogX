import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { computed, defineComponent, h, nextTick, ref } from "vue";

import JellyAiPanel from "~/components/jelly/JellyAiPanel.vue";
import type { JellyAssistantApi } from "~/composables/useJellyAssistant";
import type { JellyChatMessage } from "~/composables/useJellyPanel";

// 面板外观组件（JellyAiPanel）的渲染冒烟。
//
// 状态与流式行为归 useJellyPanel（那边 19 个用例），这里只验证「状态 → DOM 契约」：
// 非遮罩、空态快捷问句填进输入框、流式光标/停止按钮/错误态、手机抽屉、关闭入口。
// motion-v 与 Markdown 渲染在单测里用桩替换（动画框架/行内渲染不是这里要测的东西），
// 但 AnimatePresence 的存在与 role/aria 是真实渲染出来的。

const panelMock = vi.hoisted(() => ({ current: null as unknown }));

vi.mock("~/composables/useJellyPanel", () => ({
  useJellyPanel: () => panelMock.current,
}));

vi.mock("motion-v", () => {
  const Passthrough = (name: string) =>
    defineComponent({
      name,
      inheritAttrs: true,
      setup(_, { slots }) {
        return () => h("div", slots.default?.());
      },
    });

  return {
    __esModule: true,
    AnimatePresence: Passthrough("AnimatePresence"),
    Motion: Passthrough("Motion"),
  };
});

vi.mock("~/components/common/MarkdownRenderSurface.vue", () => ({
  __esModule: true,
  default: defineComponent({
    name: "MarkdownRenderSurfaceMock",
    props: { source: { type: String, default: "" } },
    setup(props) {
      return () => h("div", { class: "md-surface" }, props.source);
    },
  }),
}));

interface FakePanel {
  isOpen: ReturnType<typeof ref<boolean>>;
  isAvailable: ReturnType<typeof ref<boolean>>;
  messages: ReturnType<typeof ref<JellyChatMessage[]>>;
  draft: ReturnType<typeof ref<string>>;
  pending: ReturnType<typeof ref<boolean>>;
  canSend: ReturnType<typeof computed<boolean>>;
  showEmptyState: ReturnType<typeof computed<boolean>>;
  quickQuestions: readonly string[];
  send: ReturnType<typeof vi.fn>;
  stop: ReturnType<typeof vi.fn>;
  clear: ReturnType<typeof vi.fn>;
  fillDraft: ReturnType<typeof vi.fn>;
  openPanel: ReturnType<typeof vi.fn>;
  closePanel: ReturnType<typeof vi.fn>;
  togglePanel: ReturnType<typeof vi.fn>;
  handleComposerKeydown: ReturnType<typeof vi.fn>;
}

function makePanel(overrides: Partial<FakePanel> = {}): FakePanel {
  const panel: FakePanel = {
    isOpen: ref(true),
    isAvailable: ref(true),
    messages: ref<JellyChatMessage[]>([]),
    draft: ref(""),
    pending: ref(false),
    canSend: computed(() => Boolean(panel.draft.value.trim()) && !panel.pending.value),
    showEmptyState: computed(() => panel.messages.value.length === 0),
    quickQuestions: [
      "帮我找几篇讲 Go 并发的文章",
      "这篇长文的核心观点是什么？",
      "帮我把这段话改得更克制",
      "站内有哪些高赞的 Nuxt 笔记？",
    ],
    send: vi.fn(() => Promise.resolve()),
    stop: vi.fn(),
    clear: vi.fn(),
    fillDraft: vi.fn((question: string) => {
      panel.draft.value = question;
    }),
    openPanel: vi.fn(),
    closePanel: vi.fn(),
    togglePanel: vi.fn(),
    handleComposerKeydown: vi.fn(),
  };

  return { ...panel, ...overrides };
}

function makeAssistant(overrides: { open?: boolean; mobile?: boolean } = {}) {
  const assistant = {
    isOpen: ref(overrides.open ?? true),
    isAvailable: ref(true),
    isReducedMotion: ref(false),
    position: ref({ x: 900, y: 700 }),
  };
  return assistant as unknown as JellyAssistantApi & typeof assistant;
}

async function setup(options: { panel?: FakePanel; assistant?: ReturnType<typeof makeAssistant> } = {}) {
  const panel = options.panel ?? makePanel();
  const assistant = options.assistant ?? makeAssistant();
  panelMock.current = panel;

  const wrapper = mount(JellyAiPanel, {
    props: { assistant, aiInfo: null },
  });
  await nextTick();
  await nextTick();

  return { wrapper, panel, assistant };
}

describe("JellyAiPanel · 形态与无障碍", () => {
  it("关闭时不渲染面板（开关来自共享的 useJellyAssistant 实例）", async () => {
    const { wrapper } = await setup({ panel: makePanel({ isOpen: ref(false) }) });

    expect(wrapper.find(".jelly-panel").exists()).toBe(false);
    wrapper.unmount();
  });

  it("打开时是非遮罩 panel：role=dialog + aria-modal=false + 可读的 aria-label", async () => {
    const { wrapper } = await setup();
    const panel = wrapper.find(".jelly-panel");

    expect(panel.exists()).toBe(true);
    expect(panel.attributes("role")).toBe("dialog");
    expect(panel.attributes("aria-modal")).toBe("false");
    expect(panel.attributes("aria-label")).toContain("捏捏");
    expect(wrapper.find(".jelly-panel__mask").exists()).toBe(false);
    expect(wrapper.find(".jelly-panel__overlay").exists()).toBe(false);

    wrapper.unmount();
  });

  it("桌面从右下角生长：transform-origin 落在右下角", async () => {
    const { wrapper } = await setup();
    expect(wrapper.find(".jelly-panel").attributes("style")).toContain("transform-origin: 100% 100%");
    wrapper.unmount();
  });

  it("手机宽度下变成底部抽屉：移动端 class + 78vh 高", async () => {
    const originalWidth = window.innerWidth;
    window.innerWidth = 480;

    try {
      const { wrapper } = await setup();
      const panel = wrapper.find(".jelly-panel");

      expect(panel.classes()).toContain("jelly-panel--mobile");
      expect(panel.attributes("style")).toContain("78vh");
      wrapper.unmount();
    } finally {
      window.innerWidth = originalWidth;
    }
  });

  it("按 Esc 收起面板（键盘能退出，不要只能点关闭）", async () => {
    const { wrapper, panel } = await setup();

    await wrapper.find(".jelly-panel").trigger("keydown", { key: "Escape" });
    expect(panel.closePanel).toHaveBeenCalled();

    wrapper.unmount();
  });
});

describe("JellyAiPanel · 空态快捷问句", () => {
  it("空态给出 4 个可直接点的问句", async () => {
    const { wrapper } = await setup();
    const chips = wrapper.findAll(".jelly-panel__chip");

    expect(chips).toHaveLength(4);
    expect(chips[0].text()).toContain("Go 并发");

    wrapper.unmount();
  });

  it("点快捷问句 = 填进输入框（不直接发送）", async () => {
    const { wrapper, panel } = await setup();
    const chips = wrapper.findAll(".jelly-panel__chip");

    await chips[1].trigger("click");
    await nextTick();

    expect(panel.fillDraft).toHaveBeenCalledWith("这篇长文的核心观点是什么？");
    expect((wrapper.find("textarea").element as HTMLTextAreaElement).value).toBe(
      "这篇长文的核心观点是什么？",
    );
    expect(panel.send).not.toHaveBeenCalled();

    wrapper.unmount();
  });
});

describe("JellyAiPanel · 消息与流式反馈", () => {
  it("渲染用户与助手消息；流式回答带打字光标；停止后标记已停止", async () => {
    const panel = makePanel({
      messages: ref<JellyChatMessage[]>([
        { id: "u1", role: "user", content: "帮我讲讲 SSE" },
        { id: "a1", role: "assistant", content: "先说结论", streaming: true },
      ]),
    });
    const { wrapper } = await setup({ panel });

    expect(wrapper.find(".jelly-panel__message--user").text()).toContain("帮我讲讲 SSE");
    expect(wrapper.find(".md-surface").text()).toContain("先说结论");
    expect(wrapper.find(".jelly-panel__caret").exists()).toBe(true);

    panel.messages.value = [
      panel.messages.value[0],
      { id: "a1", role: "assistant", content: "先说结论", stopped: true },
    ];
    await nextTick();

    expect(wrapper.find(".jelly-panel__caret").exists()).toBe(false);
    expect(wrapper.find(".jelly-panel__stopped").exists()).toBe(true);

    wrapper.unmount();
  });

  it("错误消息用错误态样式（不把失败混进正常回答的样子）", async () => {
    const panel = makePanel({
      messages: ref<JellyChatMessage[]>([
        { id: "u1", role: "user", content: "问一下" },
        { id: "a1", role: "assistant", content: "等太久了…", errorKind: "timeout" },
      ]),
    });
    const { wrapper } = await setup({ panel });

    expect(wrapper.find(".jelly-panel__message--error").exists()).toBe(true);

    wrapper.unmount();
  });

  it("进行中显示「停止回答」，点了就调 stop()；关闭面板按钮调 closePanel()", async () => {
    const panel = makePanel();
    const { wrapper } = await setup({ panel });

    panel.pending.value = true;
    await nextTick();

    const stopButton = wrapper.findAll("button").find((button) => button.text().includes("停止回答"));
    expect(stopButton).toBeTruthy();
    await stopButton!.trigger("click");
    expect(panel.stop).toHaveBeenCalled();

    const closeButton = wrapper.find(".jelly-panel__close");
    await closeButton.trigger("click");
    expect(panel.closePanel).toHaveBeenCalled();

    wrapper.unmount();
  });

  it("清空对话按钮调 clear()", async () => {
    const panel = makePanel({
      messages: ref<JellyChatMessage[]>([{ id: "u1", role: "user", content: "有内容" }]),
    });
    const { wrapper } = await setup({ panel });

    await wrapper.find(".jelly-panel__clear").trigger("click");
    expect(panel.clear).toHaveBeenCalled();

    wrapper.unmount();
  });
});

describe("JellyAiPanel · 输入区", () => {
  it("有内容才能点发送；点发送调 send()", async () => {
    const panel = makePanel();
    const { wrapper } = await setup({ panel });

    const sendButton = wrapper.find(".jelly-panel__send");
    expect(sendButton.attributes("disabled")).toBeDefined();

    panel.draft.value = "有内容";
    await nextTick();
    expect(wrapper.find(".jelly-panel__send").attributes("disabled")).toBeUndefined();

    await wrapper.find(".jelly-panel__send").trigger("click");
    expect(panel.send).toHaveBeenCalled();

    wrapper.unmount();
  });

  it("输入框回车交给 composable 的键盘处理（Shift+Enter 换行由它判断）", async () => {
    const panel = makePanel();
    const { wrapper } = await setup({ panel });

    await wrapper.find("textarea").trigger("keydown", { key: "Enter" });
    expect(panel.handleComposerKeydown).toHaveBeenCalled();

    wrapper.unmount();
  });

  it("输入框受控绑定 draft（快捷问句填充后能看到文字）", async () => {
    const panel = makePanel();
    const { wrapper } = await setup({ panel });

    await wrapper.find("textarea").setValue("我自己写的");
    expect(panel.draft.value).toBe("我自己写的");

    wrapper.unmount();
  });
});
