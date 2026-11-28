import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 审核队列的「判据开关」提示（2026-11-28 / ADR-0037 spec §5.4）。
 *
 * 这一页有两处**纯展示**的语义切换，写错方向不会让页面炸，只会让审核员
 * 把"没检查"读成"检查过没问题" —— 所以用源码断言把它们钉住（与本仓库
 * admin-flip / fonts / tokens 同一类"结构断言"，不引入组件挂载的开销）：
 *
 *   - 评分关：风险列显示「未评估」，而不是「无风险」；
 *   - AI 合规关：合规列显示「未判定（未开启 AI 合规判定）」；
 *   - AI 落点 = 直接放行：另有一条提示，避免把兜底"未判定"读成"还没判"。
 */
const pageSource = readFileSync(
  join(process.cwd(), "app", "pages", "review", "article.vue"),
  "utf8",
);

describe("审核队列的判据开关提示", () => {
  it("评分关掉时风险列显示「未评估」，且不再走派生标签分支", () => {
    // 分支必须在"派生标签之前"返回：否则关掉评分后仍会显示「无风险」。
    expect(pageSource).toContain("if (!scoreEnabled.value) {");
    expect(pageSource).toContain("'未评估'");

    const guardIndex = pageSource.indexOf("if (!scoreEnabled.value) {");
    const labelIndex = pageSource.indexOf("riskLevelMeta(row.risk_level)");
    expect(guardIndex).toBeGreaterThan(-1);
    expect(labelIndex).toBeGreaterThan(guardIndex);
  });

  it("AI 合规关掉时合规列写清是「未开启」，不是笼统的未判定", () => {
    expect(pageSource).toContain("'未判定（未开启 AI 合规判定）'");
    expect(pageSource).toContain("'未判定'");
  });

  it("三条提示各自对应自己的开关，且「直接放行」那条是 else-if（互斥）", () => {
    expect(pageSource).toContain('<NAlert v-if="!scoreEnabled"');
    expect(pageSource).toContain('<NAlert v-if="!aiComplianceEnabled"');
    expect(pageSource).toContain('<NAlert v-else-if="aiReleaseFallback"');
  });

  it("读配置失败时按「都开着」处理，不能让队列整片变成未评估", () => {
    // `!== false` 是关键：undefined（配置读不到）要落到 true。
    expect(pageSource).toContain("site.article?.score_enabled !== false");
    expect(pageSource).toContain(
      "site.article?.ai_compliance_enabled !== false",
    );
    expect(pageSource).toContain(
      'site.article?.ai_failure_action === "release"',
    );
  });

  it("风险排序在评分关闭时置灰并说明原因", () => {
    expect(pageSource).toContain("风险优先（评分已关闭，不可用）");
    expect(pageSource).toContain("disabled: !scoreEnabled.value");
  });
});
