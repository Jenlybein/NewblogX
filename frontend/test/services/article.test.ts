import { beforeEach, describe, expect, it, vi } from "vitest";

import { getArticleDetail, getArticleDetailAnonymous, getHotTopArticles } from "~/services/article";

const requestMock = vi.fn();

describe("article service：首页热门置顶", () => {
  beforeEach(() => {
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
    requestMock.mockReset();
    requestMock.mockResolvedValue({ list: [], count: 0 });
  });

  it("读运营位接口 /api/articles/hot-top，且是公开请求（不带旧 type=2 参数）", async () => {
    await getHotTopArticles();

    expect(requestMock).toHaveBeenCalledWith("/api/articles/hot-top", { auth: false });
  });
});

describe("article service：文章详情有两种取数口径（ADR-0019 约束 2）", () => {
  beforeEach(() => {
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
    requestMock.mockReset();
    requestMock.mockResolvedValue({ id: "a1", title: "标题" });
  });

  it("默认带鉴权：客户端要靠它拿到 is_digg（已点赞态由客户端补齐）", async () => {
    await getArticleDetail("a1");

    expect(requestMock).toHaveBeenCalledWith("/api/articles/a1");
  });

  it("显式匿名（auth: false）：SSR 取数不得携带任何登录身份，否则缓存会串号", async () => {
    await getArticleDetailAnonymous("a1");

    expect(requestMock).toHaveBeenCalledWith("/api/articles/a1", { auth: false });
  });
});
