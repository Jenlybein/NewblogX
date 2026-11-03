import { describe, expect, it } from "vitest";

import type { OutlineTreeRow } from "~/utils/outlineTree";
import {
  markCurrentOutlineRows,
  parseOutlineExpanded,
  serializeOutlineExpanded,
  visibleOutlineRows,
} from "~/utils/outlineState";

/**
 * 展开/折叠状态的纯函数接缝：localStorage 里的旧值不能毒死整棵树，
 * 收起文件夹时它的后代都不再出现，状态变更后能序列化回 localStorage。
 */

function row(id: string, depth: number, isFolder: boolean, hasChildren = false): OutlineTreeRow {
  return {
    id,
    depth,
    indent: depth,
    isFolder,
    hasChildren,
    label: id,
    cover: "",
    ...(isFolder ? {} : { articleId: id, href: `/article/${id}` }),
  };
}

const rows: OutlineTreeRow[] = [
  row("f1", 0, true, true),
  row("a1", 1, false),
  row("f2", 1, true, true),
  row("a2", 2, false),
  row("a3", 0, false),
];

describe("parseOutlineExpanded：读取 localStorage", () => {
  it("只保留字符串数组里的非空节点 ID", () => {
    expect(parseOutlineExpanded('["f1","f2"]')).toEqual(new Set(["f1", "f2"]));
    expect(parseOutlineExpanded('["f1","",3,null]')).toEqual(new Set(["f1"]));
  });

  it("缺失、坏 JSON 或不是数组时回落空集合（默认全部展开）", () => {
    for (const raw of [null, "", "not json", "{}", "42"]) {
      expect(parseOutlineExpanded(raw)).toEqual(new Set());
    }
  });

  it("序列化后能原样读回（刷新 / 返回时状态保留）", () => {
    const stored = serializeOutlineExpanded(new Set(["f2", "f1"]));
    expect(stored).toBe('["f1","f2"]');
    expect(parseOutlineExpanded(stored)).toEqual(new Set(["f1", "f2"]));
    expect(parseOutlineExpanded(serializeOutlineExpanded(new Set()))).toEqual(new Set());
  });
});

describe("visibleOutlineRows：折叠只裁掉文件夹的后代", () => {
  it("默认展开：全部行按深度优先出现", () => {
    expect(visibleOutlineRows(rows, new Set()).map((item) => item.id)).toEqual([
      "f1",
      "a1",
      "f2",
      "a2",
      "a3",
    ]);
  });

  it("收起 f1：它自己还在（可再展开），所有后代消失", () => {
    expect(visibleOutlineRows(rows, new Set(), new Set(["f1"])).map((item) => item.id)).toEqual([
      "f1",
      "a3",
    ]);
  });

  it("收起嵌套的 f2 只影响它那一支；未知节点 ID 不影响任何行", () => {
    expect(visibleOutlineRows(rows, new Set(), new Set(["f2", "missing"])).map((item) => item.id)).toEqual([
      "f1",
      "a1",
      "f2",
      "a3",
    ]);
  });

  it("当前文章命中被收起的分支时自动展开来路文件夹（当前文章可见）", () => {
    expect(
      visibleOutlineRows(rows, new Set(["a2"]), new Set(["f1", "f2"])).map((item) => item.id),
    ).toEqual(["f1", "a1", "f2", "a2", "a3"]);
    expect(visibleOutlineRows(rows, new Set(["a3"]), new Set(["f1"])).map((item) => item.id)).toEqual([
      "f1",
      "a3",
    ]);
  });
});

describe("markCurrentOutlineRows：当前文章高亮", () => {
  it("命中的文章节点打标记，文件夹与其它文章都不高亮", () => {
    const marked = markCurrentOutlineRows(
      [{ ...row("a1", 1, false), articleId: "a1" }, row("f1", 0, true)],
      "a1",
    );

    expect(marked.map((item) => item.isCurrent)).toEqual([true, false]);
  });

  it("没有当前文章时一个节点都不高亮", () => {
    expect(markCurrentOutlineRows(rows, null).some((item) => item.isCurrent)).toBe(false);
    expect(markCurrentOutlineRows(rows, "missing").some((item) => item.isCurrent)).toBe(false);
  });
});
