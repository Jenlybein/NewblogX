import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import InboxChatView from "~/components/inbox/InboxChatView.vue";
import type { ChatMessageItem } from "~/types/api";
import { buildChatTimeline } from "~/utils/chatTimeline";

/**
 * 私信删除的二次确认（#435 / #423 第 23 项）。
 *
 * 删除会话与删除单条消息都是不可恢复的动作，必须走项目里通用的
 * `dialog.warning` 口径：先弹确认，确认前不发请求。
 *
 * 这里挂真实 `InboxChatPanel`（点击真正的「删除会话」/ 气泡上的「删除」），
 * 只把取数 composable 换成可控假 api —— 验证的是用户路径，而不是某个私有函数。
 */

const dialog = vi.hoisted(() => ({ warning: vi.fn() }));
const message = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }));

const inbox = vi.hoisted(() => ({
  activeSessionId: "chat:u1:u2",
  activeSession: {
    session_id: "chat:u1:u2",
    receiver_id: "u2",
    receiver_nickname: "乙",
    receiver_avatar: "",
    relation: 2,
    last_msg_content: "你好",
    last_msg_time: "2026-11-03T10:00:00.000Z",
    unread_count: 0,
    is_top: false,
    is_mute: false,
  },
  filteredChatSessions: [
    {
      session_id: "chat:u1:u2",
      receiver_id: "u2",
      receiver_nickname: "乙",
      receiver_avatar: "",
      relation: 2,
      last_msg_content: "你好",
      last_msg_time: "2026-11-03T10:00:00.000Z",
      unread_count: 0,
      is_top: false,
      is_mute: false,
    },
  ],
  chatTimeline: [] as unknown[],
  chatKeyword: "",
  chatDraft: "",
  chatDraftType: 1,
  imagePending: false,
  sessionPending: false,
  messagePending: false,
  hasMoreHistory: false,
  olderPending: false,
  socketStatus: "connected",
  socketError: "",
  chatSendError: "",
  removeChatSession: vi.fn(async (_sessionId: string) => {}),
  removeChatMessage: vi.fn(async (_id: string) => {}),
}));

vi.mock("naive-ui", () => {
  const NButton = defineComponent({
    name: "NButton",
    props: { disabled: { type: Boolean, default: false } },
    emits: ["click"],
    setup(props, { slots, emit }) {
      return () =>
        h("button", { type: "button", disabled: props.disabled, onClick: () => emit("click") }, slots.default?.());
    },
  });

  const NInput = defineComponent({
    name: "NInput",
    props: { value: { type: String, default: "" }, placeholder: { type: String, default: "" } },
    emits: ["update:value"],
    setup(props, { emit }) {
      return () =>
        h("textarea", {
          value: props.value,
          placeholder: props.placeholder,
          onInput: (event: Event) => emit("update:value", (event.target as HTMLTextAreaElement).value),
        });
    },
  });

  const NTag = defineComponent({
    name: "NTag",
    setup(_, { slots }) {
      return () => h("span", slots.default?.());
    },
  });

  return { NButton, NInput, NTag, useDialog: () => dialog, useMessage: () => message };
});

vi.mock("~/composables/useInboxCenter", () => ({
  useInboxCenter: () => ({
    activeSessionId: inbox.activeSessionId,
    activeSession: inbox.activeSession,
    filteredChatSessions: inbox.filteredChatSessions,
    chatTimeline: inbox.chatTimeline,
    chatKeyword: inbox.chatKeyword,
    chatDraft: inbox.chatDraft,
    chatDraftType: inbox.chatDraftType,
    imagePending: inbox.imagePending,
    sessionPending: inbox.sessionPending,
    messagePending: inbox.messagePending,
    hasMoreHistory: inbox.hasMoreHistory,
    olderPending: inbox.olderPending,
    socketStatus: inbox.socketStatus,
    socketError: inbox.socketError,
    chatSendError: inbox.chatSendError,
    removeChatSession: inbox.removeChatSession,
    removeChatMessage: inbox.removeChatMessage,
    sendCurrentChatMessage: vi.fn(),
    sendChatImage: vi.fn(),
    connectSocket: vi.fn(),
    loadOlderChatMessages: vi.fn(),
    retryChatLoad: vi.fn(),
  }),
}));

const chatStubs = {
  AppAvatar: { props: ["src", "name"], template: '<span class="avatar">{{ name }}</span>' },
  ChatMarkdownRenderer: { props: ["source"], template: "<div>{{ source }}</div>" },
};

const messageItem: ChatMessageItem = {
  id: "m1",
  sender_id: "u2",
  receiver_id: "u1",
  session_id: "chat:u1:u2",
  content: "你好",
  send_time: "2026-11-03T10:00:00.000Z",
  msg_status: 1,
  msg_type: 1,
  is_self: false,
  is_read: true,
};

function mountView() {
  return mount(InboxChatView, { global: { stubs: chatStubs } });
}

beforeEach(() => {
  inbox.chatTimeline = buildChatTimeline([messageItem]);
  vi.stubGlobal("useRoute", () => ({ query: {}, params: {}, path: "/messages/chat" }));
  vi.stubGlobal("useRouter", () => ({ replace: vi.fn() }));
});

describe("私信删除 · 先确认再执行", () => {
  it("点「删除会话」先弹确认；取消前不调用删除", async () => {
    const wrapper = mountView();

    await wrapper.findAll("button").find((button) => button.text() === "删除会话")!.trigger("click");

    expect(inbox.removeChatSession).not.toHaveBeenCalled();
    expect(dialog.warning).toHaveBeenCalledTimes(1);

    const options = dialog.warning.mock.calls[0]![0] as {
      title: string;
      content: string;
      positiveText: string;
      negativeText: string;
      onPositiveClick: () => Promise<void>;
    };
    expect(options.title).toBe("删除会话");
    expect(options.content).toContain("不可恢复");
    expect(options.positiveText).toBe("删除");
    expect(options.negativeText).toBe("取消");

    // 相当于用户点了「取消」：没有正向回调，状态与请求都不动。
    expect(inbox.removeChatSession).not.toHaveBeenCalled();

    await options.onPositiveClick();
    expect(inbox.removeChatSession).toHaveBeenCalledTimes(1);
    expect(inbox.removeChatSession).toHaveBeenCalledWith("chat:u1:u2");
  });

  it("点气泡上的「删除」也先确认；确认后只删除一次", async () => {
    const wrapper = mountView();

    await wrapper.get('[aria-label="删除这条消息"]').trigger("click");

    expect(inbox.removeChatMessage).not.toHaveBeenCalled();
    expect(dialog.warning).toHaveBeenCalledTimes(1);

    const options = dialog.warning.mock.calls[0]![0] as {
      title: string;
      content: string;
      positiveText: string;
      negativeText: string;
      onPositiveClick: () => Promise<void>;
    };
    expect(options.title).toBe("删除消息");
    expect(options.content).toContain("不可恢复");
    expect(options.positiveText).toBe("删除");
    expect(options.negativeText).toBe("取消");

    await options.onPositiveClick();
    expect(inbox.removeChatMessage).toHaveBeenCalledTimes(1);
    expect(inbox.removeChatMessage).toHaveBeenCalledWith("m1");
  });
});
