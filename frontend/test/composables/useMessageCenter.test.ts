import { flushPromises } from "@vue/test-utils";
import { computed, ref, shallowRef, toValue, watch } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { GlobalNoticeItem, SiteMessageItem } from "~/types/api";

/**
 * 消息中心取数与批量动作（#239）。
 *
 * 为什么值得测：这里有一串"批量操作走哪个接口"的判据 —— 站内信可以按分组
 * 标记 / 删除，全局通知只能按 id 列表操作，私信子项本票不取数（#240 接入）。
 * 把它们埋进页面 handler 里，任何一次重构都可能悄悄把"清空分组"变成
 * "只清当前页"，而验收看不出来。
 *
 * Q23/Q27/Q28 追加：四个非聊天子项在**首屏第一页加载成功后**自动批量已读，
 * 再刷新未读汇总、重拉第一页；失败静默重试一次；切页时在途的旧自动已读
 * 不得改到新子项。这里把调用顺序、代次守卫、重试次数都钉住。
 *
 * 只 mock 服务层（HTTP 装配不在本票职责内）；Pinia store 用同名 stub 提供
 * `refreshSummary`，断言"每次写入后未读数都会刷新"。
 */

const getSiteMessagesMock = vi.hoisted(() => vi.fn());
const getGlobalNoticesMock = vi.hoisted(() => vi.fn());
const markSiteMessageReadMock = vi.hoisted(() => vi.fn());
const markGlobalNoticeReadMock = vi.hoisted(() => vi.fn());
const deleteSiteMessageMock = vi.hoisted(() => vi.fn());
const deleteGlobalNoticeMock = vi.hoisted(() => vi.fn());
const refreshSummaryMock = vi.hoisted(() => vi.fn());

vi.mock("~/services/inbox", () => ({
  getSiteMessages: getSiteMessagesMock,
  getGlobalNotices: getGlobalNoticesMock,
  markSiteMessageRead: markSiteMessageReadMock,
  markGlobalNoticeRead: markGlobalNoticeReadMock,
  deleteSiteMessage: deleteSiteMessageMock,
  deleteGlobalNoticeForCurrentUser: deleteGlobalNoticeMock,
}));

function siteItem(overrides: Partial<SiteMessageItem> = {}): SiteMessageItem {
  return {
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
    article_title: "被评论的文章",
    link_title: "",
    link_herf: "",
    is_read: false,
    read_at: null,
    ...overrides,
  };
}

function globalItem(overrides: Partial<GlobalNoticeItem> = {}): GlobalNoticeItem {
  return {
    id: "g1",
    create_at: "2026-11-02T09:30:00.000Z",
    title: "全局公告",
    icon: "",
    content: "站点维护通知",
    herf: "",
    is_read: false,
    ...overrides,
  };
}

beforeEach(() => {
  vi.resetModules();
  getSiteMessagesMock.mockReset();
  getGlobalNoticesMock.mockReset();
  markSiteMessageReadMock.mockReset();
  markGlobalNoticeReadMock.mockReset();
  deleteSiteMessageMock.mockReset();
  deleteGlobalNoticeMock.mockReset();
  refreshSummaryMock.mockReset().mockResolvedValue({});

  getSiteMessagesMock.mockResolvedValue({ list: [], has_more: false });
  getGlobalNoticesMock.mockResolvedValue({ list: [], has_more: false });

  vi.stubGlobal("computed", computed);
  vi.stubGlobal("ref", ref);
  vi.stubGlobal("shallowRef", shallowRef);
  vi.stubGlobal("toValue", toValue);
  vi.stubGlobal("watch", watch);
  vi.stubGlobal("useMessageStore", () => ({
    summary: {},
    refreshSummary: refreshSummaryMock,
  }));
});

async function createCenter(tab: "comments" | "digg" | "system" | "global" | "chat" = "comments") {
  const { useMessageCenter } = await import("~/composables/useMessageCenter");
  const tabRef = ref(tab);
  const center = useMessageCenter({ tab: tabRef });
  await flushPromises();
  return { center, tabRef };
}

/** 批量动作用例只关心按钮这次动作：先清掉"进入子项自动已读"留下的调用记录。 */
function clearEntryCalls() {
  getSiteMessagesMock.mockClear();
  getGlobalNoticesMock.mockClear();
  markSiteMessageReadMock.mockClear();
  markGlobalNoticeReadMock.mockClear();
  refreshSummaryMock.mockClear();
}

