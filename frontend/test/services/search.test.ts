import { beforeEach, describe, expect, it, vi } from "vitest";
import { createCategory, getCategoryOptions, getTagOptions, searchArticlesWithAi } from "~/services/search";

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

  it("creates a category through the existing article-category endpoint", async () => {
    requestMock.mockResolvedValue({});

    await createCategory("  前端  ");

    // 复用既有创建接口（不新增端点）；标题原样交给服务端，由后端去空白/查重。
    expect(requestMock).toHaveBeenCalledWith("/api/articles/category", {
      method: "POST",
      body: {
        title: "  前端  ",
      },
    });
  });
});

// 下拉选项的形状回归（#320）。
//
// 两个 options 端点的 data 恒为 `{list, count}`（kernel.ListData），而 request()
// 返回的是**已解包的 data**。所以"把 data 当数组 .map"会抛 `list.map is not a
// function`，再被调用点的兜底吞掉 —— 用户看到的就是永远空白、一声不响的下拉。
// 这组用例钉住三件事：载荷形状正确、空/缺字段不抛错、值为 ID 且标题可见。
describe("search service · 下拉选项载荷形状（#320）", () => {
  beforeEach(() => {
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
    requestMock.mockReset();
  });

  it("标签下拉读 {list, count}，标题做 label、ID 做 value", async () => {
    requestMock.mockResolvedValue({
      list: [
        { id: "tag-1", title: "Go" },
        { id: "tag-2", title: "Vue" },
      ],
      count: 2,
    });

    await expect(getTagOptions()).resolves.toEqual([
      { label: "Go", value: "tag-1" },
      { label: "Vue", value: "tag-2" },
    ]);
    expect(requestMock).toHaveBeenCalledWith("/api/articles/tags/options", { auth: false });
  });

  it("分类下拉读 {list, count}，并把 user_id 带在查询串上", async () => {
    requestMock.mockResolvedValue({
      list: [{ id: "cat-1", title: "后端" }],
      count: 1,
    });

    await expect(getCategoryOptions("u1")).resolves.toEqual([{ label: "后端", value: "cat-1" }]);
    expect(requestMock).toHaveBeenCalledWith("/api/articles/category/options", {
      auth: false,
      query: { user_id: "u1" },
    });
  });

  it("空列表不抛错，映射成空选项", async () => {
    requestMock.mockResolvedValue({ list: [], count: 0 });

    await expect(getTagOptions()).resolves.toEqual([]);
    await expect(getCategoryOptions("u1")).resolves.toEqual([]);
  });

  it("条目缺 title 或 id 时跳过该条目，其余照常映射", async () => {
    requestMock.mockResolvedValue({
      list: [{ id: "cat-1", title: "后端" }, { id: "cat-2" }, { title: "没有 ID" }, {}],
      count: 4,
    });

    // 缺任意一半都无法成为可选项：没有 ID 选不中，没有标题看不出是什么。
    await expect(getCategoryOptions("u1")).resolves.toEqual([{ label: "后端", value: "cat-1" }]);
  });

  it("载荷连 list 都没有时返回空选项，不抛错", async () => {
    requestMock.mockResolvedValue({});
    await expect(getTagOptions()).resolves.toEqual([]);

    requestMock.mockResolvedValue(null);
    await expect(getCategoryOptions("u1")).resolves.toEqual([]);
  });
});
