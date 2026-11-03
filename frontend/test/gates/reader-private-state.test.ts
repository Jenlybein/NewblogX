import { readFileSync } from "node:fs";
import { join } from "node:path";

import { effectScope } from "vue";
import { describe, expect, it, vi } from "vitest";

import { useArticlePageSources } from "~/composables/useArticlePageSources";
import type { ArticleAuthorInfo, ArticleDetail, CommentRootItem } from "~/types/api";
import { createReaderSwrCache } from "~/utils/readerSwrCache";
import { installAwaitableAsyncData, installFakeNuxtApp } from "../mocks/awaitableAsyncData";

/**
 * 「hydration 后必须发出一次带鉴权的用户态请求」门禁（ADR-0024 决策 2 / 票 #235 第 6 项）。
 *
 * 背景：点赞态不进 SSR payload（ADR-0019 约束 2），必须先由匿名 HTML 渲染，
 * 再在客户端补齐。修复前有两条路同时失效：
 *   1. `useReaderSwrAsyncData` 对任何 cause 的 fresh 命中都短路 → 页面 onMounted 的
 *      `refreshArticle()` 永远不发请求；
 *   2. 页面没有一个独立的私有态取数，hydration 后没有"一定发出去"的那一次请求。
 *
 * 本门禁分两层：
 *   - **源码**：私有态取数必须 `server: false` + `getCachedData: () => undefined`
 *     （永不进 payload、永不进 30s SWR），走带鉴权的详情接口，且页面在
 *     `initializeSession()` 之后显式触发；
 *   - **行为**：真跑一遍 composable，断言这一请求确实发出、私有键不进缓存。
 *
 * 每条源码判据都配一个 canary：把违反写法喂给判据必须报红，证明它不是恒绿。
 */
const projectDir = process.cwd();

const pageSourcesSource = readFileSync(
  join(projectDir, "app/composables/useArticlePageSources.ts"),
  "utf8",
);
const articleServiceSource = readFileSync(join(projectDir, "app/services/article.ts"), "utf8");
const articlePageSource = readFileSync(join(projectDir, "app/pages/article/[id].vue"), "utf8");
const cacheKeysSource = readFileSync(join(projectDir, "app/utils/readerCacheKeys.ts"), "utf8");

/** 取私有态源的声明块（`const articlePrivateSource` 到语句结束）。 */
function privateStateSourceBlock(source: string): string {
  const start = source.indexOf("const articlePrivateSource");
  if (start < 0) {
    return "";
  }
  const end = source.indexOf("\n  );", start);
  return end < 0 ? source.slice(start) : source.slice(start, end);
}

/**
 * 源码判据：私有态源必须同时满足三件事，否则会串回 payload / 30s 缓存。
 *
 * 返回违反项而不是布尔值：失败信息要直接说清是哪一条破了，不用回头读测试。
 */
function privateStateSourceViolations(block: string): string[] {
  const violations: string[] = [];
  if (!block) {
    return ["私有态源不存在"];
  }
  if (!block.includes("readerArticlePrivateCacheKey")) {
    violations.push("缓存键没走 readerArticlePrivateCacheKey（会与正文共用键）");
  }
  if (!block.includes("server: false")) {
    violations.push("没有 server: false（私有态会进 SSR payload）");
  }
  if (!block.includes("getCachedData: () => undefined")) {
    violations.push("没有 getCachedData: () => undefined（私有态会进 SWR/payload）");
  }
  return violations;
}

/** 取 `getArticlePrivateState` 的服务实现块（到下一个顶层 export 为止）。 */
function articlePrivateServiceBlock(source: string): string {
  const start = source.indexOf("export function getArticlePrivateState");
  if (start < 0) {
    return "";
  }
  const next = source.indexOf("\nexport function ", start + 1);
  return next < 0 ? source.slice(start) : source.slice(start, next);
}

/** 取页面 `onMounted` 块（到对应的 `});`）。 */
function onMountedBlock(source: string): string {
  const start = source.indexOf("onMounted(() => {");
  if (start < 0) {
    return "";
  }
  const end = source.indexOf("\n});", start);
  return end < 0 ? source.slice(start) : source.slice(start, end);
}

