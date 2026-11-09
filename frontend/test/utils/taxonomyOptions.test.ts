import { describe, expect, it } from "vitest";

import { matchOptionByText } from "~/utils/taxonomyOptions";

// "把 AI 返回的分类/标签对上既有下拉项"只有这一处实现（#320）。
//
// 它被 AI 元信息补全用（写作页），后续的分类栏也用同一套规则：
// 先认 ID，ID 对不上再认标题（忽略大小写与首尾空白）。
// 匹配不上就返回 null —— 绝不能把 AI 给的名字当成 ID 写进表单。

const categoryOptions = [
  { label: "后端", value: "cat-1" },
  { label: "前端", value: "cat-2" },
];

const tagOptions = [
  { label: "Go", value: "tag-1" },
  { label: "Vue", value: "tag-2" },
];

describe("taxonomyOptions · 按 ID 或标题匹配既有选项", () => {
  it("ID 命中时优先用 ID，标题不一致也认", () => {
    expect(matchOptionByText(categoryOptions, { id: "cat-1", title: "不存在的名字" })).toBe("cat-1");
  });

  it("ID 对不上时按标题命中，忽略大小写与首尾空白", () => {
    expect(matchOptionByText(categoryOptions, { id: "unknown", title: "  后端  " })).toBe("cat-1");
    expect(matchOptionByText(tagOptions, { title: "go" })).toBe("tag-1");
  });

  it("都对不上时返回 null（绝不把名字当 ID）", () => {
    expect(matchOptionByText(categoryOptions, { id: "cat-9", title: "运维" })).toBeNull();
  });

  it("没有目标、没有标题、或选项为空时都返回 null", () => {
    expect(matchOptionByText(categoryOptions, null)).toBeNull();
    expect(matchOptionByText(categoryOptions, undefined)).toBeNull();
    expect(matchOptionByText(categoryOptions, {})).toBeNull();
    expect(matchOptionByText([], { id: "cat-1", title: "后端" })).toBeNull();
  });

  it("标题是纯空白时不会空匹配命中某一项", () => {
    expect(matchOptionByText(categoryOptions, { id: "unknown", title: "   " })).toBeNull();
  });
});
