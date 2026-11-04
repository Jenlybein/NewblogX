/**
 * 互动统一登录前置闸门（#238 范围 7）—— 纯判定部分。
 *
 * 病根：游客点点赞 / 收藏 / 关注 / 评论提交时，请求直接发出去，后端回
 * 「请先登录」，请求层再 `markPossiblyExpired()` 把 `isLoggedIn` 置假 —— 用户只是
 * 点了一下，自己的登录态却被"打成失效"。所以登录判定必须在**发请求之前**发生：
 * 未登录只弹登录框，一个接口都不打。
 *
 * 分享不在此列（#247）：它只写剪贴板、不打接口，游客也应该能复制链接。
 *
 * 判定口径：只有 `initializeSession()` 明确返回 true 才放行。false / 抛错都按
 * 未登录处理（宁可多弹一次登录框，也不能带着可疑的登录态去发请求）。
 *
 * 依赖以参数注入（与 `useArticleDigg` 同一套做法）：这一层不碰 pinia 与网络，
 * 接线在 `composables/useReaderLoginGate.ts`，页面的各个互动入口共用同一个实例。
 */
export type ReaderLoginGateDeps = {
  /** 登录态初始化/校验入口（真实实现是 `authStore.initializeSession`）。 */
  initializeSession: () => Promise<boolean>;
  /** 未登录时打开的登录弹窗。 */
  openAuthModal: () => void;
};

export type ReaderLoginGate = {
  /** 已登录返回 true；否则弹登录框并返回 false（调用方此时不得发请求）。 */
  requireLogin: () => Promise<boolean>;
};

export function createReaderLoginGate(deps: ReaderLoginGateDeps): ReaderLoginGate {
  async function requireLogin(): Promise<boolean> {
    const loggedIn = await deps.initializeSession().catch(() => false);
    if (!loggedIn) {
      deps.openAuthModal();
      return false;
    }

    return true;
  }

  return { requireLogin };
}