describe("用户私有态取数契约（ADR-0024 / 票 #235）", () => {
  it("私有态缓存键独立成 article-private-<id>，不与正文共用", () => {
    expect(cacheKeysSource).toContain("readerArticlePrivateCacheKey");
    const block = cacheKeysSource.slice(cacheKeysSource.indexOf("export function readerArticlePrivateCacheKey"));
    expect(block).toContain("`article-private-${articleId}`");
    expect(block).not.toContain("`article-${articleId}`");
  });

  it("私有态源：server:false + 恒 undefined 的 getCachedData（永不进 payload / SWR）", () => {
    expect(privateStateSourceViolations(privateStateSourceBlock(pageSourcesSource))).toEqual([]);
  });

  it("判据不是恒绿：server:true 或带 getCachedData 默认值的写法必须报红（canary）", () => {
    const bad = [
      "const articlePrivateSource = useAsyncData(",
      "  () => readerArticlePrivateCacheKey(articleId.value),",
      "  () => fetchArticlePrivate(articleId.value),",
      "  { server: true, lazy: true, immediate: false }",
      ");",
    ].join("\n");
    expect(privateStateSourceViolations(bad)).toEqual(
      expect.arrayContaining([
        expect.stringContaining("server: false"),
        expect.stringContaining("getCachedData"),
      ]),
    );
  });

  it("私有态取数走带鉴权的详情接口（不是匿名取数），且只投影私有字段", () => {
    const block = articlePrivateServiceBlock(articleServiceSource);
    expect(block).toContain("getArticleDetail");
    // 匿名取数会把匿名值当用户态：这里必须没有 auth: false。
    expect(block).not.toContain("auth: false");
    expect(block).toContain("pickArticlePrivateState");
  });

  it("页面在 initializeSession() 之后必须触发一次私有态请求（硬刷新后才显示已赞）", () => {
    const mounted = onMountedBlock(articlePageSource);
    expect(mounted).toContain("initializeSession");
    expect(mounted).toContain("refreshArticlePrivate");
    expect(mounted.indexOf("initializeSession")).toBeLessThan(mounted.indexOf("refreshArticlePrivate"));
  });

  it("行为：hydration 后一次刷新真发请求，payload 里的私有键不被采用，私有态不进 30s 缓存", async () => {
    const scope = effectScope();
    try {
      installFakeNuxtApp();
      const cache = createReaderSwrCache({ now: () => 1_000 });
      installAwaitableAsyncData({
        hydrating: true,
        payloadData: {
          "article-a1": {
            id: "a1",
            title: "SSR 标题",
            author_id: "u9",
          } as unknown as ArticleDetail,
          // 恶意 payload：私有态不该从 payload 读；读到就会永远显示未赞。
          "article-private-a1": { is_digg: false, is_favor: false, relation: 1 },
        },
      });

      const fetchArticlePrivate = vi.fn(async () => ({ is_digg: true, is_favor: true, relation: 4 }));
      const sources = await scope.run(() =>
        useArticlePageSources({
          mode: "hydration",
          articleId: "a1",
          swrCache: cache,
          fetchArticle: async () => ({ id: "a1", title: "SSR 标题", author_id: "u9" }) as ArticleDetail,
          fetchScore: async () => null,
          fetchComments: async () => ({ items: [] as CommentRootItem[], hasMore: false }),
          fetchAuthorInfo: async () => ({ article_count: 3 } as ArticleAuthorInfo),
          fetchAuthorProfile: async () => null,
          fetchArticlePrivate,
        }),
      );

      expect(fetchArticlePrivate).not.toHaveBeenCalled();
      await sources.refreshArticlePrivate();

      expect(fetchArticlePrivate).toHaveBeenCalledTimes(1);
      expect(sources.articlePrivate.value).toEqual({ is_digg: true, is_favor: true, relation: 4 });
      // 用户态补进了正文：按钮接下来才显示"已点赞"。
      expect(sources.article.value?.is_digg).toBe(true);
      expect(sources.article.value?.is_favor).toBe(true);
      // 私有键在 30s 缓存里必须是 miss（否则下一次重进会拿一份用户态缓存）。
      expect(cache.read("article-private-a1").state).toBe("miss");
    } finally {
      scope.stop();
    }
  });
});
