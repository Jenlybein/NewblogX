import { describe, expect, it, vi } from "vitest";

import { createReaderLoginGate } from "~/utils/readerLoginGate";

/**
 * 互动统一登录前置（#238 范围 7）。
 *
 * 病根：游客点点赞/收藏/分享/关注/评论提交 → 直接打接口 → 后端回「请先登录」→
 * 请求层 `markPossiblyExpired()` 把 `isLoggedIn` 置假 —— 点一下就把自己的登录态
 * 打成失效。修法是把登录判定收成一个前置闸门：**先问登录态，未登录只弹框、不发请求**。
 *
 * 这里测的是闸门本身（纯依赖注入，不拖 pinia / 不拖网络）：放行的唯一条件是
 * `initializeSession()` 明确的 true；其余一切（false / 抛错）都按未登录处理。
 */
describe("reader 互动登录前置闸门", () => {
  function harness(initializeSession: () => Promise<boolean>) {
    const openAuthModal = vi.fn();
    const gate = createReaderLoginGate({ initializeSession, openAuthModal });
    return { gate, openAuthModal };
  }

  it("已登录：放行，且不弹登录框", async () => {
    const { gate, openAuthModal } = harness(vi.fn().mockResolvedValue(true));

    await expect(gate.requireLogin()).resolves.toBe(true);
    expect(openAuthModal).not.toHaveBeenCalled();
  });

  it("未登录：弹登录框并返回 false，调用方据此不发请求", async () => {
    const { gate, openAuthModal } = harness(vi.fn().mockResolvedValue(false));

    await expect(gate.requireLogin()).resolves.toBe(false);
    expect(openAuthModal).toHaveBeenCalledTimes(1);
  });

  it("会话初始化抛错也不放行：按未登录处理（宁可多弹一次框，不能带着假登录态去打接口）", async () => {
    const { gate, openAuthModal } = harness(vi.fn().mockRejectedValue(new Error("network down")));

    await expect(gate.requireLogin()).resolves.toBe(false);
    expect(openAuthModal).toHaveBeenCalledTimes(1);
  });
});
