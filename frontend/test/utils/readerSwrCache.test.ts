import { describe, expect, it } from "vitest";
import {
  READER_SWR_TTL_MS,
  createReaderSwrCache,
  readerSwrCache,
  resetReaderSwrCache,
} from "~/utils/readerSwrCache";

describe("readerSwrCache：客户端 30s SWR 内存缓存（ADR-0019 决策 4）", () => {
  it("默认 TTL 是 30 秒（与票面一致，不落盘）", () => {
    expect(READER_SWR_TTL_MS).toBe(30_000);
    expect(createReaderSwrCache().ttlMs).toBe(READER_SWR_TTL_MS);
  });

  it("没写过的键就是 miss", () => {
    const cache = createReaderSwrCache({ now: () => 1_000 });
    expect(cache.read("article-a1")).toEqual({ state: "miss" });
  });

  it("30 秒内命中：fresh，读得到原值", () => {
    let clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", { title: "旧标题" });

    expect(cache.read("article-a1")).toMatchObject({
      state: "fresh",
      value: { title: "旧标题" },
      storedAt: 1_000,
    });

    clock = 1_000 + READER_SWR_TTL_MS - 1;
    expect(cache.read("article-a1").state).toBe("fresh");
  });

  it("满 30 秒即过期：stale，但旧值仍然读得到（后台静默刷新期间继续渲染）", () => {
    let clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", { title: "旧标题" });

    clock = 1_000 + READER_SWR_TTL_MS;
    expect(cache.read("article-a1")).toMatchObject({
      state: "stale",
      value: { title: "旧标题" },
    });
  });

  it("键互相隔离：不同文章 / 不同页码 / 不同搜索词不会串数据", () => {
    const clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", { title: "文章甲" });
    cache.write("article-a2", { title: "文章乙" });
    cache.write("home-latest-articles#2", { items: ["第二页"] });

    expect(cache.read("article-a1").state).not.toBe("miss");
    expect(cache.read("article-a2").state).not.toBe("miss");
    expect(cache.read("home-latest-articles#1")).toEqual({ state: "miss" });
    expect(cache.read("search:今日")).toEqual({ state: "miss" });
  });

  it("delete 只摘掉一个键，clear 清空全部", () => {
    const clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", { title: "文章甲" });
    cache.write("article-a2", { title: "文章乙" });

    cache.delete("article-a1");
    expect(cache.read("article-a1")).toEqual({ state: "miss" });
    expect(cache.read("article-a2").state).not.toBe("miss");

    cache.clear();
    expect(cache.read("article-a2")).toEqual({ state: "miss" });
  });

  it("共享实例可被重置：模块级缓存会在用例之间残留，必须能清掉", () => {
    readerSwrCache.write("article-a1", { title: "文章甲" });
    expect(readerSwrCache.read("article-a1").state).not.toBe("miss");

    resetReaderSwrCache();
    expect(readerSwrCache.read("article-a1")).toEqual({ state: "miss" });
  });
});
