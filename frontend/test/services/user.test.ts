import { beforeEach, describe, expect, it, vi } from "vitest";
import { searchUsers } from "~/services/user";

const requestMock = vi.fn();

describe("user service：公开用户搜索", () => {
  beforeEach(() => {
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
    requestMock.mockReset();
  });

  it("把关键词交给 /api/users/search，分页有默认值", async () => {
    requestMock.mockResolvedValue({ list: [], has_more: false });

    await searchUsers({ key: "JellyBin" });

    expect(requestMock).toHaveBeenCalledWith("/api/users/search", {
      query: { key: "JellyBin", page: 1, limit: 6 },
    });
  });

  it("显式分页参数原样透传", async () => {
    requestMock.mockResolvedValue({ list: [], has_more: false });

    await searchUsers({ key: "作者甲", page: 2, limit: 12 });

    expect(requestMock).toHaveBeenCalledWith("/api/users/search", {
      query: { key: "作者甲", page: 2, limit: 12 },
    });
  });
});
