import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter } from "../helpers/designFiles";

/**
 * 消息中心（#239）的视觉与路由契约门禁。
 *
 * 这些规则是验收的一部分，但都不适合用挂载断言（断点、hover 态、强调条都是 CSS）：
 *   - 子项选中态 = 抬升面底 + 左侧 2px 强调条 + 字重；未读数 0 不显示数字；
 *   - 列表未读 = 左侧强调条 + 标题加粗 + 右上小圆点（不用 chip）；
 *   - 删除按钮 hover 才出现；
 *   - <1024px 左侧导航变顶部横向 tab；
 *   - 顶栏三档：<1280 历史进头像菜单（#241 落位）、<1024 消息中心仅图标 + 红点、
 *     <640 消息中心进折叠面板。
 *
 * 页面不得自带 `<style>`：样式统一落在 tailwind.css / 组件 scoped style，
 * 这样 token 门禁与断点门禁才有唯一的扫描对象。
 */

const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

const tailwindCss = read("app/assets/css/tailwind.css");
const pageSource = read("app/pages/messages/[tab].vue");
const headerSource = read("app/components/layout/AppHeader.vue");
const redirectMiddleware = read("app/middleware/message-center-redirect.ts");
const tabMiddleware = read("app/middleware/messages-tab.ts");
const legacyInboxPage = read("app/pages/studio/inbox.vue");

