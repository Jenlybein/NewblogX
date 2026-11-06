import { describe, expect, it } from "vitest";

import type { ArticleOutlineContext } from "~/utils/outlineContext";
import {
  articleOutlineContextHref,
  outlineContextKind,
  outlineContextSource,
} from "~/utils/outlineContext";

/**
 * 文章上下文（阅读页左栏）的纯映射：归属 → 来源参数、相邻文章的链接。
 * 组件只消费这里的结果；`?from=` 的取数在 services/outline.ts 一处完成。
 */

function context(overrides: Partial<ArticleOutlineContext> = {}): ArticleOutlineContext {
  return {
    has_tree: true,
    owner_kind: 2,
    owner_id: "375",
    nodes: [
      {
        id: "n1",
        kind: 2,
        article_id: "101",
        article_title: "第一篇",
      },
    ],
    current: { id: "n1", article_id: "101", article_title: "第一篇", article_cover: "" },
    previous: null,
    next: { id: "n2", article_id: "102", article_title: "第二篇", article_cover: "" },
    ...overrides,
  };
}

describe("outlineContextKind / outlineContextSource：归属 → 来源参数", () => {
  it("合集 / 项目的数值归属映射回 `<kind>:<id>`", () => {
    expect(outlineContextKind(context({ owner_kind: 1 }))).toBe("collection");
    expect(outlineContextKind(context({ owner_kind: 2 }))).toBe("project");
    expect(outlineContextSource(context({ owner_kind: 1, owner_id: "9" }))).toBe("collection:9");
    expect(outlineContextSource(context())).toBe("project:375");
  });

  it("没有树（has_tree=false 或空）时不产生来源", () => {
    expect(outlineContextKind(null)).toBe("");
    expect(outlineContextKind(context({ has_tree: false }))).toBe("");
    expect(outlineContextSource(context({ has_tree: false }))).toBe("");
    expect(outlineContextSource(undefined)).toBe("");
  });

  it("归属 kind 未知 / 缺省时返回空串（不猜默认归属）", () => {
    for (const ownerKind of [0, 3, undefined]) {
      expect(outlineContextKind(context({ owner_kind: ownerKind }))).toBe("");
      expect(outlineContextSource(context({ owner_kind: ownerKind }))).toBe("");
    }
  });
});

describe("articleOutlineContextHref：上一篇/下一篇沿用同一棵树", () => {
  it("链接带上当前树的 from，翻页不会串到别的专题", () => {
    expect(articleOutlineContextHref(context(), "102")).toBe("/article/102?from=project:375");
    expect(articleOutlineContextHref(context({ owner_kind: 1, owner_id: "9" }), "102")).toBe(
      "/article/102?from=collection:9",
    );
  });

  it("没有树或没有文章 ID 时给空串（模板据此不渲染链接）", () => {
    expect(articleOutlineContextHref(context({ has_tree: false }), "102")).toBe("");
    expect(articleOutlineContextHref(context(), "")).toBe("");
  });
});
