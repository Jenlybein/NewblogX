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
const articlePrivateSessionSource = readFileSync(
  join(projectDir, "app/composables/useArticlePrivateSession.ts"),
  "utf8",
);
const cacheKeysSource = readFileSync(join(projectDir, "app/utils/readerCacheKeys.ts"), "utf8");
const privateFieldsSource = readFileSync(join(projectDir, "app/utils/readerPrivateFields.ts"), "utf8");

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

/**
 * 源码判据：登录态补取必须同时具备三件事（P0-4）。
 *
 * 返回违反项而不是布尔值：失败信息要直接说清是哪一条破了，不用回头读测试。
 */
function privateSessionViolations(source: string): string[] {
  if (!source) {
    return ["私有态登录补取 composable 不存在"];
  }
  const violations: string[] = [];
  if (!source.includes("watch(")) {
    violations.push("没有 watch（页内登录成功后不会补取私有态）");
  }
  if (!source.includes("if (!isClient())")) {
    violations.push("没有客户端守卫（SSR 阶段可能跑私有态请求）");
  }
  if (!source.includes("inFlight")) {
    violations.push("没有单飞闸门（初始化与 watch 会同时命中、发两次请求）");
  }
  if (!source.includes("initializeSession")) {
    violations.push("没有 initializeSession（首屏初始化路径断了）");
  }
  return violations;
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

  /**
   * 回填计数（ADR-0024「后续修订（2026-11-04）」）：
   *
   * 硬刷新命中的是 30 秒匿名 SSR HTML，payload 里的计数是写之前的快照；私有态请求
   * 本来就在响应里带着权威计数。只投影两个布尔会留下「按钮已赞、数字没变」，
   * 所以投影必须包含计数，且回填时四个字段一起写回正文。
   */
  it("私有态投影必须包含权威计数（digg_count / favor_count），并在回填时一并写回正文", () => {
    const pick = privateFieldsSource.slice(privateFieldsSource.indexOf("export function pickArticlePrivateState"));
    expect(pick).toContain("digg_count");
    expect(pick).toContain("favor_count");

    const apply = pageSourcesSource.slice(pageSourcesSource.indexOf("function applyArticlePrivateState"));
    const applyBody = apply.slice(0, apply.indexOf("\n  }"));
    expect(applyBody).toContain("digg_count: state.digg_count");
    expect(applyBody).toContain("favor_count: state.favor_count");
  });

  /**
   * canary：判据必须能对**真实文件文本**报红，否则它只是恒绿的自言自语。
   * 做法是从仓库里那份 `useArticlePageSources.ts` 取真实函数体，把两行计数删掉，
   * 再喂给同一条判据 —— 删掉就该红，说明上面那条断言真的在盯着这两行。
   */
  it("判据不是恒绿：从真实源码里删掉计数回填后必须报红（canary）", () => {
    const apply = pageSourcesSource.slice(pageSourcesSource.indexOf("function applyArticlePrivateState"));
    const realBody = apply.slice(0, apply.indexOf("\n  }"));
    const brokenBody = realBody
      .split("\n")
      .filter(line => !line.includes("digg_count: state.digg_count") && !line.includes("favor_count: state.favor_count"))
      .join("\n");

    // 前提：真实源码里确实有这两行（否则过滤不会改变任何东西，这个 canary 就没意义）。
    expect(brokenBody).not.toBe(realBody);
    expect(brokenBody).not.toContain("digg_count: state.digg_count");
    expect(brokenBody).not.toContain("favor_count: state.favor_count");
  });

  it("页面接上 useArticlePrivateSession：onMounted 初始化 + 登录态 watch（页内登录也补取）", () => {
    // P0-4 的回归点：只在 onMounted 判一次 initializeSession() 会漏掉「文章页内登录成功」。
    const mounted = onMountedBlock(articlePageSource);
    expect(mounted).toContain("articlePrivateSession.initialize()");

    // 页面把登录态与私有态取数交给 composable，不再自己写一次性的 then 链。
    expect(articlePageSource).toContain("useArticlePrivateSession");
    expect(articlePageSource).toContain("isLoggedIn: () => authStore.isLoggedIn");
    expect(articlePageSource).toContain("refreshPrivate: () => refreshArticlePrivate()");
  });

  it("composable：登录态 false→true 的 watch + 客户端守卫 + 单飞闸门", () => {
    const violations = privateSessionViolations(articlePrivateSessionSource);
    expect(violations).toEqual([]);
  });

  it("判据不是恒绿：缺 watch / 缺客户端守卫 / 缺单飞闸门必须报红（canary）", () => {
    const bad = [
      "export function useArticlePrivateSession(options) {",
      "  async function initialize() {}",
      "  return { initialize };",
      "}",
    ].join("\n");
    expect(privateSessionViolations(bad)).toEqual(
      expect.arrayContaining([
        expect.stringContaining("watch"),
        expect.stringContaining("客户端守卫"),
        expect.stringContaining("单飞闸门"),
      ]),
    );
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

      const fetchArticlePrivate = vi.fn(async () => ({ is_digg: true, is_favor: true }));
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
      expect(sources.articlePrivate.value).toEqual({ is_digg: true, is_favor: true });
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