describe("消息中心：视觉规则（CSS 源码判据）", () => {
  it("子项选中态：激活段深墨面（inverse）铺底 + 反白文字，不再用底部强调条（票 297）", () => {
    const activeSegment = blockAfter(tailwindCss, '.message-center__subnav-segment[data-active="true"] {');
    const selected = blockAfter(tailwindCss, ".message-center-tab.is-active {");

    expect(activeSegment).toContain("background: var(--color-surface-inverse)");
    expect(selected).toContain("color: var(--color-on-inverse)");
    expect(selected).toMatch(/font-weight:\s*600/u);
    // 旧口径的强调条与抬升面底必须整条退役：两套选中标记叠在一起会互相打架。
    expect(selected).not.toContain("box-shadow");
    expect(selected).not.toContain("--color-surface-raised");
  });

  it("五段贴合、整条为内容宽度（不再 self-stretch 拉满、不再留段间距）（票 297）", () => {
    const subnav = blockAfter(tailwindCss, ".message-center__subnav {");
    const segment = blockAfter(tailwindCss, ".message-center__subnav-segment {");

    expect(subnav).toContain("inline-flex");
    expect(subnav).toContain("max-w-full");
    expect(subnav).not.toContain("self-stretch");
    // 段间距会重新长回"独立胶囊"，贴合的贴合量只能由段位（margin-left）表达。
    expect(subnav).not.toMatch(/\bgap-[0-9]/u);
    expect(segment).toContain("shrink-0");
    expect(segment).toContain("whitespace-nowrap");
  });

  it("颈部：宽/位读 --neck-gap，长在张开的那段间隙里（票 297）", () => {
    const neck = blockAfter(tailwindCss, ".message-center-tab__neck {");

    // 负间隙（闭合接缝的 -1px 重叠）被 max() 钳到 0：颈部永远不会比真实间隙宽，
    // 也就不会盖到相邻段、把渐变漏到别人的圆角上。
    expect(neck).toContain("left: calc(-1 * max(0px, var(--neck-gap");
    expect(neck).toContain("width: max(0px, var(--neck-gap");
    expect(neck).toContain("pointer-events-none");
    // 段位与颈部读同一个变量，不各写一份（错开就会漏色 / 留发丝）
    expect(blockAfter(tailwindCss, ".message-center__subnav-segment {")).toContain(
      "margin-left: var(--neck-gap",
    );
  });

  it("颈部端面内缩一个圆角半径：圆弧保留、圆角与颈部之间不漏月牙", () => {
    const neck = blockAfter(tailwindCss, ".message-center-tab__neck {");

    // 段的圆角让段边只剩中间一段是直线；端面顶到条带上下沿就会压在圆弧外面，
    // 圆弧与端面之间必然空出月牙形页底色（深墨块旁边那块浅色楔形）。
    expect(neck).toContain("top: var(--neck-inset");
    expect(neck).toContain("bottom: var(--neck-inset");
  });

  it("分段条是裸轨道：容器不再带边框 / 内边距 / 底色（返工点）", () => {
    const subnav = blockAfter(tailwindCss, ".message-center__subnav {");

    // 以前是"带边线与 p-1 的圆角盒子"，五段被框在里面、接缝张开时露的是盒子自己的纸面，
    // 于是怎么调都不像参考的一体化分段条。容器只负责排布与横向滚动。
    expect(subnav).not.toContain("border");
    expect(subnav).not.toMatch(/\bp[xytblr]?-[0-9]/u);
    expect(subnav).not.toContain("background");
  });

  it("未激活段自带轨道面；贴合侧不带圆角（圆角只由组件的弹簧按敞口给）", () => {
    const segment = blockAfter(tailwindCss, ".message-center__subnav-segment {");

    expect(segment).toContain("background: var(--color-surface-raised)");
    // 静态兜底恒为方角：贴合处的圆角会在相邻两段之间啃出缺口，
    // 只有组件按"这条接缝是否张开"给的那份行内样式才是真圆角。
    expect(segment).toContain("border-radius: 0");
    expect(blockAfter(tailwindCss, ".message-center__subnav-segment:first-child {")).toContain(
      "border-top-left-radius: var(--radius-md)",
    );
  });

  it("段内边距走参考的 md 节奏：px-5 py-2.5、链接继承段的圆角", () => {
    const tab = blockAfter(tailwindCss, ".message-center-tab {");

    expect(tab).toContain("px-5");
    expect(tab).toContain("py-2.5");
    expect(tab).toContain("border-radius: inherit");
  });

  it("激活块内的未读角标是半透明反白底（票 297）", () => {
    const badge = blockAfter(tailwindCss, ".message-center-tab.is-active .message-center-tab__count {");

    expect(badge).toContain("color-mix(");
    expect(badge).toContain("var(--color-on-inverse)");
    expect(badge).toContain("color: var(--color-on-inverse)");
  });

  it("五个子项共用顶部导航条 + 单一内容卡，不再是 220px 左导航", () => {
    const layout = blockAfter(tailwindCss, ".message-center__layout {");
    const subnav = blockAfter(tailwindCss, ".message-center__subnav {");

    expect(layout).toContain("flex flex-col");
    expect(layout).not.toContain("220px");
    expect(subnav).toContain("overflow-x: auto");
    expect(pageSource).toContain('class="surface-card message-center__content"');
    expect(pageSource).toContain("InboxSubNav");
  });

  it("页面不再把实现说明讲给用户（说明文案整体退役）", () => {
    const sources = [pageSource, read("app/components/inbox/InboxChatPanel.vue"), read("app/components/inbox/InboxMessageList.vue")];
    const banned = [
      "评论、互动、系统与全局通知",
      "HTTP 收发与落库",
      "WebSocket 只推",
      "预备会话",
      "首条消息发出后",
      "只保存在前端",
      "对象存储再发出",
      "这个会话还没有消息",
      "按时间倒序排列。",
      "实时状态",
    ];
    for (const phrase of banned) {
      for (const source of sources) {
        expect(source).not.toContain(phrase);
      }
    }
  });

  it("连接状态条只在异常时渲染（健康连接不显示）", () => {
    const panel = read("app/components/inbox/InboxChatPanel.vue");
    expect(panel).toContain('v-if="showSocketBadge"');
    expect(panel).toContain('socketStatus !== "connected"');
    expect(panel).not.toContain("实时已连接");
  });

  it("列表未读：左侧强调条 + 标题加粗 + 右上小圆点（不用 chip）", () => {
    const unread = blockAfter(tailwindCss, ".message-row.is-unread {");
    const unreadTitle = blockAfter(tailwindCss, ".message-row.is-unread .message-row__title {");
    const dot = blockAfter(tailwindCss, ".message-row__dot {");

    expect(unread).toContain("box-shadow: inset 2px 0 0 var(--color-ink)");
    expect(unreadTitle).toMatch(/font-weight:\s*650/u);
    expect(dot).toContain("background: var(--color-danger)");
    expect(dot).toContain("rounded-pill");
  });

  it("删除按钮只在 hover / focus 时出现（默认 opacity: 0）", () => {
    expect(blockAfter(tailwindCss, ".message-row__delete {")).toContain("opacity: 0");
    expect(blockAfter(tailwindCss, ".message-row:hover .message-row__delete,")).toContain("opacity: 1");
    expect(blockAfter(tailwindCss, ".message-row__delete:focus-visible {")).toContain("opacity: 1");
  });

  it("窄屏导航条仍是一行可横向滚动（不切回竖导航）", () => {
    const narrow = blockAfter(tailwindCss, "@media (max-width: 1023px)", true);

    expect(narrow).toContain(".message-center__layout");
    expect(narrow).toContain("grid-template-columns: minmax(0, 1fr)");
    expect(narrow).toContain(".message-center__subnav");
    expect(narrow).toContain("flex-direction: row");
    expect(narrow).toContain("overflow-x: auto");
  });

  it("页面不自带 <style>（样式只落在 tailwind.css / 组件）", () => {
    expect(pageSource).not.toContain("<style");
  });
});

