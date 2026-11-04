import { flushPromises } from "@vue/test-utils";
import { effectScope, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useArticlePrivateSession } from "~/composables/useArticlePrivateSession";
import { useArticlePageSources } from "~/composables/useArticlePageSources";
import type { ArticleAuthorInfo, ArticleDetail, CommentRootItem } from "~/types/api";
import { createReaderSwrCache } from "~/utils/readerSwrCache";
import { installAwaitableAsyncData, installFakeNuxtApp } from "../mocks/awaitableAsyncData";

/**
 * 文章页「页内登录成功」也要补私有态（评审 #248 / P0-4）。
 *
 * 病根：页面只在 onMounted 判一次 initializeSession()。用户在文章页被弹登录框、
 * 登录成功后才变成已登录，那条"登录后补一次私有态"的路径已经走完了 ——
 * 私有态永不请求，按钮继续显示未赞（库里其实已赞）。
 *
 * 这里真跑 `useArticlePageSources` + `useArticlePrivateSession`：
 * 登录态 false→true 时必须发出**一次**带鉴权取数，并把 is_digg / is_favor
 * 补进正文（按钮据此变"已赞"）。
 */

const ARTICLE_ID = "a1";
const AUTHOR_ID = "u9";

const scopes: Array<ReturnType<typeof effectScope>> = [];

async function runInScope<T>(factory: () => T) {
  const scope = effectScope();
  scopes.push(scope);
  return await scope.run(factory);
}

afterEach(() => {
  while (scopes.length) {
    scopes.pop()?.stop();
  }
});

function makeArticle(): ArticleDetail {
  return {
    id: ARTICLE_ID,
    title: "正文标题",
    abstract: "摘要",
    content: "正文内容",
    author_id: AUTHOR_ID,
    author_name: "作者甲",
  } as unknown as ArticleDetail;
}

async function createSources(fetchArticlePrivate: () => Promise<{ is_digg: boolean; is_favor: boolean }>) {
  installAwaitableAsyncData();
  installFakeNuxtApp();
  const cache = createReaderSwrCache({ now: () => 1_000 });
  const sources = await runInScope(() =>
    useArticlePageSources({
      mode: "navigation",
      articleId: ARTICLE_ID,
      swrCache: cache,
      fetchArticle: async () => makeArticle(),
      fetchScore: async () => null,
      fetchComments: async () => ({ items: [] as CommentRootItem[], hasMore: false }),
      fetchAuthorInfo: async () => ({ article_count: 3 } as ArticleAuthorInfo),
      fetchAuthorProfile: async () => null,
      fetchArticlePrivate,
    }),
  );
  await flushPromises();
  return sources;
}

describe("useArticlePrivateSession：登录态 false→true 补一次私有态", () => {
  it("页内登录成功：发出一次带鉴权私有态请求，按钮状态变成已赞 / 已收藏", async () => {
    const fetchArticlePrivate = vi.fn(async () => ({ is_digg: true, is_favor: true }));
    const sources = await createSources(fetchArticlePrivate);

    const loggedIn = ref(false);
    // 用 wrapper 接住 composable 的原始返回值：直接 await 会被 useAsyncData 的
    // thenable 语义解包成内部 handle（丢掉 refreshIfNeeded / initialize 这类扩展方法）。
    const session = (
      await runInScope(() => ({
        session: useArticlePrivateSession({
          isLoggedIn: () => loggedIn.value,
          // 初始化时还是游客：用户在页内登录成功后登录态才翻转。
          initializeSession: async () => loggedIn.value,
          refreshPrivate: () => sources.refreshArticlePrivate(),
          isClient: () => true,
        }),
      }))
    ).session;

    await session.initialize();
    expect(fetchArticlePrivate).not.toHaveBeenCalled();
    expect(sources.article.value?.is_digg).toBeUndefined();

    loggedIn.value = true;
    await flushPromises();

    expect(fetchArticlePrivate).toHaveBeenCalledTimes(1);
    // "按钮变已赞"：私有态已经补进本次渲染的正文。
    expect(sources.article.value?.is_digg).toBe(true);
    expect(sources.article.value?.is_favor).toBe(true);
  });

  it("初始化与 watch 同时命中（登录初始化把匿名翻成已登录）也只发一次请求", async () => {
    const fetchArticlePrivate = vi.fn(async () => ({ is_digg: true, is_favor: false }));
    const sources = await createSources(fetchArticlePrivate);

    const loggedIn = ref(false);
    const session = (
      await runInScope(() => ({
        session: useArticlePrivateSession({
          isLoggedIn: () => loggedIn.value,
          initializeSession: async () => {
            loggedIn.value = true;
            return true;
          },
          refreshPrivate: () => sources.refreshArticlePrivate(),
          isClient: () => true,
        }),
      }))
    ).session;

    await session.initialize();
    await flushPromises();

    expect(fetchArticlePrivate).toHaveBeenCalledTimes(1);
    expect(sources.article.value?.is_digg).toBe(true);
  });

  it("登出后再登录是新的会话：再补一次（可能已在别的设备赞过）", async () => {
    const fetchArticlePrivate = vi.fn(async () => ({ is_digg: true, is_favor: false }));
    const sources = await createSources(fetchArticlePrivate);

    const loggedIn = ref(false);
    const session = (
      await runInScope(() => ({
        session: useArticlePrivateSession({
          isLoggedIn: () => loggedIn.value,
          initializeSession: async () => false,
          refreshPrivate: () => sources.refreshArticlePrivate(),
          isClient: () => true,
        }),
      }))
    ).session;

    loggedIn.value = true;
    await flushPromises();
    expect(fetchArticlePrivate).toHaveBeenCalledTimes(1);

    loggedIn.value = false;
    await flushPromises();
    loggedIn.value = true;
    await flushPromises();
    expect(fetchArticlePrivate).toHaveBeenCalledTimes(2);
    expect(session.ensureLoaded).toBeTypeOf("function");
  });

  it("SSR 阶段（非客户端）登录态变化不触发取数", async () => {
    const fetchArticlePrivate = vi.fn(async () => ({ is_digg: true, is_favor: true }));
    const sources = await createSources(fetchArticlePrivate);

    const loggedIn = ref(false);
    const session = (
      await runInScope(() => ({
        session: useArticlePrivateSession({
          isLoggedIn: () => loggedIn.value,
          initializeSession: async () => false,
          refreshPrivate: () => sources.refreshArticlePrivate(),
          isClient: () => false,
        }),
      }))
    ).session;

    loggedIn.value = true;
    await flushPromises();

    expect(fetchArticlePrivate).not.toHaveBeenCalled();
    expect(session.ensureLoaded).toBeTypeOf("function");
  });

  it("用户已手动点过赞（shouldSkip）：不覆盖乐观结果", async () => {
    const fetchArticlePrivate = vi.fn(async () => ({ is_digg: false, is_favor: false }));
    const sources = await createSources(fetchArticlePrivate);

    const loggedIn = ref(false);
    const touched = ref(false);
    const session = (
      await runInScope(() => ({
        session: useArticlePrivateSession({
          isLoggedIn: () => loggedIn.value,
          initializeSession: async () => false,
          refreshPrivate: () => sources.refreshArticlePrivate(),
          shouldSkip: () => touched.value,
          isClient: () => true,
        }),
      }))
    ).session;

    touched.value = true;
    loggedIn.value = true;
    await flushPromises();

    expect(fetchArticlePrivate).not.toHaveBeenCalled();
    expect(session.ensureLoaded).toBeTypeOf("function");
  });
});
