import { describe, expect, it } from "vitest";

import {
  READER_HOME_CACHE_KEYS,
  readerArticleAuthorBaseCacheKey,
  readerArticleAuthorInfoCacheKey,
  readerArticleCacheKey,
  readerArticleCommentsCacheKey,
  readerArticleScoreCacheKey,
} from "~/utils/readerCacheKeys";

/**
 * 缓存键是**线上契约**（ADR-0019 决策 4）：键变了，30 秒内的旧缓存全部命中不到。
 * 所以这里把形态锁死 —— 想改就是一次显式决策（同步改测试 + ADR/票里说明），不能顺手重命名。
 */
describe("readerCacheKeys：跨导航缓存键的形态与唯一来源", () => {
  it("首页四个键与页码无关，形状固定", () => {
    expect(READER_HOME_CACHE_KEYS).toEqual({
      banners: "home-banners",
      hotTop: "home-hot-top",
      latestArticles: "home-latest-articles",
      sidebarCards: "home-sidebar-cards",
    });
  });

  it("文章相关键都带资源 id，不同资源不会共用键", () => {
    expect(readerArticleCacheKey("a1")).toBe("article-a1");
    expect(readerArticleScoreCacheKey("a1")).toBe("article-score-summary:a1");
    expect(readerArticleAuthorInfoCacheKey("u1")).toBe("article-author-info-u1");
    expect(readerArticleAuthorBaseCacheKey("u1")).toBe("article-author-base-u1");
    expect(readerArticleCommentsCacheKey("a1")).toBe("article-comments:a1");
    expect(readerArticleCommentsCacheKey("a2")).not.toBe(readerArticleCommentsCacheKey("a1"));
  });

  it("评论键是分页缓存的前缀（页码由 usePagedResourceCache 追加）", () => {
    // usePagedResourceCache 内部写的是 `<cacheKey>#<page>`，这里只要保证前缀不自己带 #。
    expect(readerArticleCommentsCacheKey("a1")).not.toContain("#");
  });
});
