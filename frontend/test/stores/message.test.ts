import { createPinia, setActivePinia } from "pinia";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MessageSummary } from "~/types/api";

const getMessageSummaryMock = vi.hoisted(() => vi.fn());

vi.mock("~/services/user", () => ({
  getMessageSummary: getMessageSummaryMock,
}));

// 登录态用一个**真的 ref** 承载，才能测"状态变化"。
//
// store 里 watch 的是 `useAuthStore().isLoggedIn`，所以桩只要让这个 getter
// 在读取时建立依赖关系就够了 —— 一个带 getter 的对象字面量即可，
// 不需要真的造一个 pinia store。
// 用固定值（`{ isLoggedIn: true }`）的话 watch 永远收不到变化，新行为就测不到。
//
// **每个用例一份新 ref**（见 beforeEach）。共用一份会出事：前面用例创建的 store
// 实例没被销毁，它的 watch 还挂在同一个 ref 上，一改值就一起被唤醒 ——
// 实测表现是"期望调用 1 次，实际 3 次"，以及用例结束后旧 watch 再触发时报
// `useAuthStore is not defined`（此时 stub 已被 afterEach 撤掉）。
let loggedIn = ref(true);

async function createMessageStore() {
  vi.resetModules();
  setActivePinia(createPinia());
  const module = await import("~/stores/message");
  return module.useMessageStore();
}

/** 一份"全部非零"的汇总，便于一眼看出有没有被写回。 */
function summaryOf(value: number): MessageSummary {
  return {
    comment_msg_count: value,
    digg_favor_msg_count: value,
    private_msg_count: value,
    system_msg_count: value,
    global_msg_count: value,
  };
}

describe("message store", () => {
  beforeEach(() => {
    // 换一份新的 ref：上一个用例留下的 store 实例仍活着，它的 watch 只认它当时
    // 追踪的那个 ref。共用同一个的话，这里一赋值就会把旧 watch 全唤起来。
    loggedIn = ref(true);
    getMessageSummaryMock.mockReset();
    vi.stubGlobal("useAuthStore", () => ({
      get isLoggedIn() {
        return loggedIn.value;
      },
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
    getMessageSummaryMock.mockResolvedValue(summaryOf(1));

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

  // ── 以下三条测的是"本 store 自己盯登录态"（baseline §42）──
  //
  // 改之前这些行为由 auth store 主动调用本 store 完成，现在反过来：
  // auth 什么都不做，本 store 盯着 isLoggedIn。所以必须验证"没人调用它也会动"。

  it("登录后自动拉取，不需要别人来调", async () => {
    loggedIn.value = false;
    const store = await createMessageStore();
    getMessageSummaryMock.mockResolvedValue(summaryOf(3));

    loggedIn.value = true;

    // flush:"sync" 只保证回调立刻开始跑，网络结果是异步的，所以等值到位。
    await vi.waitFor(() => expect(store.totalUnread).toBe(15));
    expect(getMessageSummaryMock).toHaveBeenCalledTimes(1);
  });

  it("登出后立刻清空（同步，不留一帧上一个账号的未读数）", async () => {
    const store = await createMessageStore();
    getMessageSummaryMock.mockResolvedValue(summaryOf(2));
    await store.refreshSummary();
    expect(store.totalUnread).toBe(10);

    loggedIn.value = false;

    // 关键点：这里**没有 await** —— 清空必须同步发生。
    // 如果哪天把 flush 改回默认值，这条会红。
    expect(store.totalUnread).toBe(0);
    expect(store.fetched).toBe(false);
  });

  it("登出期间返回的请求结果被丢弃", async () => {
    let resolveFetch: ((value: MessageSummary) => void) | undefined;
    getMessageSummaryMock.mockReturnValue(
      new Promise<MessageSummary>((resolve) => {
        resolveFetch = resolve;
      }),
    );

    const store = await createMessageStore();
    const pending = store.refreshSummary();

    loggedIn.value = false; // 请求还在路上就登出了
    resolveFetch?.(summaryOf(4));
    await pending;

    // 不清掉的话，换账号会先看到上一个账号的未读数。
    expect(store.totalUnread).toBe(0);
  });

  it("并发刷新只发一个请求", async () => {
    let resolveFetch: ((value: MessageSummary) => void) | undefined;
    getMessageSummaryMock.mockReturnValue(
      new Promise<MessageSummary>((resolve) => {
        resolveFetch = resolve;
      }),
    );

    const store = await createMessageStore();
    // 登录瞬间可能有三处同时要刷新（本 store 的 watch、layout 兜底、收件箱页面）。
    const first = store.refreshSummary();
    const second = store.refreshSummary();

    expect(getMessageSummaryMock).toHaveBeenCalledTimes(1);

    resolveFetch?.(summaryOf(1));
    await Promise.all([first, second]);
    expect(store.totalUnread).toBe(5);
  });
});
