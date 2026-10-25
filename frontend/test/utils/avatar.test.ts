import { describe, expect, it, vi } from "vitest";
import { resolveAvatarInitial, resolveAvatarUrl, resolveDisplayName } from "~/utils/avatar";

describe("avatar utils", () => {
  it("keeps absolute, protocol-relative, data, and blob avatar urls unchanged", () => {
    expect(resolveAvatarUrl("https://cdn.example.com/a.png")).toBe("https://cdn.example.com/a.png");
    expect(resolveAvatarUrl("//cdn.example.com/a.png")).toBe("//cdn.example.com/a.png");
    expect(resolveAvatarUrl("data:image/png;base64,abc")).toBe("data:image/png;base64,abc");
    expect(resolveAvatarUrl("blob:http://localhost/avatar")).toBe("blob:http://localhost/avatar");
  });

  it("normalizes relative avatar urls through the configured asset proxy", () => {
    vi.stubGlobal("useRuntimeConfig", () => ({
      public: {
        assetProxyBase: "/_asset",
      },
    }));

    expect(resolveAvatarUrl("/uploads/a.png")).toBe("/_asset/uploads/a.png");
    expect(resolveAvatarUrl("_legacy/a.png")).toBe("/_asset/_legacy/a.png");
    // 已带基址的路径必须原样返回，否则会拼成 /_asset/_asset/...
    expect(resolveAvatarUrl("/_asset/uploads/a.png")).toBe("/_asset/uploads/a.png");
  });

  it("reads avatar and display name from known backend field aliases", () => {
    // 默认基址是 /uploads（后端只挂载 uploads 静态目录，没有根路径静态）
    expect(resolveAvatarUrl({ action_user_avatar: "images/me.png" })).toBe("/uploads/images/me.png");
    expect(resolveDisplayName({ action_user_nickname: " 管理员02 " })).toBe("管理员02");
  });

  it("uses a single uppercase latin initial or the first visible character", () => {
    expect(resolveAvatarInitial("alice")).toBe("A");
    expect(resolveAvatarInitial("管理员02")).toBe("管");
    expect(resolveAvatarInitial("", "系")).toBe("系");
  });
});
