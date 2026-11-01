import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter } from "../helpers/designFiles";

/**
 * 文章封面形状：列表缩略图与详情页封面都锁 16:9，跨屏不换形状。
 *
 * 为什么是静态断言而不是视觉回归：这个要求是"形状只能有一个"，
 * 一条覆盖规则就足以让同一张封面在列表里 4:3、在详情页里随视口变高。
 * 读源码逐块比对，比截图更早、更准地拦住"又加了一条 height 档"这类回退。
 *
 * 四个落点：封面组件（列表与详情的共同底座）、阅读站列表条目、
 * 文章详情页封面、创作页的历史/收藏列表缩略图。
 */
const projectDir = process.cwd();
const articleCover = readFileSync(join(projectDir, "app/components/article/ArticleCover.vue"), "utf8");
const tailwind = readFileSync(join(projectDir, "app/assets/css/tailwind.css"), "utf8");
const historyPage = readFileSync(join(projectDir, "app/pages/studio/history.vue"), "utf8");
const favoritesPage = readFileSync(join(projectDir, "app/pages/studio/favorites.vue"), "utf8");

const SPEC_RATIO = "16 / 9";

/** 取块里的比例声明，把空白压成单空格（只消除排版差异，不放过取值差异）。 */
function aspectRatioOf(block: string): string {
  return (/aspect-ratio:\s*([^;]+);/u.exec(block)?.[1] ?? "").replace(/\s+/gu, " ").trim();
}

describe("封面组件：列表与详情共用的 16:9 底座", () => {
  it("图片与兜底占位都是 16:9，且高度由比例决定而不是写死像素", () => {
    const block = blockAfter(articleCover, ".article-cover-image,");

    expect(aspectRatioOf(block)).toBe(SPEC_RATIO);
    expect(block).toContain("width: 100%");
    expect(block).not.toMatch(/height(?:\s|-|:)/u);
  });
});

describe("文章详情页封面：16:9 且高度不随视口变化", () => {
  it("用比例撑高度，不再有 vw / clamp / 固定高度档", () => {
    const block = blockAfter(tailwind, ".article-cover {");

    expect(aspectRatioOf(block)).toBe(SPEC_RATIO);
    expect(block).not.toMatch(/height\s*:/u);
    expect(block).not.toMatch(/vw|vh|clamp\(/u);
  });
});

describe("阅读站列表缩略图：不再按列表变体换比例", () => {
  it("列表条目封面锁 16:9", () => {
    expect(aspectRatioOf(blockAfter(tailwind, ".feed-entry__cover {"))).toBe(SPEC_RATIO);
  });

  it("搜索结果 / 收藏架的 1:1、旧 4:3 覆盖规则都已删除（跨屏只有一个形状）", () => {
    expect(tailwind).not.toMatch(/aspect-ratio:\s*1\s*\/\s*1/u);
    expect(tailwind).not.toMatch(/aspect-ratio:\s*4\s*\/\s*3/u);
    expect(tailwind).not.toMatch(/\.feed-entry--search\s+\.feed-entry__cover/u);
  });
});

describe("创作页列表缩略图：历史与收藏同为 16:9", () => {
  it("历史卡片缩略图锁 16:9，窄屏只改宽度、不改形状", () => {
    const block = blockAfter(historyPage, ".article-history-card__cover {");
    expect(aspectRatioOf(block)).toBe(SPEC_RATIO);
    expect(block).not.toMatch(/height\s*:/u);

    const narrow = blockAfter(blockAfter(historyPage, "@media (max-width: 768px)"), ".article-history-card__cover {");
    expect(narrow).not.toMatch(/height\s*:/u);
    expect(narrow).not.toMatch(/aspect-ratio/u);
  });

  it("收藏卡片缩略图锁 16:9，窄屏不写高度档与最小高度", () => {
    const wrap = blockAfter(favoritesPage, ".favorite-content-card__cover-wrap {");
    expect(aspectRatioOf(wrap)).toBe(SPEC_RATIO);
    expect(wrap).not.toMatch(/height\s*:/u);

    const skeleton = blockAfter(favoritesPage, ".favorite-content-card__cover-skeleton {");
    expect(aspectRatioOf(skeleton)).toBe(SPEC_RATIO);
    expect(skeleton).not.toMatch(/height\s*:/u);

    // 旧实现用 min-height 与窄屏高度档撑封面，这两者都等于"第二个形状"。
    expect(favoritesPage).not.toMatch(/min-height\s*:/u);
  });
});
