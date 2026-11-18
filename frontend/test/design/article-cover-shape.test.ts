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
// 票 327：历史与收藏的短卡片收敛成同一个组件，缩略图形状住在组件里。
const miniCard = readFileSync(join(projectDir, "app/components/article/ArticleMiniCard.vue"), "utf8");

const SPEC_RATIO = "16 / 9";

/** 取块里的比例声明，把空白压成单空格（只消除排版差异，不放过取值差异）。 */
function aspectRatioOf(block: string): string {
  return (/aspect-ratio:\s*([^;]+);/u.exec(block)?.[1] ?? "").replace(/\s+/gu, " ").trim();
}

describe("封面组件：列表与详情共用的 16:9 底座", () => {
  it("图片是 16:9，高度由比例决定而不是写死像素", () => {
    const block = blockAfter(articleCover, ".article-cover-image {");

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

  it("没有封面时不画任何兜底形状：组件只有「有图」一个分支（票 440）", () => {
    expect(articleCover).not.toMatch(/article-cover-(?:plate|default)/u);
    expect(articleCover).not.toMatch(/v-else/u);
    expect(articleCover).toMatch(/v-if="showImage"/u);
  });
});

/**
 * 没封面就不画封面框（票 440）。
 *
 * 两种旧口径都被用户否掉：同底色空白（像图挂了）、首字墨片（更丑）。
 * 现在的约定是"没有就不显示"——组件只渲染有图那一支，图片失败时把 `fail` 抛给
 * 消费方，由消费方整个拿掉封面槽并换成无封面排版。所以这里锁两件事：
 * 组件里没有兜底形状；各消费方都按 `hasCover` 决定要不要那一列/那一块。
 */
describe("没有封面就不显示封面框", () => {
  it("组件里没有任何兜底节点、没有旧文案，也不自己找图（失败只抛 fail）", () => {
    expect(articleCover).not.toMatch(/NO COVER|暂无封面/u);
    expect(articleCover).not.toMatch(/background-image|mask-image|gradient/u);
    expect(articleCover).toMatch(/emit\("fail"\)/u);
  });

  it("封面形状仍然只有 16:9 一份（兜底退场不改变有图时的比例）", () => {
    const image = blockAfter(articleCover, ".article-cover-image {");

    expect(aspectRatioOf(image)).toBe(SPEC_RATIO);
    expect(image).toContain("object-fit: cover");
  });
});
/**
 * 无封面排版（票 440）：不是"画一块占位"，而是"换一种排版"。
 *
 * 信息流：封面列整个不渲染，正文占满整行；网格卡 / 短卡：封面块不渲染，标题上移。
 * 这条一旦回退（又出现空框或占位列），下面几条会先红。
 */
describe("无封面排版：换版式，不画占位", () => {
  const feedItem = readFileSync(join(projectDir, "app/components/article/ArticleFeedItem.vue"), "utf8");
  const gridCard = readFileSync(join(projectDir, "app/components/article/ArticleGridCard.vue"), "utf8");
  const miniCardSource = readFileSync(join(projectDir, "app/components/article/ArticleMiniCard.vue"), "utf8");

  it("信息流条目：封面槽跟着 hasCover 出现，无封面时挂 --no-cover 变体", () => {
    expect(feedItem).toContain("useCoverVisibility");
    expect(feedItem).toMatch(/v-if="hasData && hasCover"/u);
    expect(feedItem).toMatch(/feed-entry--no-cover/u);
    expect(feedItem).toContain('@fail="markCoverFailed"');
  });

  it("网格卡与短卡：封面块用 v-if=hasCover 门控，图片失败时整块消失", () => {
    expect(gridCard).toMatch(/v-if="hasCover"/u);
    expect(gridCard).toContain('@error="markCoverFailed"');
    expect(miniCardSource).toMatch(/v-if="interactive && to && hasCover"/u);
    expect(miniCardSource).toMatch(/v-else-if="hasCover"/u);
  });

  it("无封面变体只在 md 档收成单列（窄屏本来就是单列）", () => {
    const block = blockAfter(tailwind, ".feed-entry--no-cover {");

    expect(block).toContain("md:grid-cols-[minmax(0,1fr)]");
  });

  it("旧的空封面样式已删除：不留 cover-empty 这类空框规则", () => {
    expect(tailwind).not.toMatch(/feed-entry__cover--empty|article-grid-card__cover-empty/u);
    expect(articleCover).not.toMatch(/article-cover-plate|article-cover-default/u);
  });
});
describe("七处消费点同一口径：缺封面只在共用组件里实现一次", () => {
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
    expect(source).not.toMatch(/article-cover-(?:plate|default)/u);
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
    expect(detailPage).not.toContain("article-cover-plate");
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
  it("共享短卡片的缩略图锁 16:9，窄屏只放大触摸留白、不改形状", () => {
    const block = blockAfter(miniCard, ".article-mini-card__cover {");
    expect(aspectRatioOf(block)).toBe(SPEC_RATIO);
    expect(block).not.toMatch(/height\s*:/u);

    // #194 起历史页改成栅格短卡片；票 327 把这张卡抽成共享组件后，
    // 窄屏块只允许动移出按钮的触摸留白，不得再出现任何第二形状。
    const narrow = blockAfter(miniCard, "@media (max-width: 768px)");
    expect(narrow).toContain(".article-mini-card__remove");
    expect(narrow).not.toMatch(/height\s*:/u);
    expect(narrow).not.toMatch(/aspect-ratio/u);
  });

  it("历史页与收藏页都不自己重写一份缩略图形状（形状只有组件一份）", () => {
    // 历史页整页都不该再出现 aspect-ratio：封面形状一旦在页面里被覆盖，就是第二份实现。
    expect(historyPage).not.toMatch(/aspect-ratio/u);

    // 收藏页（票 345 改成"网格 + 大弹窗"）同样不自己画形状：骨架与真实封面
    // 都用共享卡片的多列卡 `.article-grid-card__cover`，页面里一个 aspect-ratio 都没有。
    expect(favoritesPage).not.toMatch(/aspect-ratio/u);
    expect(favoritesPage).not.toMatch(/\.favorite-content-card/u);
    // 旧实现用 min-height 与窄屏高度档撑封面，这两者都等于"第二个形状"。
    expect(favoritesPage).not.toMatch(/min-height\s*:/u);

    // 多列卡的封面形状（骨架与真图共用同一份）锁 16:9，且不按高度撑。
    const gridCover = blockAfter(tailwind, ".article-grid-card__cover {");
    expect(aspectRatioOf(gridCover)).toBe(SPEC_RATIO);
    expect(gridCover).not.toMatch(/height\s*:/u);
  });
});
