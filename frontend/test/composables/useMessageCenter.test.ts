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

describe("消息中心取数", () => {
  it("进入子项即拉第一页（每页 20）并刷新未读汇总", async () => {
    getSiteMessagesMock.mockResolvedValue({ list: [siteItem()], has_more: true });

    const { center } = await createCenter("comments");

    expect(getSiteMessagesMock).toHaveBeenCalledWith({ group: 1, page: 1, limit: 20 });
    expect(refreshSummaryMock).toHaveBeenCalledTimes(1);
    expect(center.items.value.map((item) => item.id)).toEqual(["m1"]);
    expect(center.hasMore.value).toBe(true);
    expect(center.page.value).toBe(1);
  });

  it("点赞与收藏 / 系统通知分别用分组 2 / 3，全局通知走 global_notif", async () => {
    await createCenter("digg");
    expect(getSiteMessagesMock).toHaveBeenCalledWith({ group: 2, page: 1, limit: 20 });

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
  });

  it("滚动加载更多：只追加下一页，加载完不再请求", async () => {
    getSiteMessagesMock
      .mockResolvedValueOnce({ list: [siteItem({ id: "m1" })], has_more: true })
      .mockResolvedValueOnce({ list: [siteItem({ id: "m2" })], has_more: false });

    const { center } = await createCenter("comments");
    expect(center.hasMore.value).toBe(true);

    await center.loadMore();
    expect(getSiteMessagesMock).toHaveBeenLastCalledWith({ group: 1, page: 2, limit: 20 });
    expect(center.items.value.map((item) => item.id)).toEqual(["m1", "m2"]);
    expect(center.hasMore.value).toBe(false);

    await center.loadMore();
    expect(getSiteMessagesMock).toHaveBeenCalledTimes(2);
  });

  it("接口失败落在 error 上，不把页面打崩", async () => {
    getSiteMessagesMock.mockRejectedValueOnce(new Error("网络错误"));

    const { center } = await createCenter("comments");

    expect(center.error.value).toBe("网络错误");
    expect(center.items.value).toEqual([]);
    expect(center.pending.value).toBe(false);
  });
});

describe("消息中心批量动作", () => {
  it("全部已读：站内信按分组标记一次，刷新未读并回第一页", async () => {
    getSiteMessagesMock.mockResolvedValue({ list: [siteItem()], has_more: false });

    const { center } = await createCenter("comments");
    await center.markAllRead();

    expect(markSiteMessageReadMock).toHaveBeenCalledWith({ group: 1 });
    expect(refreshSummaryMock).toHaveBeenCalledTimes(2);
    expect(getSiteMessagesMock).toHaveBeenCalledTimes(2);
  });

  it("全局通知全部已读：翻页收集未读 id 后批量标记（不碰站内信接口）", async () => {
    getGlobalNoticesMock
      .mockResolvedValueOnce({
        list: [globalItem({ id: "g1" }), globalItem({ id: "g2", is_read: true })],
        has_more: true,
      })
      .mockResolvedValueOnce({ list: [globalItem({ id: "g3" })], has_more: false });

    const { center } = await createCenter("global");
    await center.markAllRead();

    expect(markGlobalNoticeReadMock).toHaveBeenCalledWith(["g1", "g3"]);
    expect(markSiteMessageReadMock).not.toHaveBeenCalled();
    expect(refreshSummaryMock).toHaveBeenCalledTimes(2);
  });

  it("清空当前分组（站内信）：按分组删除，一次请求清空整组", async () => {
    const { center } = await createCenter("system");

    await center.clearCurrentGroup();

    expect(deleteSiteMessageMock).toHaveBeenCalledWith({ group: 3 });
    expect(refreshSummaryMock).toHaveBeenCalledTimes(2);
  });

  it("清空当前分组（全局通知）：先翻页收集全部 id，再按列表删除", async () => {
    getGlobalNoticesMock
      .mockResolvedValueOnce({ list: [globalItem({ id: "g1" })], has_more: true })
      .mockResolvedValueOnce({ list: [globalItem({ id: "g2" })], has_more: false });

    const { center } = await createCenter("global");
    await center.clearCurrentGroup();

    expect(deleteGlobalNoticeMock).toHaveBeenCalledWith(["g1", "g2"]);
    expect(deleteSiteMessageMock).not.toHaveBeenCalled();
    expect(refreshSummaryMock).toHaveBeenCalledTimes(2);
  });

  it("单条删除：站内信 / 全局通知各走各的接口，删后刷新未读并回第一页", async () => {
    const site = await createCenter("comments");
    await site.center.removeEntry("m1");
    expect(deleteSiteMessageMock).toHaveBeenCalledWith({ id: "m1" });

    const global = await createCenter("global");
    await global.center.removeEntry("g1");
    expect(deleteGlobalNoticeMock).toHaveBeenCalledWith(["g1"]);
    expect(refreshSummaryMock).toHaveBeenCalledTimes(4);
  });

  it("没有任何提醒时「全部已读」不空发请求", async () => {
    const { center } = await createCenter("global");
    await center.markAllRead();

    expect(markGlobalNoticeReadMock).not.toHaveBeenCalled();
    expect(getGlobalNoticesMock).toHaveBeenCalledTimes(1);
  });
});
