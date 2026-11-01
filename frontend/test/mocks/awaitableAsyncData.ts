import { ref, shallowRef } from "vue";
import { vi } from "vitest";

/**
 * 把自动导入的 `useAsyncData` 换成"与 Nuxt 一致"的版本：
 *
 * - 返回值是 thenable：`await useAsyncData(...)` 真的会等到这次取数落定；
 * - 支持 `immediate: false`：调用时不取数，`refresh()` 才发起并返回"这次取数"的 promise。
 *
 * 为什么需要它：默认的 `#imports` mock（`test/mocks/nuxt-imports.ts`）拿到的是普通对象，
 * await 立即返回 —— 于是**串行的 await 链也会看起来像并行**，测不出本票要治的病。
 * 这里让"等数据"真的会等，串行实现就会在测试里暴露（第二个请求在第一个回来前不会发起）。
 */
export function installAwaitableAsyncData() {
  const useAsyncData = vi.fn((...args: unknown[]) => {
    let handler: (() => unknown) | null = null;
    let options: { immediate?: boolean } = {};
    for (const arg of args) {
      if (typeof arg === "function") {
        handler = arg as () => unknown;
      } else if (arg && typeof arg === "object") {
        options = arg as { immediate?: boolean };
      }
    }

    const data = shallowRef<unknown>(null);
    const pending = ref(false);
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

    const thenable =
      options.immediate === false ? Promise.resolve(handle) : execute().then(() => handle);

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
