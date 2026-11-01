import { flushPromises } from "@vue/test-utils";
import { effectScope } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useArticlePageSources } from "~/composables/useArticlePageSources";
import { createReaderSwrCache } from "~/utils/readerSwrCache";
import type {
  AiArticleScoringResponseData,
  ArticleAuthorInfo,
  ArticleDetail,
  CommentRootItem,
  UserBaseInfo,
} from "~/types/api";
import { installAwaitableAsyncData, installFakeNuxtApp } from "../mocks/awaitableAsyncData";

const scopes: Array<ReturnType<typeof effectScope>> = [];

async function runInScope<T>(factory: () => Promise<T>) {
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

function blockedAfter(ms = 200) {
  return new Promise<"blocked">((resolve) => setTimeout(() => resolve("blocked"), ms));
}

const ARTICLE_ID = "a1";
const AUTHOR_ID = "u9";

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

function makeArticleWith(title: string): ArticleDetail {
  return { ...makeArticle(), title } as ArticleDetail;
}

afterEach(() => {
  while (scopes.length) {
    scopes.pop()?.stop();
  }
});

interface ArticleFakes {
  starts: string[];
  article: ReturnType<typeof deferred<ArticleDetail>>;
  score: ReturnType<typeof deferred<AiArticleScoringResponseData | null>>;
  comments: ReturnType<typeof deferred<{ items: CommentRootItem[]; hasMore: boolean }>>;
  authorInfo: ReturnType<typeof deferred<ArticleAuthorInfo>>;
  authorProfile: ReturnType<typeof deferred<UserBaseInfo | null>>;
  options: Parameters<typeof useArticlePageSources>[0];
}

function createArticleFakes(mode: "server" | "hydration" | "navigation"): ArticleFakes {
  const starts: string[] = [];
  const article = deferred<ArticleDetail>();
  const score = deferred<AiArticleScoringResponseData | null>();
  const comments = deferred<{ items: CommentRootItem[]; hasMore: boolean }>();
  const authorInfo = deferred<ArticleAuthorInfo>();
  const authorProfile = deferred<UserBaseInfo | null>();

  const options = {
    mode,
    articleId: ARTICLE_ID,
    fetchArticle: vi.fn(() => {
      starts.push("article");
      return article.promise;
    }),
    fetchScore: vi.fn(() => {
      starts.push("score");
      return score.promise;
    }),
    fetchComments: vi.fn(() => {
      starts.push("comments");
      return comments.promise;
    }),
    fetchAuthorInfo: vi.fn(() => {
      starts.push("author-info");
      return authorInfo.promise;
    }),
    fetchAuthorProfile: vi.fn(() => {
      starts.push("author-profile");
      return authorProfile.promise;
    }),
  } as unknown as Parameters<typeof useArticlePageSources>[0];

  return { starts, article, score, comments, authorInfo, authorProfile, options };
}

describe("useArticlePageSources：正文优先 + 第一波并行 + 作者第二波", () => {
  it("客户端跳转：正文/评论/评分在正文返回前都已发起，句柄先回来（骨架可观察）", async () => {
    installAwaitableAsyncData();
    installFakeNuxtApp();
    const fakes = createArticleFakes("navigation");

    const pendingSources = runInScope(() => useArticlePageSources(fakes.options));
    const settled = await Promise.race([pendingSources, blockedAfter()]);
    expect(settled).not.toBe("blocked");
    const sources = settled as Awaited<typeof pendingSources>;

    // 第一波并行的证据：正文还没回来，评论与评分请求已经在路上。
    expect(fakes.starts.slice().sort()).toEqual(["article", "comments", "score"]);
    // 作者资料要等正文里的 author_id，此刻还没发起（第二波）。
    expect(fakes.starts).not.toContain("author-info");
    expect(fakes.starts).not.toContain("author-profile");

    // 骨架状态：正文未到、评论区也未到。
    expect(sources.articlePending.value).toBe(true);
    expect(sources.article.value).toBeNull();
    expect(sources.commentsPager.pending.value).toBe(true);

    fakes.article.resolve(makeArticle());
    await flushPromises();

    expect(fakes.starts).toContain("author-info");
    expect(fakes.starts).toContain("author-profile");
    expect(sources.article.value?.title).toBe("正文标题");

    fakes.comments.resolve({ items: [] as CommentRootItem[], hasMore: false });
    fakes.authorInfo.resolve({ article_count: 3 } as ArticleAuthorInfo);
    fakes.authorProfile.resolve(null);
    fakes.score.resolve(null);
    await flushPromises();

    expect(sources.commentsPager.currentItems.value).toEqual([]);
    expect(sources.authorInfo.value).toEqual({ article_count: 3 });
    expect(sources.articleScoreSummary.value).toBeNull();
  });

  it("服务端：正文/评论/评分等齐，且作者第二波也落定后才返回（SSR HTML 里作者区不缺数据）", async () => {
    installAwaitableAsyncData();
    installFakeNuxtApp();
    const fakes = createArticleFakes("server");

    const pendingSources = runInScope(() => useArticlePageSources(fakes.options));
    let resolved = false;
    pendingSources.then(() => {
      resolved = true;
    });

    fakes.comments.resolve({ items: [] as CommentRootItem[], hasMore: false });
    fakes.score.resolve(null);
    await flushPromises();
    expect(resolved).toBe(false);

    fakes.article.resolve(makeArticle());
    await flushPromises();
    expect(resolved).toBe(false);

    fakes.authorInfo.resolve({ article_count: 3 } as ArticleAuthorInfo);
    fakes.authorProfile.resolve(null);
    await flushPromises();
    expect(resolved).toBe(true);

    const sources = await pendingSources;
    expect(sources.article.value?.author_id).toBe(AUTHOR_ID);
    expect(sources.authorInfo.value).toEqual({ article_count: 3 });
  });

  it("首屏 hydration：作者资料不重复请求（payload 里已有），作者主页资料仍补齐", async () => {
    installAwaitableAsyncData();
    installFakeNuxtApp();
    const fakes = createArticleFakes("hydration");

    const pendingSources = runInScope(() => useArticlePageSources(fakes.options));
    let resolved = false;
    pendingSources.then(() => {
      resolved = true;
    });

    fakes.comments.resolve({ items: [] as CommentRootItem[], hasMore: false });
    await flushPromises();
    expect(resolved).toBe(true);
    const sources = await pendingSources;

    fakes.article.resolve(makeArticle());
    await flushPromises();

    expect(fakes.starts).toContain("author-profile");
    expect(fakes.starts).not.toContain("author-info");
    expect(sources.commentsPager.currentPage.value).toBe(1);
  });

  // ---- 30s SWR（票 #85 验收项） ----
  // 观众路径：首页 → 文章 → 返回首页 → 再进同一篇文章，30 秒内正文/评论不重复请求。
  it("30 秒内重进同一篇文章：正文与评论命中缓存不再请求，私有字段不进缓存", async () => {
    installAwaitableAsyncData();
    installFakeNuxtApp();
    const cache = createReaderSwrCache({ now: () => 1_000 });

    const articleWithPrivate = {
      ...makeArticle(),
      is_digg: true,
      is_favor: true,
    } as unknown as ArticleDetail;
    const comment = {
      id: "c1",
      content: "缓存里的评论",
      is_digg: true,
      relation: 1,
    } as unknown as CommentRootItem;

    // 第一次进文章：正文与评论各取一次，并把剔除私有字段后的副本写进缓存。
    await runInScope(() =>
      useArticlePageSources({
        mode: "navigation",
        articleId: ARTICLE_ID,
        swrCache: cache,
        fetchArticle: async () => articleWithPrivate,
        fetchScore: async () => null,
        fetchComments: async () => ({ items: [comment], hasMore: false }),
        fetchAuthorInfo: async () => ({ article_count: 3 } as ArticleAuthorInfo),
        fetchAuthorProfile: async () => null,
      }),
    );
    await flushPromises();
    await flushPromises();

    const cachedArticle = cache.read<Record<string, unknown>>("article-a1");
    expect(cachedArticle.state).toBe("fresh");
    if (cachedArticle.state !== "miss") {
      expect(cachedArticle.value).not.toHaveProperty("is_digg");
      expect(cachedArticle.value).not.toHaveProperty("is_favor");
      expect(cachedArticle.value.title).toBe("正文标题");
    }

    const cachedComments = cache.read<{ items: Array<Record<string, unknown>> }>("article-comments:a1#1");
    expect(cachedComments.state).toBe("fresh");
    if (cachedComments.state !== "miss") {
      expect(cachedComments.value.items[0]).not.toHaveProperty("is_digg");
      expect(cachedComments.value.items[0]).not.toHaveProperty("relation");
    }

    // 30 秒内重进：正文 / 评论都不再请求，界面直接拿缓存（作者资料仍走第二波）。
    const fetchArticle = vi.fn(async () => makeArticle());
    const fetchScore = vi.fn(async () => null);
    const fetchComments = vi.fn(async () => ({ items: [] as CommentRootItem[], hasMore: false }));
    const fetchAuthorInfo = vi.fn(async () => ({ article_count: 3 } as ArticleAuthorInfo));
    const fetchAuthorProfile = vi.fn(async () => null);
    const second = await runInScope(() =>
      useArticlePageSources({
        mode: "navigation",
        articleId: ARTICLE_ID,
        swrCache: cache,
        fetchArticle,
        fetchScore,
        fetchComments,
        fetchAuthorInfo,
        fetchAuthorProfile,
      }),
    );
    await flushPromises();
    await flushPromises();

    expect(fetchArticle).not.toHaveBeenCalled();
    expect(fetchComments).not.toHaveBeenCalled();
    expect(second.article.value?.title).toBe("正文标题");
    expect(second.article.value).not.toHaveProperty("is_digg");
    expect(second.commentsPager.currentItems.value).toEqual([{ id: "c1", content: "缓存里的评论" }]);
    expect(second.commentsPager.pending.value).toBe(false);
  });

  it("超过 30 秒重进：旧值先渲染，正文与评论在后台静默刷新（不闪）", async () => {
    installAwaitableAsyncData();
    installFakeNuxtApp();
    let clock = 1_000;
    const cache = createReaderSwrCache({ now: () => clock });
    cache.write("article-a1", {
      id: ARTICLE_ID,
      title: "旧标题",
      author_id: AUTHOR_ID,
      author_name: "作者甲",
    });
    cache.write("article-comments:a1#1", { items: [], hasMore: false });

    clock = 1_000 + 30_001;
    const article = deferred<ArticleDetail>();
    const comments = deferred<{ items: CommentRootItem[]; hasMore: boolean }>();
    const fetchArticle = vi.fn(() => article.promise);
    const fetchComments = vi.fn(() => comments.promise);
    const second = await runInScope(() =>
      useArticlePageSources({
        mode: "navigation",
        articleId: ARTICLE_ID,
        swrCache: cache,
        fetchArticle,
        fetchScore: async () => null,
        fetchComments,
        fetchAuthorInfo: async () => ({ article_count: 3 } as ArticleAuthorInfo),
        fetchAuthorProfile: async () => null,
      }),
    );
    await flushPromises();

    // 旧正文已经上屏；重验证在后台跑，数据没有被清空。
    expect(second.article.value?.title).toBe("旧标题");
    expect(second.commentsPager.currentItems.value).toEqual([]);
    expect(second.commentsPager.pending.value).toBe(false);
    expect(fetchArticle).toHaveBeenCalledTimes(1);
    expect(fetchComments).toHaveBeenCalledTimes(1);

    article.resolve(makeArticleWith("新标题"));
    comments.resolve({ items: [] as CommentRootItem[], hasMore: false });
    await flushPromises();

    expect(second.article.value?.title).toBe("新标题");
  });

  it("不同文章 id 不串数据：A 的缓存不会喂给 B", async () => {
    installAwaitableAsyncData();
    installFakeNuxtApp();
    const cache = createReaderSwrCache({ now: () => 1_000 });
    cache.write("article-a1", { id: "a1", title: "文章甲", author_id: AUTHOR_ID });
    cache.write("article-comments:a1#1", { items: [], hasMore: false });

    const fetchArticle = vi.fn(async () => ({ ...makeArticleWith("文章乙"), id: "a2" }) as ArticleDetail);
    const fetchComments = vi.fn(async () => ({ items: [] as CommentRootItem[], hasMore: false }));
    const second = await runInScope(() =>
      useArticlePageSources({
        mode: "navigation",
        articleId: "a2",
        swrCache: cache,
        fetchArticle,
        fetchScore: async () => null,
        fetchComments,
        fetchAuthorInfo: async () => ({ article_count: 3 } as ArticleAuthorInfo),
        fetchAuthorProfile: async () => null,
      }),
    );
    await flushPromises();

    expect(fetchArticle).toHaveBeenCalledTimes(1);
    expect(fetchComments).toHaveBeenCalledTimes(1);
    expect(second.article.value?.title).toBe("文章乙");
  });
});
