import { describe, expect, it } from "vitest";

import {
  REVIEW_RISK_LEVELS,
  riskLabelText,
  riskLevelMeta,
  riskLevelRank,
} from "../../app/utils/reviewRisk";

/**
 * 审核列表的风险文案与颜色只在这一个模块映射（票 311）。
 *
 * 后端契约下发的是稳定 code（risk_level / risk_labels），中文与颜色留在前端：
 * 运营改措辞不必重新生成 OpenAPI，排序也只看等级高低。
 */
describe("审核风险等级映射", () => {
  it("四个等级各有中文标签与 naive-ui 标签类型", () => {
    expect(riskLevelMeta("high")).toEqual({ label: "高风险", type: "error" });
    expect(riskLevelMeta("medium")).toEqual({ label: "中风险", type: "warning" });
    expect(riskLevelMeta("low")).toEqual({ label: "低风险", type: "info" });
    expect(riskLevelMeta("none")).toEqual({ label: "无风险", type: "default" });
  });

  it("未知 / 缺失等级按无风险处理，不把干净文章排到最前", () => {
    expect(riskLevelMeta("blocked")).toEqual(riskLevelMeta("none"));
    expect(riskLevelMeta(undefined)).toEqual(riskLevelMeta("none"));
  });

  it("等级排序序：high > medium > low > none", () => {
    expect(REVIEW_RISK_LEVELS).toEqual(["none", "low", "medium", "high"]);
    expect(riskLevelRank("high")).toBeGreaterThan(riskLevelRank("medium"));
    expect(riskLevelRank("medium")).toBeGreaterThan(riskLevelRank("low"));
    expect(riskLevelRank("low")).toBeGreaterThan(riskLevelRank("none"));
    expect(riskLevelRank("weird")).toBe(riskLevelRank("none"));
  });

  it("风险标签 code 映射成中文，未知 code 原样返回", () => {
    expect(riskLabelText("block_word")).toBe("阻断级词");
    expect(riskLabelText("watch_word")).toBe("观察级词");
    expect(riskLabelText("low_score")).toBe("低分");
    expect(riskLabelText("dimension_collapse")).toBe("维度塌陷");
    expect(riskLabelText("length_abnormal")).toBe("篇幅异常");
    expect(riskLabelText("future_code")).toBe("future_code");
  });
});