describe("消息中心：路由与入口接线", () => {
  it("页面声明 auth 中间件 + 子项校验", () => {
    expect(pageSource).toContain("definePageMeta");
    expect(pageSource).toContain('"auth"');
    expect(pageSource).toContain("messages-tab");
  });

  it("页面用共享的子项表与容器组件，不自己重复定义五个入口", () => {
    expect(pageSource).toContain("messageCenterNavItems");
    expect(pageSource).toContain("InboxSubNav");
    expect(pageSource).toContain("InboxMessageList");
    expect(pageSource).toContain("useMessageCenter");
  });

  it("旧 /studio/inbox 与 /messages 都走同一个重定向中间件", () => {
    expect(redirectMiddleware).toContain("resolveMessageCenterRedirect");
    expect(redirectMiddleware).toContain("navigateTo");
    expect(legacyInboxPage).toContain("message-center-redirect");
  });

  it("/messages/[tab] 的非法子项会回落到默认子项", () => {
    expect(tabMiddleware).toContain("isMessageCenterTab");
    expect(tabMiddleware).toContain("/messages/comments");
  });

  it("顶栏 hover 面板与页面导航消费同一份子项表", () => {
    expect(headerSource).toContain("messageCenterNavItems");
    expect(headerSource).toContain("useMessageStore");
    expect(pageSource).toContain("messageCenterNavItems");
  });
});

describe("顶栏入口：三档窄屏规则", () => {
  it("≥1024px 显示「消息中心」文字；<1024px 仅图标 + 红点", () => {
    // 默认（窄屏）隐藏文字
    expect(headerSource).toMatch(/\.site-header__messages-label\s*\{\s*display:\s*none;/u);
    const desktop = blockAfter(headerSource, "@media (min-width: 1024px)");
    expect(desktop).toMatch(/\.site-header__messages-label\s*\{\s*display:\s*inline;/u);
    // 红点是 pill 圆点（不能用裸 50% 圆角逃逸圆角阶梯）
    expect(blockAfter(headerSource, ".site-header__dot {")).toContain("border-radius: var(--radius-pill)");
  });

  it("<640px 消息中心从动作区收进折叠面板（两种 DOM 靠断点切换，不复制逻辑）", () => {
    const mobile = blockAfter(headerSource, "@media (max-width: 639px)");

    expect(mobile).toContain(".site-messages {");
    expect(mobile).toContain("display: none");
    expect(mobile).toContain(".site-messages-mobile {");
    expect(mobile).toContain("display: flex");
    // 默认隐藏折叠面板里的入口；只在 <640 打开菜单时出现。
    expect(blockAfter(headerSource, ".site-messages-mobile {")).toContain("display: none");
  });

  it("hover 展开：面板由指针进入 / 键盘聚焦打开，离开 / 失焦收起", () => {
    expect(headerSource).toContain("@pointerenter");
    expect(headerSource).toContain("@pointerleave");
    expect(headerSource).toContain("@focusin");
    expect(headerSource).toContain("@focusout");
    expect(headerSource).toContain('role="menu"');
  });

  it("「历史」入口由 #241 落位：不再留空位，也不再渲染死链", () => {
    // #239 只留了结构位置（头像菜单选项是数组，历史项由 #241 插入，当时不能出现
    // /history 链接 —— 页面还不存在）。#241 落位后这条反转为"必须真的接上"：
    // 动作区普通链接 + 640–1279 头像菜单项 + <640 折叠面板，三档各一份。
    // 账号菜单后来为压页头行数外移到 useHeaderUserMenu，所以菜单项在那边断言。
    const userMenuSource = read("app/composables/useHeaderUserMenu.ts");
    expect(headerSource).toContain('"/history"');
    expect(userMenuSource).toContain("const userMenuOptions");
    expect(userMenuSource).toContain('label: "浏览历史"');
    expect(headerSource).toMatch(/\.site-header__history\s*\{\s*display:\s*none;/u);
    expect(headerSource).toContain(".site-history-mobile");
  });
});
