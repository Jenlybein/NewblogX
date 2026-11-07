import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import InboxSubNav from "~/components/inbox/InboxSubNav.vue";
import type { MessageSummary } from "~/types/api";
import { messageCenterNavItems } from "~/utils/messageCenter";
import {
  NECK_CLOSED_GAP,
  NECK_CORNER_PX,
  NECK_SEPARATION_PX,
  NECK_WAIST,
  neckPath,
} from "~/utils/subnavNeck";

/**
 * 消息中心子项导航（票 239 建立，票 297 改成一体化分段条，本次按参考返工）。
 *
 * 断言的是**外部行为**：顺序与 href、`aria-current`、未读数规则、激活块的可观察标记、
 * 颈部只出现在激活段两侧、圆角只给敞口一侧、减少动态下的落位。像素级视觉不断言
 * （ADR-0012 已否决截图比对），配色与贴合规则落在 `test/design/message-center.test.ts`
 * 里按 CSS 源码判。
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
 * 段位与颈部宽度都从这一个变量派生（`margin-left` 取原值、颈部取 `max(0px, …)`），
 * 所以只看它就够 —— 这正是"两者不会错开"的证据。
 */
function segmentGaps(wrapper: ReturnType<typeof mountNav>): string[] {
  return segmentStyles(wrapper).map((element) => element.style.getPropertyValue("--neck-gap"));
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

  it("闭合接缝重叠 1px：间隙为负 1，段位与颈部都按它派生", () => {
    const wrapper = mountNav("comments");
    const gaps = segmentGaps(wrapper);

    expect(gaps[0]).toBe("0px");
    expect(gaps[2]).toBe(`${NECK_CLOSED_GAP}px`);
    expect(gaps[3]).toBe(`${NECK_CLOSED_GAP}px`);
    expect(gaps[4]).toBe(`${NECK_CLOSED_GAP}px`);
    // 段位不再由 JS 另写一份 margin-left：CSS 从同一个 --neck-gap 派生，
    // 于是"段位偏移"与"颈部宽度"在任何一帧都不可能错开。
    expect(segmentStyles(wrapper).map((element) => element.style.marginLeft)).toEqual(["", "", "", "", ""]);
  });

  it("圆角只给敞口一侧：激活块四角皆圆，闭合接缝旁是方的（贴合处不啃缺）", () => {
    const wrapper = mountNav("system");
    const radii = segmentStyles(wrapper).map(segmentRadii);

    // 段 1：左接缝闭合（方）、右接缝张开（圆）
    expect(radii[1]).toEqual(["0px", `${NECK_CORNER_PX}px`, `${NECK_CORNER_PX}px`, "0px"]);
    // 段 2 = 激活块：两侧接缝都张开 → 四角皆圆
    expect(radii[2]).toEqual([
      `${NECK_CORNER_PX}px`,
      `${NECK_CORNER_PX}px`,
      `${NECK_CORNER_PX}px`,
      `${NECK_CORNER_PX}px`,
    ]);
    // 段 3：左接缝张开（圆）、右接缝闭合（方）
    expect(radii[3]).toEqual([`${NECK_CORNER_PX}px`, "0px", "0px", `${NECK_CORNER_PX}px`]);
    // 首段的左角、末段的右角是控件自己的那一头，永远圆
    expect(radii[0]![0]).toBe(`${NECK_CORNER_PX}px`);
    expect(radii[4]![1]).toBe(`${NECK_CORNER_PX}px`);
  });
});

