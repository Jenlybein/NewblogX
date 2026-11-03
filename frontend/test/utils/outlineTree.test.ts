import { describe, expect, it } from "vitest";

import type { OutlineNode } from "~/utils/outlineTree";
import {
  firstPublicArticleCover,
  outlineArticleHref,
  outlineSource,
  outlineTreeCount,
  outlineTreeRows,
  parseOutlineSource,
} from "~/utils/outlineTree";

/**
 * 目录树（读者侧）的纯函数接缝：一棵嵌套树 → 带层级的行、深度优先封面回退、
 * 「我从哪棵树来」的来源参数。
 *
 * 这里只钉外部行为：同样一棵树无论怎么折叠，行的顺序、层级与封面回退都不变；
 * 页面用它渲染，不另写一份遍历。
 */

const tree: OutlineNode[] = [
  {
    id: "n1",
    kind: 1,
    title: "第一章",
    article_id: "",
    children: [
      {
        id: "n2",
        kind: 2,
        title: "",
        article_id: "a1",
        article_title: "开篇",
        article_cover: "",
        children: [],
      },
      {
        id: "n3",
        kind: 1,
        title: "深入",
        article_id: "",
        children: [
          {
            id: "n4",
            kind: 2,
            title: "",
            article_id: "a2",
            article_title: "第二篇",
            article_cover: "https://img.example/second.png",
            children: [],
          },
        ],
      },
    ],
  },
  {
    id: "n5",
    kind: 2,
    title: "",
    article_id: "a3",
    article_title: "附录",
    article_cover: "https://img.example/last.png",
    children: [],
  },
];

describe("outlineTreeRows：嵌套树 → 带层级的行", () => {
  it("按深度优先展开，文件夹有折叠箭头、文章有带来源的链接", () => {
    const rows = outlineTreeRows(tree, "collection", "c1");

    expect(rows.map((row) => [row.id, row.depth, row.label])).toEqual([
      ["n1", 0, "第一章"],
      ["n2", 1, "开篇"],
      ["n3", 1, "深入"],
      ["n4", 2, "第二篇"],
      ["n5", 0, "附录"],
    ]);
    expect(rows.map((row) => row.isFolder)).toEqual([true, false, true, false, false]);
    expect(rows.map((row) => row.hasChildren)).toEqual([true, false, true, false, false]);
    expect(rows[1]?.href).toBe("/article/a1?from=collection:c1");
    expect(rows[2]?.href).toBeUndefined();
    expect(rows[3]?.href).toBe("/article/a2?from=collection:c1");
  });

  it("层级靠缩进表达，不再给「第 N 篇」位置标记", () => {
    const rows = outlineTreeRows(tree, "project", "p1");

    expect(rows.every((row) => !("positionLabel" in row))).toBe(true);
    expect(rows.map((row) => row.indent)).toEqual([0, 1, 1, 2, 0]);
    expect(rows[0]?.href).toBeUndefined();
    expect(rows[4]?.href).toBe("/article/a3?from=project:p1");
  });

  it("空标题退化成中性占位，空封面交给 ArticleCover 的透明空白", () => {
    const rows = outlineTreeRows(
      [
        {
          id: "f1",
          kind: 1,
          title: "   ",
          article_id: "",
          children: [
            {
              id: "a1",
              kind: 2,
              title: "",
              article_id: "42",
              article_title: "  ",
              article_cover: "  ",
              children: [],
            },
          ],
        },
      ],
      "collection",
      "c1",
    );

    expect(rows[0]?.label).toBe("未命名章节");
    expect(rows[1]?.label).toBe("未命名文章");
    expect(rows[1]?.cover).toBe("");
  });

  it("空树与只有空文件夹的树都只有零篇文章", () => {
    expect(outlineTreeRows([], "collection", "c1")).toEqual([]);
    expect(outlineTreeCount(tree)).toBe(3);
    expect(outlineTreeCount([{ id: "f", kind: 1, title: "空章节", article_id: "", children: [] }])).toBe(0);
  });
});

describe("firstPublicArticleCover：深度优先第一篇公开文章", () => {
  it("跳过没有封面的文章，取第一个真正有封面的公开文章节点", () => {
    expect(firstPublicArticleCover(tree)).toBe("https://img.example/second.png");
  });

  it("树里没有文章或都没有封面时给空串（不编造封面）", () => {
    expect(firstPublicArticleCover([])).toBe("");
    expect(firstPublicArticleCover(tree.slice(0, 1).map((node) => ({
      ...node,
      children: node.children?.filter((child) => child.kind === 1).map((folder) => ({
        ...folder,
        children: [],
      })),
    })))).toBe("");
  });
});

describe("outlineSource：来源参数", () => {
  it("只认 project / collection 两种字面值，拼成 <kind>:<id>", () => {
    expect(outlineSource("project", "375")).toBe("project:375");
    expect(outlineSource("collection", "c1")).toBe("collection:c1");
  });

  it("生成文章链接时带 from，不会把来源拼进路径", () => {
    expect(outlineArticleHref("42", outlineSource("project", "375"))).toBe(
      "/article/42?from=project:375",
    );
  });

  it("解析只接受完整且非空的 <kind>:<id>", () => {
    expect(parseOutlineSource("project:375")).toEqual({ kind: "project", id: "375" });
    expect(parseOutlineSource("collection:c1")).toEqual({ kind: "collection", id: "c1" });
    for (const invalid of ["", "project", "project:", ":375", "article:375", "project:375:extra"]) {
      expect(parseOutlineSource(invalid)).toBeNull();
    }
  });
});
