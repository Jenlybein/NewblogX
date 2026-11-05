import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import InboxChatPanel from "~/components/inbox/InboxChatPanel.vue";
import type { ChatMessageItem, ChatSessionItem } from "~/types/api";
import { buildChatTimeline } from "~/utils/chatTimeline";

/**
 * 私信面板（2026-11-05 重构后）。
 *
 * 验收点：
 *   - 会话列表（未读在会话行）+ 聊天窗共处一个面板；宽屏双栏、<1024px 二选一；
 *   - 空会话 / 未选中会话完全留白，不再有卡片式说明空状态；
 *   - 面板只画递到手上的时间线（日期分隔 / 分组 / 回执都由唯一持有者算好）；
 *   - 连接状态条健康时不渲染，异常才出现；失败给「重试」；
 *   - 前插更早一页后滚动位置不跳。
 *   - 说明文案整体退役（由 design/message-center.test.ts 一并钉住）。
 */

const session: ChatSessionItem = {
  session_id: "chat:u1:u2",
  receiver_id: "u2",
  receiver_nickname: "乙",
  receiver_avatar: "",
  relation: 2,
  last_msg_content: "你好",
  last_msg_time: "2026-11-03T10:00:00.000Z",
  unread_count: 2,
  is_top: false,
  is_mute: false,
};

const secondSession: ChatSessionItem = {
  ...session,
  session_id: "chat:u1:u3",
  receiver_id: "u3",
  receiver_nickname: "丙",
  unread_count: 0,
};

const message: ChatMessageItem = {
  id: "m1",
  sender_id: "u2",
  receiver_id: "u1",
  session_id: session.session_id,
  content: "你好",
  send_time: "2026-11-03T10:00:00.000Z",
  msg_status: 1,
  msg_type: 1,
  is_self: false,
  is_read: true,
};

const stubs = {
  NuxtLink: { props: ["to"], template: '<a :href="to"><slot /></a>' },
  NInput: {
    props: ["value", "placeholder", "type"],
    emits: ["update:value"],
    template: '<textarea :value="value" @input="$emit(\'update:value\', $event.target.value)" />',
  },
  NButton: {
    emits: ["click"],
    template: '<button type="button" @click="$emit(\'click\')"><slot /></button>',
  },
  NTag: { template: "<span><slot /></span>" },
  AppAvatar: {
    props: ["src", "name"],
    template: '<span class="avatar">{{ name }}</span>',
  },
  ChatMarkdownRenderer: { props: ["source"], template: "<div>{{ source }}</div>" },
};

function mountPanel(overrides: Record<string, unknown> = {}) {
  return mount(InboxChatPanel, {
    props: {
      sessions: [session, secondSession],
      activeSessionId: null,
      currentSession: null,
      timeline: [],
      keyword: "",
      draft: "",
      draftType: 1,
      sessionPending: false,
      messagePending: false,
      imagePending: false,
      hasMoreHistory: false,
      olderPending: false,
      socketStatus: "connected",
      socketError: "",
      ...overrides,
    },
    global: { stubs },
  });
}

