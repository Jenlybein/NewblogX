import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import InboxChatPanel from "~/components/inbox/InboxChatPanel.vue";
import type { ChatMessageItem, ChatSessionItem } from "~/types/api";

/**
 * 私信面板（票 #240）。
 *
 * 验收点：
 *   - 会话列表（未读在会话行）+ 聊天窗共处一个面板；
 *   - 宽屏双栏、**<1024px 二选一**：进来看列表，选中会话切聊天窗，可返回列表；
 *   - 从作者主页「发私信」带 draft 进来时，直接落在这个人的会话上（不先给列表）。
 *
 * 断点本身是 CSS（`@media (max-width: 1024px)` 隐藏另一栏），组件测试断言
 * 切换状态（`is-narrow-list` / `is-narrow-chat`），样式规则由设计门禁钉住。
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
  NSpace: { template: "<span><slot /></span>" },
  NTag: { template: "<span><slot /></span>" },
  NAlert: { template: '<div role="alert"><slot /></div>' },
  AppAvatar: {
    props: ["src", "name"],
    template: '<span class="avatar">{{ name }}</span>',
  },
  StudioEmptyState: {
    props: ["title", "description"],
    template: '<div data-testid="empty">{{ title }} {{ description }}</div>',
  },
};

function mountPanel(overrides: Record<string, unknown> = {}) {
  return mount(InboxChatPanel, {
    props: {
      sessions: [session, secondSession],
      activeSessionId: null,
      currentSession: null,
      items: [],
      keyword: "",
      draft: "",
      draftType: 1,
      sessionPending: false,
      messagePending: false,
      imagePending: false,
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

  it("没有选中会话时停在列表态（窄屏先看会话列表）", () => {
    const wrapper = mountPanel();

    expect(wrapper.get(".studio-inbox-grid").classes()).toContain("is-narrow-list");
    expect(wrapper.text()).toContain("还没有选中会话");
  });

  // 窄屏二选一：点会话 → 聊天窗；返回按钮 → 会话列表。
  it("窄屏：选中会话切到聊天窗，返回按钮切回列表", async () => {
    const wrapper = mountPanel();

    await wrapper.findAll(".studio-chat-session")[0]!.trigger("click");
    // 受控组件的 activeSessionId 由页面回写；这里模拟页面把 props 更新回来。
    await wrapper.setProps({ activeSessionId: session.session_id, currentSession: session, items: [message] });

    expect(wrapper.get(".studio-inbox-grid").classes()).toContain("is-narrow-chat");
    expect(wrapper.get(".studio-chat-back").exists()).toBe(true);

    await wrapper.get(".studio-chat-back").trigger("click");
    expect(wrapper.get(".studio-inbox-grid").classes()).toContain("is-narrow-list");
  });

  // 从作者主页带 draft 进来：直接落在这个人的会话上，不该先让用户在窄屏里翻列表。
  it("草稿会话已选中时直接显示聊天窗（不回落到列表）", () => {
    const wrapper = mountPanel({
      sessions: [session],
      activeSessionId: session.session_id,
      currentSession: session,
      items: [message],
    });

    expect(wrapper.get(".studio-inbox-grid").classes()).toContain("is-narrow-chat");
    expect(wrapper.text()).toContain("乙");
    expect(wrapper.text()).toContain("你好");
  });

  it("聊天窗可以发送：按钮发出 send 事件，输入框同步 draft", async () => {
    const wrapper = mountPanel({
      activeSessionId: session.session_id,
      currentSession: session,
      items: [message],
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
