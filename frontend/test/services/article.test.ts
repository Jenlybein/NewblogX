import { beforeEach, describe, expect, it, vi } from "vitest";

import { getHotTopArticles } from "~/services/article";

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