describe("消息中心取数", () => {
  it("进入子项即拉第一页（每页 20）并刷新未读汇总", async () => {
    getSiteMessagesMock.mockResolvedValue({ list: [siteItem()], has_more: true });

    const { center } = await createCenter("comments");

    expect(getSiteMessagesMock).toHaveBeenCalledWith({ group: 1, page: 1, limit: 20 });
    // 入场 1 次 + 自动已读收尾 1 次。
    expect(refreshSummaryMock).toHaveBeenCalledTimes(2);
    expect(center.items.value.map((item) => item.id)).toEqual(["m1"]);
    expect(center.hasMore.value).toBe(true);
    expect(center.page.value).toBe(1);
  });

  it("点赞与收藏 / 系统通知分别用分组 2 / 3，全局通知走 global_notif", async () => {
    await createCenter("digg");
    expect(getSiteMessagesMock).toHaveBeenCalledWith({ group: 2, page: 1, limit: 20 });
    expect(markSiteMessageReadMock).toHaveBeenCalledWith({ group: 2 });

    getSiteMessagesMock.mockClear();
    const global = await createCenter("global");
    expect(getSiteMessagesMock).not.toHaveBeenCalled();
    expect(getGlobalNoticesMock).toHaveBeenCalledWith({ page: 1, limit: 20, type: 1 });
    expect(global.center.items.value).toEqual([]);
  });

  it("私信子项不取列表（内容由 #240 接入），但仍刷新未读数", async () => {
    const { center } = await createCenter("chat");

    expect(getSiteMessagesMock).not.toHaveBeenCalled();
    expect(getGlobalNoticesMock).not.toHaveBeenCalled();
    expect(refreshSummaryMock).toHaveBeenCalledTimes(1);
    expect(center.items.value).toEqual([]);
  });

  it("切换子项会丢掉上一组数据并拉新分组", async () => {
    const { center, tabRef } = await createCenter("comments");
    getSiteMessagesMock.mockResolvedValue({ list: [siteItem({ id: "m2", type: 2 })], has_more: false });

    tabRef.value = "digg";
    await flushPromises();

    expect(getSiteMessagesMock).toHaveBeenLastCalledWith({ group: 2, page: 1, limit: 20 });
    expect(center.items.value.map((item) => item.id)).toEqual(["m2"]);
    expect(markSiteMessageReadMock).toHaveBeenLastCalledWith({ group: 2 });
  });

  it("滚动加载更多：只追加下一页，加载完不再请求", async () => {
    // 入场自动已读会重拉第一页：按 page 给稳定响应，避免一次性的
    // mockResolvedValueOnce 被自动已读消费掉。
    getSiteMessagesMock.mockImplementation(async ({ page }) =>
      page === 1
        ? { list: [siteItem({ id: "m1" })], has_more: true }
        : { list: [siteItem({ id: "m2" })], has_more: false },
    );

    const { center } = await createCenter("comments");
    expect(center.hasMore.value).toBe(true);

    await center.loadMore();
    expect(getSiteMessagesMock).toHaveBeenLastCalledWith({ group: 1, page: 2, limit: 20 });
    expect(center.items.value.map((item) => item.id)).toEqual(["m1", "m2"]);
    expect(center.hasMore.value).toBe(false);

    const callsBefore = getSiteMessagesMock.mock.calls.length;
    await center.loadMore();
    expect(getSiteMessagesMock).toHaveBeenCalledTimes(callsBefore);
  });

  it("接口失败落在 error 上，不把页面打崩", async () => {
    getSiteMessagesMock.mockRejectedValueOnce(new Error("网络错误"));

    const { center } = await createCenter("comments");

    expect(center.error.value).toBe("网络错误");
    expect(center.items.value).toEqual([]);
    expect(center.pending.value).toBe(false);
    // 首屏没成功就不自动已读，也不二次重拉把 error 抹掉。
    expect(markSiteMessageReadMock).not.toHaveBeenCalled();
    expect(getSiteMessagesMock).toHaveBeenCalledTimes(1);
  });
});

