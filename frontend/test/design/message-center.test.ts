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
  it("子项选中态：抬升面底 + 左侧 2px 强调条 + 字重", () => {
    const selected = blockAfter(tailwindCss, ".message-center-tab.is-active {");

    expect(selected).toContain("background: var(--color-surface-raised)");
    expect(selected).toContain("box-shadow: inset 2px 0 0 var(--color-ink)");
    expect(selected).toMatch(/font-weight:\s*600/u);
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

  it("<1024px 左侧导航变顶部横向 tab", () => {
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
    expect(headerSource).toContain('"/history"');
    expect(headerSource).toContain("const userMenuOptions");
    expect(headerSource).toContain('label: "浏览历史"');
    expect(headerSource).toMatch(/\.site-header__history\s*\{\s*display:\s*none;/u);
    expect(headerSource).toContain(".site-history-mobile");
  });
});
