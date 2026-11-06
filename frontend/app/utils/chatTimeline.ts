import type { ChatMessageItem } from "~/types/api";
import { resolveChatMessageContent, type ResolvedChatMessage } from "~/utils/chatMessage";

/**
 * 私信时间线的纯函数（唯一实现）。
 *
 * 后端 `GET /api/chat/messages` 直接返回**升序**（`send_time ASC, id ASC`，最旧在前），
 * 用 `(before_send_time, before_id)` 游标往前翻更早一页；界面从上到下原样渲染，
 * **不再有任何反转**。日期分组、2 分钟合并分组与回执也都在这里算好，交给面板
 * 只画结果 —— 组件里不再有第二份日期键或排序。
 *
 * 另一条硬约束：消息 id 是 FNV 哈希字符串，**不是**单调递增的数字。任何用
 * `Number(id)` 排序的逻辑都会把历史顺序打乱；这里只有「新追加的那一批」之间
 * 用 `send_time` 做个提示性的稳定排序，已累积的历史永远保持原样。
 */

/** 两分钟合并分组的窗口（用于气泡连排，不是分页窗口）。 */
const CHAT_GROUP_WINDOW_MS = 2 * 60 * 1000;

function pad2(value: number) {
  return String(value).padStart(2, "0");
}

/**
 * 解析后端时间字符串。RFC3339Nano（9 位小数秒）与不带小数秒的 RFC3339 都会进来，
 * 空串 / 非法串一律返回 null —— 调用方据此决定「不分组 / 不显示」。
 */
function parseChatTime(sendTime: string): Date | null {
  if (typeof sendTime !== "string" || !sendTime.trim()) {
    return null;
  }
  const timestamp = Date.parse(sendTime);
  if (!Number.isFinite(timestamp)) {
    return null;
  }
  return new Date(timestamp);
}