describe("私信面板：会话列表 + 聊天窗", () => {
  it("左侧列出会话（昵称、最后一条、未读），点击某条发出选中事件", async () => {
    const wrapper = mountPanel();
    const sessions = wrapper.findAll(".studio-chat-session");

    expect(sessions).toHaveLength(2);
    expect(sessions[0]!.text()).toContain("乙");
    expect(sessions[0]!.text()).toContain("你好");
    expect(sessions[0]!.text()).toContain("2");

    await sessions[1]!.trigger("click");
    expect(wrapper.emitted("update:activeSessionId")?.[0]).toEqual([secondSession.session_id]);
  });

  it("未选中会话时右侧完全留白（没有卡片、图标、说明文字）", () => {
    const wrapper = mountPanel();

    expect(wrapper.get(".studio-inbox-grid").classes()).toContain("is-narrow-list");
    expect(wrapper.find(".studio-chat-empty").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("还没有选中会话");
    expect(wrapper.find(".studio-chat-error").exists()).toBe(false);
  });

  it("空会话也完全留白，只剩输入区", () => {
    const wrapper = mountPanel({
      activeSessionId: session.session_id,
      currentSession: session,
      timeline: [],
    });

    expect(wrapper.find(".studio-chat-bubble").exists()).toBe(false);
    expect(wrapper.find(".studio-chat-composer").exists()).toBe(true);
    expect(wrapper.text()).not.toContain("这个会话还没有消息");
  });

  it("窄屏：选中会话切到聊天窗，返回按钮切回列表", async () => {
    const wrapper = mountPanel();

    await wrapper.findAll(".studio-chat-session")[0]!.trigger("click");
    await wrapper.setProps({ activeSessionId: session.session_id, currentSession: session, timeline: buildChatTimeline([message]) });

    expect(wrapper.get(".studio-inbox-grid").classes()).toContain("is-narrow-chat");
    expect(wrapper.get(".studio-chat-back").exists()).toBe(true);

    await wrapper.get(".studio-chat-back").trigger("click");
    expect(wrapper.get(".studio-inbox-grid").classes()).toContain("is-narrow-list");
  });

  it("消息按日期分组渲染，收到的消息不显示未读标签", () => {
    const wrapper = mountPanel({
      activeSessionId: session.session_id,
      currentSession: session,
      timeline: buildChatTimeline([message]),
    });

    expect(wrapper.find(".studio-chat-date-separator").exists()).toBe(true);
    expect(wrapper.get(".studio-chat-bubble").classes()).not.toContain("is-self");
    expect(wrapper.text()).not.toContain("未读");
  });

  it("只有自己最后一条消息保留回执", () => {
    const own1: ChatMessageItem = { ...message, id: "s1", sender_id: "u1", is_self: true, is_read: false };
    const own2: ChatMessageItem = { ...message, id: "s2", sender_id: "u1", is_self: true, is_read: true };
    const wrapper = mountPanel({
      activeSessionId: session.session_id,
      currentSession: session,
      timeline: buildChatTimeline([message, own1, own2]),
    });

    const receipts = wrapper.findAll(".studio-chat-receipt");
    expect(receipts).toHaveLength(2);
    expect(receipts[0]!.text()).toBe("");
    expect(receipts[1]!.text()).toBe("已读");
  });

  it("健康连接不渲染状态条；异常时才出现且可重连", async () => {
    const healthy = mountPanel({ activeSessionId: session.session_id, currentSession: session, timeline: buildChatTimeline([message]) });
    expect(healthy.find(".studio-socket-badge").exists()).toBe(false);

    const broken = mountPanel({
      activeSessionId: session.session_id,
      currentSession: session,
      timeline: buildChatTimeline([message]),
      socketStatus: "error",
      socketError: "实时连接发生异常。",
    });
    expect(broken.get(".studio-socket-badge").text()).toContain("实时连接发生异常。");
    await broken.findAll("button").find((item) => item.text() === "重连")!.trigger("click");
    expect(broken.emitted("reconnect")).toHaveLength(1);
  });

  it("发送失败给「重试」按钮，上滑到顶发出加载更早事件", async () => {
    const wrapper = mountPanel({
      activeSessionId: session.session_id,
      currentSession: session,
      timeline: buildChatTimeline([message]),
      hasMoreHistory: true,
      sendError: "网络连接不可用",
    });

    expect(wrapper.get(".studio-chat-error").text()).toContain("网络连接不可用");
    await wrapper.findAll("button").find((item) => item.text() === "重试")!.trigger("click");
    expect(wrapper.emitted("retry")).toHaveLength(1);

    const scroller = wrapper.get(".studio-chat-scroller");
    Object.defineProperty(scroller.element, "scrollTop", { value: 0, writable: true, configurable: true });
    Object.defineProperty(scroller.element, "scrollHeight", { value: 400, writable: true, configurable: true });
    Object.defineProperty(scroller.element, "clientHeight", { value: 200, writable: true, configurable: true });
    await scroller.trigger("scroll");
    expect(wrapper.emitted("loadOlder")).toHaveLength(1);
  });

  it("前插更早一页后滚动位置不跳（用 scrollHeight 差值钉回原锚点）", async () => {
    const wrapper = mountPanel({
      activeSessionId: session.session_id,
      currentSession: session,
      timeline: buildChatTimeline([message]),
      hasMoreHistory: true,
    });

    const scroller = wrapper.get(".studio-chat-scroller");
    const el = scroller.element as HTMLElement;
    Object.defineProperty(el, "scrollTop", { value: 0, writable: true, configurable: true });
    Object.defineProperty(el, "scrollHeight", { value: 400, writable: true, configurable: true });
    Object.defineProperty(el, "clientHeight", { value: 200, writable: true, configurable: true });

    await scroller.trigger("scroll");
    expect(wrapper.emitted("loadOlder")).toHaveLength(1);

    // 更早的一页前插进来：内容高度 +200px。
    const older: ChatMessageItem = { ...message, id: "m0", content: "更早", send_time: "2026-11-02T10:30:00.000Z" };
    Object.defineProperty(el, "scrollHeight", { value: 600, writable: true, configurable: true });
    await wrapper.setProps({ timeline: buildChatTimeline([older, message]) });
    await flushPromises();

    // 视口跟着原来的锚点往下挪 200px，用户看到的内容没有跳。
    expect(el.scrollTop).toBe(200);
  });

  it("聊天窗可以发送：按钮发出 send 事件，输入框同步 draft", async () => {
    const wrapper = mountPanel({
      activeSessionId: session.session_id,
      currentSession: session,
      timeline: buildChatTimeline([message]),
      draft: "继续聊",
    });

    const composer = wrapper.get("textarea");
    await composer.setValue("新的一句话");
    expect(wrapper.emitted("update:draft")?.[0]).toEqual(["新的一句话"]);

    const send = wrapper.findAll("button").find((item) => item.text().includes("发送文本"));
    await send!.trigger("click");
    expect(wrapper.emitted("send")).toHaveLength(1);
  });
});
