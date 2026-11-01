import { toValue } from "vue";
import type { AsyncDataOptions } from "#app";
import type { MaybeRefOrGetter } from "vue";
import type { ReaderRenderMode } from "~/utils/readerRenderMode";
import { readerSwrCache, type ReaderSwrCache, type ReaderSwrLookup } from "~/utils/readerSwrCache";

/**
 * 给 `useAsyncData` 套一层客户端 30s SWR（ADR-0019 决策 4）。
 *
 * #84 把取数改成了并行 + 骨架优先，但每次路由重进仍会重新发请求：`useAsyncData`
 * 的默认 `getCachedData` 只在首屏 hydration 时读 payload，站内导航没有 payload。
 * 这里补的是**跨导航**的那一层：
 *
 * - **30 秒内命中**：直接拿缓存值渲染，不发请求（`getCachedData` → 命中即不执行 handler）；
 * - **超过 30 秒（stale）**：先把旧值渲染出来，再后台 `refresh()` 静默重取；
 *   刷新期间 data 不会被清空，页面不会闪回骨架；
 * - **服务端渲染**：完全不碰客户端缓存（模块级 Map 在 Node 进程里跨请求共享，
 *   读会串请求、写会串用户；服务端缓存归 #86，不在本票范围内）；
 * - **首屏 hydration**：优先用 SSR payload（与已渲染的 HTML 一致），顺手把这份数据
 *   （剔除私有字段后）预热进缓存，这样"从首页进文章再返回首页"能直接命中。
 *
 * 写缓存前一律经过 `sanitizeForCache`：`is_digg` / `is_favor` / `relation` 这类
 * 登录态字段不进入缓存（见 `readerPrivateFields.ts`）。
 */

/**
 * 调度选项（immediate / lazy / server / watch）**直接沿用 Nuxt 自己的类型**：
 * 不要再手写成 `watch?: unknown[]` —— 那既过不了 `useAsyncData` 的重载，
 * 还会让返回值的泛型退化成 `unknown`，页面里拿到的数据就没有类型了。
 */
type ReaderSwrScheduling<TValue> = Pick<AsyncDataOptions<TValue>, "immediate" | "lazy" | "server" | "watch">;

export interface UseReaderSwrAsyncDataOptions<TValue> extends ReaderSwrScheduling<TValue> {
  /** `useAsyncData` 的缓存键：必须含路由参数（文章 id 等），不同资源不共用键。 */
  key: MaybeRefOrGetter<string>;
  /** 真正的取数函数（失败语义由调用方自己决定，这层不吞业务错误）。 */
  handler: () => Promise<TValue>;
  /** 渲染环境：`server` 下自动禁用客户端缓存。 */
  mode: ReaderRenderMode;
  /** 注入缓存实例（单测用假时钟）；默认客户端用共享实例、服务端禁用。 */
  cache?: ReaderSwrCache | null;
  /** 写缓存前的"去私有字段"处理；默认原样写入（纯公开载荷不需要它）。 */
  sanitizeForCache?: (value: TValue) => TValue;
}

interface NuxtCachedDataAppLike {
  isHydrating?: boolean;
  payload?: { data?: Record<string, unknown> };
  static?: { data?: Record<string, unknown> };
}

/**
 * 复刻 Nuxt 默认 `getCachedData` 的口径（它没有导出，只能照实现来）：
 * hydrating 时读 payload；其它 initial 场景读 static data（预渲染用）。
 * 服务端缓存/用户态分流都不在这里，只在 hydration 场景生效。
 */
function readNuxtDefaultCachedData(nuxtApp: unknown, key: string, cause: string) {
  const app = nuxtApp as NuxtCachedDataAppLike | undefined;
  if (!app) {
    return undefined;
  }

  if (app.isHydrating) {
    return app.payload?.data?.[key];
  }

  if (cause !== "refresh:manual" && cause !== "refresh:hook") {
    return app.static?.data?.[key];
  }

  return undefined;
}

