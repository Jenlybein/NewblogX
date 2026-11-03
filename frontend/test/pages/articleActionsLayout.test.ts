import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { findColorLiterals } from "../../../scripts/check-design-tokens.mjs";

/**
 * 阅读页互动条的版式接线（#236 / 决定清单第 2 节）。
 *
 * hero 与右栏的重排是纯模板改动，判定收在可单测的组件 / composable 里
 * （`ArticleActionsBar.test.ts`、`useArticleCommentJump.test.ts`）。这里沿用 #213 / #220
 * 的先例只锁"接线"：
 *   1. hero 的 `.article-actions` 只剩质量评分 + 编辑，编辑紧贴评分右侧；
 *   2. 右栏顺序固定为 作者简介 → 互动条 → 目录；
 *   3. <1024px 底部固定条与右栏互动条用**同一个 props 对象**（同一状态源，不会不同步）；
 *   4. 评论按钮走"先问登录再滚动 / 聚焦"的 composable，且真的接到 `#comments` 与评论框；
 *   5. 页面仍旧不带 `<style>`、没有裸色值（`articleOutlineColumn.test.ts` 也盯着这条）。
 */

const PAGE_FILE = "app/pages/article/[id].vue";

function pageSource(): string {
  return readFileSync(join(process.cwd(), PAGE_FILE), "utf8");
}

/** hero（`<header …>` … `</header>`）：质量评分与编辑唯一出现的地方。 */
function heroSource(source: string): string {
  const start = source.indexOf('<header ref="revealTarget"');
  expect(start, "页面里找不到 hero 的 <header>").toBeGreaterThan(-1);
  const end = source.indexOf("</header>", start);
  expect(end, "hero 的 <header> 没有闭合").toBeGreaterThan(start);
  return source.slice(start, end);
}

/** hero 里的按钮组（`<div class="article-actions">` … 闭合的 `</div>`，内部无嵌套 div）。 */
function heroActionsSource(source: string): string {
  const hero = heroSource(source);
  const start = hero.indexOf('<div class="article-actions">');
  expect(start, "hero 里找不到 .article-actions").toBeGreaterThan(-1);
  const end = hero.indexOf("</div>", start);
  expect(end).toBeGreaterThan(start);
  return hero.slice(start, end);
}

/** 右栏（`<aside class="article-aside">` … `</aside>`）。 */
function asideSource(source: string): string {
  const start = source.indexOf('<aside class="article-aside">');
  expect(start, "页面里找不到右栏 .article-aside").toBeGreaterThan(-1);
  const end = source.indexOf("</aside>", start);
  expect(end, "右栏没有闭合").toBeGreaterThan(start);
  return source.slice(start, end);
}

/** 正文布局的收尾位置（`articleOutlineColumn.test.ts` 用的同一个锚点）。 */
function layoutEndIndex(source: string): number {
  const marker = "</aside>\n      </div>";
  const index = source.indexOf(marker);
  expect(index, "找不到正文布局的收尾").toBeGreaterThan(-1);
  return index + marker.length;
}

describe("hero：只留质量评分 + 编辑", () => {
  it("按钮组里只有质量评分与编辑入口，四个互动动作不再出现在 hero", () => {
    const actions = heroActionsSource(pageSource());

    expect(actions).toContain("articleScoreButtonLabel");
    expect(actions).toContain("articleEditEntry");
    expect(actions).not.toContain("ArticleActionsBar");
    expect(actions).not.toContain("handleLike");
    expect(actions).not.toContain("handleFavorite");
    expect(actions).not.toContain("handleShare");
    expect(actions).not.toContain("handleCommentJump");
  });

  it("编辑紧贴质量评分右侧：DOM 顺序是评分在前、编辑在后", () => {
    const actions = heroActionsSource(pageSource());

    const score = actions.indexOf("articleScoreButtonLabel");
    const edit = actions.indexOf("articleEditEntry.href");
    expect(score).toBeGreaterThan(-1);
    expect(edit).toBeGreaterThan(score);
  });

  it("编辑入口搬出元信息行：`.article-meta` 只剩五项计数（不再有编辑项）", () => {
    const source = pageSource();
    const start = source.indexOf('<dl class="article-meta">');
    const end = source.indexOf("</dl>", start);
    const meta = source.slice(start, end);

    expect(meta).not.toContain("articleEditEntry");
    expect(meta).not.toContain("article-meta__item--edit");
    // 五项计数仍在（发布 / 阅读 / 点赞 / 收藏 / 评论）。
    expect(meta.match(/class="article-meta__item"/gu)).toHaveLength(5);
  });

  it("编辑入口的门控不变：仍是 articleEditEntry（作者本人 + 站点发布开关）", () => {
    const actions = heroActionsSource(pageSource());

    expect(actions).toContain('v-if="articleEditEntry"');
    expect(actions).toContain(':to="articleEditEntry.href"');
    expect(actions).toContain(':aria-label="articleEditEntry.ariaLabel"');
  });
});

