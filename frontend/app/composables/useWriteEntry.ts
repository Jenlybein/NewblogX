import { navigateTo, useRouter } from "#imports";
import { useAuthStore } from "~/stores/auth";
import { useUiStore } from "~/stores/ui";

type OpenWriteEntryOptions = {
  articleId?: string;
};

export function useWriteEntry() {
  const router = useRouter();
  const authStore = useAuthStore();
  const uiStore = useUiStore();

  function buildWriteLocation(options?: OpenWriteEntryOptions) {
    if (options?.articleId) {
      return {
        path: "/studio/write",
        query: {
          article_id: options.articleId,
        },
      } as const;
    }

    return {
      path: "/studio/write",
    } as const;
  }

  function openWriteEntry(options?: OpenWriteEntryOptions) {
    if (!authStore.isLoggedIn) {
      uiStore.openAuthModal();
      return false;
    }

    const target = buildWriteLocation(options);
    const canOpenNewTab = typeof window !== "undefined" && typeof window.open === "function";

    if (!canOpenNewTab) {
      void navigateTo(target);
      return true;
    }

    const href = router.resolve(target).href;
    const opened = window.open(href, "_blank", "noopener,noreferrer");
    return Boolean(opened);
  }

  return {
    openWriteEntry,
  };
}
