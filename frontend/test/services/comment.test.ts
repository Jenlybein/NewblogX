import { beforeEach, describe, expect, it, vi } from "vitest";

import { getRootComments, getRootCommentsAnonymous } from "~/services/comment";

const requestMock = vi.fn();

describe("comment service：根评论有两种取数口径（ADR-0019 约束 2）", () => {
  beforeEach(() => {
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
    requestMock.mockReset();
    requestMock.mockResolvedValue({ list: [], has_more: false });
  });

  it("默认带鉴权：客户端要靠它拿到 is_digg / relation（登录态由客户端补齐）", async () => {
    await getRootComments("a1", 1, 7);

    expect(requestMock).toHaveBeenCalledWith("/api/comments", {
      query: { article_id: "a1", page: 1, limit: 7 },
    });
  });

  it("显式匿名（auth: false）：SSR 取评论不得携带登录身份，否则共享缓存会串号", async () => {
    await getRootCommentsAnonymous("a1", 2, 7);

    expect(requestMock).toHaveBeenCalledWith("/api/comments", {
      auth: false,
      query: { article_id: "a1", page: 2, limit: 7 },
    });
  });
});
