import { describe, expect, it } from "vitest";

import type { OutlineNode } from "~/utils/outlineTree";
import {
  outlineArticleHref,
  outlineSource,
  outlineTreeRows,
  parseOutlineSource,
} from "~/utils/outlineTree";

/**
 * 目录树（读者侧）的纯函数接缝：一棵嵌套树 → 带层级的行、「我从哪棵树来」
 * 的来源参数。
 *
 * 这里只钉外部行为：同样一棵树无论怎么折叠，行的顺序与层级都不变；页面用它
 * 渲染，不另写一份遍历。篇数读后端算好的读模型，前端不重算；文章封面已从目录树
 * 退场（三个使用点都只渲染纯文字行），行视图不再有封面字段。
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

  it("行视图不再携带封面字段：文章的 article_cover 给了也不映射到行上", () => {
    const rows = outlineTreeRows(tree, "collection", "c1");

    // 树的三个使用点（两个详情页 + 阅读页抽屉）都只渲染纯文字行。
    expect(rows.every((row) => !("cover" in row))).toBe(true);
    // 契约里仍有 article_cover（输入形状保留），但读者侧不再消费它。
    expect(tree[0]?.children?.[1]?.children?.[0]?.article_cover).toBe(
      "https://img.example/second.png",
    );
  });

  it("层级只由 depth 表达，不再给「第 N 篇」位置标记", () => {
    const rows = outlineTreeRows(tree, "project", "p1");

    expect(rows.every((row) => !("positionLabel" in row))).toBe(true);
    expect(rows.map((row) => row.depth)).toEqual([0, 1, 1, 2, 0]);
    expect(rows[0]?.href).toBeUndefined();
    expect(rows[4]?.href).toBe("/article/a3?from=project:p1");
  });

  it("未知 / 缺失的节点 kind 整行不渲染（不默认成文章节点）", () => {
    const rows = outlineTreeRows(
      [
        { id: "x", kind: 3, title: "未知类型", article_id: "9", children: [] },
        { id: "y", title: "缺 kind", article_id: "10", children: [] },
        { id: "a1", kind: 2, article_id: "11", article_title: "正常文章", children: [] },
      ],
      "collection",
      "c1",
    );

    expect(rows.map((row) => row.id)).toEqual(["a1"]);
  });

  it("来源 kind 缺失时不生成带来源的文章链接（不猜默认归属）", () => {
    const rows = outlineTreeRows(tree, "", "c1");

    expect(rows.filter((row) => row.href)).toEqual([]);
  });

  it("空标题退化成中性占位；封面退场后行上只有文字", () => {
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
    expect(rows[1]).not.toHaveProperty("cover");
  });

  it("空树不产生行，只有空文件夹的树只有文件夹行", () => {
    expect(outlineTreeRows([], "collection", "c1")).toEqual([]);

    const folders = outlineTreeRows(
      [{ id: "f", kind: 1, title: "空章节", article_id: "", children: [] }],
      "collection",
      "c1",
    );
    expect(folders.map((row) => [row.isFolder, row.href])).toEqual([[true, undefined]]);
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
