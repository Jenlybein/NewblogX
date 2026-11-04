import { useAuthStore } from "~/stores/auth";
import { useUiStore } from "~/stores/ui";
import { createReaderLoginGate } from "~/utils/readerLoginGate";

/**
 * 互动统一登录前置（#238 范围 7）的接线层。
 *
 * 判定与提示都在 `utils/readerLoginGate.ts`（纯依赖注入、有独立用例）；这里只做
 * 一件薄事：把会话状态与登录弹窗两个既有 store 接上去，返回页面各处共用的
 * `requireLogin()` —— 未登录时它只弹登录框并返回 false，调用方此时不得发请求。
 */
export function useReaderLoginGate(): () => Promise<boolean> {
  const authStore = useAuthStore();
  const uiStore = useUiStore();

  const { requireLogin } = createReaderLoginGate({
    initializeSession: () => authStore.initializeSession(),
    openAuthModal: () => uiStore.openAuthModal(),
  });

  return requireLogin;
}
