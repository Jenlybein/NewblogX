import { beforeEach, describe, expect, it, vi } from "vitest";
import { searchArticlesWithAi } from "~/services/search";

const requestMock = vi.fn();

describe("search service", () => {
  beforeEach(() => {
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
    requestMock.mockReset();
  });

  it("posts intelligent article-search prompts to the AI search-list endpoint", async () => {
    requestMock.mockResolvedValue({
      list: [],
      pagination: {
        mode: "has_more",
        page: 1,
        limit: 10,
        has_more: false,
      },
    });

    await searchArticlesWithAi("找一些关于 Nuxt SSR 落地的文章");

    expect(requestMock).toHaveBeenCalledWith("/api/ai/search/list", {
      method: "POST",
      body: {
        content: "找一些关于 Nuxt SSR 落地的文章",
      },
    });
  });
});
