import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import InboxMessageList from "~/components/inbox/InboxMessageList.vue";
import type { MessageCenterEntry } from "~/utils/messageCenter";

/**
 * 消息中心列表面板（#239）。
 *
 * 验收点：
 *   - 列表行 = 文章标题 + 一行内容摘要 + 时间；
 *   - 未读 = 左侧强调条 + 标题加粗 + 右上小圆点（**不用「未读」chip**）；
 *   - 删除按钮 hover 才出现（样式门禁在 test/design/message-center.test.ts）；
 *   - 批量动作只有「全部已读 / 清空当前分组（二次确认）/ 单条删除」，不做多选；
 *   - 滚动加载更多（每页 20，由 composable 保证），底部「没有更多了」收尾。
 */

const entry: MessageCenterEntry = {
  id: "m1",
  title: "被评论的文章",
  summary: "River 评论了你的文章：写得很清楚",
  createdAt: "2026-11-01T10:00:00.000Z",
  href: "/article/a1",
  unread: true,
};

const stubs = {
  NuxtLink: {
    props: ["to"],
    template: '<a :href="to"><slot /></a>',
  },
  NButton: {
    emits: ["click"],
    template: '<button type="button" @click="$emit(\'click\')"><slot /></button>',
  },
  StudioEmptyState: {
    props: ["title", "description"],
    template: '<div data-testid="empty">{{ title }} {{ description }}</div>',
  },
};

function mountList(props: Partial<Record<string, unknown>> = {}) {
  return mount(InboxMessageList, {
    props: {
      items: [entry],
      pending: false,
      hasMore: false,
      activeTab: "comments",
      emptyTitle: "还没有评论与回复",
      emptyDescription: "有人评论或回复你时，消息会出现在这里。",
      ...props,
    },
    global: { stubs },
  });
}

describe("消息中心列表行", () => {
  it("每行展示文章标题（可点回原文）、一行内容摘要与时间", () => {
    const wrapper = mountList();
    const row = wrapper.get(".message-row");
    const title = row.get(".message-row__title");

    expect(title.text()).toBe("被评论的文章");
    expect(title.attributes("href")).toBe("/article/a1");
    expect(row.get(".message-row__summary").text()).toBe("River 评论了你的文章：写得很清楚");
    expect(row.get(".message-row__time").text()).toMatch(/\d{2}\/\d{2} \d{2}:\d{2}/u);
  });

  it("未读行：强调条 class + 右上小圆点；不渲染「未读」chip", () => {
    const wrapper = mountList();
    const row = wrapper.get(".message-row");

    expect(row.classes()).toContain("is-unread");
    expect(row.find(".message-row__dot").exists()).toBe(true);
    expect(wrapper.text()).not.toContain("未读");
    // 列表行不再用 NTag 之类的 chip 表达状态。
    expect(wrapper.find(".n-tag").exists()).toBe(false);
  });

  it("已读行没有强调条 class 与圆点", () => {
    const wrapper = mountList({ items: [{ ...entry, unread: false }] });
    const row = wrapper.get(".message-row");

    expect(row.classes()).not.toContain("is-unread");
    expect(row.find(".message-row__dot").exists()).toBe(false);
  });

  it("删除是单条动作（无勾选框、无多选），按钮有明确无障碍文案", async () => {
    const wrapper = mountList();
    const remove = wrapper.get(".message-row__delete");

    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(0);
    expect(remove.attributes("aria-label")).toBe("删除这条消息");
    expect(remove.attributes("type")).toBe("button");

    await remove.trigger("click");
    expect(wrapper.emitted("remove")?.[0]).toEqual(["m1"]);
  });
});

describe("消息中心批量动作", () => {
  it("全部已读直接触发，不做二次确认", async () => {
    const wrapper = mountList();
    const button = wrapper.findAll("button").find((item) => item.text() === "全部已读");

    await button!.trigger("click");
    expect(wrapper.emitted("markAllRead")).toHaveLength(1);
  });

  it("清空当前分组必须二次确认：第一次点击只出确认条，确认后才触发", async () => {
    const wrapper = mountList();
    const clear = wrapper.findAll("button").find((item) => item.text() === "清空当前分组");

    await clear!.trigger("click");
    expect(wrapper.emitted("clearGroup")).toBeUndefined();
    expect(wrapper.text()).toContain("清空后不可恢复");

    await wrapper
      .findAll("button")
      .find((item) => item.text() === "确认清空")!
      .trigger("click");
    expect(wrapper.emitted("clearGroup")).toHaveLength(1);
  });

  it("确认条可以取消，取消后不触发清空", async () => {
    const wrapper = mountList();
    const clear = wrapper.findAll("button").find((item) => item.text() === "清空当前分组");

    await clear!.trigger("click");
    await wrapper
      .findAll("button")
      .find((item) => item.text() === "取消")!
      .trigger("click");

    expect(wrapper.emitted("clearGroup")).toBeUndefined();
    expect(wrapper.text()).not.toContain("清空后不可恢复");
  });
});

describe("消息中心加载与空态", () => {
  it("滚动到底部会请求更多（每页 20 由 composable 决定）", async () => {
    const wrapper = mountList({ hasMore: true });

    await flushPromises();
    expect(wrapper.emitted("loadMore")).toHaveLength(1);
  });

  it("请求在途时不重复触发加载更多，并给出加载状态", async () => {
    const wrapper = mountList({ hasMore: true, pending: true });

    await flushPromises();
    expect(wrapper.emitted("loadMore")).toBeUndefined();
    expect(wrapper.text()).toContain("正在加载更多消息");
  });

  it("没有更多时以「没有更多了」收尾", () => {
    const wrapper = mountList();

    expect(wrapper.text()).toContain("没有更多了");
  });

  it("空列表渲染空态（带标题与下一步说明）", () => {
    const wrapper = mountList({ items: [] });

    expect(wrapper.get('[data-testid="empty"]').text()).toContain("还没有评论与回复");
    expect(wrapper.text()).toContain("有人评论或回复你时");
  });
});
