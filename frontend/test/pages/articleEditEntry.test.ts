import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { findColorLiterals } from "../../../scripts/check-design-tokens.mjs";

/**
 * 阅读页「编辑」入口的接线（票 213 / 决定清单 C4；位置由票 236 修订）。
 *
 * 文章页没有既有的组件测试（它是 700 行的 SSR 路由页，依赖一整套取数 composable），
 * 所以这里不做整页 mount，而是断言页面源码里的三件事：
 *   1. 判定走可单测的纯函数（`resolveArticleEditEntry`），输入是既有两个信号；
 *   2. 模板只在拿到入口时渲染，链接带 `article_id` 并带 C4 的 aria-label；
 *   3. **位置**：票 236 / 决定清单第 2 节把入口从 `.article-meta` 行尾搬到 hero 按钮组里、
 *      紧贴质量评分右侧；元信息行回到纯五项计数（不改它们的结构，也不给页面加新样式）。
 *
 * 纯函数的四种可见性组合由 `test/utils/articleEdit.test.ts` 覆盖。
 */

const PAGE_FILE = "app/pages/article/[id].vue";

function pageSource(): string {
  return readFileSync(join(process.cwd(), PAGE_FILE), "utf8");
}

/** hero 的按钮组（`<div class="article-actions">` … 闭合的 `</div>`，内部无嵌套 div）。 */
function actionsSource(source: string): string {
  const start = source.indexOf('<div class="article-actions">');
  expect(start, "页面里找不到 hero 按钮组 .article-actions").toBeGreaterThan(-1);
  const end = source.indexOf("</div>", start);
  expect(end).toBeGreaterThan(start);
  return source.slice(start, end);
}

/** 截出模板里的元信息行（`<dl class="article-meta">` … `</dl>`）。 */
function metaRowSource(source: string): string {
  const start = source.indexOf('<dl class="article-meta">');
  expect(start, "页面里找不到元信息行 <dl class=\"article-meta\">").toBeGreaterThan(-1);
  const end = source.indexOf("</dl>", start);
  return source.slice(start, end);
}

/** 截出模板里编辑入口的 `<NuxtLink` 开标签（属性断言用，避开空白与续行）。 */
function editLinkTag(source: string): string {
  const start = source.indexOf("<NuxtLink");
  expect(start, "按钮组里找不到编辑入口的 NuxtLink").toBeGreaterThan(-1);
  return source.slice(start, source.indexOf(">", start));
}

describe("文章页：编辑入口的判定接线", () => {
  it("用纯函数判定，输入是本人身份 + 站点发布开关 + 当前文章 id", () => {
    const source = pageSource();

    expect(source).toContain('import { resolveArticleEditEntry } from "~/utils/articleEdit";');
    expect(source).toContain('import { useArticleWritePolicy } from "~/composables/useArticleWritePolicy";');
    expect(source).toMatch(/const \{ entryVisible: writeEntryVisible \} = useArticleWritePolicy\(\);/u);
    expect(source).toMatch(
      /resolveArticleEditEntry\(\{\s*articleId: articleId\.value,\s*isSelfAuthor: isSelfAuthor\.value,\s*writeEntryVisible: writeEntryVisible\.value,\s*\}\)/u,
    );
  });

  it("复用既有的 isSelfAuthor（不另算一份作者判定），发布开关只从 useArticleWritePolicy 取", () => {
    const source = pageSource();

    // 页面上唯一的作者判定：profileId === author_id。
    expect(source.match(/const isSelfAuthor = computed/g)).toHaveLength(1);
    // 文章页不自己查站点 article 配置（开关只有 useArticleWritePolicy 一个来源）。
    expect(source).not.toMatch(/allow_non_admin_publish/u);
  });
});

describe("文章页：入口在 hero 按钮组里、紧贴质量评分右侧", () => {
  it("入口在 hero 的 .article-actions 里，且只有拿到入口才渲染", () => {
    const row = actionsSource(pageSource());

    expect(row).toContain('v-if="articleEditEntry"');
    expect(editLinkTag(row)).toContain(':to="articleEditEntry.href"');
  });

  it("排布：质量评分在前、编辑在后（票 236 的「编辑紧贴质量评分右侧」）", () => {
    const row = actionsSource(pageSource());

    const score = row.indexOf("articleScoreButtonLabel");
    const edit = row.indexOf("articleEditEntry.href");
    expect(score).toBeGreaterThan(-1);
    expect(edit).toBeGreaterThan(score);
  });

  it("链接是站内导航（不是按钮 / window.open），跳创作页并加载该文章的编辑态", () => {
    const row = actionsSource(pageSource());
    const tag = editLinkTag(row);

    expect(tag).toContain(":to=");
    expect(tag).not.toContain("@click");
    expect(row).not.toContain("window.open");
    // 地址形状由纯函数负责：/studio/write?article_id=<id>。
    expect(row).not.toMatch(/to="\/studio\/write"/u);
  });

  it("aria-label（以及 title 提示）是 C4 的「编辑这篇文章」，可见文案是「编辑」", () => {
    const row = actionsSource(pageSource());
    const tag = editLinkTag(row);

    expect(tag).toContain(':aria-label="articleEditEntry.ariaLabel"');
    expect(tag).toContain(':title="articleEditEntry.ariaLabel"');
    expect(row).toMatch(/>\s*编辑\s*<\/NuxtLink>/u);
  });

  it("沿用既有 article-action 家族，不给页面新增一套按钮样式或裸色值", () => {
    const source = pageSource();
    const tag = editLinkTag(actionsSource(source));

    expect(tag).toContain('class="article-action article-action--compact"');
    // 页面原本没有 <style> 块；这个入口不引入第二套视觉语言。
    expect(source).not.toContain("<style");
    // 用真门禁的扫描器判裸色值（手写正则会把「票 213」这类引用误判成十六进制色值）。
    expect(findColorLiterals(source)).toEqual([]);
  });
});

describe("文章页：元信息行回到纯五项计数", () => {
  it("既有五项仍是同一组 dt/dd，编辑入口不再塞进行尾", () => {
    const row = metaRowSource(pageSource());
    const existingItems = [
      "formatDateTimeLabel(article?.created_at)",
      "formatCount(article?.view_count || 0)",
      "formatCount(articleDiggCount)",
      "formatCount(article?.favor_count || 0)",
      "formatCount(article?.comment_count || 0)",
    ];

    for (const expression of existingItems) {
      expect(row).toContain(expression);
    }

    // 入口已经搬到按钮组（票 236）：元信息行既没有编辑项，也没有编辑链接。
    expect(row).not.toContain("articleEditEntry");
    expect(row).not.toContain("article-meta__item--edit");
    expect(row).not.toContain("<NuxtLink");

    // 五项各自仍是 dt + dd 的一对（没有为了塞按钮把计数项拍平）。
    expect(row.match(/class="article-meta__item"/gu)).toHaveLength(5);
  });
});
