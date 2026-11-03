import { describe, expect, it } from "vitest";

import {
  MESSAGE_CENTER_PAGE_SIZE,
  MESSAGE_CENTER_TABS,
  messageCenterNavItems,
  messageCenterPath,
  resolveLegacyInboxLocation,
  resolveMessageCenterRedirect,
  resolveMessageCenterTab,
  shouldShowUnreadCount,
  toGlobalNoticeEntry,
  toSiteMessageEntry,
} from "~/utils/messageCenter";
import type { GlobalNoticeItem, MessageSummary, SiteMessageItem } from "~/types/api";

/**
 * 消息中心（#239）的纯逻辑：
 *   1. 五个子项是一张固定的表（顺序 / 路径 / 未读来源），顶栏与页面共用；
 *   2. 旧 `/studio/inbox` 的 `tab` / `group` query 语义要映射成新子项，不能 404；
 *   3. 未读数 0 时不显示数字（视觉规则落成判据，而不是散在各模板里）。
 *
 * 这些判据不碰 DOM、不碰接口：顶部导航与页面内导航必须用同一份数据，
 * 靠"两处各自写一遍 label/count"迟早会漂移。
 */

const summary: MessageSummary = {
  comment_msg_count: 3,
  digg_favor_msg_count: 0,
  private_msg_count: 2,
  system_msg_count: 5,
  global_msg_count: 1,
};

const siteItem: SiteMessageItem = {
  id: "m1",
  created_at: "2026-11-01T10:00:00.000Z",
  updated_at: "2026-11-01T10:00:00.000Z",
  type: 1,
  receiver_id: "u1",
  action_user_id: "u2",
  action_user_nickname: "River",
  action_user_avatar: "",
  content: "评论了你的文章",
  article_id: "a1",
  comment_id: "c1",
  article_title: "一篇被评论的文章",
  link_title: "",
  link_herf: "",
  is_read: false,
  read_at: null,
};

const globalNotice: GlobalNoticeItem = {
  id: "g1",
  create_at: "2026-11-02T09:30:00.000Z",
  title: "全局公告",
  icon: "",
  content: "站点维护通知",
  herf: "/article/a9",
  is_read: false,
};

describe("消息中心：五子项表", () => {
  it("顺序固定为 评论与回复 → 点赞与收藏 → 系统通知 → 全局通知 → 私信", () => {
    expect(MESSAGE_CENTER_TABS).toEqual(["comments", "digg", "system", "global", "chat"]);
    expect(MESSAGE_CENTER_TABS.map(messageCenterPath)).toEqual([
      "/messages/comments",
      "/messages/digg",
      "/messages/system",
      "/messages/global",
      "/messages/chat",
    ]);
  });

  it("每页 20 条（滚动加载的请求口径）", () => {
    expect(MESSAGE_CENTER_PAGE_SIZE).toBe(20);
  });

  it("未读数来自 GET /api/sitemsg/user 的五个字段，顺序与子项一一对应", () => {
    const items = messageCenterNavItems(summary);

    expect(items.map((item) => item.label)).toEqual([
      "评论与回复",
      "点赞与收藏",
      "系统通知",
      "全局通知",
      "私信",
    ]);
    expect(items.map((item) => item.count)).toEqual([3, 0, 5, 1, 2]);
    expect(items.map((item) => item.key)).toEqual([...MESSAGE_CENTER_TABS]);
  });

  it("未读数 0 不显示数字（正数才显示）", () => {
    expect(shouldShowUnreadCount(0)).toBe(false);
    expect(shouldShowUnreadCount(-1)).toBe(false);
    expect(shouldShowUnreadCount(1)).toBe(true);
  });

  it("未知子项一律回落到默认的「评论与回复」", () => {
    expect(resolveMessageCenterTab("global")).toBe("global");
    expect(resolveMessageCenterTab(["digg"])).toBe("digg");
    expect(resolveMessageCenterTab(undefined)).toBe("comments");
    expect(resolveMessageCenterTab("history")).toBe("comments");
  });
});

