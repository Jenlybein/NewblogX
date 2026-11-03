import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import InboxSubNav from "~/components/inbox/InboxSubNav.vue";
import type { MessageSummary } from "~/types/api";
import { messageCenterNavItems } from "~/utils/messageCenter";

/**
 * 消息中心左侧子项导航（#239）。
 *
 * 验收点：五个子项都能进、未读数与 `GET /api/sitemsg/user` 一致、**0 不显示数字**。
 * 选中态（抬升面底 + 左侧 2px 强调条 + 字重）是纯 CSS，断言样式源码而不是类名细节。
 */

const summary: MessageSummary = {
  comment_msg_count: 3,
  digg_favor_msg_count: 0,
  private_msg_count: 2,
  system_msg_count: 5,
  global_msg_count: 1,
};

const navItems = messageCenterNavItems(summary);

const stubs = {
  NuxtLink: {
    props: ["to"],
    template: '<a :href="to"><slot /></a>',
  },
};

function mountNav(activeTab: "comments" | "digg" | "system" | "global" | "chat" = "comments") {
  return mount(InboxSubNav, {
    props: { items: navItems, activeTab },
    global: { stubs },
  });
}

describe("消息中心子项导航", () => {
  it("五个子项按固定顺序：评论与回复 → 点赞与收藏 → 系统通知 → 全局通知 → 私信", () => {
    const wrapper = mountNav();
    const labels = wrapper.findAll(".message-center-tab").map((item) => item.text().replace(/\d+/gu, "").trim());

    expect(labels).toEqual(["评论与回复", "点赞与收藏", "系统通知", "全局通知", "私信"]);
  });

  it("每个子项都指向自己的 /messages/{tab} 路由", () => {
    const wrapper = mountNav();
    const hrefs = wrapper.findAll(".message-center-tab").map((item) => item.attributes("href"));

    expect(hrefs).toEqual([
      "/messages/comments",
      "/messages/digg",
      "/messages/system",
      "/messages/global",
      "/messages/chat",
    ]);
  });

  it("未读数来自未读汇总；0 的那一项不显示数字", () => {
    const wrapper = mountNav();
    const counts = wrapper
      .findAll(".message-center-tab")
      .map((item) => item.find(".message-center-tab__count").exists());
    const numbers = wrapper.findAll(".message-center-tab__count").map((item) => item.text());

    expect(counts).toEqual([true, false, true, true, true]);
    expect(numbers).toEqual(["3", "5", "1", "2"]);
  });

  it("当前子项带 aria-current，其它不带", () => {
    const wrapper = mountNav("digg");
    const current = wrapper.findAll("[aria-current=page]");

    expect(current).toHaveLength(1);
    expect(current[0]!.attributes("href")).toBe("/messages/digg");
    expect(current[0]!.classes()).toContain("is-active");
  });

  it("0 的那一项渲染结果里完全没有任何数字", () => {
    const wrapper = mountNav();
    const zeroItem = wrapper.findAll(".message-center-tab")[1]!;

    expect(zeroItem.text()).toBe("点赞与收藏");
  });
});
