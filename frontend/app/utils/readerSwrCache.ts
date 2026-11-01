/**
 * 读者站的客户端 SWR 内存缓存（ADR-0019 决策 4 / 约束 4）。
 *
 * 只解决一件事：**同一路由在 30 秒内返回或重进时直接命中、立刻渲染**，
 * 同时在后台静默刷新；缓存键由调用方拼进路由参数（文章 id、分页、搜索词）。
 *
 * 刻意不做的三件事：
 * 1. **不落盘、不做 per-user 持久化** —— 只是本页面的内存 Map，刷新即空；
 * 2. **不做服务端缓存** —— SSR 进程里的模块级 Map 会跨请求串数据（服务端缓存归 #86）；
 * 3. **不缓存用户私有字段** —— 写缓存前由调用方剔除 `is_digg` / `is_favor` /
 *    `relation` 这类登录态字段（见 `readerPrivateFields.ts`），本模块不替调用方做决定。
 *
 * 时钟与 TTL 都可注入：单测用假时钟把时间推过 30 秒，而不是真的 `setTimeout` 睡 30 秒。
 */

/** 客户端缓存有效期：30 秒（票面与 ADR-0019 决策 4 的硬性数字）。 */
export const READER_SWR_TTL_MS = 30_000;

export interface ReaderSwrCacheOptions {
  /** 缓存有效期，默认 {@link READER_SWR_TTL_MS}。 */
  ttlMs?: number;
  /** 时钟，默认 `Date.now`；单测注入假时钟。 */
  now?: () => number;
}

/** 一次读缓存的结果：miss / 30 秒内命中（fresh）/ 已过期但仍可先渲染（stale）。 */
export type ReaderSwrLookup<TValue> =
  | { state: "miss" }
  | { state: "fresh"; value: TValue; storedAt: number }
  | { state: "stale"; value: TValue; storedAt: number };

export interface ReaderSwrCache {
  /** 当前生效的 TTL（毫秒）。 */
  readonly ttlMs: number;
  read: <TValue>(key: string) => ReaderSwrLookup<TValue>;
  write: <TValue>(key: string, value: TValue) => void;
  delete: (key: string) => void;
  clear: () => void;
}

interface ReaderSwrEntry {
  value: unknown;
  storedAt: number;
}

export function createReaderSwrCache(options: ReaderSwrCacheOptions = {}): ReaderSwrCache {
  const ttlMs = options.ttlMs ?? READER_SWR_TTL_MS;
  const now = options.now ?? Date.now;
  // 用 Map 而不是对象字面量：键里带文章 id / 搜索词，任何 `__proto__` 式的键名
  // 都不该有机会影响原型链。
  const entries = new Map<string, ReaderSwrEntry>();

  function read<TValue>(key: string): ReaderSwrLookup<TValue> {
    const entry = entries.get(key);
    if (!entry) {
      return { state: "miss" };
    }

    const ageMs = now() - entry.storedAt;
    return {
      // 满 30 秒即过期：stale 仍然带着旧值，调用方先渲染再后台重取（不闪）。
      state: ageMs >= ttlMs ? "stale" : "fresh",
      value: entry.value as TValue,
      storedAt: entry.storedAt,
    };
  }

  function write<TValue>(key: string, value: TValue) {
    entries.set(key, { value, storedAt: now() });
  }

  function remove(key: string) {
    entries.delete(key);
  }

  function clear() {
    entries.clear();
  }

  return { ttlMs, read, write, delete: remove, clear };
}

/**
 * 客户端跨导航共享的缓存实例。
 *
 * 注意：**服务端渲染不能读写它** —— SSR 的模块状态在同一个 Node 进程里跨请求共享，
 * 读它会串请求、写它会串用户。所以接入层（`usePagedResourceCache` /
 * `useReaderSwrAsyncData`）用 `import.meta.client` / 渲染环境判定把它在服务端关掉。
 */
export const readerSwrCache = createReaderSwrCache();

/** 单测用：模块级 Map 会在用例之间残留，每个用例开始前清空。 */
export function resetReaderSwrCache() {
  readerSwrCache.clear();
}
