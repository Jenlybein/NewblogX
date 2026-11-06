import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import InboxSubNav from "~/components/inbox/InboxSubNav.vue";
import type { MessageSummary } from "~/types/api";
import { messageCenterNavItems } from "~/utils/messageCenter";
import { NECK_CLOSED_GAP, NECK_RADIUS_ACTIVE_PX, NECK_RADIUS_IDLE_PX, NECK_SEPARATION_PX } from "~/utils/subnavNeck";

/**
 * 消息中心子项导航（票 239 建立，票 297 改成一体化分段条）。
 *
 * 断言的是**外部行为**：顺序与 href、`aria-current`、未读数规则、激活块的可观察标记、
 * 颈部只出现在激活段两侧、减少动态下的落位。像素级视觉不断言（ADR-0012 已否决
 * 截图比对），配色与贴合规则落在 `test/design/message-center.test.ts` 里按 CSS 源码判。
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

type Tab = "comments" | "digg" | "system" | "global" | "chat";

function mountNav(activeTab: Tab = "comments") {
  return mount(InboxSubNav, {
    props: { items: navItems, activeTab },
    global: { stubs },
  });
}

/** 让「减少动态」为真：只改 matchMedia 的匹配结果，其余形状与测试装置的桩一致。 */
function stubReducedMotion(matches: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
}

