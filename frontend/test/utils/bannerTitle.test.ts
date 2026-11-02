import { describe, expect, it } from "vitest";

import { bannerTitleText } from "~/utils/bannerTitle";

/**
 * 轮播标题是**纯文本**：只收敛首尾空白，不做任何标记语言解析。
 *
 * 读者站据此决定"渲染不渲染文字区"：空/纯空白/缺失一律不渲染。
 */
describe("bannerTitleText", () => {
  it("原样返回普通标题，只去掉首尾空白", () => {
    expect(bannerTitleText("  春季上新  ")).toBe("春季上新");
    expect(bannerTitleText("春季上新")).toBe("春季上新");
  });

  it("空、纯空白、缺失都收敛成空串（调用方据此不渲染文字区）", () => {
    expect(bannerTitleText("")).toBe("");
    expect(bannerTitleText("   ")).toBe("");
    expect(bannerTitleText(undefined)).toBe("");
    expect(bannerTitleText(null)).toBe("");
  });

  it("HTML 标签与实体符号原样保留，不被解析也不被剥离", () => {
    const raw = '<b>粗体</b> & "引号"';
    expect(bannerTitleText(raw)).toBe(raw);
  });
});
