import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import MessageCenterPage from "~/pages/messages/[tab].vue";

/**
 * 消息中心单条删除的二次确认（#435 / #423 第 23 项）。
 *
 * 与私信侧同一口径：点列表行上的垃圾桶先弹 `dialog.warning`，
 * 确认前不调用 `removeEntry`，取消后列表状态不变。
 */

const dialog = vi.hoisted(() => ({ warning: vi.fn() }));
const message = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }));

const center = vi.hoisted(() => ({
  items: [
    {
      id: "m1",
      title: "被评论的文章",
      summary: "River 评论了你的文章：写得很清楚",
      createdAt: "2026-11-01T10:00:00.000Z",
      href: "/article/a1",
      unread: true,
    },
  ],
  pending: false,
  hasMore: false,
  removeEntry: vi.fn(async (_id: string) => {}),
  clearCurrentGroup: vi.fn(async () => {}),
}));

vi.mock("naive-ui", async () => {
  const { defineComponent, h } = await import("vue");
  const NButton = defineComponent({
    name: "NButton",
    props: { disabled: { type: Boolean, default: false } },
    emits: ["click"],
    setup(props, { slots, emit }) {
      return () =>
        h("button", { type: "button", disabled: props.disabled, onClick: () => emit("click") }, slots.default?.());
    },
  });

  return { NButton, useDialog: () => dialog, useMessage: () => message };
});

vi.mock("~/composables/useMessageCenter", async () => {
  const { ref } = await import("vue");
  return {
    useMessageCenter: () => ({
      items: ref(center.items),
      pending: ref(center.pending),
      hasMore: ref(center.hasMore),
      loadMore: vi.fn(),
      markAllRead: vi.fn(async () => {}),
      clearCurrentGroup: center.clearCurrentGroup,
      removeEntry: center.removeEntry,
    }),
  };
});

function mountPage() {
  return mount(MessageCenterPage, {
    global: {
      stubs: {
        InboxSubNav: { props: ["items", "activeTab"], template: '<div data-test="subnav" />' },
        InboxChatView: true,
        NuxtLink: { props: ["to"], template: '<a :href="to"><slot /></a>' },
        StudioEmptyState: { props: ["title", "description"], template: '<div data-test="empty">{{ title }}</div>' },
      },
    },
  });
}

beforeEach(() => {
  center.removeEntry.mockClear();
  center.clearCurrentGroup.mockClear();
  dialog.warning.mockClear();
  vi.stubGlobal("definePageMeta", () => undefined);
  vi.stubGlobal("useSeoMeta", () => undefined);
  vi.stubGlobal("useNoIndexPage", () => undefined);
  vi.stubGlobal("useRoute", () => ({ params: { tab: "comments" }, query: {} }));
  vi.stubGlobal("useMessageStore", () => ({
    summary: {
      comment_msg_count: 1,
      digg_favor_msg_count: 0,
      system_msg_count: 0,
      global_msg_count: 0,
      private_msg_count: 0,
    },
  }));
});

describe("消息中心单条删除 · 先确认再执行", () => {
  it("点垃圾桶先弹确认，确认前不调用 removeEntry", async () => {
    const wrapper = mountPage();

    await wrapper.get(".message-row__delete").trigger("click");

    expect(center.removeEntry).not.toHaveBeenCalled();
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
    expect(center.removeEntry).toHaveBeenCalledTimes(1);
    expect(center.removeEntry).toHaveBeenCalledWith("m1");
  });
});

describe("消息中心清空分组 · 先确认再执行（票 423 第 23 项）", () => {
  it("点「清空当前分组」先弹确认，确认前不调用 clearCurrentGroup", async () => {
    const wrapper = mountPage();
    const clear = wrapper.findAll("button").find((item) => item.text() === "清空当前分组");

    await clear!.trigger("click");

    expect(center.clearCurrentGroup).not.toHaveBeenCalled();
    expect(dialog.warning).toHaveBeenCalledTimes(1);

    const options = dialog.warning.mock.calls[0]![0] as {
      title: string;
      content: string;
      positiveText: string;
      negativeText: string;
      onPositiveClick: () => Promise<void>;
    };
    expect(options.title).toBe("清空分组");
    expect(options.content).toContain("不可恢复");
    expect(options.positiveText).toBe("清空");
    expect(options.negativeText).toBe("取消");

    await options.onPositiveClick();
    expect(center.clearCurrentGroup).toHaveBeenCalledTimes(1);
  });

  it("取消（不点确认）不动状态：一个请求都不发", async () => {
    const wrapper = mountPage();
    const clear = wrapper.findAll("button").find((item) => item.text() === "清空当前分组");

    await clear!.trigger("click");
    // 只有一次 warning，没有 onNegativeClick 之类的副作用回调；
    // 不点确认 → clearCurrentGroup 永远不被调用。
    expect(dialog.warning).toHaveBeenCalledTimes(1);
    expect(center.clearCurrentGroup).not.toHaveBeenCalled();
    expect(center.removeEntry).not.toHaveBeenCalled();
  });
});