/** 分段容器上由弹簧写到位的段位偏移（px 字符串）。 */
function segmentMargins(wrapper: ReturnType<typeof mountNav>): string[] {
  return wrapper
    .findAll(".message-center__subnav-segment")
    .map((segment) => (segment.element as HTMLElement).style.marginLeft);
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

describe("一体化分段条：五段贴合", () => {
  it("每个子项各占一个分段容器，五段相邻（不再是一个个独立胶囊）", () => {
    const wrapper = mountNav();
    const segments = wrapper.findAll(".message-center__subnav-segment");

    expect(segments).toHaveLength(5);
    expect(segments.map((segment) => segment.findAll(".message-center-tab").length)).toEqual([1, 1, 1, 1, 1]);
  });

  it("激活块由激活段铺底、链接继承反白：激活段标记 data-active，链接仍带 is-active", () => {
    const wrapper = mountNav("system");
    const activeSegments = wrapper.findAll('.message-center__subnav-segment[data-active="true"]');

    expect(activeSegments).toHaveLength(1);
    const link = activeSegments[0]!.find(".message-center-tab");
    expect(link.attributes("href")).toBe("/messages/system");
    expect(link.classes()).toContain("is-active");
    expect(wrapper.findAll('.message-center__subnav-segment[data-active="false"]')).toHaveLength(4);
  });

  it("激活块的未读角标仍在链接内（半透明反白底由 CSS 给）", () => {
    const wrapper = mountNav("system");
    const activeLink = wrapper.find('.message-center__subnav-segment[data-active="true"] .message-center-tab');

    expect(activeLink.find(".message-center-tab__count").text()).toBe("5");
    expect(activeLink.classes()).toContain("is-active");
  });

  it("闭合接缝重叠 1px：非激活段落在负 1px 的段位上", () => {
    const wrapper = mountNav("comments");
    const margins = segmentMargins(wrapper);

    expect(margins[0]).toBe("0px");
    expect(margins[2]).toBe(`${NECK_CLOSED_GAP}px`);
    expect(margins[3]).toBe(`${NECK_CLOSED_GAP}px`);
    expect(margins[4]).toBe(`${NECK_CLOSED_GAP}px`);
  });
});

describe("颈部：只在激活段两侧的接缝上", () => {
  /** 某子项激活时，哪些分段容器上挂着颈部（= 哪些接缝张开了）。 */
  function openSeams(activeTab: Tab): number[] {
    const wrapper = mountNav(activeTab);
    return wrapper
      .findAll(".message-center__subnav-segment")
      .map((segment, index) => (segment.find(".message-center-tab__neck").exists() ? index : -1))
      .filter((index) => index >= 0);
  }

  it("首段激活只有右侧一条颈部，中间段激活有左右两条，末段只有左侧一条", () => {
    expect(openSeams("comments")).toEqual([1]);
    expect(openSeams("digg")).toEqual([1, 2]);
    expect(openSeams("system")).toEqual([2, 3]);
    expect(openSeams("global")).toEqual([3, 4]);
    expect(openSeams("chat")).toEqual([4]);
  });

  it("张开的接缝按张开量拉开，颈部就长在张开的那段间隙里", () => {
    const wrapper = mountNav("system");
    const margins = segmentMargins(wrapper);

    expect(margins[2]).toBe(`${NECK_SEPARATION_PX}px`);
    expect(margins[3]).toBe(`${NECK_SEPARATION_PX}px`);
    expect(margins[1]).toBe(`${NECK_CLOSED_GAP}px`);
  });

  it("颈部路径是纯几何助手的产物：闭合、无 NaN", () => {
    const necks = mountNav("system").findAll(".message-center-tab__neck");
    const paths = necks.map((neck) => neck.find("path").attributes("d") ?? "");

    expect(paths).toHaveLength(2);
    for (const path of paths) {
      expect(path.startsWith("M0 0")).toBe(true);
      expect(path.endsWith("Z")).toBe(true);
      expect(path).not.toMatch(/NaN|undefined/u);
    }
    // 颈部是装饰：对读屏器隐藏
    for (const neck of necks) expect(neck.attributes("aria-hidden")).toBe("true");
  });

  it("颈部渐变的两端分别取左右相邻段的填充色", () => {
    const wrapper = mountNav("system");
    const segments = wrapper.findAll(".message-center__subnav-segment");
    const stopsOf = (index: number) =>
      segments[index]!.findAll("stop").map((stop) => stop.attributes("stop-color"));

    // 接缝 2：左邻是未激活段（纸面）→ 右邻是激活块（深墨面）
    expect(stopsOf(2)).toEqual(["var(--color-surface)", "var(--color-surface-inverse)"]);
    // 接缝 3：左邻是激活块 → 右邻是未激活段
    expect(stopsOf(3)).toEqual(["var(--color-surface-inverse)", "var(--color-surface)"]);
  });

  it("渐变沿 x 轴铺满颈部（端点在左右两条边上，不在中间）", () => {
    const gradient = mountNav("system").find("linearGradient");

    expect(gradient.attributes("x1")).toBe("0");
    expect(gradient.attributes("x2")).toBe("1");
    expect(mountNav("system").findAll("stop").map((stop) => stop.attributes("offset"))).toEqual([
      "0",
      "1",
      "0",
      "1",
    ]);
  });
});

describe("动效：弹簧只从 motion.ts 的预设取，减少动态落位", () => {
  it("默认用弹簧", () => {
    const wrapper = mountNav("system");

    expect(wrapper.find(".message-center__subnav").attributes("data-neck-motion")).toBe("spring");
  });

  it("减少动态下段位直接落位（markup 与常态一致，只是过渡归零）", () => {
    stubReducedMotion(true);
    const wrapper = mountNav("system");

    expect(wrapper.find(".message-center__subnav").attributes("data-neck-motion")).toBe("jump");
    expect(segmentMargins(wrapper)).toEqual([
      "0px",
      `${NECK_CLOSED_GAP}px`,
      `${NECK_SEPARATION_PX}px`,
      `${NECK_SEPARATION_PX}px`,
      `${NECK_CLOSED_GAP}px`,
    ]);
  });

  it("圆角：激活块涨到圆角阶梯的上一档，其余是控件档", () => {
    const wrapper = mountNav("system");
    const radii = wrapper
      .findAll(".message-center__subnav-segment")
      .map((segment) => (segment.element as HTMLElement).style.borderRadius);

    expect(radii).toEqual([
      `${NECK_RADIUS_IDLE_PX}px`,
      `${NECK_RADIUS_IDLE_PX}px`,
      `${NECK_RADIUS_ACTIVE_PX}px`,
      `${NECK_RADIUS_IDLE_PX}px`,
      `${NECK_RADIUS_IDLE_PX}px`,
    ]);
  });
});