describe("进入子项自动已读（Q23/Q27/Q28）", () => {
  it("进入评论与回复：首屏成功后按 group 1 批量已读，再刷新未读 + 重拉第一页", async () => {
    const order: string[] = [];
    let pageCalls = 0;
    getSiteMessagesMock.mockImplementation(async () => {
      order.push("page");
      pageCalls += 1;
      return {
        list: [siteItem({ is_read: pageCalls > 1 })],
        has_more: false,
      };
    });
    markSiteMessageReadMock.mockImplementation(async () => {
      order.push("mark");
    });
    refreshSummaryMock.mockImplementation(async () => {
      order.push("summary");
      return {};
    });

    const { center } = await createCenter("comments");

    expect(order).toEqual(["summary", "page", "mark", "summary", "page"]);
    expect(getSiteMessagesMock).toHaveBeenNthCalledWith(1, { group: 1, page: 1, limit: 20 });
    expect(markSiteMessageReadMock).toHaveBeenCalledTimes(1);
    expect(markSiteMessageReadMock).toHaveBeenCalledWith({ group: 1 });
    expect(getSiteMessagesMock).toHaveBeenNthCalledWith(2, { group: 1, page: 1, limit: 20 });
    expect(refreshSummaryMock).toHaveBeenCalledTimes(2);
    // 重拉第一页后行未读态被清掉（is_read 由服务端返回 true）。
    expect(center.items.value.map((item) => item.unread)).toEqual([false]);
  });

  it.each([
    ["comments", 1],
    ["digg", 2],
    ["system", 3],
  ] as const)("进入 %s 自动标记分组 %i", async (tab, group) => {
    await createCenter(tab);
    expect(markSiteMessageReadMock).toHaveBeenCalledWith({ group });
    expect(markGlobalNoticeReadMock).not.toHaveBeenCalled();
  });

  it("进入全局通知：复用 100/页扫描收齐未读 id 后批量标记（不碰站内信接口）", async () => {
    getGlobalNoticesMock.mockImplementation(async ({ page, limit }) => {
      if (limit === 20) {
        return { list: [globalItem({ id: "g1" }), globalItem({ id: "g2", is_read: true })], has_more: true };
      }
      return page === 2
        ? { list: [globalItem({ id: "g3" })], has_more: false }
        : { list: [], has_more: false };
    });

    const { center } = await createCenter("global");

    expect(getGlobalNoticesMock).toHaveBeenCalledWith({ page: 1, limit: 20, type: 1 });
    expect(getGlobalNoticesMock).toHaveBeenCalledWith({ page: 2, limit: 100, type: 1 });
    expect(markGlobalNoticeReadMock).toHaveBeenCalledWith(["g1", "g3"]);
    expect(markSiteMessageReadMock).not.toHaveBeenCalled();
    expect(refreshSummaryMock).toHaveBeenCalledTimes(2);
    // 收尾重拉的仍是第一页（不是扫描页）。
    expect(getGlobalNoticesMock).toHaveBeenLastCalledWith({ page: 1, limit: 20, type: 1 });
    expect(center.items.value.map((item) => item.id)).toEqual(["g1", "g2"]);
  });

  it("loadMore() 不触发自动已读 / 不刷新未读汇总", async () => {
    getSiteMessagesMock.mockImplementation(async ({ page }) =>
      page === 1
        ? { list: [siteItem({ id: "m1" })], has_more: true }
        : { list: [siteItem({ id: "m2" })], has_more: false },
    );

    const { center } = await createCenter("comments");
    expect(markSiteMessageReadMock).toHaveBeenCalledTimes(1); // 只有入场那一次
    clearEntryCalls();

    await center.loadMore();

    expect(getSiteMessagesMock).toHaveBeenLastCalledWith({ group: 1, page: 2, limit: 20 });
    expect(markSiteMessageReadMock).not.toHaveBeenCalled();
    expect(markGlobalNoticeReadMock).not.toHaveBeenCalled();
    expect(refreshSummaryMock).not.toHaveBeenCalled();
    expect(center.items.value.map((item) => item.id)).toEqual(["m1", "m2"]);
  });

  it("快速切页：旧首屏迟到的响应不会自动已读旧分组，也不覆盖新子项的行", async () => {
    let resolveOldFirstPage: ((value: { list: SiteMessageItem[]; has_more: boolean }) => void) | undefined;
    getSiteMessagesMock.mockImplementation(({ group }: { group: number; page: number; limit: number }) => {
      if (group === 1) {
        return new Promise((resolve) => {
          resolveOldFirstPage = resolve;
        });
      }
      return Promise.resolve({ list: [siteItem({ id: "m2", type: 2 })], has_more: false });
    });

    const { center, tabRef } = await createCenter("comments");
    expect(markSiteMessageReadMock).not.toHaveBeenCalled(); // 旧首屏还没回来

    tabRef.value = "digg";
    await flushPromises();
    expect(getSiteMessagesMock).toHaveBeenLastCalledWith({ group: 2, page: 1, limit: 20 });
    expect(markSiteMessageReadMock).toHaveBeenCalledWith({ group: 2 });

    markSiteMessageReadMock.mockClear();
    resolveOldFirstPage?.({ list: [siteItem({ id: "m1" })], has_more: false });
    await flushPromises();

    expect(markSiteMessageReadMock).not.toHaveBeenCalled();
    expect(center.items.value.map((item) => item.id)).toEqual(["m2"]);
    // group 1 只发生过那一次在途的首屏请求，没有自动已读带来的二次请求。
    expect(getSiteMessagesMock.mock.calls.filter(([params]) => params.group === 1)).toHaveLength(1);
  });

  it("快速切页：旧自动已读在途时，迟到的结果不刷新角标 / 不重拉新子项", async () => {
    let resolveOldMark: (() => void) | undefined;
    markSiteMessageReadMock.mockImplementation(({ group }: { group: number }) => {
      if (group === 1) {
        return new Promise<void>((resolve) => {
          resolveOldMark = resolve;
        });
      }
      return Promise.resolve();
    });
    getSiteMessagesMock.mockImplementation(async ({ group }) => ({
      list: [siteItem({ id: group === 1 ? "m1" : "m2" })],
      has_more: false,
    }));

    const { center, tabRef } = await createCenter("comments");
    expect(markSiteMessageReadMock).toHaveBeenCalledTimes(1);
    expect(markSiteMessageReadMock).toHaveBeenCalledWith({ group: 1 });

    tabRef.value = "digg";
    await flushPromises();
    expect(markSiteMessageReadMock).toHaveBeenLastCalledWith({ group: 2 });
    expect(center.items.value.map((item) => item.id)).toEqual(["m2"]);

    clearEntryCalls();
    resolveOldMark?.();
    await flushPromises();

    expect(getSiteMessagesMock).not.toHaveBeenCalled();
    expect(refreshSummaryMock).not.toHaveBeenCalled();
    expect(markSiteMessageReadMock).not.toHaveBeenCalled();
    expect(center.items.value.map((item) => item.id)).toEqual(["m2"]);
  });

  it("快速切页：全局通知旧扫描迟到的 id 不会提交到新子项", async () => {
    let resolveScan: ((value: { list: GlobalNoticeItem[]; has_more: boolean }) => void) | undefined;
    getGlobalNoticesMock.mockImplementation(({ limit }: { page: number; limit: number; type: number }) => {
      if (limit === 20) {
        return Promise.resolve({ list: [globalItem({ id: "g1" })], has_more: true });
      }
      return new Promise((resolve) => {
        resolveScan = resolve;
      });
    });

    const { center, tabRef } = await createCenter("global");
    expect(markGlobalNoticeReadMock).not.toHaveBeenCalled(); // 100/页扫描还在途

    tabRef.value = "comments";
    await flushPromises();
    expect(markSiteMessageReadMock).toHaveBeenCalledWith({ group: 1 });

    clearEntryCalls();
    resolveScan?.({ list: [globalItem({ id: "g2" })], has_more: false });
    await flushPromises();

    expect(markGlobalNoticeReadMock).not.toHaveBeenCalled();
    expect(getGlobalNoticesMock).not.toHaveBeenCalled();
    expect(refreshSummaryMock).not.toHaveBeenCalled();
    expect(center.items.value).toEqual([]);
  });

  it("自动已读两次都失败：同一个批量请求静默重试一次，不抛错 / 不显示错误态", async () => {
    markSiteMessageReadMock
      .mockRejectedValueOnce(new Error("写入失败"))
      .mockRejectedValueOnce(new Error("又失败"));
    getSiteMessagesMock.mockResolvedValue({ list: [siteItem()], has_more: false });

    const { center } = await createCenter("comments");

    expect(markSiteMessageReadMock).toHaveBeenCalledTimes(2);
    expect(markSiteMessageReadMock).toHaveBeenNthCalledWith(1, { group: 1 });
    expect(markSiteMessageReadMock).toHaveBeenNthCalledWith(2, { group: 1 });
    expect(center.error.value).toBe(""); // 无 toast / 无 prompt / 不落到 error
    expect(refreshSummaryMock).toHaveBeenCalledTimes(2);
    expect(getSiteMessagesMock).toHaveBeenCalledTimes(2);
    expect(center.items.value.map((item) => item.id)).toEqual(["m1"]);
  });

  it("自动已读首次失败、重试成功：只提交两次并照常刷新", async () => {
    markSiteMessageReadMock.mockRejectedValueOnce(new Error("临时失败"));

    await createCenter("comments");

    expect(markSiteMessageReadMock).toHaveBeenCalledTimes(2);
    expect(markSiteMessageReadMock).toHaveBeenNthCalledWith(2, { group: 1 });
    expect(refreshSummaryMock).toHaveBeenCalledTimes(2);
    expect(getSiteMessagesMock).toHaveBeenCalledTimes(2);
  });

  it("chat 子项（含从其它子项切到 chat）从不调用任何标记已读接口", async () => {
    await createCenter("chat");
    expect(markSiteMessageReadMock).not.toHaveBeenCalled();
    expect(markGlobalNoticeReadMock).not.toHaveBeenCalled();

    const { tabRef } = await createCenter("comments");
    clearEntryCalls();
    tabRef.value = "chat";
    await flushPromises();

    expect(markSiteMessageReadMock).not.toHaveBeenCalled();
    expect(markGlobalNoticeReadMock).not.toHaveBeenCalled();
  });
});