describe("颈部：每条接缝常驻，宽度由张开量决定", () => {
  /** 某子项激活时，哪些接缝是张开的（= 哪些段的颈部是满宽的）。 */
  function openSeams(activeTab: Tab): number[] {
    const wrapper = mountNav(activeTab);
    return wrapper
      .findAll(".message-center__subnav-segment")
      .map((segment, index) => (segment.find('.message-center-tab__neck[data-seam="open"]').exists() ? index : -1))
      .filter((index) => index >= 0);
  }

  it("首段激活只有右侧一条接缝张开，中间段激活有左右两条，末段只有左侧一条", () => {
    expect(openSeams("comments")).toEqual([1]);
    expect(openSeams("digg")).toEqual([1, 2]);
    expect(openSeams("system")).toEqual([2, 3]);
    expect(openSeams("global")).toEqual([3, 4]);
    expect(openSeams("chat")).toEqual([4]);
  });

  it("每条内部接缝都挂着一条颈部：闭合的那几条宽度是 0（贴合时正好消失）", () => {
    const wrapper = mountNav("system");
    const necks = wrapper.findAll(".message-center-tab__neck");

    // 第 0 段左边没有接缝，所以是 4 条
    expect(necks).toHaveLength(4);
    expect(wrapper.findAll('.message-center-tab__neck[data-seam="closed"]')).toHaveLength(2);
  });

  it("张开的接缝按张开量拉开，颈部就长在张开的那段间隙里", () => {
    const wrapper = mountNav("system");
    const gaps = segmentGaps(wrapper);

    expect(gaps[2]).toBe(`${NECK_SEPARATION_PX}px`);
    expect(gaps[3]).toBe(`${NECK_SEPARATION_PX}px`);
    expect(gaps[1]).toBe(`${NECK_CLOSED_GAP}px`);
  });

  it("间隙只写一份：张开的接缝是 20px，闭合的是负 1（颈部在 CSS 里钳到 0）", () => {
    const wrapper = mountNav("system");

    expect(segmentGaps(wrapper)).toEqual([
      "0px",
      `${NECK_CLOSED_GAP}px`,
      `${NECK_SEPARATION_PX}px`,
      `${NECK_SEPARATION_PX}px`,
      `${NECK_CLOSED_GAP}px`,
    ]);
  });

  it("颈部路径两端满高、中点收腰（哑铃形，不是两头收窄的细缝）", () => {
    const necks = mountNav("system").findAll(".message-center-tab__neck");
    const paths = necks.map((neck) => neck.find("path").attributes("d") ?? "");

    expect(paths).toHaveLength(4);
    for (const path of paths) {
      expect(path).toBe(neckPath());
      expect(path.startsWith("M0 0")).toBe(true);
      expect(path.endsWith("Z")).toBe(true);
      expect(path).not.toMatch(/NaN|undefined/u);
    }
    // 腰部比两端薄：路径里同时出现 waist 与 span-waist 两个控制点
    expect(paths[0]).toContain(`${NECK_WAIST}`);
    expect(paths[0]).toContain(`${100 - NECK_WAIST}`);
    // 颈部是装饰：对读屏器隐藏
    for (const neck of necks) expect(neck.attributes("aria-hidden")).toBe("true");
    // 宽高都交给 CSS 拉伸，SVG 自己不带像素宽高
    for (const neck of necks) expect(neck.attributes("preserveAspectRatio")).toBe("none");
  });

  it("颈部渐变的两端分别取左右相邻段的填充色", () => {
    const wrapper = mountNav("system");
    const segments = wrapper.findAll(".message-center__subnav-segment");
    const stopsOf = (index: number) =>
      segments[index]!.findAll("stop").map((stop) => stop.attributes("stop-color"));

    // 接缝 2：左邻是未激活段（轨道面）→ 右邻是激活块（深墨面）
    expect(stopsOf(2)).toEqual(["var(--color-surface-raised)", "var(--color-surface-inverse)"]);
    // 接缝 3：左邻是激活块 → 右邻是未激活段
    expect(stopsOf(3)).toEqual(["var(--color-surface-inverse)", "var(--color-surface-raised)"]);
    // 闭合接缝两侧都是轨道面：连上的那一刻不会闪出第二种颜色
    expect(stopsOf(1)).toEqual(["var(--color-surface-raised)", "var(--color-surface-raised)"]);
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
    expect(segmentGaps(wrapper)).toEqual([
      "0px",
      `${NECK_CLOSED_GAP}px`,
      `${NECK_SEPARATION_PX}px`,
      `${NECK_SEPARATION_PX}px`,
      `${NECK_CLOSED_GAP}px`,
    ]);
  });
});

describe("肩：用激活块自己的底色填平圆角缺角", () => {
  /** 某子项激活时，哪些段挂着肩、挂在哪一侧。 */
  function shouldersOf(activeTab: Tab): string[] {
    return mountNav(activeTab)
      .findAll(".message-center__subnav-segment")
      .map((segment) => {
        const sides = segment
          .findAll(".message-center-tab__shoulder")
          .map((shoulder) => shoulder.attributes("data-side"));
        return sides.length > 0 ? sides.join("+") : "";
      });
  }

  it("只有激活段挂肩，且只挂在朝向接缝的那一侧", () => {
    // 中间段激活：两侧都是敞口接缝
    expect(shouldersOf("system")).toEqual(["", "", "left+right", "", ""]);
    // 首段激活：左边是控件自己的圆角
    expect(shouldersOf("comments")).toEqual(["right", "", "", "", ""]);
    // 末段激活：右边是控件自己的圆角
    expect(shouldersOf("chat")).toEqual(["", "", "", "", "left"]);
  });

  it("肩是纯装饰：对读屏器隐藏，也不吃指针事件", () => {
    const shoulders = mountNav("system").findAll(".message-center-tab__shoulder");

    expect(shoulders).toHaveLength(2);
    for (const shoulder of shoulders) expect(shoulder.attributes("aria-hidden")).toBe("true");
  });

  it("肩在分段容器内、链接之前：缺角由段自己的底色盖住，链接的可点区域不变", () => {
    const active = mountNav("system").find('.message-center__subnav-segment[data-active="true"]')!;
    const children = active.element.children;

    // 顺序：左肩 / 右肩 / 颈部 / 链接
    expect(children[0]!.className).toContain("message-center-tab__shoulder");
    expect(children[1]!.className).toContain("message-center-tab__shoulder");
    expect(children[2]!.className).toContain("message-center-tab__neck");
    expect(children[3]!.className).toContain("message-center-tab");
  });
});
