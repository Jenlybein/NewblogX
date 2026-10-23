import { shallowRef } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useArticleSearch } from "~/composables/useArticleSearch";
import type { SearchArticleResponse } from "~/types/api";

const searchArticlesMock = vi.hoisted(() => vi.fn());

vi.mock("~/services/search", () => ({
  searchArticles: searchArticlesMock,
}));

function installUseAsyncDataStub() {
  const useAsyncData = vi.fn(async (_key: unknown, handler: () => Promise<SearchArticleResponse>, options: { immediate?: boolean } = {}) => {
    const data = shallowRef<SearchArticleResponse | null>(null);
    const pending = shallowRef(false);
    const error = shallowRef<unknown>(null);
    const refresh = vi.fn(async () => {
      data.value = await handler();
      return data.value;
    });

    if (options.immediate !== false) {
      data.value = await handler();
    }

    return {
      data,
      pending,
      error,
      refresh,
    };
  });

  vi.stubGlobal("useAsyncData", useAsyncData);
  return useAsyncData;
}

describe("useArticleSearch", () => {
  beforeEach(() => {
    searchArticlesMock.mockReset();
  });

  it("returns article search data when the request succeeds", async () => {
    installUseAsyncDataStub();
    searchArticlesMock.mockResolvedValue({
      list: [{
        id: "a1",
        created_at: "2026-04-17T00:00:00Z",
        updated_at: "2026-04-17T00:00:00Z",
        title: "文章",
        cover: "",
        view_count: 0,
        digg_count: 0,
        comment_count: 0,
        favor_count: 0,
        comments_toggle: true,
        publish_status: 3,
        visibility_status: "visible",
        tags: [],
        author: {
          id: "u1",
          nickname: "作者",
          avatar: "",
        },
      }],
      pagination: {
        mode: "count",
        page: 1,
        limit: 12,
        has_more: false,
        total: 1,
        total_pages: 1,
      },
    });

    const result = await useArticleSearch({
      type: 3,
      author_id: "u1",
      page: 1,
      limit: 12,
      page_mode: "count",
    });

    expect(searchArticlesMock).toHaveBeenCalledWith({
      type: 3,
      author_id: "u1",
      page: 1,
      limit: 12,
      page_mode: "count",
    });
    expect(result.articles.value).toHaveLength(1);
    expect(result.total.value).toBe(1);
    expect(result.requestError.value).toBeNull();
  });

  it("returns a stable fallback and records the request error when search fails", async () => {
    installUseAsyncDataStub();
    const requestError = new Error("502 Bad Gateway");
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    searchArticlesMock.mockRejectedValue(requestError);

    const result = await useArticleSearch({
      type: 3,
      author_id: "u1",
      page: 2,
      limit: 9,
      page_mode: "count",
    });

    expect(result.articles.value).toEqual([]);
    expect(result.pagination.value).toMatchObject({
      mode: "count",
      page: 2,
      limit: 9,
      has_more: false,
      total: 0,
      total_pages: 0,
    });
    expect(result.requestError.value).toBe(requestError);
    expect(warnSpy).toHaveBeenCalledWith(
      expect.stringContaining("[useArticleSearch] request failed, using fallback"),
      expect.objectContaining({ author_id: "u1" }),
    );
  });

  it("passes lazy and server options through to Nuxt async data", async () => {
    const useAsyncData = installUseAsyncDataStub();
    searchArticlesMock.mockResolvedValue({
      list: [],
      pagination: {
        mode: "has_more",
        page: 1,
        limit: 12,
        has_more: false,
      },
    });

    await useArticleSearch(
      { type: 1, page: 1 },
      {
        lazy: true,
        server: false,
        immediate: false,
      },
    );

    expect(useAsyncData).toHaveBeenCalledWith(expect.any(Function), expect.any(Function), expect.objectContaining({
      lazy: true,
      server: false,
      immediate: false,
    }));
    expect(searchArticlesMock).not.toHaveBeenCalled();
  });
});
