import { beforeEach, describe, expect, it, vi } from "vitest";

const resolveMock = vi.fn();
const openAuthModalMock = vi.fn();
const navigateToMock = vi.fn();
const useAuthStoreMock = vi.hoisted(() => vi.fn());
const useUiStoreMock = vi.hoisted(() => vi.fn());

vi.mock("~/stores/auth", () => ({
  useAuthStore: useAuthStoreMock,
}));

vi.mock("~/stores/ui", () => ({
  useUiStore: useUiStoreMock,
}));

async function createComposable() {
  vi.resetModules();
  const module = await import("~/composables/useWriteEntry");
  return module.useWriteEntry();
}

describe("useWriteEntry", () => {
  beforeEach(() => {
    resolveMock.mockReset();
    openAuthModalMock.mockReset();
    navigateToMock.mockReset();
    useAuthStoreMock.mockReturnValue({
      isLoggedIn: true,
    });
    useUiStoreMock.mockReturnValue({
      openAuthModal: openAuthModalMock,
    });
    vi.stubGlobal("__useRouterMock", () => ({
      resolve: resolveMock,
    }));
    vi.stubGlobal("__navigateToMock", () => navigateToMock);
  });

  it("opens the write page in a new tab for logged-in users without navigating the current page", async () => {
    const openMock = vi.spyOn(window, "open").mockImplementation(() => ({ closed: false } as Window));
    resolveMock.mockReturnValue({ href: "/studio/write" });

    const { openWriteEntry } = await createComposable();
    const opened = openWriteEntry();

    expect(opened).toBe(true);
    expect(resolveMock).toHaveBeenCalledWith({ path: "/studio/write" });
    expect(openMock).toHaveBeenCalledWith("/studio/write", "_blank", "noopener,noreferrer");
    expect(navigateToMock).not.toHaveBeenCalled();
    expect(openAuthModalMock).not.toHaveBeenCalled();
  });

  it("includes the article id when opening an editor tab for an existing article", async () => {
    const openMock = vi.spyOn(window, "open").mockImplementation(() => ({ closed: false } as Window));
    resolveMock.mockReturnValue({ href: "/studio/write?article_id=art-1" });

    const { openWriteEntry } = await createComposable();
    openWriteEntry({ articleId: "art-1" });

    expect(resolveMock).toHaveBeenCalledWith({
      path: "/studio/write",
      query: {
        article_id: "art-1",
      },
    });
    expect(openMock).toHaveBeenCalledWith("/studio/write?article_id=art-1", "_blank", "noopener,noreferrer");
  });

  it("opens the auth modal instead of opening a tab when the user is not logged in", async () => {
    useAuthStoreMock.mockReturnValue({
      isLoggedIn: false,
    });
    const openMock = vi.spyOn(window, "open").mockImplementation(() => ({ closed: false } as Window));

    const { openWriteEntry } = await createComposable();
    const opened = openWriteEntry();

    expect(opened).toBe(false);
    expect(openAuthModalMock).toHaveBeenCalledTimes(1);
    expect(resolveMock).not.toHaveBeenCalled();
    expect(openMock).not.toHaveBeenCalled();
    expect(navigateToMock).not.toHaveBeenCalled();
  });
});
