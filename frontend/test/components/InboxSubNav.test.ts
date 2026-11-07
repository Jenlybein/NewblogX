import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import InboxSubNav from "~/components/inbox/InboxSubNav.vue";
import type { MessageSummary } from "~/types/api";
import { messageCenterNavItems } from "~/utils/messageCenter";
import { SEAM_CLOSED_GAP, SEAM_CORNER_PX, SEAM_OPEN_PX } from "~/utils/subnavNeck";

/**
 * 消息中心子项导航（票 239 建立，票 297 改成一体化分段条，本次去掉颈部连接）。
 *
 * 断言的是**外部行为**：顺序与 href、`aria-current`、未读数规则、激活块的可观察标记、
 * 激活块两侧的接缝张开、圆角只给敞口一侧、减少动态下的落位。像素级视觉不断言
 * （ADR-0012 已否决截图比对），配色与贴合规则落在 `test/design/message-center.test.ts`
 * 里按 CSS 源码判。
 *
 * 颈部退役后这里只剩一条相关断言：DOM 里**没有任何**连接形状（`svg` / `path` /
 * `linearGradient` 一个都不许有）—— 激活块与邻居之间只有留白。
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

function segmentStyles(wrapper: ReturnType<typeof mountNav>): HTMLElement[] {
  return wrapper.findAll(".message-center__subnav-segment").map((segment) => segment.element as HTMLElement);
}

/**
 * 分段容器上由弹簧写到位的接缝间隙（px 字符串）。
 *
 * 段位（`margin-left`）在 CSS 里从这一个变量派生，所以只看它就够 ——
 * 这正是"段位与到位状态不会错开"的证据。
 */
function segmentGaps(wrapper: ReturnType<typeof mountNav>): string[] {
  return segmentStyles(wrapper).map((element) => element.style.getPropertyValue("--seam-gap"));
}

/** 一段的四角圆角（px 字符串，顺序：左上 / 右上 / 右下 / 左下）。 */
function segmentRadii(element: HTMLElement): string[] {
  return [
    element.style.borderTopLeftRadius,
    element.style.borderTopRightRadius,
    element.style.borderBottomRightRadius,
    element.style.borderBottomLeftRadius,
  ];
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

  it("闭合接缝重叠 1px：间隙为负 1，段位在 CSS 里按它派生", () => {
    const wrapper = mountNav("comments");
    const gaps = segmentGaps(wrapper);

    expect(gaps[0]).toBe("0px");
    expect(gaps[2]).toBe(`${SEAM_CLOSED_GAP}px`);
    expect(gaps[3]).toBe(`${SEAM_CLOSED_GAP}px`);
    expect(gaps[4]).toBe(`${SEAM_CLOSED_GAP}px`);
    // 段位不由 JS 另写一份 margin-left：CSS 从同一个 --seam-gap 派生。
    expect(segmentStyles(wrapper).map((element) => element.style.marginLeft)).toEqual(["", "", "", "", ""]);
  });

  it("圆角只给敞口一侧：激活块四角皆圆，闭合接缝旁是方的（贴合处不啃缺）", () => {
    const wrapper = mountNav("system");
    const radii = segmentStyles(wrapper).map(segmentRadii);

    // 段 1：左接缝闭合（方）、右接缝张开（圆）
    expect(radii[1]).toEqual(["0px", `${SEAM_CORNER_PX}px`, `${SEAM_CORNER_PX}px`, "0px"]);
    // 段 2 = 激活块：两侧接缝都张开 → 四角皆圆
    expect(radii[2]).toEqual([
      `${SEAM_CORNER_PX}px`,
      `${SEAM_CORNER_PX}px`,
      `${SEAM_CORNER_PX}px`,
      `${SEAM_CORNER_PX}px`,
    ]);
    // 段 3：左接缝张开（圆）、右接缝闭合（方）
    expect(radii[3]).toEqual([`${SEAM_CORNER_PX}px`, "0px", "0px", `${SEAM_CORNER_PX}px`]);
    // 首段的左角、末段的右角是控件自己的那一头，永远圆
    expect(radii[0]![0]).toBe(`${SEAM_CORNER_PX}px`);
    expect(radii[4]![1]).toBe(`${SEAM_CORNER_PX}px`);
  });
});

describe("去掉颈部：激活块与邻居之间不再有任何连接形状", () => {
  it("DOM 里没有任何 SVG / 路径 / 渐变（颈部整块退役，不是宽度收到 0）", () => {
    for (const tab of ["comments", "digg", "system", "global", "chat"] as Tab[]) {
      const wrapper = mountNav(tab);

      expect(wrapper.findAll("svg"), tab).toHaveLength(0);
      expect(wrapper.findAll("path"), tab).toHaveLength(0);
      expect(wrapper.findAll("linearGradient"), tab).toHaveLength(0);
      expect(wrapper.find(".message-center-tab__neck").exists(), tab).toBe(false);
    }
  });

  it("每条段只有一个链接子节点：没有藏在段里的装饰元素", () => {
    const wrapper = mountNav("system");

    for (const segment of wrapper.findAll(".message-center__subnav-segment")) {
      expect(segment.element.children).toHaveLength(1);
      expect(segment.element.children[0]!.classList.contains("message-center-tab")).toBe(true);
    }
  });

  /** 某子项激活时，哪些段左侧的接缝是张开的。 */
  function openSeams(activeTab: Tab): number[] {
    return mountNav(activeTab)
      .findAll(".message-center__subnav-segment")
      .map((segment, index) => (segment.attributes("data-seam") === "open" ? index : -1))
      .filter((index) => index >= 0);
  }

  it("只有激活段两侧的接缝张开：首段一条、中间两条、末段一条", () => {
    expect(openSeams("comments")).toEqual([1]);
    expect(openSeams("digg")).toEqual([1, 2]);
    expect(openSeams("system")).toEqual([2, 3]);
    expect(openSeams("global")).toEqual([3, 4]);
    expect(openSeams("chat")).toEqual([4]);
  });

  it("张开的接缝只留 10px 间隙：激活块读成一块独立的胶囊", () => {
    expect(segmentGaps(mountNav("system"))).toEqual([
      "0px",
      `${SEAM_CLOSED_GAP}px`,
      `${SEAM_OPEN_PX}px`,
      `${SEAM_OPEN_PX}px`,
      `${SEAM_CLOSED_GAP}px`,
    ]);
  });
});

describe("动效：弹簧只从 motion.ts 的预设取，减少动态落位", () => {
  it("默认用弹簧", () => {
    const wrapper = mountNav("system");

    expect(wrapper.find(".message-center__subnav").attributes("data-seam-motion")).toBe("spring");
  });

  it("减少动态下段位直接落位（markup 与常态一致，只是过渡归零）", () => {
    stubReducedMotion(true);
    const wrapper = mountNav("system");

    expect(wrapper.find(".message-center__subnav").attributes("data-seam-motion")).toBe("jump");
    expect(segmentGaps(wrapper)).toEqual([
      "0px",
      `${SEAM_CLOSED_GAP}px`,
      `${SEAM_OPEN_PX}px`,
      `${SEAM_OPEN_PX}px`,
      `${SEAM_CLOSED_GAP}px`,
    ]);
  });
});
