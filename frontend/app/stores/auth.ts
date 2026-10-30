import { computed, ref, shallowRef } from "vue";
import { defineStore } from "pinia";
import { useRequestHeaders, useRuntimeConfig } from "#imports";
import { loginWithEmailCode, loginWithPassword, logoutCurrentSession, registerWithEmail } from "~/services/auth";
import { isAuthLikeError } from "~/services/http/errors";
import { getSelfUserDetail } from "~/services/user";
import type { UserSelfDetail } from "~/types/api";
import { resolveAvatarUrl } from "~/utils/avatar";

/**
 * 刷新令牌请求的超时上限（毫秒）。
 *
 * 与 `services/http/client.ts` 的 `DEFAULT_TIMEOUT_MS` 取同一个数量级，但**不能
 * 直接复用**：那个常量属于 API 客户端，而这里走的是裸 `$fetch`（刻意如此——
 * 刷新接口本身不能依赖"鉴权失败自动刷新"那套逻辑，否则递归）。所以超时要自己带一份。
 */
const refreshTimeoutMs = 15000;

export const useAuthStore = defineStore("auth", () => {
  const tokenStorageKey = "blogx_access_token";
  const profileStorageKey = "blogx_profile_snapshot";
  const canUseBrowserStorage = typeof window !== "undefined" && typeof localStorage !== "undefined";
  function isValidUserSnapshot(input: unknown): input is UserSelfDetail {
    if (!input || typeof input !== "object") {
      return false;
    }

    const candidate = input as Partial<UserSelfDetail>;
    return typeof candidate.id === "string" && (typeof candidate.nickname === "string" || typeof candidate.username === "string");
  }

  const initialToken = canUseBrowserStorage ? localStorage.getItem(tokenStorageKey) || "" : "";
  const initialProfile = (() => {
    if (!canUseBrowserStorage) {
      return null;
    }

    const raw = localStorage.getItem(profileStorageKey);
    if (!raw) {
      return null;
    }

    try {
      const parsed = JSON.parse(raw) as unknown;
      if (isValidUserSnapshot(parsed)) {
        return parsed;
      }
      localStorage.removeItem(profileStorageKey);
      return null;
    } catch {
      localStorage.removeItem(profileStorageKey);
      return null;
    }
  })();

  const accessToken = shallowRef(initialToken);
  const initialized = shallowRef(false);
  const pending = shallowRef(false);
  const currentUser = ref<UserSelfDetail | null>(initialProfile);
  let initPromise: Promise<boolean> | null = null;
  let refreshPromise: Promise<boolean> | null = null;

  const pickAvatar = resolveAvatarUrl;

  function persistCurrentUser(user: UserSelfDetail | null) {
    if (!canUseBrowserStorage) {
      return;
    }

    if (user) {
      localStorage.setItem(profileStorageKey, JSON.stringify(user));
      return;
    }

    localStorage.removeItem(profileStorageKey);
  }

  function restoreCachedProfile() {
    if (!canUseBrowserStorage) {
      return null;
    }

    const raw = localStorage.getItem(profileStorageKey);
    if (!raw) {
      return null;
    }

    try {
      const parsed = JSON.parse(raw) as unknown;
      if (!isValidUserSnapshot(parsed)) {
        localStorage.removeItem(profileStorageKey);
        return null;
      }
      currentUser.value = {
        ...parsed,
        avatar: pickAvatar(parsed),
      };
      return parsed;
    } catch {
      localStorage.removeItem(profileStorageKey);
      return null;
    }
  }

  function setAccessToken(token: string) {
    accessToken.value = token;
    if (canUseBrowserStorage) {
      if (token) {
        localStorage.setItem(tokenStorageKey, token);
      } else {
        localStorage.removeItem(tokenStorageKey);
      }
    }
  }

  function clearSession() {
    accessToken.value = "";
    currentUser.value = null;
    if (canUseBrowserStorage) {
      localStorage.removeItem(tokenStorageKey);
      localStorage.removeItem(profileStorageKey);
    }
    // 这里**不再**去清 message / chat 两个 store。
    //
    // 它们自己 watch 本 store 的 isLoggedIn：变成 false 时清未读数、重置连接状态。
    // 方向因此是单向的（message/chat 依赖 auth），本文件不需要知道谁在关心登录态。
    // 见 docs/archive/v1/05-baseline-and-process.md §三（M6 决策留档）。
  }

  async function fetchCurrentUser(options: { throwOnError?: boolean; retryAuth?: boolean } = {}) {
    if (!accessToken.value) {
      currentUser.value = null;
      return null;
    }

    try {
      const detail = await getSelfUserDetail({ retryAuth: options.retryAuth });
      currentUser.value = {
        ...detail,
        avatar: pickAvatar(detail),
      };
      persistCurrentUser(currentUser.value);
      return currentUser.value;
    } catch (error) {
      if (options.throwOnError) {
        throw error;
      }
      return null;
    }
  }

  async function refreshSession() {
    if (refreshPromise) return refreshPromise;

    refreshPromise = (async () => {
      try {
        const config = useRuntimeConfig();
        const headers = import.meta.server ? useRequestHeaders(["cookie"]) : undefined;
        // 这个请求绕过了 services/http/client.ts 的 withRequestTimeout，必须自带超时。
        // 少了它，服务端连接一停顿这个 Promise 就永不 settle，下面 finally 里的
        // refreshPromise = null 也永不执行，整个会话的自动刷新被永久毒化。
        const response = await $fetch<{ code: number; msg: string; data: string }>("/api/users/refresh", {
          baseURL: config.public.apiBase,
          method: "POST",
          credentials: "include",
          headers,
          retry: 0,
          timeout: refreshTimeoutMs,
        });

        if (response.code !== 0 || !response.data) {
          clearSession();
          return false;
        }

        setAccessToken(response.data);
        // retryAuth: false 是死锁的唯一解法：此刻 refreshPromise 正指向当前这个
        // 未完成的 Promise，若这次校验又走到自动刷新，就会 await 到自己，
        // 表现是请求永远不返回（点赞按钮一直转圈、且不弹任何错误）。
        await fetchCurrentUser({ retryAuth: false });
        return true;
      } catch {
        clearSession();
        return false;
      } finally {
        refreshPromise = null;
      }
    })();

    return refreshPromise;
  }

  async function initializeSession() {
    if (canUseBrowserStorage) {
      if (!accessToken.value) {
        const cached = localStorage.getItem(tokenStorageKey) || "";
        if (cached) {
          setAccessToken(cached);
        }
      }
      if (accessToken.value && !currentUser.value) {
        restoreCachedProfile();
      }
    }

    if (initialized.value) {
      if (accessToken.value && !currentUser.value) {
        try {
          await fetchCurrentUser({ throwOnError: true });
        } catch (error) {
          if (isAuthLikeError(error)) {
            clearSession();
          }
        }
      }
      return !!accessToken.value;
    }
    if (initPromise) return initPromise;

    initPromise = (async () => {
      if (!accessToken.value) {
        initialized.value = true;
        initPromise = null;
        return false;
      }

      try {
        await fetchCurrentUser({ throwOnError: true });
      } catch (error) {
        // 仅在明确鉴权失败时清会话；网络波动/服务短暂异常不应直接踢掉本地登录态。
        if (isAuthLikeError(error)) {
          clearSession();
        }
      }

      initialized.value = true;
      initPromise = null;
      return !!accessToken.value;
    })();

    return initPromise;
  }

  async function loginByPassword(payload: { username: string; password: string }) {
    pending.value = true;
    try {
      const token = await loginWithPassword(payload);
      setAccessToken(token);
      await fetchCurrentUser();
      initialized.value = true;
      return true;
    } finally {
      pending.value = false;
    }
  }

  async function loginByEmailCode(payload: { email_id: string; email_code: string }) {
    pending.value = true;
    try {
      const token = await loginWithEmailCode(payload);
      setAccessToken(token);
      await fetchCurrentUser();
      initialized.value = true;
      return true;
    } finally {
      pending.value = false;
    }
  }

  async function registerByEmail(payload: { pwd: string; email_id: string; email_code: string }) {
    pending.value = true;
    try {
      const token = await registerWithEmail(payload);
      setAccessToken(token);
      await fetchCurrentUser();
      initialized.value = true;
      return true;
    } finally {
      pending.value = false;
    }
  }

  async function logout() {
    try {
      if (accessToken.value) {
        await logoutCurrentSession();
      }
    } finally {
      clearSession();
      initialized.value = true;
    }
  }

  const isLoggedIn = computed(() => !!accessToken.value);
  const profileId = computed(() => currentUser.value?.id ?? null);
  const profileName = computed(() => {
    if (!currentUser.value) {
      return "未登录";
    }
    return currentUser.value.nickname || currentUser.value.username || "";
  });
  const profileAvatar = computed(() => pickAvatar(currentUser.value));

  return {
    accessToken,
    initialized,
    pending,
    currentUser,
    isLoggedIn,
    profileId,
    profileName,
    profileAvatar,
    setAccessToken,
    clearSession,
    fetchCurrentUser,
    refreshSession,
    initializeSession,
    loginByPassword,
    loginByEmailCode,
    registerByEmail,
    logout,
  };
});
