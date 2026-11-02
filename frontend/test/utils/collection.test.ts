import { describe, expect, it } from "vitest";

import {
  collectionArticleView,
  collectionCardView,
  collectionHref,
  seriesPositionLabel,
} from "~/utils/collection";

/**
 * 合集卡片的纯映射。
 *
 * 钉的是三种"后端可能给空"的情况：空标题、空封面、没有公开文章。
 * 它们都是布局会破的地方，也是最容易被"看起来没问题"漏过的回归点。
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

describe("seriesPositionLabel：系列位置", () => {
  it("从 1 开始编号", () => {
    expect(seriesPositionLabel(1)).toBe("第 1 篇");
    expect(seriesPositionLabel(7)).toBe("第 7 篇");
  });

  it("非法序号给占位，不渲染成「第 0 篇」", () => {
    expect(seriesPositionLabel(0)).toBe("未编号");
    expect(seriesPositionLabel(-1)).toBe("未编号");
  });
});

describe("collectionArticleView", () => {
  it("带出作者与分类的副标题，两者缺一不拼多余的分隔符", () => {
    const view = collectionArticleView({
      article_id: "a1",
      sort_order: 3,
      title: "第一篇",
      abstract: "摘要",
      cover: "",
      view_count: 12,
      author_nickname: "作者",
      category_title: "前端",
    });

    expect(view.href).toBe("/article/a1");
    expect(view.positionLabel).toBe("第 3 篇");
    expect(view.byline).toBe("作者 · 前端");
  });

  it("作者与分类都缺时副标题为空串", () => {
    const view = collectionArticleView({
      article_id: "a2",
      sort_order: 1,
      title: "",
      abstract: "",
      cover: "",
      view_count: 0,
      author_nickname: "",
      category_title: "",
    });

    expect(view.title).toBe("未命名文章");
    expect(view.byline).toBe("");
  });
});
