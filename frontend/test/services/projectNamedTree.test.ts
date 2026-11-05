import { beforeEach, describe, expect, it, vi } from "vitest";

import { getArticleOutlineContext } from "~/services/outline";
import { getProjectDetail, getProjectOutlineTree } from "~/services/project";

/**
 * #276 项目侧消费端迁移的行为接缝：读者项目详情页与阅读页左栏只经 #274 的
 * 项目具名端点 `/projects/{id}/outline-tree` 找回同一棵树。
 *
 * 这里只观察「页面取数实际打到哪个 HTTP 接口、带了什么查询参数」，不断言源码
 * 里有没有某个函数名；`useArticleOutlineContext` 的取数时序与共用呈现组件由
 * 既有行为夹具覆盖。合集侧分派归 #275，本文件不碰它的形状。
 */

const requestMock = vi.fn();

beforeEach(() => {
  vi.stubGlobal("__useNuxtAppMock", () => ({ $api: { request: requestMock } }));
  requestMock.mockReset();
  requestMock.mockResolvedValue({
    has_tree: true,
    owner_kind: 2,
    owner_id: "p1",
    nodes: [],
    article_count: 0,
    cover: "",
    current: null,
    previous: null,
    next: null,
  });
});

describe("项目详情页：改走项目具名端点", () => {
  it("取树只打 GET /api/projects/<id>/outline-tree，且公开读", async () => {
    await getProjectOutlineTree("p1");

    expect(requestMock).toHaveBeenCalledWith("/api/projects/p1/outline-tree", { auth: false });
  });

  it("详情并发读项目主表与项目具名树，合成回既有 ProjectDetail 形状", async () => {
    requestMock.mockImplementation((path: string) => {
      if (path === "/api/projects/p1") {
        return Promise.resolve({
          id: "p1",
          title: "重构实战",
          description: "按项目成组阅读",
          cover: "",
          created_at: "2026-01-01T00:00:00Z",
          article_count: 99,
          outline: { owner_kind: 2, owner_id: "p1", nodes: [{ id: "legacy" }] },
        });
      }
      return Promise.resolve({
        has_tree: true,
        owner_kind: 2,
        owner_id: "p1",
        nodes: [{ id: "named" }],
        article_count: 2,
        cover: "https://cdn.example/first.png",
        current: null,
        previous: null,
        next: null,
      });
    });

    const detail = await getProjectDetail("p1");

    expect(requestMock).toHaveBeenCalledWith("/api/projects/p1", { auth: false });
    expect(requestMock).toHaveBeenCalledWith("/api/projects/p1/outline-tree", { auth: false });
    // 树、篇数、封面回退只认项目具名端点；主表的旧树节点不被页面看到。
    expect(detail.title).toBe("重构实战");
    expect(detail.outline.nodes).toEqual([{ id: "named" }]);
    expect(detail.article_count).toBe(2);
    expect(detail.cover).toBe("https://cdn.example/first.png");
  });
});

describe("阅读页左栏：按 ?from=project:<id> 找回同一棵项目树", () => {
  it("项目来路打项目具名端点，from 与 article_id 都不丢", async () => {
    await getArticleOutlineContext("101", "project:p1");

    expect(requestMock).toHaveBeenCalledWith("/api/projects/p1/outline-tree", {
      auth: false,
      query: { from: "project:p1", article_id: "101" },
    });
  });

  it("项目来路的响应形状被收成既有左栏读模型（owner_kind / owner_id 不丢）", async () => {
    requestMock.mockResolvedValueOnce({
      has_tree: true,
      owner_kind: 2,
      owner_id: "p1",
      nodes: [{ id: "n1", kind: 2, article_id: "101", article_title: "第一篇" }],
      article_count: 1,
      cover: "https://cdn.example/first.png",
      current: { id: "n1", article_id: "101", article_title: "第一篇", article_cover: "" },
      previous: null,
      next: { id: "n2", article_id: "102", article_title: "第二篇", article_cover: "" },
    });

    const context = await getArticleOutlineContext("101", "project:p1");

    expect(context.has_tree).toBe(true);
    expect(context.owner_kind).toBe(2);
    expect(context.owner_id).toBe("p1");
    expect(context.nodes).toHaveLength(1);
    expect(context.current?.article_id).toBe("101");
    expect(context.next?.article_id).toBe("102");
  });

  it("来路不是项目时不打项目具名端点（合集分派归 #275）", async () => {
    requestMock.mockResolvedValueOnce({
      has_tree: true,
      owner_kind: 1,
      owner_id: "c1",
      nodes: [],
      current: null,
      previous: null,
      next: null,
    });

    await getArticleOutlineContext("101", "collection:c1");

    const [path] = requestMock.mock.calls[0] as [string, unknown];
    expect(path).not.toBe("/api/projects/c1/outline-tree");
  });
});
