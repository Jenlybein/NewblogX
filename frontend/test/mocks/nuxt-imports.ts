import { ref } from "vue";

type TestFactory<T> = (() => T) | undefined;

function readFactory<T>(key: string): TestFactory<T> {
  const candidate = (globalThis as Record<string, unknown>)[key];
  return typeof candidate === "function" ? (candidate as () => T) : undefined;
}

export function useNuxtApp() {
  const factory = readFactory<unknown>("__useNuxtAppMock");
  if (!factory) {
    throw new Error("useNuxtApp mock is not configured");
  }
  return factory();
}

export function useRuntimeConfig() {
  const factory = readFactory<unknown>("__useRuntimeConfigMock");
  return factory ? factory() : { public: {} };
}

export function useRequestHeaders() {
  const factory = readFactory<Record<string, string>>("__useRequestHeadersMock");
  return factory ? factory() : {};
}

export function useRouter() {
  const factory = readFactory<unknown>("__useRouterMock");
  if (!factory) {
    throw new Error("useRouter mock is not configured");
  }
  return factory();
}

export function navigateTo(...args: unknown[]) {
  const factory = readFactory<(...callArgs: unknown[]) => unknown>("__navigateToMock");
  if (!factory) {
    throw new Error("navigateTo mock is not configured");
  }
  return factory()(...args);
}

// useRoute：默认给一个"空查询串"的路由。
//
// 为什么不像 useRouter 那样没配就抛错：读路由的模块（比如写作页靠
// ?article_id= 判断是不是编辑态）绝大多数场景只是想读 query，
// 让每个用例都去配工厂函数纯属噪音。需要特定 query 的用例用
// vi.stubGlobal("__useRouteMock", () => ({ query: { article_id: "1" } })) 覆盖即可。
export function useRoute() {
  const factory = readFactory<unknown>("__useRouteMock");
  return factory ? factory() : { query: {}, params: {}, path: "/" };
}

// useAsyncData：默认实现是"立刻发起取数，结果落进 data / error 两个 ref"。
//
// 与 Nuxt 真实行为的差异（写测试时要知道）：
//   1. 不做去重与缓存 —— 每次调用都是一次新的取数；
//   2. 不阻塞 await —— `await useAsyncData(...)` 拿到的是对象本身，
//      此刻 data 还没填充。所以依赖 data 的 watch 要等一个宏任务 +
//      nextTick 才会跑（测试里用 settle() 辅助函数）；
//   3. 不支持 options 里的 server/lazy/transform 等，只认取数函数。
//
// key 允许是字符串或函数，取数函数固定取"最后一个函数参数"：
//   useAsyncData(handler)             → args=[fn]
//   useAsyncData("k", handler)        → args=["k", fn]
//   useAsyncData(() => "k", fn, opts) → args=[keyFn, fn, opts]（opts 是对象，不会被误选）
export function useAsyncData(...args: unknown[]) {
  const factory = readFactory<(...callArgs: unknown[]) => unknown>("__useAsyncDataMock");
  if (factory) {
    return factory(...args);
  }

  let handler: (() => unknown) | undefined;
  for (const arg of args) {
    if (typeof arg === "function") {
      handler = arg as () => unknown;
    }
  }

  const data = ref<unknown>(null);
  const error = ref<unknown>(null);
  const pending = ref(true);

  const promise = Promise.resolve()
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

  return {
    data,
    error,
    pending,
    status: pending,
    refresh: () => promise,
    execute: () => promise,
  };
}
