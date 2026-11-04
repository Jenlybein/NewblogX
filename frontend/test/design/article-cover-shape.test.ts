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
const historyPage = readFileSync(join(projectDir, "app/pages/history.vue"), "utf8");
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

  it("图片固定居中（object-fit: cover / center），不消费取景参数（#204）", () => {
    const image = blockAfter(articleCover, ".article-cover-image {");

    expect(image).toContain("object-fit: cover");
    expect(image).toContain("object-position: center");
    // 取景体系已全量退场：组件不再引用取景算术，也没有 crop / framing 样式出口。
    expect(articleCover).not.toMatch(/framingStyle|bannerFraming|crop/u);
    expect(articleCover).not.toMatch(/:style=/u);
  });

  it("没有封面时落到空封面兜底，兜底也占同一个 16:9 形状", () => {
    expect(articleCover).toContain("article-cover-default");
    expect(articleCover).toMatch(/v-else/u);
  });
});

/**
 * 封面兜底退场（#210）：缺封面 = 父容器底色的 16:9 空白。
 *
 * 「没有封面」第一次被当成"没有内容"而不是"一种内容"：NO COVER 文案与围绕它的
 * 装饰（网格纹理、光斑、品牌小签）整套退场，空占位不进无障碍树。
 * 采样的是组件标签与样式块本身——形状已由上面锁住，这里锁"空白里什么都没有"。
 */
describe("封面兜底退场：空占位里没有文字、没有装饰", () => {
  /** 组件里空占位的那个开标签（属性顺序无关，只看它带了什么）。 */
  function placeholderTag(): string {
    const match = /<div[^>]*class="article-cover-default"[^>]*>/u.exec(articleCover);
    expect(match, "找不到 .article-cover-default 的开标签").not.toBeNull();
    return match![0];
  }

  it("NO COVER 文案与「暂无封面」朗读串都已退场", () => {
    expect(articleCover).not.toMatch(/NO COVER/u);
    expect(articleCover).not.toMatch(/暂无封面/u);
  });

  it("围绕它的装饰（网格纹理 / 光斑 / 品牌小签）连样式一起删除", () => {
    expect(articleCover).not.toMatch(/article-cover-default__(?:grid|orb|content)/u);
    expect(articleCover).not.toMatch(/background-image|mask-image/u);
  });

  it("空占位与父容器同底色：background transparent，不画渐变也不留圆角框", () => {
    // 兜底规则在文件末尾单独成块（前一次出现是它与图片共享的 16:9 底座）。
    const block = blockAfter(articleCover, ".article-cover-default {", true);

    expect(block).toContain("background: transparent");
    expect(block).not.toMatch(/gradient/u);
    expect(block).not.toMatch(/border-radius/u);
  });

  it("空占位对辅助技术隐藏：aria-hidden，且不包 role / aria-label", () => {
    const tag = placeholderTag();

    expect(tag).toContain('aria-hidden="true"');
    expect(tag).not.toMatch(/\brole=/u);
    expect(tag).not.toMatch(/aria-label/u);
  });
});

/**
 * 五处消费点同一口径（#210）：首页卡片 / 合集列表与详情 / 项目列表与详情 / 收藏 / 历史，
 * 封面只把 src 交给共用组件，不再给"没有封面"配任何文案或装饰；
 * 文章详情页首屏无封面时干脆不渲染图区，正文直接从标题开始。
 */
describe("五处消费点同一口径：无封面不再是一种内容", () => {
  const consumers = [
    "app/components/home/HomeFeedSection.vue",
    "app/pages/collections/index.vue",
    "app/pages/collections/[id].vue",
    "app/pages/projects/index.vue",
    "app/pages/projects/[id].vue",
    "app/pages/studio/favorites.vue",
    "app/pages/history.vue",
  ];

  function readConsumer(relativePath: string): string {
    return readFileSync(join(projectDir, relativePath), "utf8");
  }

  it.each(consumers)("%s 不自己画兜底、不给封面传文案参数", (relativePath) => {
    const source = readConsumer(relativePath);

    expect(source).not.toMatch(/NO COVER|暂无封面/u);
    expect(source).not.toMatch(/article-cover-default/u);
    // 品牌小签的 `label` 接线一并拆除：组件不再接受它，消费点也不再传。
    expect(source).not.toMatch(/<ArticleCover[^>]*\blabel=/u);
  });

  it("收藏页不再自带兜底样式（空封面只在共用组件里有一份实现）", () => {
    expect(readConsumer("app/pages/studio/favorites.vue")).not.toContain("favorite-content-card__cover-empty");
  });

  it("文章详情页无封面时不渲染图区：封面 section 由 article.cover 门控", () => {
    const detailPage = readConsumer("app/pages/article/[id].vue");

    expect(detailPage).toMatch(/<section v-if="article\?\.cover"/u);
    expect(detailPage).toContain('class="article-cover"');
    // 详情页不消费列表兜底：没有封面就没有任何图区（不是一块占位）。
    expect(detailPage).not.toContain("article-cover-default");
  });
});

describe("文章详情页封面：16:9 且高度不随视口变化", () => {
  const detailPage = readFileSync(join(projectDir, "app/pages/article/[id].vue"), "utf8");

  it("用比例撑高度，不再有 vw / clamp / 固定高度档", () => {
    const block = blockAfter(tailwind, ".article-cover {");

    expect(aspectRatioOf(block)).toBe(SPEC_RATIO);
    expect(block).not.toMatch(/height\s*:/u);
    expect(block).not.toMatch(/vw|vh|clamp\(/u);
  });

  it("详情页封面固定居中：吃 object-cover，且不再贴取景行内样式（#204）", () => {
    const block = blockAfter(tailwind, ".article-cover {");

    expect(block).toContain("object-cover");
    expect(detailPage).not.toMatch(/framingStyle|articleCoverStyle/u);
    expect(detailPage).toMatch(/class="article-cover"/u);
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
  it("历史卡片缩略图锁 16:9，窄屏只改列宽与按钮可见性、不改形状", () => {
    const block = blockAfter(historyPage, ".article-history-card__cover {");
    expect(aspectRatioOf(block)).toBe(SPEC_RATIO);
    expect(block).not.toMatch(/height\s*:/u);

    // #194 起历史页改成栅格短卡片：封面宽度完全由列宽决定，窄屏块只让移出按钮常显，
    // 不得再出现任何第二形状（height / aspect-ratio 覆盖）。
    const narrow = blockAfter(historyPage, "@media (max-width: 768px)");
    expect(narrow).toContain(".article-history-card__remove");
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
