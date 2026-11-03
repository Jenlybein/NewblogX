import { describe, expect, it } from "vitest";

import { projectArticleView, projectCardView, projectHref } from "~/utils/project";

/**
 * 项目卡片的纯映射（#193「项目」读者侧）。
 *
 * 项目与合集并列、页面形态复用合集，但内部文案一律写「项目」——所以映射里的
 * 占位文案必须是「未命名项目 / 这个项目还没有介绍」，不能把合集的文案直接搬过来。
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

describe("projectArticleView：复用合集的收录文章映射", () => {
  it("带出作者与分类的副标题，位置文案与合集同口径", () => {
    const view = projectArticleView({
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

  it("作者与分类都缺时副标题为空串，空标题不渲染成空白", () => {
    const view = projectArticleView({
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
