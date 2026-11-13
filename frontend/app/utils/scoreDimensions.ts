/**
 * 六个评分维度的中文名（唯一一处映射）。
 *
 * 维度键由后端评分侧定义（clarity / structure / completeness / readability /
 * persuasiveness / language），中文名只在这里，后端不重复维护一份（ADR-0023）。
 * 评分弹窗与「质量不达标」的驳回卡片都消费它，避免同一维度在两处叫不同名字。
 */
export const SCORE_DIMENSION_LABELS: Record<string, string> = {
  clarity: "清晰度",
  structure: "结构性",
  completeness: "完整度",
  readability: "可读性",
  persuasiveness: "说服力",
  language: "语言规范",
};

/** 未知维度（后端加了新维度但前端还没跟上）的兜底名。 */
export const SCORE_DIMENSION_FALLBACK_LABEL = "其他";

/** scoreDimensionLabel 把维度键翻成中文名；查不到时给兜底名而不是空串。 */
export function scoreDimensionLabel(name: string | undefined | null): string {
  const key = String(name ?? "").trim().toLowerCase();
  return SCORE_DIMENSION_LABELS[key] ?? SCORE_DIMENSION_FALLBACK_LABEL;
}
