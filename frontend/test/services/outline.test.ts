import { beforeEach, describe, expect, it, vi } from "vitest";

import { getArticleOutlineContext } from "~/services/outline";

const requestMock = vi.fn();

/**
 * #275 阅读页左栏迁移的行为接缝：`?from=collection:<id>` 改走**合集具名端点**
 * `GET /api/collections/<id>/outline-tree`，并把当前文章 ID 作为 `article_id`
  * 查询参数带上（回答上一章 / 下一篇），来路原样透传为 `from`。
 *
 * 与 #276 共用这一条取数链：项目来源走项目具名端点、无来源沿用旧的
 * 文章上下文回退，本票只钉住合集侧与无来源侧。
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

  it("没有来源时沿用文章上下文回退（左栏仍只发一条请求）", async () => {
    await getArticleOutlineContext("101");

    expect(requestMock).toHaveBeenCalledWith(
      "/api/articles/101/outline-context",
      { auth: false, query: {} },
    );
  });

  it("来源有首尾空白时透传修剪后的字面值，不猜默认归属", async () => {
    await getArticleOutlineContext("101", "  collection:c1  ");

    expect(requestMock).toHaveBeenCalledWith("/api/collections/c1/outline-tree", {
      auth: false,
      query: { from: "collection:c1", article_id: "101" },
    });
  });

  it("不认识的来源不给具名树乱猜归属（退回文章上下文）", async () => {
    await getArticleOutlineContext("101", "unknown:9");

    expect(requestMock).toHaveBeenCalledWith(
      "/api/articles/101/outline-context",
      { auth: false, query: { from: "unknown:9" } },
    );
  });

  it("文章 ID 编码进路径之外，也作为 article_id 查询参数给出", async () => {
    await getArticleOutlineContext("a 1", "collection:c1");

    expect(requestMock).toHaveBeenCalledWith("/api/collections/c1/outline-tree", {
      auth: false,
      query: { from: "collection:c1", article_id: "a 1" },
    });
  });
});
