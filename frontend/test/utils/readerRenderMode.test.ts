import { describe, expect, it } from "vitest";
import { readerRenderMode } from "~/utils/readerRenderMode";

describe("readerRenderMode：页面在哪种渲染环境里跑", () => {
  it("服务端渲染", () => {
    expect(readerRenderMode({ server: true, client: false, isHydrating: false })).toBe("server");
  });

  it("客户端首屏 hydration（有 payload，分页数据仍需取一次）", () => {
    expect(readerRenderMode({ server: false, client: true, isHydrating: true })).toBe("hydration");
  });

  it("客户端路由跳转（没有供该路由用的 payload）", () => {
    expect(readerRenderMode({ server: false, client: true, isHydrating: false })).toBe("navigation");
  });
});