export function useReaderSwrAsyncData<TValue>(options: UseReaderSwrAsyncDataOptions<TValue>) {
  // 服务端一律禁用：`import.meta.server` 与渲染环境判定都要求它是 null。
  const activeCache: ReaderSwrCache | null =
    options.mode === "server" ? null : (options.cache === undefined ? readerSwrCache : options.cache);
  const sanitizeForCache = options.sanitizeForCache ?? ((value: TValue) => value);

  const readKey = () => toValue(options.key);
  // 首屏这次到底命中没有：只有 stale 才需要后台重取；fresh 命中就是"这 30 秒不重复请求"。
  let initialLookup: ReaderSwrLookup<TValue> | null = null;

  // 用 Nuxt 自己的 `getCachedData` 签名（`nuxtApp` 是真实 `NuxtApp`、`cause` 是
  // `AsyncDataRefreshCause`）：手写 `nuxtApp: unknown` 会让 options 对象过不了重载。
  const readCachedData: NonNullable<AsyncDataOptions<TValue>["getCachedData"]> = (key, nuxtApp, context) => {
    const cause = context.cause;
    if (!activeCache) {
      return readNuxtDefaultCachedData(nuxtApp, key, cause) as TValue | undefined;
    }

    // 首屏 hydration 用 payload：SSR 已经按这份数据渲染过 HTML，换一份会闪 / mismatch。
    const payloadData = readNuxtDefaultCachedData(nuxtApp, key, cause);
    if (payloadData !== undefined) {
      // 顺手预热：返回首页 / 重进文章时这里就是命中的来源。
      activeCache.write(key, sanitizeForCache(payloadData as TValue));
      return payloadData as TValue;
    }

    const lookup = activeCache.read<TValue>(key);
    if (lookup.state === "miss") {
      return undefined;
    }

    if (cause === "initial") {
      initialLookup = lookup;
    }

    // fresh 命中才复用；stale 在"手动 / 钩子刷新"时必须返回 undefined，让 handler 真取一次。
    // 否则 Nuxt 的 granularCachedData 会拿这里的返回值短路 handler（refresh 永不发请求），
    // 决策 4 的"过期后后台静默刷新"就永远不会发生。
    if (cause !== "initial" && lookup.state === "stale") {
      return undefined;
    }

    return lookup.value;
  };

  async function handlerWithCache() {
    const requestKey = readKey();
    const value = await options.handler();
    if (activeCache) {
      // 私有字段不落缓存；本次渲染仍返回带私有字段的完整值。
      activeCache.write(requestKey, sanitizeForCache(value));
    }
    return value;
  }

  // 调度选项原样透传：作者资料这类"第二波、依赖正文 id"的源要靠 `immediate: false` +
  // 显式 refresh；评分源靠 `lazy` + watch。显式写泛型 `useAsyncData<TValue>`，让返回值的
  // `data` 上带着具体类型（否则页面里会退化成 unknown）。
  const source = useAsyncData<TValue>(readKey, handlerWithCache, {
    ...(options.immediate === undefined ? {} : { immediate: options.immediate }),
    ...(options.lazy === undefined ? {} : { lazy: options.lazy }),
    ...(options.server === undefined ? {} : { server: options.server }),
    ...(options.watch === undefined ? {} : { watch: options.watch }),
    ...(activeCache ? { getCachedData: readCachedData } : {}),
  });

  // 首屏这次是否命中了一个过期值（赋值发生在 readCachedData 里，这里用函数读取，
  // 避免 TS 把捕获变量按"只赋过 null"收窄成 null → never）。
  function hasStaleInitialLookup() {
    return initialLookup?.state === "stale";
  }

  // stale：旧值已经上屏，后台静默重取；失败只落进 error ref（不打断已渲染的界面）。
  if (activeCache && options.mode === "navigation" && hasStaleInitialLookup()) {
    void Promise.resolve(source.refresh()).catch(() => null);
  }

  return source;
}
