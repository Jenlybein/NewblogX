import { computed, shallowRef, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";

import {
  deleteGlobalNoticeForCurrentUser,
  deleteSiteMessage,
  getGlobalNotices,
  getSiteMessages,
  markGlobalNoticeRead,
  markSiteMessageRead,
} from "~/services/inbox";
import type { GlobalNoticeItem } from "~/types/api";
import { errorMessage } from "~/utils/errorMessage";
import {
  MESSAGE_CENTER_PAGE_SIZE,
  type MessageCenterEntry,
  type MessageCenterTab,
  siteMessageGroupOf,
  toGlobalNoticeEntry,
  toSiteMessageEntry,
} from "~/utils/messageCenter";

/**
 * 消息中心（#239）的取数与批量动作。
 *
 * 分工：
 *   - 五个子项的数据源不同 —— 评论 / 互动 / 系统走站内信（分组 1/2/3），
 *     全局通知走 `/api/global_notif`；
 *   - 私信子项已由 `InboxChatView` / `useInboxCenter` 接管（#240）：本 composable
 *     不再取私信列表，只保留 `chat` 的 tab 位置与未读汇总刷新；
 *   - 站内信支持"按分组标记已读 / 按分组删除"；全局通知只有"按 id 列表"的接口，
 *     所以"全部已读 / 清空"要先把全部 id 翻页收集齐，再一次性提交；
 *   - 每次写入后都刷新 `GET /api/sitemsg/user` 的未读汇总，保证顶栏与页面角标同步归零；
 *   - 进入四个非聊天子项时，首屏第一页加载成功后立即按子项批量自动已读（Q23/Q27/Q28），
 *     再刷新未读汇总并重拉第一页清掉行未读态；「全部已读」按钮保留为手动兜底。
 *     不引入轮询 / WebSocket，也不在 `loadMore()` 时重复触发。
 */

/** 全局通知没有"按分组"接口：翻页收集 id 时用后端允许的单页上限（100）。 */
const GLOBAL_SCAN_PAGE_SIZE = 100;
/** 批量删除的提交上限（与后端单页上限同档，避免一次塞入过大的 id 列表）。 */
const GLOBAL_DELETE_CHUNK_SIZE = 100;

interface UseMessageCenterOptions {
  tab: MaybeRefOrGetter<MessageCenterTab>;
}

interface PagePayload {
  entries: MessageCenterEntry[];
  hasMore: boolean;
}

export function useMessageCenter(options: UseMessageCenterOptions) {
  const messageStore = useMessageStore();

  const activeTab = computed(() => toValue(options.tab));
  const items = shallowRef<MessageCenterEntry[]>([]);
  const pending = shallowRef(false);
  const hasMore = shallowRef(false);
  const page = shallowRef(1);
  const error = shallowRef("");

  // 请求序号：切换子项后，上一个子项在途的响应必须被丢弃，否则会短暂显示错分组的数据。
  let requestId = 0;
  // 「进入子项」代次：watch 每触发一次自增。自动已读（含它之后的汇总刷新 / 重拉第一页）
  // 必须确认代次仍是自己那次，否则旧子项在途的自动已读会改掉新子项的行 / 未读角标。
  let entryGeneration = 0;
  // 最近一次首屏请求的结果：只有"第一页确实成功、且仍是当前请求"才允许自动已读，
  // 首屏接口失败时不能假装成功去发批量已读（也不能二次重拉把 error 清掉）。
  let firstPageStatus: { requestId: number; ok: boolean } = { requestId: -1, ok: false };

  async function fetchPage(tab: MessageCenterTab, pageNumber: number): Promise<PagePayload> {
    if (tab === "global") {
      const payload = await getGlobalNotices({ page: pageNumber, limit: MESSAGE_CENTER_PAGE_SIZE, type: 1 });
      return { entries: payload.list.map(toGlobalNoticeEntry), hasMore: payload.has_more };
    }

    const group = siteMessageGroupOf(tab);
    if (!group) {
      return { entries: [], hasMore: false };
    }

    const payload = await getSiteMessages({ group, page: pageNumber, limit: MESSAGE_CENTER_PAGE_SIZE });
    return { entries: payload.list.map(toSiteMessageEntry), hasMore: payload.has_more };
  }

  /** 拉第一页（切换子项 / 批量动作后的统一出口）。 */
  async function loadFirstPage() {
    const tab = activeTab.value;
    if (tab === "chat") {
      items.value = [];
      hasMore.value = false;
      page.value = 1;
      error.value = "";
      firstPageStatus = { requestId: -1, ok: false };
      return [];
    }

    const currentRequest = (requestId += 1);
    firstPageStatus = { requestId: currentRequest, ok: false };
    pending.value = true;
    error.value = "";
    try {
      const payload = await fetchPage(tab, 1);
      if (currentRequest !== requestId) return [];
      items.value = payload.entries;
      hasMore.value = payload.hasMore;
      page.value = 1;
      firstPageStatus = { requestId: currentRequest, ok: true };
      return payload.entries;
    } catch (reason) {
      if (currentRequest === requestId) {
        error.value = errorMessage(reason, "消息加载失败");
        items.value = [];
        hasMore.value = false;
      }
      return [];
    } finally {
      if (currentRequest === requestId) {
        pending.value = false;
      }
    }
  }

  /** 滚动到底部时追加下一页（每页 20）。 */
  async function loadMore() {
    const tab = activeTab.value;
    if (tab === "chat" || pending.value || !hasMore.value) {
      return [];
    }

    const currentRequest = (requestId += 1);
    pending.value = true;
    try {
      const payload = await fetchPage(tab, page.value + 1);
      if (currentRequest !== requestId) return [];
      items.value = [...items.value, ...payload.entries];
      hasMore.value = payload.hasMore;
      page.value += 1;
      return payload.entries;
    } catch (reason) {
      if (currentRequest === requestId) {
        error.value = errorMessage(reason, "消息加载失败");
      }
      return [];
    } finally {
      if (currentRequest === requestId) {
        pending.value = false;
      }
    }
  }

  /**
   * 全局通知没有分组接口：从已加载的第一页起，把后续页的 id 也收齐。
   *
   * `unreadOnly` 给"全部已读"用（只标记未读），否则给"清空当前分组"用（全部删除）。
   * 已加载的条目已经带 `unread` 状态，直接从内存收；后续页用后端单页上限翻。
   */
  async function collectGlobalIds(options: { unreadOnly: boolean }): Promise<string[]> {
    const ids = new Set<string>();
    const keep = (entry: MessageCenterEntry | GlobalNoticeItem, unread: boolean) =>
      !options.unreadOnly || unread;

    for (const entry of items.value) {
      if (keep(entry, entry.unread)) ids.add(entry.id);
    }

    // 第一页就 has_more=false 时，内存里已经是全量：不再多发一次翻页请求。
    if (!hasMore.value) {
      return [...ids];
    }

    let nextPage = page.value + 1;
    for (;;) {
      const payload = await getGlobalNotices({ page: nextPage, limit: GLOBAL_SCAN_PAGE_SIZE, type: 1 });
      for (const item of payload.list) {
        if (keep(item, !item.is_read)) ids.add(item.id);
      }
      if (!payload.has_more || payload.list.length === 0) break;
      nextPage += 1;
    }

    return [...ids];
  }

  async function refreshSummary() {
    await messageStore.refreshSummary().catch(() => undefined);
  }

  /** 全部已读（手动兜底按钮）：站内信按分组建走；全局通知收集未读 id 后批量标记。 */
  async function markAllRead() {
    const tab = activeTab.value;
    if (tab === "chat") return;

    if (tab === "global") {
      const ids = await collectGlobalIds({ unreadOnly: true });
      // 没有未读就不发请求，也不刷新列表：界面本来就没有变化。
      if (!ids.length) return;
      await markGlobalNoticeRead(ids);
    } else {
      const group = siteMessageGroupOf(tab);
      if (!group) return;
      await markSiteMessageRead({ group });
    }

    await Promise.all([refreshSummary(), loadFirstPage()]);
  }

  /**
   * 进入子项后的自动已读（Q23/Q27/Q28）：首屏第一页成功后调用一次。
   *
   * - 与「全部已读」复用同一组接口：站内信 `markSiteMessageRead({ group })`；
   *   全局通知用 `collectGlobalIds({ unreadOnly: true })` 收齐未读 id 再
   *   `markGlobalNoticeRead(ids)`。服务端幂等，重复提交无副作用；
   * - `chat` 完全跳过；
   * - 失败静默重试**同一个批量请求**一次，仍失败就放弃（不抛错 / 不 toast / 不 prompt）；
   * - `generation` + `firstPageRequestId` 双保险：切页后旧子项不得再提交批量已读，
   *   也不得用旧结果 `refreshSummary()` / `loadFirstPage()` 覆盖新子项的行与角标；
   * - 成功、无未读、或两次失败，都照常刷新汇总 + 重拉第一页，让行 `unread` 与角标归零。
   */
  async function autoReadOnEntry(tab: MessageCenterTab, generation: number, firstPageRequestId: number) {
    if (tab === "chat") return;
    const isCurrent = () => generation === entryGeneration && firstPageRequestId === requestId;

    // 一次完整的"组装 + 提交"尝试。站内信就是按 group 的批量调用；
    // 全局通知先用同一份 100/页扫描收齐未读 id，再按列表提交。
    const attempt = async () => {
      if (tab === "global") {
        const ids = await collectGlobalIds({ unreadOnly: true });
        if (!isCurrent() || !ids.length) return;
        await markGlobalNoticeRead(ids);
        return;
      }
      const group = siteMessageGroupOf(tab);
      if (!group || !isCurrent()) return;
      await markSiteMessageRead({ group });
    };

    try {
      await attempt();
    } catch {
      // 静默重试一次：不抛错、不 toast、不 prompt。
      if (isCurrent()) {
        try {
          await attempt();
        } catch {
          // 仍失败就放弃；下面的汇总刷新 + 重拉第一页照常进行，UI 不会停在坏状态。
        }
      }
    }

    if (!isCurrent()) return;
    await Promise.all([refreshSummary(), loadFirstPage()]);
  }

  /** 清空当前分组（不可逆，页面负责二次确认）。站内信按分组删；全局通知按 id 列表删。 */
  async function clearCurrentGroup() {
    const tab = activeTab.value;
    if (tab === "chat") return;

    if (tab === "global") {
      const ids = await collectGlobalIds({ unreadOnly: false });
      if (!ids.length) return;
      for (let index = 0; index < ids.length; index += GLOBAL_DELETE_CHUNK_SIZE) {
        await deleteGlobalNoticeForCurrentUser(ids.slice(index, index + GLOBAL_DELETE_CHUNK_SIZE));
      }
    } else {
      const group = siteMessageGroupOf(tab);
      if (!group) return;
      await deleteSiteMessage({ group });
    }

    await Promise.all([refreshSummary(), loadFirstPage()]);
  }

  /** 单条删除。 */
  async function removeEntry(id: string) {
    const tab = activeTab.value;
    if (tab === "chat") return;

    if (tab === "global") {
      await deleteGlobalNoticeForCurrentUser([id]);
    } else {
      await deleteSiteMessage({ id });
    }

    await Promise.all([refreshSummary(), loadFirstPage()]);
  }

  // 进入子项 / 切换子项：刷新未读数（顶栏订阅同一个 store）+ 拉第一页；
  // 第一页成功后立即自动批量已读，再刷新一次未读、重拉第一页清掉行未读态。
  watch(
    activeTab,
    async (tab) => {
      const generation = ++entryGeneration;
      await Promise.all([refreshSummary(), loadFirstPage()]);
      // 首屏失败、期间切页、或期间有别的列表请求（loadMore / 批量动作）时都不自动已读：
      // 这是防止"旧子项在途的自动已读清掉新子项行状态"的第一道闸。
      if (tab === "chat" || generation !== entryGeneration) return;
      if (!firstPageStatus.ok || firstPageStatus.requestId !== requestId) return;
      await autoReadOnEntry(tab, generation, firstPageStatus.requestId);
    },
    { immediate: true },
  );

  return {
    activeTab,
    items,
    pending,
    hasMore,
    page,
    error,
    loadFirstPage,
    loadMore,
    markAllRead,
    clearCurrentGroup,
    removeEntry,
  };
}
