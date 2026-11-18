import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter } from "../helpers/designFiles";

/**
 * 评分弹窗的 980 玻璃面板里只留一个"面"（票 423 第 11 项 / 票 437 评审）。
 *
 * 问题：玻璃面板（`.glass-thick`）内部又铺了 hero、AI 原始分、每个维度、每个分区、
 * 每条问题的 `--color-surface-raised` / `--color-bg-subtle` 不透明块 —— 玻璃里套
 * 五六层纸板，就是用户说的"卡中卡中卡"。契约：
 *   - 玻璃面板本身保留（它是浮层，符合 ADR-0011 约束 6）；
 *   - 内部**最多一个**不透明面（总分块），其余全透明，只留分隔线与标题色；
 *   - 正文仍用墨色 / 次级墨（对比度与原来同档）。
 */

const projectDir = process.cwd();
const modal = readFileSync(
  join(projectDir, "app/components/article/ArticleScoreModal.vue"),
  "utf8",
);

/** 必须透明的内部分区（除 hero 之外的所有"卡"）。 */
const TRANSPARENT_BLOCKS = [
  ".article-score-hero__side",
  ".article-score-dimension",
  ".article-score-section",
  ".article-score-section--compact",
  ".article-score-issue",
  ".article-score-issue__index",
] as const;

const OPAQUE_FACES = ["--color-surface-raised", "--color-bg-subtle"];

describe("ArticleScoreModal：玻璃面板里不再叠卡", () => {
  it("面板本身仍是玻璃浮层（ADR-0011 约束 6：浮层可以上玻璃）", () => {
    expect(modal).toContain("glass-thick");
    expect(modal).toContain("var(--glass-thick-fill)");
  });

  it("除总分块外，内部分区一律透明：只留分隔线与标题色", () => {
    for (const selector of TRANSPARENT_BLOCKS) {
      const block = blockAfter(modal, `${selector} {`);

      expect(block, `${selector} 应显式透明`).toMatch(/background:\s*transparent;/u);
      for (const face of OPAQUE_FACES) {
        expect(block, `${selector} 不应再铺 ${face}`).not.toContain(face);
      }
    }
  });

  it("分区靠一条发丝线 + 标题色撑层级（没有整块描边的卡）", () => {
    const section = blockAfter(modal, ".article-score-section {");
    const dimension = blockAfter(modal, ".article-score-dimension {");

    for (const block of [section, dimension]) {
      expect(block).toContain("border-top: 1px solid var(--color-line-faint)");
      expect(block).toMatch(/border-radius:\s*0;/u);
    }
    expect(modal).toContain(".article-score-section__title");
    expect(blockAfter(modal, ".article-score-section__title {")).toContain("var(--color-ink)");
  });

  it("总分块是唯一保留的面，且正文用墨色 token（对比度不靠底色反转）", () => {
    const hero = blockAfter(modal, ".article-score-hero {");

    expect(hero).toMatch(/background:\s*var\(--color-bg-subtle\)/u);
    expect(blockAfter(modal, ".article-score-section__comment {")).toContain(
      "var(--color-ink-muted)",
    );
    expect(blockAfter(modal, ".article-score-issue__reason {")).toContain("var(--color-ink)");
  });
});
