import { describe, expect, it } from "vitest";

import {
  SCORE_DIMENSION_FALLBACK_LABEL,
  SCORE_DIMENSION_LABELS,
  scoreDimensionLabel,
} from "~/utils/scoreDimensions";

/**
 * 六个评分维度的中文名唯一映射（ADR-0023）。
 *
 * 评分弹窗与「质量不达标」的驳回卡片共用它：同一维度在两处叫不同名字，
 * 作者会以为系统在说两件事。
 */
describe("scoreDimensionLabel", () => {
  it("六个维度都有中文名，与后端维度键一一对应", () => {
    expect(Object.keys(SCORE_DIMENSION_LABELS).sort()).toEqual(
      ["clarity", "completeness", "language", "persuasiveness", "readability", "structure"].sort(),
    );
    expect(scoreDimensionLabel("clarity")).toBe("清晰度");
    expect(scoreDimensionLabel("structure")).toBe("结构性");
    expect(scoreDimensionLabel("completeness")).toBe("完整度");
    expect(scoreDimensionLabel("readability")).toBe("可读性");
    expect(scoreDimensionLabel("persuasiveness")).toBe("说服力");
    expect(scoreDimensionLabel("language")).toBe("语言规范");
  });

  it("后端口径是大写或带空白时也能命中", () => {
    expect(scoreDimensionLabel(" Completeness ")).toBe("完整度");
    expect(scoreDimensionLabel("LANGUAGE")).toBe("语言规范");
  });

  it("未知 / 缺失维度给兜底名，而不是空串或 undefined", () => {
    expect(scoreDimensionLabel("brand_new_dimension")).toBe(SCORE_DIMENSION_FALLBACK_LABEL);
    expect(scoreDimensionLabel(undefined)).toBe(SCORE_DIMENSION_FALLBACK_LABEL);
    expect(scoreDimensionLabel(null)).toBe(SCORE_DIMENSION_FALLBACK_LABEL);
    expect(scoreDimensionLabel("")).toBe(SCORE_DIMENSION_FALLBACK_LABEL);
  });
});
