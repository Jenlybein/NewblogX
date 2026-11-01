import { flushPromises } from "@vue/test-utils";
import { effectScope } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useReaderSwrAsyncData } from "~/composables/useReaderSwrAsyncData";
import { createReaderSwrCache } from "~/utils/readerSwrCache";
import { withoutPrivateFields } from "~/utils/readerPrivateFields";
import { installAwaitableAsyncData } from "../mocks/awaitableAsyncData";

const scopes: Array<ReturnType<typeof effectScope>> = [];

async function runInScope<T>(factory: () => T) {
  const scope = effectScope();
  scopes.push(scope);
  return await scope.run(factory);
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

afterEach(() => {
  while (scopes.length) {
    scopes.pop()?.stop();
  }
});

describe("useReaderSwrAsyncData：useAsyncData 之上的 30s SWR", () => {
  it("miss：正常取数，并把剔除私有字段后的副本写进缓存", async () => {
    const clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    installAwaitableAsyncData();

    const request = deferred<{ title: string; is_digg?: boolean }>();
    const handler = vi.fn(() => request.promise);
    const pendingSource = runInScope(() =>
      useReaderSwrAsyncData({
        key: "article-a1",
        handler,
        mode: "navigation",
        cache,
        sanitizeForCache: withoutPrivateFields,
      }),
    );

    // mock 与 Nuxt 一样把首次取数放进微任务，等一拍再断言。
    await Promise.resolve();
    expect(handler).toHaveBeenCalledTimes(1);
    request.resolve({ title: "甲", is_digg: true });
    await flushPromises();
    const source = await pendingSource;

    expect(source.data.value).toMatchObject({ title: "甲", is_digg: true });
    const cached = cache.read<{ title: string }>("article-a1");
    expect(cached.state).toBe("fresh");
    expect(cached.state !== "miss" ? cached.value : null).toEqual({ title: "甲" });
  });

  it("30 秒内命中：同步拿旧值、不发起请求、不进入 pending", async () => {
    const clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", { title: "缓存标题" });
    installAwaitableAsyncData();

    const handler = vi.fn(async () => ({ title: "新标题" }));
    const pendingSource = runInScope(() =>
      useReaderSwrAsyncData({ key: "article-a1", handler, mode: "navigation", cache }),
    );

    const source = await pendingSource;
    expect(handler).not.toHaveBeenCalled();
    expect(source.data.value).toEqual({ title: "缓存标题" });
    expect(source.pending.value).toBe(false);
  });

  it("手工 refresh 也遵守 SWR：30 秒内命中不重复请求（Nuxt granularCachedData 语义）", async () => {
    // Nuxt 4 的 granularCachedData 默认开：refresh() 也会先问 getCachedData。
    // 若 fresh 命中时还发请求，"30 秒内不重复请求"的验收在第二波源上就会失守。
    const clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", { title: "缓存标题" });
    installAwaitableAsyncData();

    const handler = vi.fn(async () => ({ title: "新标题" }));
    const pendingSource = runInScope(() =>
      useReaderSwrAsyncData({ key: "article-a1", handler, mode: "navigation", cache }),
    );
    const source = await pendingSource;

    await source.refresh();
    expect(handler).not.toHaveBeenCalled();
    expect(source.data.value).toEqual({ title: "缓存标题" });
  });

  it("超过 30 秒：先渲染旧值，后台静默刷新，刷新期间数据不被清空（不闪）", async () => {
    let clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", { title: "旧标题" });
    clock = 1_000 + 30_001;
    installAwaitableAsyncData();

    const refresh = deferred<{ title: string }>();
    const handler = vi.fn(() => refresh.promise);
    const pendingSource = runInScope(() =>
      useReaderSwrAsyncData({ key: "article-a1", handler, mode: "navigation", cache }),
    );

    const source = await pendingSource;
    // 命中旧值立刻可渲染，后台重验证已经发起。
    expect(source.data.value).toEqual({ title: "旧标题" });
    expect(handler).toHaveBeenCalledTimes(1);
    // 刷新中：pending 变真，但界面拿到的仍是旧值（不是空）。
    expect(source.pending.value).toBe(true);
    expect(source.data.value).toEqual({ title: "旧标题" });

    refresh.resolve({ title: "新标题" });
    await flushPromises();
    expect(source.data.value).toEqual({ title: "新标题" });
    expect(source.pending.value).toBe(false);
    const cached = cache.read<{ title: string }>("article-a1");
    expect(cached.state).toBe("fresh");
    expect(cached.state !== "miss" ? cached.value : null).toEqual({ title: "新标题" });
  });

  it("首屏 hydration：优先 payload（与 SSR 渲染一致），并顺手预热缓存（剔除私有字段）", async () => {
    const clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    installAwaitableAsyncData({
      hydrating: true,
      payloadData: { "article-a1": { title: "SSR 标题", is_digg: true } },
    });

    const handler = vi.fn(async () => ({ title: "客户端又取了一次" }));
    const pendingSource = runInScope(() =>
      useReaderSwrAsyncData({
        key: "article-a1",
        handler,
        mode: "hydration",
        cache,
        sanitizeForCache: withoutPrivateFields,
      }),
    );

    const source = await pendingSource;
    expect(handler).not.toHaveBeenCalled();
    expect(source.data.value).toEqual({ title: "SSR 标题", is_digg: true });
    const cached = cache.read<{ title: string }>("article-a1");
    expect(cached.state).toBe("fresh");
    expect(cached.state !== "miss" ? cached.value : null).toEqual({ title: "SSR 标题" });
  });

  it("服务端：不读也不写客户端缓存（进程内 Map 会跨请求串数据）", async () => {
    const clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", { title: "客户端缓存" });
    installAwaitableAsyncData();

    const handler = vi.fn(async () => ({ title: "服务端取数" }));
    const pendingSource = runInScope(() =>
      useReaderSwrAsyncData({ key: "article-a1", handler, mode: "server", cache }),
    );

    const source = await pendingSource;
    expect(handler).toHaveBeenCalledTimes(1);
    expect(source.data.value).toEqual({ title: "服务端取数" });
    // 服务端这次取数没有覆盖客户端缓存里的值。
    const cached = cache.read<{ title: string }>("article-a1");
    expect(cached.state !== "miss" ? cached.value : null).toEqual({ title: "客户端缓存" });
  });

  it("键不同则不命中：不同文章 id 不会串数据", async () => {
    const clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", { title: "文章甲" });
    installAwaitableAsyncData();

    const handler = vi.fn(async () => ({ title: "文章乙" }));
    const pendingSource = runInScope(() =>
      useReaderSwrAsyncData({ key: "article-a2", handler, mode: "navigation", cache }),
    );

    const source = await pendingSource;
    expect(handler).toHaveBeenCalledTimes(1);
    expect(source.data.value).toEqual({ title: "文章乙" });
  });
});
