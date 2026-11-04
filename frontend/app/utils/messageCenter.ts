import type { GlobalNoticeItem, MessageSummary, SiteMessageItem } from "~/types/api";
import { firstQueryValue } from "~/utils/queryValue";

/**
 * 消息中心（#239）的纯逻辑：子项表、未读数、旧 query 映射、列表行视图模型。
 *
 * 为什么单独一张表：
 *   - 顶栏 hover 面板与页面左侧导航必须显示**同一组**子项与未读数，
 *     两处各写一遍 label / count 迟早漂移；
 *   - 旧 `/studio/inbox` 的 `tab` / `group` query 需要语义映射，
 *     这段判据要能被单测钉住，而不是埋在一个页面中间件里。
 *
 * 页面与中间件只负责取数 / 跳转，不重复定义子项与未读来源。
 */

export type MessageCenterTab = "comments" | "digg" | "system" | "global" | "chat";

/** 五个子项的固定顺序（顶栏与页面共用）。 */
export const MESSAGE_CENTER_TABS: readonly MessageCenterTab[] = [
  "comments",
  "digg",
  "system",
  "global",
  "chat",
] as const;

/** 每页条数：滚动加载每次请求 20 条。 */
export const MESSAGE_CENTER_PAGE_SIZE = 20;

const TAB_LABELS: Record<MessageCenterTab, string> = {
  comments: "评论与回复",
  digg: "点赞与收藏",
  system: "系统通知",
  global: "全局通知",
  chat: "私信",
};

/** 子项对应的顶级路径（路由契约的唯一来源）。 */
export function messageCenterPath(tab: MessageCenterTab): `/messages/${MessageCenterTab}` {
  return `/messages/${tab}`;
}

export function messageCenterTabLabel(tab: MessageCenterTab): string {
  return TAB_LABELS[tab];
}

/** 子项导航项：`count` 直接来自 `GET /api/sitemsg/user` 的五个字段。 */
export interface MessageCenterNavItem {
  key: MessageCenterTab;
  label: string;
  to: `/messages/${MessageCenterTab}`;
  count: number;
}

export function messageCenterNavItems(summary: MessageSummary): MessageCenterNavItem[] {
  const counts: Record<MessageCenterTab, number> = {
    comments: summary.comment_msg_count,
    digg: summary.digg_favor_msg_count,
    system: summary.system_msg_count,
    global: summary.global_msg_count,
    chat: summary.private_msg_count,
  };

  return MESSAGE_CENTER_TABS.map((key) => ({
    key,
    label: TAB_LABELS[key],
    to: messageCenterPath(key),
    count: counts[key] ?? 0,
  }));
}

/**
 * 未读数是否要显示数字。
 *
 * 「0 不显示数字」是明确的设计规则：数字 0 只会制造噪声，未读态本身靠
 * 列表行的强调条 / 圆点表达。判据放在这里，模板只做 `v-if`。
 */
export function shouldShowUnreadCount(count: number): boolean {
  return count > 0;
}

/** 子项 key 判据（中间件与页面共用）。 */
export function isMessageCenterTab(value: unknown): value is MessageCenterTab {
  return MESSAGE_CENTER_TABS.includes(firstQueryValue(value) as MessageCenterTab);
}

/** 路由参数 → 子项；无法识别一律回落到默认的「评论与回复」。 */
export function resolveMessageCenterTab(value: unknown): MessageCenterTab {
  const raw = firstQueryValue(value);
  return isMessageCenterTab(raw) ? raw : "comments";
}

/** 站内信分组编号（1 评论 / 2 互动 / 3 系统）；全局通知与私信不走这个接口。 */
export function siteMessageGroupOf(tab: MessageCenterTab): 1 | 2 | 3 | null {
  switch (tab) {
    case "comments":
      return 1;
    case "digg":
      return 2;
    case "system":
      return 3;
    default:
      return null;
  }
}

/** 列表行的统一视图模型：标题 + 一行摘要 + 时间 + 未读 + 原文链接。 */
export interface MessageCenterEntry {
  id: string;
  title: string;
  summary: string;
  createdAt: string;
  href: string;
  unread: boolean;
}

/** 站内信 → 列表行。标题优先文章标题，系统消息没有文章标题时回落到链接标题。 */
export function toSiteMessageEntry(item: SiteMessageItem): MessageCenterEntry {
  return {
    id: item.id,
    title: item.article_title || item.link_title || "站内通知",
    summary: item.content,
    createdAt: item.created_at,
    href: item.link_herf || (item.article_id ? `/article/${item.article_id}` : ""),
    unread: !item.is_read,
  };
}

/** 全局通知 → 列表行（字段名沿用后端历史契约：`create_at` / `herf`）。 */
export function toGlobalNoticeEntry(item: GlobalNoticeItem): MessageCenterEntry {
  return {
    id: item.id,
    title: item.title,
    summary: item.content,
    createdAt: item.create_at,
    href: item.herf,
    unread: !item.is_read,
  };
}

/** 丢掉 tab / group，其余 query 原样带过去（私信草稿 `draft_*` 靠这一步保留）。 */
function preserveUnrelatedQuery(query: Record<string, unknown>): Record<string, string> {
  const preserved: Record<string, string> = {};
  for (const [key, value] of Object.entries(query)) {
    if (key === "tab" || key === "group") continue;
    const raw = firstQueryValue(value);
    if (raw) preserved[key] = raw;
  }
  return preserved;
}

/**
 * 旧 `/studio/inbox?tab=site|chat&group=1|2|3|global` → 新子项。
 *
 * 旧页面里 `tab=chat` 优先于 `group`（site tab 才看 group），映射必须保持一致，
 * 否则收藏在旧链接里的书签会落进错误的分组。
 */
export function resolveLegacyInboxLocation(query: Record<string, unknown>): {
  path: `/messages/${MessageCenterTab}`;
  query: Record<string, string>;
} {
  const tab = firstQueryValue(query.tab);
  const group = firstQueryValue(query.group);
  let target: MessageCenterTab = "comments";

  if (tab === "chat") {
    target = "chat";
  } else if (group === "global") {
    target = "global";
  } else if (group === "2") {
    target = "digg";
  } else if (group === "3") {
    target = "system";
  }

  return { path: messageCenterPath(target), query: preserveUnrelatedQuery(query) };
}

/**
 * 路由级重定向（页面中间件消费）。
 *
 * `/studio/inbox` 是旧入口（按 query 映射）；`/messages` 本身没有内容，
 * 落到默认子项。其它路径返回 null，中间件据此直接放行。
 */
export function resolveMessageCenterRedirect(
  path: string,
  query: Record<string, unknown>,
): { path: `/messages/${MessageCenterTab}`; query: Record<string, string> } | null {
  if (path === "/studio/inbox") {
    return resolveLegacyInboxLocation(query);
  }
  if (path === "/messages") {
    return { path: messageCenterPath("comments"), query: preserveUnrelatedQuery(query) };
  }
  return null;
}
