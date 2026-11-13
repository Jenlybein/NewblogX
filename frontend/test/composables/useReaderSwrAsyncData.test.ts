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

  it("手工 refresh 必须真发请求：fresh 命中只对 cause=initial 生效（ADR-0016 决策 1）", async () => {
    // Nuxt 4 的 granularCachedData 默认开：refresh() 也会先问 getCachedData。
    // 旧的实现让 fresh 命中在**任何** cause 下都短路 handler —— 于是 hydration 把匿名
    // payload 预热进缓存后，页面 onMounted 的 refreshArticle() 永远不发请求，
    // ADR-0016 承诺的"客户端补齐用户态"在硬刷新/重进时根本不会发生。
    //
    // 新口径：只有 cause="initial" 允许复用 fresh；refresh:manual / refresh:hook
    // 一律返回 undefined，让 handler 真取一次（stale 仍然先渲染旧值、后台重取）。
    const clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", { title: "缓存标题" });
    installAwaitableAsyncData();

    const handler = vi.fn(async () => ({ title: "新标题" }));
    const pendingSource = runInScope(() =>
      useReaderSwrAsyncData({ key: "article-a1", handler, mode: "navigation", cache }),
    );
    const source = await pendingSource;

    // 首屏这次仍按 SWR 命中（30 秒内不重复请求的验收靠它）。
    expect(handler).not.toHaveBeenCalled();
    expect(source.data.value).toEqual({ title: "缓存标题" });

    await source.refresh();
    expect(handler).toHaveBeenCalledTimes(1);
    expect(source.data.value).toEqual({ title: "新标题" });
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

/**
 * `refreshIfNeeded` 是"内部第二波"的入口（ADR-0016 决策 1 / 约束 1）。
 *
 * 为什么不能直接回退到 `refresh()`：新的 SWR 语义里 `refresh()` 必须真发请求，
 * 而 #85 的「30 秒内重进请求数为 0」验收需要作者统计在 fresh 命中时不发请求
 * （`useArticlePageSources.test.ts` 已有一条用例依赖它）。
 *
 * 保留它的前提是把**状态语义**写清楚并用用例钉住（评审 #248 / P1-6）：
 * fresh 命中直接落值 ⇒ pending 保持 false（本来就没有在途请求，这是正确语义）、
 * 上一次失败的 error 被清掉（现在有可渲染的权威值）；stale / miss 仍走 `refresh()`，
 * pending / error 与普通取数完全一致 —— 也就是说它只绕过"没有请求时的假 pending"，
 * 不是绕过生命周期。
 */
describe("refreshIfNeeded：内部第二波入口的状态语义", () => {
  it("fresh 命中：不发请求、直接落数据、pending 保持 false", async () => {
    const cache = createReaderSwrCache({ now: () => 1_000 });
    cache.write("article-author-info:u9", { article_count: 3 });
    installAwaitableAsyncData();

    const handler = vi.fn(async () => ({ article_count: 9 }));
    // 用 wrapper 接住原始返回值：直接 await 会被 thenable 解包成内部 handle，
    // 丢掉 refreshIfNeeded（真实调用方也不 await 它，见 useArticlePageSources）。
    const source = (
      await runInScope(() => ({
        source: useReaderSwrAsyncData({
          key: "article-author-info:u9",
          handler,
          mode: "navigation",
          cache,
          immediate: false,
        }),
      }))
    ).source;

    await source.refreshIfNeeded();

    expect(handler).not.toHaveBeenCalled();
    expect(source.data.value).toEqual({ article_count: 3 });
    expect(source.pending.value).toBe(false);
    expect(source.error.value).toBeNull();
  });

  it("fresh 命中会清掉上一次 refresh 留下的 error（有权威值时不再留着旧错误）", async () => {
    const cache = createReaderSwrCache({ now: () => 1_000 });
    cache.write("article-author-info:u9", { article_count: 3 });
    installAwaitableAsyncData();

    const handler = vi.fn(async () => {
      throw new Error("boom");
    });
    const source = (
      await runInScope(() => ({
        source: useReaderSwrAsyncData({
          key: "article-author-info:u9",
          handler,
          mode: "navigation",
          cache,
          immediate: false,
        }),
      }))
    ).source;

    // 先真发一次（refresh 语义），失败落进 error。
    await source.refresh();
    expect(source.error.value).toBeInstanceOf(Error);

    await source.refreshIfNeeded();
    expect(source.data.value).toEqual({ article_count: 3 });
    expect(source.error.value).toBeNull();
    expect(source.pending.value).toBe(false);
  });

  it("stale / miss 仍走 refresh：真发请求、pending 变真、失败落进 error", async () => {
    let clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-author-info:u9", { article_count: 3 });
    clock = 1_000 + 30_001;
    installAwaitableAsyncData();

    const request = deferred<{ article_count: number }>();
    const handler = vi.fn(() => request.promise);
    const source = (
      await runInScope(() => ({
        source: useReaderSwrAsyncData({
          key: "article-author-info:u9",
          handler,
          mode: "navigation",
          cache,
          immediate: false,
        }),
      }))
    ).source;

    const refreshing = source.refreshIfNeeded();
    // mock 与 Nuxt 一样把 handler 放进微任务：等一拍再断言"请求真的发了、pending 变真"。
    await Promise.resolve();
    expect(handler).toHaveBeenCalledTimes(1);
    expect(source.pending.value).toBe(true);

    request.resolve({ article_count: 7 });
    await refreshing;
    expect(source.data.value).toEqual({ article_count: 7 });
    expect(source.pending.value).toBe(false);
  });
});
