import { toValue } from "vue";
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

export interface UseReaderSwrAsyncDataOptions<TValue> {
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

  function readCachedData(key: string, nuxtApp: unknown, context?: { cause?: string }): TValue | undefined {
    const cause = context?.cause ?? "initial";
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

    return lookup.value;
  }

  async function handlerWithCache() {
    const requestKey = readKey();
    const value = await options.handler();
    if (activeCache) {
      // 私有字段不落缓存；本次渲染仍返回带私有字段的完整值。
      activeCache.write(requestKey, sanitizeForCache(value));
    }
    return value;
  }

  const source = useAsyncData(
    readKey,
    handlerWithCache,
    activeCache ? { getCachedData: readCachedData } : undefined,
  );

  // stale：旧值已经上屏，后台静默重取；失败只落进 error ref（不打断已渲染的界面）。
  if (activeCache && options.mode === "navigation" && initialLookup?.state === "stale") {
    void Promise.resolve(source.refresh()).catch(() => null);
  }

  return source;
}
