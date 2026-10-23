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
    expect(resolveAvatarUrl("/_backend/images/a.png")).toBe("/_asset/images/a.png");
  });

  it("reads avatar and display name from known backend field aliases", () => {
    expect(resolveAvatarUrl({ action_user_avatar: "images/me.png" })).toBe("/_origin/images/me.png");
    expect(resolveDisplayName({ action_user_nickname: " 管理员02 " })).toBe("管理员02");
  });

  it("uses a single uppercase latin initial or the first visible character", () => {
    expect(resolveAvatarInitial("alice")).toBe("A");
    expect(resolveAvatarInitial("管理员02")).toBe("管");
    expect(resolveAvatarInitial("", "系")).toBe("系");
  });
});
