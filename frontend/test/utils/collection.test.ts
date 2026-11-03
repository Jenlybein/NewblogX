import { describe, expect, it } from "vitest";

import type { CollectionDetail } from "~/types/api";
import type { OutlineNode } from "~/utils/outlineTree";
import { collectionCardView, collectionHref, collectionOutlineView } from "~/utils/collection";

/**
 * 合集（读者站）的纯映射。
 *
 * 卡片钉的是三种"后端可能给空"的情况：空标题、空封面、没有公开文章；
 * 详情从 #219 起是嵌套目录树，钉的是"节点原样交给 OutlineTree、篇数读后端"。
 * 它们都是布局会破、或者标题与树会对不上的地方，也是最容易被"看起来没问题"
 * 漏过的回归点。
 */

describe("collectionHref：路由形状", () => {
  it("详情固定去 /collections/<id>", () => {
    expect(collectionHref("c1")).toBe("/collections/c1");
    expect(collectionHref(42)).toBe("/collections/42");
  });
});

describe("collectionCardView", () => {
  it("正常卡片：标题、描述、封面、篇数照搬", () => {
    const view = collectionCardView({
      id: "c1",
      title: "Nuxt 部署实战",
      description: "从零到上线",
      cover: "https://img.example/cover.png",
      article_count: 6,
      created_at: "2026-01-01T12:00:00Z",
    });

    expect(view.href).toBe("/collections/c1");
    expect(view.title).toBe("Nuxt 部署实战");
    expect(view.description).toBe("从零到上线");
    expect(view.cover).toBe("https://img.example/cover.png");
    expect(view.countLabel).toBe("6 篇文章");
  });

  it("空标题退化成占位文案（不渲染成一条空白）", () => {
    const view = collectionCardView({
      id: "c2",
      title: "   ",
      description: "",
      cover: "",
      article_count: 0,
      created_at: "",
    });

    expect(view.title).toBe("未命名合集");
    expect(view.description).toBe("这个合集还没有介绍。");
    expect(view.countLabel).toBe("暂无文章");
  });

  it("没有封面时给空串（交给既有封面底纹，而不是破图）", () => {
    const view = collectionCardView({
      id: "c3",
      title: "没封面",
      description: "描述",
      cover: undefined,
      article_count: 2,
      created_at: "",
    });

    expect(view.cover).toBe("");
    expect(view.id).toBe("c3");
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
        article_id: "101",
        article_title: "开篇",
        article_cover: "",
        children: [],
      },
    ],
  },
  {
    id: "a2",
    kind: 2,
    article_id: "102",
    article_title: "附录",
    article_cover: "https://img.example/cover.png",
    children: [],
  },
];

function detail(overrides: Partial<CollectionDetail> = {}): CollectionDetail {
  return {
    id: "c1",
    title: "Nuxt 部署实战",
    description: "从零到上线",
    cover: "",
    created_at: "2026-01-01T00:00:00Z",
    article_count: 2,
    outline: { owner_id: "c1", owner_kind: 1, nodes: outlineNodes },
    ...overrides,
  } as CollectionDetail;
}

describe("collectionOutlineView：详情 → 目录树视图", () => {
  it("嵌套树原样交给页面，篇数读后端算好的 article_count", () => {
    const view = collectionOutlineView(detail());

    expect(view.nodes).toEqual(outlineNodes);
    expect(view.articleCount).toBe(2);
  });

  it("篇数不在前端按树重算：后端说 0 篇（全是失效行）时就是 0", () => {
    const view = collectionOutlineView(detail({ article_count: 0 }));

    expect(view.nodes).toHaveLength(2);
    expect(view.articleCount).toBe(0);
  });

  it("空树 / 响应缺失都是空节点 + 0 篇（交给页面渲染中性空态）", () => {
    const empty = collectionOutlineView(detail({
      article_count: 0,
      outline: { owner_id: "c1", owner_kind: 1, nodes: [] },
    }));
    expect(empty).toEqual({ nodes: [], articleCount: 0 });

    expect(collectionOutlineView(null)).toEqual({ nodes: [], articleCount: 0 });
    expect(collectionOutlineView(undefined)).toEqual({ nodes: [], articleCount: 0 });
  });

  it("篇数只读契约里的 article_count，不按树节点数重算（老响应缺字段时是 0）", () => {
    // 契约必返 article_count；缺失时按 0 篇展示，前端不另写一份可见性遍历。
    const legacy = { outline: { nodes: outlineNodes } } as unknown as CollectionDetail;

    expect(collectionOutlineView(legacy)).toEqual({ nodes: outlineNodes, articleCount: 0 });
  });
});
