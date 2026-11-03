import { describe, expect, it } from "vitest";

import type { ProjectDetail } from "~/types/api";
import type { OutlineNode } from "~/utils/outlineTree";
import { projectCardView, projectHref, projectOutlineView } from "~/utils/project";

/**
 * 项目（读者站）的纯映射（#193 / #219）。
 *
 * 项目与合集并列、页面形态复用合集，但内部文案一律写「项目」——所以卡片的
 * 占位文案必须是「未命名项目 / 这个项目还没有介绍」，不能把合集的文案直接搬过来。
 * 详情从 #219 起与合集同构（嵌套目录树），映射复用合集那一份，避免两套口径漂移。
 */

describe("projectHref：路由形状", () => {
  it("详情固定去 /projects/<id>", () => {
    expect(projectHref("p1")).toBe("/projects/p1");
    expect(projectHref(42)).toBe("/projects/42");
  });
});

describe("projectCardView", () => {
  it("正常卡片：标题、描述、封面、篇数照搬", () => {
    const view = projectCardView({
      id: "p1",
      title: "重构实战",
      description: "从零到上线",
      cover: "https://img.example/cover.png",
      article_count: 6,
      created_at: "2026-01-01T12:00:00Z",
    });

    expect(view.href).toBe("/projects/p1");
    expect(view.title).toBe("重构实战");
    expect(view.description).toBe("从零到上线");
    expect(view.cover).toBe("https://img.example/cover.png");
    expect(view.countLabel).toBe("6 篇文章");
  });

  it("空标题与空描述退化成「项目」口径的占位文案", () => {
    const view = projectCardView({
      id: "p2",
      title: "   ",
      description: "",
      cover: "",
      article_count: 0,
      created_at: "",
    });

    expect(view.title).toBe("未命名项目");
    expect(view.description).toBe("这个项目还没有介绍。");
    expect(view.countLabel).toBe("暂无文章");
  });

  it("没有封面时给空串（交给既有封面底纹，而不是破图）", () => {
    const view = projectCardView({
      id: "p3",
      title: "没封面",
      description: "描述",
      cover: undefined,
      article_count: 2,
      created_at: "",
    });

    expect(view.cover).toBe("");
    expect(view.id).toBe("p3");
  });
});

const outlineNodes: OutlineNode[] = [
  {
    id: "f1",
    kind: 1,
    title: "第一章",
    children: [
      {
        id: "a1",
        kind: 2,
        article_id: "201",
        article_title: "开篇",
        article_cover: "",
        children: [],
      },
    ],
  },
  {
    id: "a2",
    kind: 2,
    article_id: "202",
    article_title: "附录",
    article_cover: "",
    children: [],
  },
];

function detail(overrides: Partial<ProjectDetail> = {}): ProjectDetail {
  return {
    id: "p1",
    title: "重构实战",
    description: "从零到上线",
    cover: "",
    created_at: "2026-01-01T00:00:00Z",
    article_count: 2,
    outline: { owner_id: "p1", owner_kind: 2, nodes: outlineNodes },
    ...overrides,
  } as ProjectDetail;
}

describe("projectOutlineView：复用合集的目录树映射", () => {
  it("嵌套树原样交给页面，篇数读后端算好的 article_count", () => {
    const view = projectOutlineView(detail());

    expect(view.nodes).toEqual(outlineNodes);
    expect(view.articleCount).toBe(2);
  });

  it("篇数不在前端按树重算：后端说 0 篇（全是失效行）时就是 0", () => {
    const view = projectOutlineView(detail({ article_count: 0 }));

    expect(view.nodes).toHaveLength(2);
    expect(view.articleCount).toBe(0);
  });

  it("空树 / 响应缺失都是空节点 + 0 篇（交给页面渲染中性空态）", () => {
    const empty = projectOutlineView(detail({
      article_count: 0,
      outline: { owner_id: "p1", owner_kind: 2, nodes: [] },
    }));
    expect(empty).toEqual({ nodes: [], articleCount: 0 });

    expect(projectOutlineView(null)).toEqual({ nodes: [], articleCount: 0 });
    expect(projectOutlineView(undefined)).toEqual({ nodes: [], articleCount: 0 });
  });
});
