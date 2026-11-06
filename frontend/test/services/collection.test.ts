import { beforeEach, describe, expect, it, vi } from "vitest";

import { getCollectionDetail, getCollections } from "~/services/collection";

const requestMock = vi.fn();

/**
 * #275 合集读者侧迁移的行为接缝：详情页的树改由 #274 的**合集具名端点**
 * `GET /api/collections/<id>/outline-tree` 给出，实体标题/描述仍由详情端点给。
 * 两次请求都是匿名公开读（读者侧呈现不携带登录身份）。
 *
 * 断言的是可观察结果：请求打到哪条路径、以及合成后的视图形状是否仍能被
 * 详情页消费（`outline.nodes` / `article_count` / `cover`）。
 */
function namedTree(overrides: Record<string, unknown> = {}) {
  return {
    has_tree: true,
    owner_kind: 1,
    owner_id: "c1",
    nodes: [{ id: "n1", kind: 2, article_id: "101", article_title: "开篇" }],
    article_count: 3,
    cover: "https://cdn.example.com/first.png",
    current: null,
    previous: null,
    next: null,
    ...overrides,
  };
}

function detailMeta(overrides: Record<string, unknown> = {}) {
  return {
    id: "c1",
    title: "Nuxt 部署实战",
    description: "从零到上线",
    cover: "",
    created_at: "2026-01-01T00:00:00Z",
    article_count: 0,
    outline: { owner_kind: 1, owner_id: "c1", nodes: [] },
    ...overrides,
  };
}

describe("collection service：合集详情页的树走具名端点", () => {
  beforeEach(() => {
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: { request: requestMock },
    }));
    requestMock.mockReset();
    requestMock.mockImplementation((path: string) =>
      Promise.resolve(path.endsWith("/outline-tree") ? namedTree() : detailMeta()),
    );
  });

  it("列表仍走 /api/collections（本票只切详情与左栏）", async () => {
    await getCollections(2, 8);

    expect(requestMock).toHaveBeenCalledWith("/api/collections", {
      auth: false,
      query: { page: 2, limit: 8 },
    });
  });

  it("详情同时请求实体详情与 /api/collections/<id>/outline-tree，均匿名读", async () => {
    await getCollectionDetail("c1");

    expect(requestMock).toHaveBeenCalledWith("/api/collections/c1", { auth: false });
    expect(requestMock).toHaveBeenCalledWith("/api/collections/c1/outline-tree", { auth: false });
  });

  it("树、篇数与封面回退来自具名端点，标题/描述仍来自详情端点", async () => {
    const detail = await getCollectionDetail("c1");

    expect(detail.title).toBe("Nuxt 部署实战");
    expect(detail.description).toBe("从零到上线");
    expect(detail.outline.nodes).toEqual(namedTree().nodes);
    expect(detail.article_count).toBe(3);
    expect(detail.cover).toBe("https://cdn.example.com/first.png");
  });

  it("合集自己上传的封面优先于具名树的回退封面", async () => {
    requestMock.mockImplementation((path: string) =>
      Promise.resolve(
        path.endsWith("/outline-tree")
          ? namedTree()
          : detailMeta({ cover: "https://cdn.example.com/uploaded.png" }),
      ),
    );

    const detail = await getCollectionDetail("c1");

    expect(detail.cover).toBe("https://cdn.example.com/uploaded.png");
  });

  it("未展示合集：详情端点失败时整体按不存在处理，不吞错误", async () => {
    requestMock.mockImplementation((path: string) =>
      path.endsWith("/outline-tree")
        ? Promise.resolve(namedTree({ has_tree: false, nodes: [], article_count: 0 }))
        : Promise.reject(new Error("not found")),
    );

    await expect(getCollectionDetail("c1")).rejects.toThrow("not found");
  });

  it("数字 ID 也走同一具名端点", async () => {
    await getCollectionDetail(42);

    expect(requestMock).toHaveBeenCalledWith("/api/collections/42/outline-tree", { auth: false });
  });
});
