import { watch } from "vue";

/**
 * 文章页「登录后的用户私有态补取」（ADR-0024 决策 2 / 约束 3；评审票 248 的 P0-4）。
 *
 * 病根：页面只在 `onMounted` 里判一次 `initializeSession()`。用户在文章页被弹登录框、
 * 登录成功后，那条「登录后补一次私有态」的路径已经走完了 —— 私有态（已赞 / 已收藏）
 * **永不请求**，按钮继续显示未赞（库里其实已赞），正是本批要修的 bug 换了个入口。
 *
 * 这里把两条触发合并成一条单飞通道：
 *   - `initialize()`：页面在 `onMounted` 里显式调用（登录初始化之后补一次）；
 *   - `watch(isLoggedIn)`：登录态 false→true 时补一次（`initializeSession()` 把匿名
 *     翻成已登录，会同时命中上面那条，所以必须有单飞闸门，否则同一次登录发两次请求）。
 *
 * 边界：
 *   - 只在客户端跑：`isClient` 判定放在 watch 回调里，SSR 阶段登录态变化不触发取数；
 *   - 一次登录会话只补一次（`loaded`）；登出（false）后复位，下次登录重新补；
 *   - 用户已手动操作过点赞（`shouldSkip`）时不覆盖乐观结果。
 */
export interface UseArticlePrivateSessionOptions {
  /** 当前登录态（如 `() => authStore.isLoggedIn`）。 */
  isLoggedIn: () => boolean;
  /** 登录初始化；页面在 onMounted 里经 `initialize()` 调用。 */
  initializeSession: () => Promise<boolean>;
  /** 取一次私有态并补进当前正文（`useArticlePageSources` 的 `refreshArticlePrivate`）。 */
  refreshPrivate: () => Promise<unknown>;
  /** 用户已手动点过赞：不覆盖乐观结果（可选）。 */
  shouldSkip?: () => boolean;
  /** 客户端判定（单测注入；默认 `import.meta.client`）。 */
  isClient?: () => boolean;
}

export interface ArticlePrivateSession {
  /** 登录初始化之后补一次私有态（页面 onMounted 调用）。 */
  initialize: () => Promise<void>;
  /** 保证本次登录会话已补过（幂等；watch 与 initialize 共用）。 */
  ensureLoaded: () => Promise<void>;
}

export function useArticlePrivateSession(options: UseArticlePrivateSessionOptions): ArticlePrivateSession {
  const isClient = options.isClient ?? (() => import.meta.client);
  // 单飞：`inFlight` 挡并发调用，`loaded` 挡同一登录会话的重复补取。
  let loaded = false;
  let inFlight: Promise<void> | null = null;

  function ensureLoaded(): Promise<void> {
    if (inFlight) {
      return inFlight;
    }
    if (loaded) {
      return Promise.resolve();
    }
    if (!options.isLoggedIn()) {
      // 未登录不补：匿名 payload 就是最终形态，多打一次接口纯属浪费。
      return Promise.resolve();
    }
    if (options.shouldSkip?.()) {
      loaded = true;
      return Promise.resolve();
    }

    inFlight = Promise.resolve()
      .then(() => options.refreshPrivate())
      .then(() => {
        loaded = true;
      })
      .catch(() => {
        // 私有态是增强数据：失败不打断页面；不置 loaded，下次登录切换还能再试。
      })
      .finally(() => {
        inFlight = null;
      });
    return inFlight;
  }

  watch(
    options.isLoggedIn,
    (now, previous) => {
      if (!isClient()) {
        return;
      }
      if (!now) {
        // 登出：下一次登录是新的会话，私有态要重新取（可能已在别的设备赞过）。
        loaded = false;
        return;
      }
      if (!previous) {
        void ensureLoaded();
      }
    },
  );

  async function initialize(): Promise<void> {
    const loggedIn = await options.initializeSession();
    if (loggedIn) {
      await ensureLoaded();
    }
  }

  return { initialize, ensureLoaded };
}
