import { describe, expect, it } from "vitest";

import { applyArticleFavoriteResult } from "~/utils/articleFavorite";

/**
 * 收藏写响应的空值防御（评审 #248 / P0-2）。
 *
 * 病根：失败分支返回 `data: {}`（甚至没有 data），页面却直接读 `payload.is_favor`。
 * 空值防御是纯函数，这里钉住"缺失载荷时原样返回、绝不抛 TypeError"。
 */

const article = { id: "a1", title: "正文标题", is_favor: true, favor_count: 3 };

describe("applyArticleFavoriteResult", () => {
  it("合法载荷：patch is_favor / favor_count，返回新对象（原对象不被就地修改）", () => {
    const next = applyArticleFavoriteResult(article, { is_favor: false, favor_count: 2 });

    expect(next).toEqual({ ...article, is_favor: false, favor_count: 2 });
    expect(next).not.toBe(article);
    expect(article.is_favor).toBe(true);
  });

  it("payload 缺失（null / undefined）：原样返回，不抛 TypeError", () => {
    expect(() => applyArticleFavoriteResult(article, null)).not.toThrow();
    expect(applyArticleFavoriteResult(article, null)).toBe(article);
    expect(applyArticleFavoriteResult(article, undefined)).toBe(article);
  });

  it("payload 是空对象（旧失败分支的 `data: {}`）：字段不全 → 原样返回", () => {
    expect(applyArticleFavoriteResult(article, {})).toBe(article);
    expect(applyArticleFavoriteResult(article, { is_favor: false })).toBe(article);
    expect(applyArticleFavoriteResult(article, { favor_count: 2 })).toBe(article);
  });

  it("正文还没到：保持 null，不伪造一份半成品", () => {
    expect(applyArticleFavoriteResult(null, { is_favor: false, favor_count: 2 })).toBeNull();
    expect(applyArticleFavoriteResult(undefined, { is_favor: false, favor_count: 2 })).toBeUndefined();
  });
});
