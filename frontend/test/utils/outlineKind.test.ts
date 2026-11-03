import { describe, expect, it } from "vitest";

import {
  isOutlineSourceKind,
  outlineNodeKind,
  outlineSourceKind,
} from "~/utils/outlineKind";

/**
 * 契约里的 kind 数值只有一份判定：节点 kind（1 文件夹 / 2 文章）与归属 kind
 * （1 合集 / 2 项目）。未知值一律返回 null，由调用方显式处理——不猜默认类型，
 * 也不默认成某个归属。
 */
describe("outlineNodeKind：节点 kind 数值 → 字面值", () => {
  it("只认 1=文件夹、2=文章节点", () => {
    expect(outlineNodeKind(1)).toBe("folder");
    expect(outlineNodeKind(2)).toBe("article");
  });

  it("未知 / 缺失的 kind 返回 null（不默认成文章节点）", () => {
    for (const kind of [0, 3, -1, 1.5, null, undefined, Number.NaN]) {
      expect(outlineNodeKind(kind)).toBeNull();
    }
  });
});

describe("outlineSourceKind：归属 kind 数值 → 来源 kind", () => {
  it("只认 1=合集、2=项目", () => {
    expect(outlineSourceKind(1)).toBe("collection");
    expect(outlineSourceKind(2)).toBe("project");
  });

  it("未知 / 缺失的归属返回 null（不猜默认归属）", () => {
    for (const kind of [0, 3, -1, 1.5, null, undefined, Number.NaN]) {
      expect(outlineSourceKind(kind)).toBeNull();
    }
  });
});

describe("isOutlineSourceKind：来源字面值合法性", () => {
  it("只接受 project / collection 两个契约字面值", () => {
    expect(isOutlineSourceKind("project")).toBe(true);
    expect(isOutlineSourceKind("collection")).toBe(true);
  });

  it("其它值一律拒绝（含空串、数字与大小写变体）", () => {
    for (const value of ["", "article", "Project", "collection ", 1, 2, null, undefined]) {
      expect(isOutlineSourceKind(value)).toBe(false);
    }
  });
});
