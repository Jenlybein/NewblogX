import { describe, expect, it } from "vitest";

import { ARTICLE_EDIT_ARIA_LABEL, resolveArticleEditEntry } from "~/utils/articleEdit";

/**
 * 阅读页「编辑」入口的门控（#213 / 决定清单 C4）。
 *
 * 判定收在一个纯函数里：页面只负责把两个既有信号（`isSelfAuthor` / 发布开关）
 * 和当前文章 id 传进来。这样"谁能看到"不散在模板里，也就能脱离页面单独断言。
 *
 * 期望值来自票面验收：
 *   - 作者本人可见，文案与 C4 指定的 aria-label 一致，地址沿用 `/studio/write?article_id=`；
 *   - 非作者、游客、未登录（没有本人身份）一律不出现；
 *   - 站点关闭发布时同样不出现（与页头「创作」同一发布开关）。
 */
function entry(overrides: Partial<Parameters<typeof resolveArticleEditEntry>[0]> = {}) {
  return resolveArticleEditEntry({
    articleId: "art-1",
    isSelfAuthor: true,
    writeEntryVisible: true,
    ...overrides,
  });
}

describe("resolveArticleEditEntry：作者本人看到「编辑」", () => {
  it("作者本人拿到创作页编辑态地址（article_id 带的是当前文章）", () => {
    expect(entry()).toEqual({
      href: "/studio/write?article_id=art-1",
      ariaLabel: ARTICLE_EDIT_ARIA_LABEL,
    });
  });

  it("aria-label 是 C4 定稿的那句（读屏用户听到的是「编辑这篇文章」）", () => {
    expect(ARTICLE_EDIT_ARIA_LABEL).toBe("编辑这篇文章");
  });

  it("文章 id 里的保留字符会被转义，不会拼出坏地址", () => {
    expect(entry({ articleId: "art 1/2" })?.href).toBe("/studio/write?article_id=art%201%2F2");
  });
});

describe("resolveArticleEditEntry：别人看不到", () => {
  it("非作者（登录着的其他用户）拿不到入口", () => {
    expect(entry({ isSelfAuthor: false })).toBeNull();
  });

  it("游客 / 未登录（没有本人身份 → isSelfAuthor 为假）同样拿不到", () => {
    expect(entry({ isSelfAuthor: false, articleId: "art-1" })).toBeNull();
  });

  it("没有文章 id 时宁可不渲染，也不拼出一个会跳错文章的链接", () => {
    expect(entry({ articleId: null })).toBeNull();
    expect(entry({ articleId: undefined })).toBeNull();
    expect(entry({ articleId: "" })).toBeNull();
  });
});

describe("resolveArticleEditEntry：遵守站点发布开关", () => {
  it("站点关闭发布入口时，作者本人也看不到「编辑」（与页头「创作」同一开关）", () => {
    expect(entry({ writeEntryVisible: false })).toBeNull();
  });
});
