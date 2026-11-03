import { describe, expect, it } from "vitest";

import type { ArticleOutlineContext } from "~/utils/outlineContext";
import {
  articleOutlineContextHref,
  outlineContextKind,
  outlineContextQuery,
  outlineContextSource,
} from "~/utils/outlineContext";

/**
 * 文章上下文（阅读页左栏）的纯映射（接缝五）：
 * `?from=` 透传、归属 → 来源参数、相邻文章的链接。组件只消费这里的结果。
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

describe("outlineContextQuery：把 ?from= 原样透传给后端", () => {
  it("没有来源时返回空对象（由服务端回退到第一棵归属树）", () => {
    expect(outlineContextQuery(undefined)).toEqual({});
    expect(outlineContextQuery("")).toEqual({});
    expect(outlineContextQuery("   ")).toEqual({});
  });

  it("有来源时只带 from，不做第二份格式判定（格式校验在服务端一处）", () => {
    expect(outlineContextQuery("project:375")).toEqual({ from: "project:375" });
    expect(outlineContextQuery("  collection:9  ")).toEqual({ from: "collection:9" });
  });
});

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
