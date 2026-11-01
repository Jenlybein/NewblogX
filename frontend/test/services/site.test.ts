import { beforeEach, describe, expect, it, vi } from "vitest";

import { getBannerList } from "~/services/site";

const requestMock = vi.fn();

describe("site service：首页轮播", () => {
  beforeEach(() => {
    vi.stubGlobal("__useNuxtAppMock", () => ({
      $api: {
        request: requestMock,
      },
    }));
    requestMock.mockReset();
    requestMock.mockResolvedValue({ list: [], has_more: false });
  });

  it("读者站始终显式只请求展示中的轮播", async () => {
    await getBannerList();

    expect(requestMock).toHaveBeenCalledWith("/api/banners", {
      query: {
        show: true,
        page: 1,
        limit: 8,
      },
      auth: false,
    });
  });
});