describe("右栏：作者简介 → 互动条 → 目录", () => {
  it("顺序固定，互动条夹在作者卡与目录之间", () => {
    const aside = asideSource(pageSource());

    const author = aside.indexOf("article-author");
    const actions = aside.indexOf("<ArticleActionsBar");
    const toc = aside.indexOf("<ArticleTocAnchor");
    expect(author, "右栏找不到作者简介").toBeGreaterThan(-1);
    expect(actions, "右栏找不到互动条").toBeGreaterThan(-1);
    expect(toc, "右栏找不到目录").toBeGreaterThan(-1);
    expect(actions).toBeGreaterThan(author);
    expect(toc).toBeGreaterThan(actions);
  });

  it("互动条四个动作各接页面动作；点赞/收藏不另造状态（真相在 useArticleDigg 与正文）", () => {
    const aside = asideSource(pageSource());

    expect(aside).toContain('@like="handleLike"');
    expect(aside).toContain('@favorite="handleFavorite"');
    expect(aside).toContain('@share="handleShare"');
    expect(aside).toContain('@comment="handleCommentJump"');
  });
});

describe("<1024px 底部固定条：同一组件、同一状态源", () => {
  it("底部条渲染在正文布局之外（右栏收在 <1024px 时才由 CSS 接过入口）", () => {
    const source = pageSource();
    const layoutEnd = layoutEndIndex(source);
    const fixed = source.indexOf('variant="fixed"');

    expect(fixed, "找不到底部固定条").toBeGreaterThan(-1);
    expect(fixed).toBeGreaterThan(layoutEnd);
  });

  it("右栏互动条与底部条绑同一个 props 对象：两处不可能显示不同的赞/藏态", () => {
    const source = pageSource();

    // 页面里只有两个 ArticleActionsBar 实例：右栏一个、底部一个。
    expect(source.match(/<ArticleActionsBar/gu)).toHaveLength(2);
    expect(source.match(/v-bind="articleActionProps"/gu)).toHaveLength(2);
    // 状态只有一个来源对象，第二个实例不再自己拼一份 props。
    expect(source.match(/const articleActionProps = computed/gu)).toHaveLength(1);
  });

  it("口径对齐真相：点赞取 useArticleDigg 的乐观值，收藏/评论取正文权威计数", () => {
    const source = pageSource();
    const start = source.indexOf("const articleActionProps = computed");
    const block = source.slice(start, source.indexOf("});", start));

    expect(block).toContain("diggCount: articleDiggCount.value");
    expect(block).toContain("isDigged: articleIsDigged.value");
    expect(block).toContain("likePending: articleLikePending.value");
    expect(block).toContain("likeDebouncing: articleLikeDebouncing.value");
    expect(block).toContain("favorCount: article.value?.favor_count ?? 0");
    expect(block).toContain("commentCount: article.value?.comment_count ?? 0");
    expect(block).toContain("isFavor: Boolean(article.value?.is_favor)");
  });
});

describe("评论按钮：点到 #comments 并聚焦评论框（未登录不滚动不聚焦）", () => {
  it("用 useArticleCommentJump 收口登录前置与顺序，而不是在模板里手写", () => {
    const source = pageSource();

    expect(source).toContain('import { useArticleCommentJump } from "~/composables/useArticleCommentJump";');
    expect(source).toMatch(/useArticleCommentJump\(\{\s*ensureLoggedIn: \(\) => authStore\.initializeSession\(\)/u);
  });

  it("滚动真的落在评论区（#comments 区块的 ref），焦点真的进评论框（组件暴露的 focus）", () => {
    const source = pageSource();

    expect(source).toContain('const commentsSectionRef = ref<HTMLElement | null>(null);');
    expect(source).toContain('id="comments"');
    expect(source).toContain('ref="commentsSectionRef"');
    expect(source).toContain("scrollIntoView(");
    // 票 238 与票 236 各自加过一个「把焦点放进评论框」的 ref，合并时统一成 composerRef（两种语义同组件暴露）。
    expect(source).toContain('ref="composerRef"');
    expect(source).toContain("focusCommentComposer");
    // 评论框组件必须真的暴露 focus 入口（页面只是调用它）。
    const composer = readFileSync(join(process.cwd(), "app/components/comment/CommentComposer.vue"), "utf8");
    expect(composer).toContain("defineExpose({ focusInput, focus })");
    expect(composer).toMatch(/function focus\(\)/u);
  });

  it("未登录的判断交给 composable（页面不先滚动再检查登录）", () => {
    const source = pageSource();
    const start = source.indexOf("function handleCommentJump");
    const block = source.slice(start, source.indexOf("\n}", start));

    expect(block).toContain("jumpToComment");
    expect(block).not.toContain("scrollIntoView");
    expect(block).not.toContain("openAuthModal");
  });
});

describe("页面模板约束（本票验收）", () => {
  it("仍旧没有 <style> 块，也没有裸色值：互动条样式全部落在 tailwind.css", () => {
    const source = pageSource();

    expect(source).not.toContain("<style");
    expect(findColorLiterals(source)).toEqual([]);
  });
});
