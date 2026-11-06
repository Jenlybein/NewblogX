import { beforeEach, describe, expect, it, vi } from "vitest";

import { getArticleOutlineContext } from "~/services/outline";

const requestMock = vi.fn();

/**
 * #277 收缩后的阅读页左栏取数行为：只有 `?from=<kind>:<id>` 指向某一棵具名树
 * 时才打对应具名端点；来源缺失 / 不认识时按「没有可展示的树」返回空读模型，
 * 不打任何请求。旧共享端点 `/articles/{id}/outline-context` 不再有回退分支。
 */
describe("outline service：阅读页左栏按来路取具名树", () => {
  beforeEach(() => {
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: { request: requestMock },
    }));
    requestMock.mockReset();
    requestMock.mockResolvedValue({
      has_tree: false,
      owner_kind: 0,
      owner_id: "",
      nodes: [],
      current: null,
      previous: null,
      next: null,
    });
  });

  it("collection: 来源 → 合集具名端点，带 from 与 article_id，匿名读", async () => {
    await getArticleOutlineContext("101", "collection:c1");

    expect(requestMock).toHaveBeenCalledWith("/api/collections/c1/outline-tree", {
      auth: false,
      query: { from: "collection:c1", article_id: "101" },
    });
  });

  it("project: 来源 → 项目具名端点", async () => {
    await getArticleOutlineContext("101", "project:p1");

    expect(requestMock).toHaveBeenCalledWith("/api/projects/p1/outline-tree", {
      auth: false,
      query: { from: "project:p1", article_id: "101" },
    });
  });

  it("没有来源时不打请求，返回没有树的空读模型", async () => {
    const got = await getArticleOutlineContext("101");

    expect(requestMock).not.toHaveBeenCalled();
    expect(got.has_tree).toBe(false);
    expect(got.nodes).toEqual([]);
    expect(got.current).toBeNull();
  });

  it("来源有首尾空白时透传修剪后的字面值，不猜默认归属", async () => {
    await getArticleOutlineContext("101", "  collection:c1  ");

    expect(requestMock).toHaveBeenCalledWith("/api/collections/c1/outline-tree", {
      auth: false,
      query: { from: "collection:c1", article_id: "101" },
    });
  });

  it("不认识的来源也不打请求，返回没有树的空读模型", async () => {
    const got = await getArticleOutlineContext("101", "unknown:9");

    expect(requestMock).not.toHaveBeenCalled();
    expect(got.has_tree).toBe(false);
  });

  it("文章 ID 编码进路径之外，也作为 article_id 查询参数给出", async () => {
    await getArticleOutlineContext("a 1", "collection:c1");

    expect(requestMock).toHaveBeenCalledWith("/api/collections/c1/outline-tree", {
      auth: false,
      query: { from: "collection:c1", article_id: "a 1" },
    });
  });
});
