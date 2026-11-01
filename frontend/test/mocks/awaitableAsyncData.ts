import { ref, shallowRef } from "vue";
import { vi } from "vitest";

/**
 * 把自动导入的 `useAsyncData` 换成"与 Nuxt 一致"的版本：
 *
 * - 返回值是 thenable：`await useAsyncData(...)` 真的会等到这次取数落定；
 * - 支持 `immediate: false`：调用时不取数，`refresh()` 才发起并返回"这次取数"的 promise；
 * - 支持 `getCachedData`：首屏（cause `initial`）先问缓存，命中就不发起取数 ——
 *   这是 #85 的 SWR 接入点，不模拟它就没法验证"命中不重复请求"；
 * - 服务端渲染的 payload 可以注入（`hydrating` + `payloadData`），用来验证首屏优先用 payload。
 *
 * 为什么需要它：默认的 `#imports` mock（`test/mocks/nuxt-imports.ts`）拿到的是普通对象，
 * await 立即返回 —— 于是**串行的 await 链也会看起来像并行**，测不出本票要治的病。
 * 这里让"等数据"真的会等，串行实现就会在测试里暴露（第二个请求在第一个回来前不会发起）。
 *
 * 与真实 Nuxt 仍有差异（写测试时要知道）：
 *   - 不做 key 变化时的重建（测试里换 key 直接新建一个调用即可）；
 *   - 不模拟 `_asyncData` 的跨组件去重；
 *   - `transform` / `pick` / `lazy` 不生效。
 */

export interface AwaitableAsyncDataEnv {
  /** 首屏 hydration：true 时 `getCachedData` 会读到 `payloadData`（与 Nuxt 默认行为一致）。 */
  hydrating?: boolean;
  /** 注入 SSR payload 里的现有数据（键为 useAsyncData 的 key）。 */
  payloadData?: Record<string, unknown>;
}

interface AwaitableAsyncDataOptions {
  immediate?: boolean;
  getCachedData?: (key: string, nuxtApp: unknown, context: { cause: string }) => unknown;
}

export function installAwaitableAsyncData(env: AwaitableAsyncDataEnv = {}) {
  const nuxtApp = {
    isHydrating: env.hydrating ?? false,
    payload: { data: { ...(env.payloadData ?? {}) }, _errors: {} },
    static: { data: {} as Record<string, unknown> },
  };

  const useAsyncData = vi.fn((...args: unknown[]) => {
    let keySource: string | (() => string) = "";
    let handler: (() => unknown) | null = null;
    let options: AwaitableAsyncDataOptions = {};

    if (typeof args[1] === "function") {
      keySource = args[0] as string | (() => string);
      handler = args[1] as () => unknown;
      if (args[2] && typeof args[2] === "object") {
        options = args[2] as AwaitableAsyncDataOptions;
      }
    } else {
      handler = (args[0] as () => unknown) ?? null;
    }

    const keyValue = () => (typeof keySource === "function" ? keySource() : keySource);
    const initialCachedData =
      options.immediate === false || !options.getCachedData
        ? undefined
        : options.getCachedData(keyValue(), nuxtApp, { cause: "initial" });
    const hasCachedData = initialCachedData !== undefined;

    const data = shallowRef<unknown>(hasCachedData ? initialCachedData : null);
    const pending = ref(options.immediate === false ? false : !hasCachedData);
    const error = shallowRef<unknown>(null);

    function execute() {
      pending.value = true;
      return Promise.resolve()
        .then(() => (handler ? handler() : null))
        .then((value) => {
          data.value = value;
        })
        .catch((reason) => {
          error.value = reason;
        })
        .finally(() => {
          pending.value = false;
        });
    }

    const handle = {
      data,
      pending,
      error,
      status: pending,
      refresh: () => execute(),
      execute: () => execute(),
    };

    let initialFetch: Promise<void>;
    if (options.immediate === false) {
      initialFetch = Promise.resolve();
    } else if (hasCachedData) {
      // 与 Nuxt 一致：首屏缓存命中 → 采用缓存值，不再发起取数。
      initialFetch = Promise.resolve();
    } else {
      initialFetch = execute();
    }

    const thenable = options.immediate === false ? Promise.resolve(handle) : initialFetch.then(() => handle);

    return Object.assign(thenable, handle);
  });

  vi.stubGlobal("useAsyncData", useAsyncData);
  return useAsyncData;
}

/**
 * 给 `useNuxtApp()` 一个最小替身：composable 在跨 await 之后要用 `runWithContext`
 * 把第二次取数包回 Nuxt 上下文里（服务端渲染的真实约束），测试里只需要直通执行。
 */
export function installFakeNuxtApp() {
  vi.stubGlobal("useNuxtApp", () => ({
    isHydrating: false,
    runWithContext: <T>(run: () => T) => run(),
  }));
}