describe("旧 /studio/inbox 的 query 语义映射", () => {
  it("tab=chat → /messages/chat（group 同时存在时忽略 group）", () => {
    expect(resolveLegacyInboxLocation({ tab: "chat", group: "2" })).toEqual({
      path: "/messages/chat",
      query: {},
    });
  });

  it("group 分别映射到 comments / digg / system / global", () => {
    expect(resolveLegacyInboxLocation({ tab: "site", group: "1" }).path).toBe("/messages/comments");
    expect(resolveLegacyInboxLocation({ group: "2" }).path).toBe("/messages/digg");
    expect(resolveLegacyInboxLocation({ group: "3" }).path).toBe("/messages/system");
    expect(resolveLegacyInboxLocation({ group: "global" }).path).toBe("/messages/global");
    expect(resolveLegacyInboxLocation({}).path).toBe("/messages/comments");
  });

  it("无法识别的 group 回落到默认子项，不抛错", () => {
    expect(resolveLegacyInboxLocation({ group: "99" }).path).toBe("/messages/comments");
    expect(resolveLegacyInboxLocation({ group: ["2"] }).path).toBe("/messages/digg");
  });

  it("保留草稿 query（从作者主页点「发私信」会带 draft_*），丢掉 tab / group", () => {
    expect(
      resolveLegacyInboxLocation({
        tab: "chat",
        group: "global",
        draft_receiver_id: "u9",
        draft_receiver_nickname: "乙",
        draft_relation: "2",
        draft_receiver_avatar: "",
      }),
    ).toEqual({
      path: "/messages/chat",
      query: {
        draft_receiver_id: "u9",
        draft_receiver_nickname: "乙",
        draft_relation: "2",
      },
    });
  });
});

describe("顶级页 /messages 的入口重定向", () => {
  it("/studio/inbox 走 legacy query 映射", () => {
    expect(resolveMessageCenterRedirect("/studio/inbox", { tab: "chat" })).toEqual({
      path: "/messages/chat",
      query: {},
    });
  });

  it("/messages 落到默认子项，并保留非 tab/group 的 query", () => {
    expect(resolveMessageCenterRedirect("/messages", { from: "header" })).toEqual({
      path: "/messages/comments",
      query: { from: "header" },
    });
  });

  it("其它路径不是消息中心的重定向职责", () => {
    expect(resolveMessageCenterRedirect("/article/a1", {})).toBeNull();
  });
});

describe("列表行：标题 / 摘要 / 时间 / 未读", () => {
  it("站内信取文章标题，没有文章标题时回落到链接标题", () => {
    expect(toSiteMessageEntry(siteItem)).toEqual({
      id: "m1",
      title: "一篇被评论的文章",
      summary: "评论了你的文章",
      createdAt: "2026-11-01T10:00:00.000Z",
      href: "/article/a1",
      unread: true,
    });

    expect(toSiteMessageEntry({ ...siteItem, article_title: "", link_title: "系统提醒" }).title).toBe("系统提醒");
    expect(toSiteMessageEntry({ ...siteItem, article_title: "", link_title: "", article_id: "" }).title).toBe("站内通知");
  });

  it("链接优先用后端给的 link_herf，没有就按文章 id 拼", () => {
    expect(toSiteMessageEntry({ ...siteItem, link_herf: "/article/a1#c1" }).href).toBe("/article/a1#c1");
    expect(toSiteMessageEntry({ ...siteItem, article_id: "" }).href).toBe("");
  });

  it("全局通知用 title / content / herf，已读不再是未读", () => {
    expect(toGlobalNoticeEntry(globalNotice)).toEqual({
      id: "g1",
      title: "全局公告",
      summary: "站点维护通知",
      createdAt: "2026-11-02T09:30:00.000Z",
      href: "/article/a9",
      unread: true,
    });
    expect(toGlobalNoticeEntry({ ...globalNotice, is_read: true }).unread).toBe(false);
  });
});
