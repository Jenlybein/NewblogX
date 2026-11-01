import { flushPromises } from "@vue/test-utils";
import { effectScope } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useArticlePageSources } from "~/composables/useArticlePageSources";
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
});

describe("SSR 匿名化：服务端渲染的数据里没有用户私有状态（ADR-0019 约束 2）", () => {
  it("服务端：文章里的 is_digg / is_favor 被剔除，字段名都不留在 SSR 数据里", async () => {
    installAwaitableAsyncData();
    installFakeNuxtApp();
    const fakes = createArticleFakes("server");

    const pendingSources = runInScope(() => useArticlePageSources(fakes.options));
    fakes.comments.resolve({ items: [] as CommentRootItem[], hasMore: false });
    fakes.score.resolve(null);
    fakes.authorInfo.resolve({ article_count: 3 } as ArticleAuthorInfo);
    fakes.article.resolve({ ...makeArticle(), is_digg: true, is_favor: true } as unknown as ArticleDetail);
    fakes.authorProfile.resolve(null);
    const sources = await pendingSources;

    expect(sources.article.value).not.toBeNull();
    expect("is_digg" in (sources.article.value as object)).toBe(false);
    expect("is_favor" in (sources.article.value as object)).toBe(false);
    // 公开字段不能跟着丢：分享卡片与页面内容靠它们。
    expect(sources.article.value?.title).toBe("正文标题");
    expect(sources.article.value?.content).toBe("正文内容");
  });

  it("服务端：评论条目的 is_digg / relation 也被剔除（评论区点赞态同样是登录用户私有状态）", async () => {
    installAwaitableAsyncData();
    installFakeNuxtApp();
    const fakes = createArticleFakes("server");

    const pendingSources = runInScope(() => useArticlePageSources(fakes.options));
    fakes.comments.resolve({
      items: [
        { id: "c1", content: "评论正文", is_digg: true, relation: 1 } as unknown as CommentRootItem,
      ],
      hasMore: false,
    });
    fakes.score.resolve(null);
    fakes.authorInfo.resolve({ article_count: 3 } as ArticleAuthorInfo);
    fakes.article.resolve(makeArticle());
    fakes.authorProfile.resolve(null);
    const sources = await pendingSources;

    const [comment] = sources.commentsPager.currentItems.value;
    expect(comment).toEqual({ id: "c1", content: "评论正文" });
    expect("is_digg" in (comment as object)).toBe(false);
    expect("relation" in (comment as object)).toBe(false);
  });

  it("客户端（跳转 / 首屏 hydration）：用户态保留 —— 客户端正是靠它补齐已点赞态", async () => {
    // canary：把剔除写成"一律剔除"时这条会红（客户端会永远显示未点赞）。
    installAwaitableAsyncData();
    installFakeNuxtApp();
    const fakes = createArticleFakes("navigation");

    const pendingSources = runInScope(() => useArticlePageSources(fakes.options));
    const sources = await pendingSources;

    fakes.article.resolve({ ...makeArticle(), is_digg: true, is_favor: true } as unknown as ArticleDetail);
    await flushPromises();

    expect(sources.article.value?.is_digg).toBe(true);
    expect(sources.article.value?.is_favor).toBe(true);
  });
});