describe("消息中心批量动作", () => {
  it("全部已读（手动兜底）：站内信按分组标记一次，刷新未读并回第一页", async () => {
    getSiteMessagesMock.mockResolvedValue({ list: [siteItem()], has_more: false });

    const { center } = await createCenter("comments");
    clearEntryCalls(); // 只断言按钮这次动作
    await center.markAllRead();

    expect(markSiteMessageReadMock).toHaveBeenCalledTimes(1);
    expect(markSiteMessageReadMock).toHaveBeenCalledWith({ group: 1 });
    expect(refreshSummaryMock).toHaveBeenCalledTimes(1);
    expect(getSiteMessagesMock).toHaveBeenCalledTimes(1);
    expect(getSiteMessagesMock).toHaveBeenCalledWith({ group: 1, page: 1, limit: 20 });
  });

  it("全局通知全部已读：翻页收集未读 id 后批量标记（不碰站内信接口）", async () => {
    // 入场自动已读与按钮各自都会翻页收集 id：按 limit 区分首屏页与扫描页，
    // 让两次收集拿到同一份未读集合。
    getGlobalNoticesMock.mockImplementation(async ({ page, limit }) => {
      if (limit === 20) {
        return { list: [globalItem({ id: "g1" }), globalItem({ id: "g2", is_read: true })], has_more: true };
      }
      return page === 2
        ? { list: [globalItem({ id: "g3" })], has_more: false }
        : { list: [], has_more: false };
    });

    const { center } = await createCenter("global");
    clearEntryCalls();
    await center.markAllRead();

    expect(markGlobalNoticeReadMock).toHaveBeenCalledWith(["g1", "g3"]);
    expect(markSiteMessageReadMock).not.toHaveBeenCalled();
    expect(refreshSummaryMock).toHaveBeenCalledTimes(1);
  });

  it("清空当前分组（站内信）：按分组删除，一次请求清空整组", async () => {
    const { center } = await createCenter("system");

    await center.clearCurrentGroup();

    expect(deleteSiteMessageMock).toHaveBeenCalledWith({ group: 3 });
    // 入场 1 + 自动已读 1 + 清空动作 1。
    expect(refreshSummaryMock).toHaveBeenCalledTimes(3);
  });

  it("清空当前分组（全局通知）：先翻页收集全部 id，再按列表删除", async () => {
    getGlobalNoticesMock.mockImplementation(async ({ page, limit }) => {
      if (limit === 20) return { list: [globalItem({ id: "g1" })], has_more: true };
      return page === 2
        ? { list: [globalItem({ id: "g2" })], has_more: false }
        : { list: [], has_more: false };
    });

    const { center } = await createCenter("global");
    await center.clearCurrentGroup();

    expect(deleteGlobalNoticeMock).toHaveBeenCalledWith(["g1", "g2"]);
    expect(deleteSiteMessageMock).not.toHaveBeenCalled();
    expect(refreshSummaryMock).toHaveBeenCalledTimes(3);
  });

  it("单条删除：站内信 / 全局通知各走各的接口，删后刷新未读并回第一页", async () => {
    const site = await createCenter("comments");
    await site.center.removeEntry("m1");
    expect(deleteSiteMessageMock).toHaveBeenCalledWith({ id: "m1" });

    const global = await createCenter("global");
    await global.center.removeEntry("g1");
    expect(deleteGlobalNoticeMock).toHaveBeenCalledWith(["g1"]);
    // 每个子项：入场 1 + 自动已读 1 + 删除动作 1 = 3。
    expect(refreshSummaryMock).toHaveBeenCalledTimes(6);
  });

  it("没有任何提醒时「全部已读」不空发请求", async () => {
    const { center } = await createCenter("global");
    await center.markAllRead();

    expect(markGlobalNoticeReadMock).not.toHaveBeenCalled();
    // 空列表时自动已读仍会重拉一次第一页；按钮本身不再发请求。
    expect(getGlobalNoticesMock).toHaveBeenCalledTimes(2);
    expect(getGlobalNoticesMock).toHaveBeenLastCalledWith({ page: 1, limit: 20, type: 1 });
  });
});