/** 本地时区的日历日 `YYYY-MM-DD`（不碰 UTC，跨午夜按用户所在时区判定）。 */
function localDateKey(date: Date) {
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`;
}

/** 时间串 → 本地日历日键；非法 / 空时间归到同一段「未知日期」。 */
function chatDayKey(sendTime: string): string {
  const date = parseChatTime(sendTime);
  return date ? localDateKey(date) : "unknown";
}

/**
 * 把一批新取到的消息合并进已累积的升序时间线。
 *
 * 规则（顺序很重要）：
 *   1. `current` 的每一项保持在原来的位置，绝不重排、绝不丢弃；
 *   2. id 相同则以 `incoming` 里的那一行**替换**（WS 通知带来的新鲜
 *      `is_read` / `msg_status` 就是这样落到旧行上的）；
 *   3. 只在 `current` 里做去重（重复 id 只留第一条）；
 *   4. 真正没见过的新行追加到**尾部**，彼此之间按 `send_time` 做稳定升序；
 *      时间相同或非法时保持 `incoming` 的原有次序；
 *   5. `incoming` 自身也按 id 去重（后出现的覆盖先出现的）。
 *
 * 乐观发送的 `temp-…` 行与服务端真实行 id 不同，因此不会被本函数替换：
 * 那一步由 composable 在 HTTP 成功回调里按 temp id 就地换掉。
 */
export function mergeChatMessages(
  current: readonly ChatMessageItem[],
  incoming: readonly ChatMessageItem[],
): ChatMessageItem[] {
  const incomingById = new Map<string, ChatMessageItem>();
  for (const item of incoming) {
    incomingById.set(item.id, item);
  }

  const seen = new Set<string>();
  const merged: ChatMessageItem[] = [];
  for (const item of current) {
    if (seen.has(item.id)) {
      continue;
    }
    seen.add(item.id);
    merged.push(incomingById.get(item.id) ?? item);
  }

  const fresh = [...incomingById.entries()]
    .filter(([id]) => !seen.has(id))
    .map(([, item], index) => ({
      item,
      index,
      time: parseChatTime(item.send_time)?.getTime() ?? null,
    }));

  // 只对「新追加的这一批」排序；已累积的行不参与比较。
  fresh.sort((left, right) => {
    if (left.time === null && right.time === null) {
      return left.index - right.index;
    }
    if (left.time === null) {
      return 1;
    }
    if (right.time === null) {
      return -1;
    }
    return left.time === right.time ? left.index - right.index : left.time - right.time;
  });

  return [...merged, ...fresh.map(({ item }) => item)];
}

/**
 * 相邻两条是否属于同一个「两分钟合并组」。
 *
 * 三个条件同时成立：同一个发送者、本地日历日相同、时间差绝对值不超过 2 分钟。
 * 用绝对值是为了让「传参顺序颠倒」也不影响判定（调用方本应按升序传入）；跨午夜
 * （23:59 → 00:01）即使只差 2 分钟也因为日历日不同而拆成两组。
 * 任一时间缺失/非法 → false（宁可多一个分组头，也不要把不确定的消息粘在一起）。
 */
export function isSameChatGroup(a: ChatMessageItem, b: ChatMessageItem): boolean {
  if (a.sender_id !== b.sender_id) {
    return false;
  }
  const dateA = parseChatTime(a.send_time);
  const dateB = parseChatTime(b.send_time);
  if (!dateA || !dateB) {
    return false;
  }
  if (localDateKey(dateA) !== localDateKey(dateB)) {
    return false;
  }
  return Math.abs(dateB.getTime() - dateA.getTime()) <= CHAT_GROUP_WINDOW_MS;
}

/**
 * 日期分隔条的文案：今天 / 昨天 / `M月D日` / 跨年时 `YYYY年M月D日`。
 *
 * 全部用本地时区的年月日判定；`now` 可注入以便测试。昨天是「`now` 的本地日历日
 * 减一天」，用 Date 构造而不是减 86400000 毫秒，才不会被夏令时切换带偏。
 * 时间非法时返回空串，由调用方跳过这个分隔条。
 */
export function chatDateSeparatorLabel(sendTime: string, now: Date = new Date()): string {
  const target = parseChatTime(sendTime);
  if (!target) {
    return "";
  }

  const targetKey = localDateKey(target);
  if (targetKey === localDateKey(now)) {
    return "今天";
  }
  const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
  if (targetKey === localDateKey(yesterday)) {
    return "昨天";
  }

  const month = target.getMonth() + 1;
  const day = target.getDate();
  if (target.getFullYear() !== now.getFullYear()) {
    return `${target.getFullYear()}年${month}月${day}日`;
  }
  return `${month}月${day}日`;
}

/**
 * 悬停 `title` 用的完整本地时间 `YYYY-MM-DD HH:mm:ss`。
 * 时间非法 → 空串（渲染出来的是空 title，而不是 `Invalid Date`）。
 */
export function fullChatTimestamp(sendTime: string): string {
  const date = parseChatTime(sendTime);
  if (!date) {
    return "";
  }
  return `${localDateKey(date)} ${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}`;
}

/** 日期分隔条（每个自然日一段）。 */
export interface ChatTimelineDateEntry {
  kind: "date";
  key: string;
  label: string;
}

/** 一个气泡：消息本体 + 已解析的内容 + 回执文案（只有最后一条自己发的才有）。 */
export interface ChatTimelineBubble {
  item: ChatMessageItem;
  resolved: ResolvedChatMessage;
  receipt: "" | "已读" | "已送达";
}

/** 一组连续气泡（同一发送人、同一天、2 分钟内）。 */
export interface ChatTimelineGroupEntry {
  kind: "group";
  key: string;
  isSelf: boolean;
  entries: ChatTimelineBubble[];
}

export type ChatTimelineEntry = ChatTimelineDateEntry | ChatTimelineGroupEntry;

/**
 * 把升序消息数组切成「日期分隔条 + 合并组」的时间线。
 *
 * 这是**唯一**的分组实现：面板只遍历结果，不再自己算日期键、分组或回执。
 * 不做任何排序（入参已由 `useInboxCenter` 维护为升序），也不修改入参。
 *
 * `now` 可注入，只为让「今天 / 昨天」的文案在测试里可复现；生产用当前时间。
 */
export function buildChatTimeline(
  items: readonly ChatMessageItem[],
  now: Date = new Date(),
): ChatTimelineEntry[] {
  // 只有「自己发的最后一条」保留回执：先倒着找一次，避免每条都扫全数组。
  let lastSelfMessageId: string | null = null;
  for (let i = items.length - 1; i >= 0; i -= 1) {
    if (items[i]?.is_self) {
      lastSelfMessageId = items[i]!.id;
      break;
    }
  }

  const out: ChatTimelineEntry[] = [];
  let lastDay = "";
  let lastItem: ChatMessageItem | null = null;
  let group: ChatTimelineGroupEntry | null = null;

  for (const item of items) {
    const day = chatDayKey(item.send_time);
    if (day !== lastDay) {
      out.push({ kind: "date", key: `date-${day}-${item.id}`, label: chatDateSeparatorLabel(item.send_time, now) });
      lastDay = day;
      lastItem = null;
      group = null;
    }

    const sameGroup = lastItem !== null && group !== null && isSameChatGroup(lastItem, item);
    if (!sameGroup) {
      group = { kind: "group", key: `group-${item.id}`, isSelf: Boolean(item.is_self), entries: [] };
      out.push(group);
    }

    group!.entries.push({
      item,
      resolved: resolveChatMessageContent(item),
      receipt: item.id === lastSelfMessageId ? (item.is_read ? "已读" : "已送达") : "",
    });
    lastItem = item;
  }

  return out;
}
