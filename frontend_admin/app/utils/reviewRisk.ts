/**
 * 审核列表的风险文案与颜色（票 311）。
 *
 * 后端契约下发的是稳定 code：`risk_level` 是 none / low / medium / high，
 * `risk_labels` 是 block_word / watch_word / low_score / dimension_collapse /
 * length_abnormal。中文与颜色只在前端这一处映射，运营改措辞不必重新生成
 * OpenAPI，排序也只看等级高低。
 */

export type ReviewRiskLevel = "none" | "low" | "medium" | "high";

/** naive-ui NTag 的 type 取值。 */
export type ReviewRiskTagType = "default" | "error" | "warning" | "info" | "success";

/** 由低到高的等级序列；索引就是排序权重。 */
export const REVIEW_RISK_LEVELS: ReviewRiskLevel[] = ["none", "low", "medium", "high"];

interface RiskMeta {
  label: string;
  type: ReviewRiskTagType;
}

const RISK_META: Record<ReviewRiskLevel, RiskMeta> = {
  none: { label: "无风险", type: "default" },
  low: { label: "低风险", type: "info" },
  medium: { label: "中风险", type: "warning" },
  high: { label: "高风险", type: "error" },
};

const RISK_LABEL_TEXT: Record<string, string> = {
  block_word: "阻断级词",
  watch_word: "观察级词",
  low_score: "低分",
  dimension_collapse: "维度塌陷",
  length_abnormal: "篇幅异常",
};

/** 把契约里的等级收敛到四个已知值，未知 / 缺失一律按 none。 */
export function normalizeRiskLevel(level: string | undefined | null): ReviewRiskLevel {
  return REVIEW_RISK_LEVELS.includes(level as ReviewRiskLevel)
    ? (level as ReviewRiskLevel)
    : "none";
}

/** 等级 → 中文标签 + 颜色。 */
export function riskLevelMeta(level: string | undefined | null): RiskMeta {
  return RISK_META[normalizeRiskLevel(level)];
}

/** 等级 → 排序权重（none 0 … high 3）。 */
export function riskLevelRank(level: string | undefined | null): number {
  return REVIEW_RISK_LEVELS.indexOf(normalizeRiskLevel(level));
}

/** 风险标签 code → 中文；未知 code 原样返回，不吞掉新标签。 */
export function riskLabelText(code: string): string {
  return RISK_LABEL_TEXT[code] ?? code;
}
