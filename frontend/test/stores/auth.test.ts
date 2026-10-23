import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";

const loginWithPasswordMock = vi.hoisted(() => vi.fn());
const loginWithEmailCodeMock = vi.hoisted(() => vi.fn());
const registerWithEmailMock = vi.hoisted(() => vi.fn());
const logoutCurrentSessionMock = vi.hoisted(() => vi.fn());
const getSelfUserDetailMock = vi.hoisted(() => vi.fn());
const isAuthLikeErrorMock = vi.hoisted(() => vi.fn());
const useMessageStoreMock = vi.hoisted(() => vi.fn());
const useChatStoreMock = vi.hoisted(() => vi.fn());

vi.mock("~/services/auth", () => ({
  loginWithPassword: loginWithPasswordMock,
  loginWithEmailCode: loginWithEmailCodeMock,
  registerWithEmail: registerWithEmailMock,
  logoutCurrentSession: logoutCurrentSessionMock,
}));

vi.mock("~/services/user", () => ({
  getSelfUserDetail: getSelfUserDetailMock,
}));

vi.mock("~/services/http/errors", () => ({
  isAuthLikeError: isAuthLikeErrorMock,
}));

vi.mock("~/stores/message", () => ({
  useMessageStore: useMessageStoreMock,
}));

vi.mock("~/stores/chat", () => ({
  useChatStore: useChatStoreMock,
}));

async function createAuthStore() {
  vi.resetModules();
  setActivePinia(createPinia());
  const module = await import("~/stores/auth");
  return module.useAuthStore();
}

describe("auth store", () => {
  beforeEach(() => {
    localStorage.clear();
    loginWithPasswordMock.mockReset();
    loginWithEmailCodeMock.mockReset();
    registerWithEmailMock.mockReset();
    logoutCurrentSessionMock.mockReset();
    getSelfUserDetailMock.mockReset();
    isAuthLikeErrorMock.mockReset();
    useMessageStoreMock.mockReturnValue({
      clear: vi.fn(),
      refreshSummary: vi.fn().mockResolvedValue(undefined),
    });
    useChatStoreMock.mockReturnValue({
      resetSocketState: vi.fn(),
    });
    vi.stubGlobal("__useRuntimeConfigMock", () => ({
      public: {
        apiBase: "/_backend",
      },
    }));
    vi.stubGlobal("__useRequestHeadersMock", () => ({}));
    vi.stubGlobal("$fetch", vi.fn());
  });

  it("restores the cached token and profile while refreshing the latest user data", async () => {
    localStorage.setItem("blogx_access_token", "token-1");
    localStorage.setItem("blogx_profile_snapshot", JSON.stringify({
      id: "u1",
      nickname: "管理员02",
      avatar: "/uploads/cached.png",
    }));
    getSelfUserDetailMock.mockResolvedValue({
      id: "u1",
      nickname: "管理员02",
      avatar: "/uploads/live.png",
    });
    isAuthLikeErrorMock.mockReturnValue(false);

    const authStore = await createAuthStore();
    const initialized = await authStore.initializeSession();

    expect(initialized).toBe(true);
    expect(authStore.isLoggedIn).toBe(true);
    expect(authStore.profileId).toBe("u1");
    expect(authStore.profileAvatar).toBe("/_origin/uploads/live.png");
    expect(getSelfUserDetailMock).toHaveBeenCalledTimes(1);
    expect(JSON.parse(localStorage.getItem("blogx_profile_snapshot") || "{}")).toMatchObject({
      id: "u1",
      avatar: "/_origin/uploads/live.png",
    });
  });

  it("keeps the local session when initializeSession hits a non-auth failure", async () => {
    localStorage.setItem("blogx_access_token", "token-2");
    localStorage.setItem("blogx_profile_snapshot", JSON.stringify({
      id: "u2",
      nickname: "River",
      avatar: "/uploads/river.png",
    }));
    getSelfUserDetailMock.mockRejectedValue(new Error("502 Bad Gateway"));
    isAuthLikeErrorMock.mockReturnValue(false);

    const authStore = await createAuthStore();
    const initialized = await authStore.initializeSession();

    expect(initialized).toBe(true);
    expect(authStore.isLoggedIn).toBe(true);
    expect(authStore.profileId).toBe("u2");
    expect(authStore.profileAvatar).toBe("/_origin/uploads/river.png");
    expect(localStorage.getItem("blogx_access_token")).toBe("token-2");
  });

  it("clears the local session when initializeSession receives an auth-like failure", async () => {
    localStorage.setItem("blogx_access_token", "token-3");
    localStorage.setItem("blogx_profile_snapshot", JSON.stringify({
      id: "u3",
      nickname: "Louis",
      avatar: "/uploads/louis.png",
    }));
    const authError = new Error("token expired");
    getSelfUserDetailMock.mockRejectedValue(authError);
    isAuthLikeErrorMock.mockImplementation((error: unknown) => error === authError);

    const authStore = await createAuthStore();
    const initialized = await authStore.initializeSession();

    expect(initialized).toBe(false);
    expect(authStore.isLoggedIn).toBe(false);
    expect(authStore.currentUser).toBeNull();
    expect(localStorage.getItem("blogx_access_token")).toBeNull();
    expect(localStorage.getItem("blogx_profile_snapshot")).toBeNull();
  });

  it("deduplicates concurrent initializeSession calls into a single user-detail request", async () => {
    localStorage.setItem("blogx_access_token", "token-4");
    getSelfUserDetailMock.mockResolvedValue({
      id: "u4",
      nickname: "Admin",
      avatar: "/uploads/admin.png",
    });
    isAuthLikeErrorMock.mockReturnValue(false);

    const authStore = await createAuthStore();
    const [first, second] = await Promise.all([
      authStore.initializeSession(),
      authStore.initializeSession(),
    ]);

    expect(first).toBe(true);
    expect(second).toBe(true);
    expect(getSelfUserDetailMock).toHaveBeenCalledTimes(1);
  });
});
