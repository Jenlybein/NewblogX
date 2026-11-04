import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, stripComments } from "../helpers/designFiles";

/**
 * 私信并入消息中心（票 #240）的接线与生命周期门禁。
 *
 * 这些验收点不适合挂载断言：
 *   - 私信页复用哪个组件 / 哪份取数（源码接线）；
 *   - `<1024px` 二选一是 CSS（隐藏另一栏 + 返回按钮）；
 *   - WS 生命周期不再是"页面级"（源码里不能有 onBeforeUnmount 关连接）；
 *   - 死引用清理（InboxSitePanel 文件已删、useInboxCenter 不再取站内信）。
 */

const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

const tailwindCss = read("app/assets/css/tailwind.css");
const pageSource = read("app/pages/messages/[tab].vue");
const chatViewSource = read("app/components/inbox/InboxChatView.vue");
const inboxCenterSource = read("app/composables/useInboxCenter.ts");
const chatSocketSource = read("app/composables/useChatSocket.ts");
const realtimeSource = read("app/composables/useMessageRealtime.ts");
const profileSource = read("app/pages/users/[id].vue");
const articleSource = read("app/pages/article/[id].vue");
const draftJumpSource = read("app/composables/useDraftChatJump.ts");
const chatUtilsSource = read("app/utils/chat.ts");
const sidebarSource = read("app/components/studio/StudioSidebarNav.vue");
const pluginPath = "app/plugins/02.message-realtime.client.ts";

describe("私信子项：/messages/chat 的右内容", () => {
  it("复用现存的私信面板与会话 composable，不再留占位空壳", () => {
    expect(pageSource).toContain("InboxChatView");
    expect(chatViewSource).toContain("InboxChatPanel");
    expect(chatViewSource).toContain("useInboxCenter");
    expect(pageSource).not.toContain("会话列表与聊天窗会在这里展开");
  });

  it("草稿直达会话：页面消费 draft_receiver_* 的解析结果，而不是手抄字段", () => {
    expect(chatViewSource).toContain("parseDraftSessionSeed");
    // 首条消息发出后要清掉 query，否则刷新页面又落回"预备会话"。
    expect(chatViewSource).toContain("clearDraftSessionQuery");
    expect(chatViewSource).toContain("draft_receiver_id");
  });

  it("介绍文案不再说私信是空壳（五子项都真的可用）", () => {
    expect(pageSource).toContain("私信");
    expect(pageSource).toContain("InboxSubNav");
    expect(pageSource).toContain("InboxMessageList");
  });
});

describe("窄屏二选一（<1024px 列表 ↔ 聊天窗）", () => {
  it("窄屏隐藏另一栏：列表态藏聊天窗，聊天态藏会话列表", () => {
    const narrow = blockAfter(tailwindCss, "@media (max-width: 1024px)", true);

    expect(narrow).toContain(".studio-inbox-grid.is-narrow-list .studio-inbox-grid__main");
    expect(narrow).toContain(".studio-inbox-grid.is-narrow-chat .studio-inbox-grid__aside");
    expect(narrow).toContain("display: none");
    // 返回列表的按钮只在窄屏出现（宽屏双栏常显）。
    expect(narrow).toContain(".studio-chat-back");
    expect(blockAfter(tailwindCss, ".studio-chat-back {")).toContain("display: none");
  });
});

describe("未读订阅与 WS 生命周期回到全局", () => {
  it("连接不再绑在私信页：useChatSocket 里没有 onBeforeUnmount 关连接", () => {
    const source = stripComments(chatSocketSource);

    expect(source).not.toContain("onBeforeUnmount");
    // 连接与通知订阅解耦：消费方订阅，连接属于登录会话。
    expect(source).toContain("onNotification");
  });

  it("app 插件在登录后建连 / 登出断开（任何页面都能收到未读 +1）", () => {
    expect(existsSync(join(projectDir, pluginPath))).toBe(true);
    const plugin = read(pluginPath);

    expect(plugin).toContain("useMessageRealtime");
  });

  it("useMessageRealtime：订阅私信通知并盯登录态", () => {
    expect(realtimeSource).toContain("isNewChatMessageNotification");
    expect(realtimeSource).toContain("bumpPrivateUnread");
    expect(realtimeSource).toContain("isLoggedIn");
    expect(realtimeSource).toContain("socket.close()");
  });

  it("离开私信页只退订通知，不关连接（连接归登录会话）", () => {
    expect(inboxCenterSource).toContain("onScopeDispose");
    expect(inboxCenterSource).toContain("stopNotifications");
  });
});

describe("死引用清理", () => {
  it("InboxSitePanel 不再被任何页面引用，文件与其单测一并删除", () => {
    expect(existsSync(join(projectDir, "app/components/inbox/InboxSitePanel.vue"))).toBe(false);
    expect(existsSync(join(projectDir, "test/components/InboxSitePanel.test.ts"))).toBe(false);
  });

  it("useInboxCenter 只剩私信：不再取站内信 / 全局通知", () => {
    const source = stripComments(inboxCenterSource);

    expect(source).not.toContain("getSiteMessages");
    expect(source).not.toContain("getGlobalNotices");
    expect(source).not.toContain("activeSiteGroup");
    expect(source).not.toContain("siteCategories");
  });
});

describe("消息入口只有一个来源（顶栏）", () => {
  it("个人中心侧栏不再有消息 / 私信入口（同步到 #239 的迁移结果）", () => {
    const source = stripComments(sidebarSource);

    expect(source).not.toContain("/messages");
    expect(source).not.toContain("/studio/inbox");
  });
});

describe("draft_receiver_* 契约原样保留", () => {
  it("四个 draft 参数由 buildDraftChatQuery 单一来源生成", () => {
    for (const key of [
      "draft_receiver_id",
      "draft_receiver_nickname",
      "draft_receiver_avatar",
      "draft_relation",
    ]) {
      expect(chatUtilsSource).toContain(key);
    }
    expect(chatUtilsSource).toContain("export function buildDraftChatQuery");
    expect(draftJumpSource).toContain("buildDraftChatQuery");
    expect(draftJumpSource).toContain('path: "/messages/chat"');
  });

  it("作者主页与文章页两个入口都走 useDraftChatJump（不再手写 query）", () => {
    for (const [label, source] of [
      ["users/[id].vue", profileSource],
      ["article/[id].vue", articleSource],
    ] as const) {
      expect(source, label).toContain("useDraftChatJump");
      expect(source, label).toContain("jumpToDraftChat(");
      // 手写 query 会把契约抄成第二份；两个页面都不许再出现字段名。
      expect(source, label).not.toContain("draft_receiver_id:");
    }
  });

  it("文章页「私信」不再弹接入中 toast，真的跳到 /messages/chat", () => {
    expect(articleSource).not.toContain("私信页面接入中");
    expect(articleSource).not.toContain("后续会直接跳转");
    expect(articleSource).toContain("requireLogin");
    expect(articleSource).toContain("isSelfAuthor");
    expect(draftJumpSource).toContain("/messages/chat");
  });
});
