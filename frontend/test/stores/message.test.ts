import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";

const getMessageSummaryMock = vi.hoisted(() => vi.fn());

vi.mock("~/services/user", () => ({
  getMessageSummary: getMessageSummaryMock,
}));

async function createMessageStore() {
  vi.resetModules();
  setActivePinia(createPinia());
  const module = await import("~/stores/message");
  return module.useMessageStore();
}

describe("message store", () => {
  beforeEach(() => {
    getMessageSummaryMock.mockReset();
    vi.stubGlobal("useAuthStore", () => ({
      isLoggedIn: true,
    }));
  });

  it("includes global notifications in the unread total", async () => {
    getMessageSummaryMock.mockResolvedValue({
      comment_msg_count: 1,
      digg_favor_msg_count: 2,
      private_msg_count: 3,
      system_msg_count: 4,
      global_msg_count: 5,
    });

    const store = await createMessageStore();
    await store.refreshSummary();

    expect(store.summary.global_msg_count).toBe(5);
    expect(store.totalUnread).toBe(15);
  });

  it("clears all unread buckets including global notifications", async () => {
    getMessageSummaryMock.mockResolvedValue({
      comment_msg_count: 1,
      digg_favor_msg_count: 1,
      private_msg_count: 1,
      system_msg_count: 1,
      global_msg_count: 1,
    });

    const store = await createMessageStore();
    await store.refreshSummary();
    store.clear();

    expect(store.summary).toEqual({
      comment_msg_count: 0,
      digg_favor_msg_count: 0,
      private_msg_count: 0,
      system_msg_count: 0,
      global_msg_count: 0,
    });
    expect(store.totalUnread).toBe(0);
  });
});
