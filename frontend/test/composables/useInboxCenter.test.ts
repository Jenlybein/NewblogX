import { flushPromises } from "@vue/test-utils";
import { computed, nextTick, ref, shallowRef, toValue, watch } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { GlobalNoticeItem, SiteMessageItem } from "~/types/api";

const getSiteMessagesMock = vi.hoisted(() => vi.fn());
const getGlobalNoticesMock = vi.hoisted(() => vi.fn());
const messageSummaryMock = vi.hoisted(() => ({
  comment_msg_count: 1,
  digg_favor_msg_count: 6,
  private_msg_count: 0,
  system_msg_count: 2,
  global_msg_count: 1,
}));

vi.mock("~/services/inbox", () => ({
  deleteChatMessages: vi.fn(),
  deleteChatSessions: vi.fn(),
  deleteGlobalNoticeForCurrentUser: vi.fn(),
  deleteSiteMessage: vi.fn(),
  getChatMessages: vi.fn(),
  getChatSessions: vi.fn(),
  getGlobalNotices: getGlobalNoticesMock,
  getSiteMessages: getSiteMessagesMock,
  markChatMessagesRead: vi.fn(),
  markGlobalNoticeRead: vi.fn(),
  markSiteMessageRead: vi.fn(),
}));

vi.mock("~/utils/chat", () => ({
  buildPreparedChatSession: vi.fn(),
  isDraftChatSessionId: (id: string) => id.startsWith("draft-"),
}));

const commentMessage = {
  id: "comment-1",
  created_at: "2026-04-17T00:00:00.000+08:00",
  updated_at: "2026-04-17T00:00:00.000+08:00",
  type: 1,
  receiver_id: "u1",
  action_user_id: "u2",
  action_user_nickname: "River",
  action_user_avatar: "",
  content: "评论了你的文章",
  article_id: "a1",
  comment_id: "c1",
  article_title: "文章",
  link_title: "",
  link_herf: "",
  is_read: false,
  read_at: null,
} satisfies SiteMessageItem;

const globalNotice = {
  id: "global-1",
  create_at: "2026-04-17T00:00:00.000+08:00",
  title: "全局通知",
  icon: "",
  content: "全局通知内容",
  herf: "",
  is_read: false,
} satisfies GlobalNoticeItem;

async function createInboxCenter() {
  vi.resetModules();
  const module = await import("~/composables/useInboxCenter");
  const inbox = module.useInboxCenter();
  await flushPromises();
  await nextTick();
  return inbox;
}

describe("useInboxCenter", () => {
  beforeEach(() => {
    getSiteMessagesMock.mockReset();
    getGlobalNoticesMock.mockReset();
    messageSummaryMock.comment_msg_count = 1;
    messageSummaryMock.digg_favor_msg_count = 6;
    messageSummaryMock.private_msg_count = 0;
    messageSummaryMock.system_msg_count = 2;
    messageSummaryMock.global_msg_count = 1;

    vi.stubGlobal("computed", computed);
    vi.stubGlobal("ref", ref);
    vi.stubGlobal("shallowRef", shallowRef);
    vi.stubGlobal("toValue", toValue);
    vi.stubGlobal("watch", watch);
    vi.stubGlobal("useAuthStore", () => ({
      isLoggedIn: true,
      profileId: "u1",
    }));
    vi.stubGlobal("useMessageStore", () => ({
      summary: messageSummaryMock,
      refreshSummary: vi.fn(async () => messageSummaryMock),
    }));
    vi.stubGlobal("useChatStore", () => ({
      activeSessionId: null,
      setActiveSession: vi.fn(),
    }));
    vi.stubGlobal("useChatSocket", () => ({
      socketStatus: shallowRef("closed"),
      socketError: shallowRef(null),
      connect: vi.fn(),
      sendMessage: vi.fn(),
      consumeIncomingMessages: vi.fn(() => []),
      inbox: shallowRef([]),
    }));
  });

  it("prefetches global notices on the site tab so the global badge is not stuck at zero", async () => {
    getSiteMessagesMock.mockResolvedValue({ list: [commentMessage], has_more: false });
    getGlobalNoticesMock.mockResolvedValue({ list: [globalNotice], has_more: false });

    const inbox = await createInboxCenter();

    expect(getGlobalNoticesMock).toHaveBeenCalledWith({ page: 1, limit: 9, type: 1 });
    expect(inbox.globalNotices.value).toEqual([globalNotice]);
  });

  it("uses the separated summary field for the system notification badge", async () => {
    getSiteMessagesMock.mockImplementation(({ group }: { group: 1 | 2 | 3 }) =>
      Promise.resolve({
        list: group === 3 ? [] : [commentMessage],
        has_more: false,
      }),
    );
    getGlobalNoticesMock.mockResolvedValue({ list: [globalNotice], has_more: false });

    const inbox = await createInboxCenter();

    expect(getSiteMessagesMock).not.toHaveBeenCalledWith({ group: 3, page: 1, limit: 9 });
    expect(inbox.siteCategories.value.find((item) => item.key === 3)?.count).toBe(2);
  });
});
