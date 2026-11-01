import { describe, expect, it } from "vitest";

import {
  categoryTitleError,
  findCategoryOptionByTitle,
  normalizeCategoryTitle,
} from "~/utils/studioCategory";

// 写作页"内联新建分类"的纯函数规则（#127）。
//
// 这些规则被下拉旁的输入框与文章数据域共用：空白标题在 UI 上要被挡住，
// 创建成功后要按"去掉首尾空白后的标题"在下拉里找到新分类并选中。
// 单独抽出来测，是因为它们不依赖任何组件/network，也最容易在两处走样。

describe("studioCategory", () => {
  it("normalizeCategoryTitle 去掉首尾空白", () => {
    expect(normalizeCategoryTitle("  前端  ")).toBe("前端");
    expect(normalizeCategoryTitle("\n\tGo\t\n")).toBe("Go");
    expect(normalizeCategoryTitle("前端")).toBe("前端");
  });

  it("空标题与纯空白标题都给出可读提示", () => {
    for (const raw of ["", "   ", "\t\n", "\u3000"]) {
      expect(categoryTitleError(raw)).toBe("请输入分类名称");
    }
  });

  it("合法标题没有错误", () => {
    expect(categoryTitleError("前端")).toBeNull();
    expect(categoryTitleError("  前端  ")).toBeNull();
  });

  it("按标题在下拉里找到新建的分类（忽略首尾空白）", () => {
    const options = [
      { label: "前端", value: "cat-1" },
      { label: "后端", value: "cat-2" },
    ];

    expect(findCategoryOptionByTitle(options, "  后端  ")).toEqual({ label: "后端", value: "cat-2" });
  });

  it("找不到时返回 null，调用方要给出可见提示而不是默默不选", () => {
    expect(findCategoryOptionByTitle([{ label: "前端", value: "cat-1" }], "数据库")).toBeNull();
    expect(findCategoryOptionByTitle([], "前端")).toBeNull();
  });
});
