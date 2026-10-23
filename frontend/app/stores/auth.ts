import { computed, ref, shallowRef } from "vue";
import { defineStore } from "pinia";
import { useRequestHeaders, useRuntimeConfig } from "#imports";
import { loginWithEmailCode, loginWithPassword, logoutCurrentSession, registerWithEmail } from "~/services/auth";
import { isAuthLikeError } from "~/services/http/errors";
import { getSelfUserDetail } from "~/services/user";
import { useChatStore } from "~/stores/chat";
import { useMessageStore } from "~/stores/message";
import type { UserSelfDetail } from "~/types/api";
import { resolveAvatarUrl } from "~/utils/avatar";

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
    useMessageStore().clear();
    useChatStore().resetSocketState();
  }

  async function fetchCurrentUser(options: { throwOnError?: boolean } = {}) {
    if (!accessToken.value) {
      currentUser.value = null;
      return null;
    }

    try {
      const detail = await getSelfUserDetail();
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
        const response = await $fetch<{ code: number; msg: string; data: string }>("/api/users/refresh", {
          baseURL: config.public.apiBase,
          method: "POST",
          credentials: "include",
          headers,
          retry: 0,
        });

        if (response.code !== 0 || !response.data) {
          clearSession();
          return false;
        }

        setAccessToken(response.data);
        await fetchCurrentUser();
        await useMessageStore().refreshSummary().catch(() => undefined);
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
      await useMessageStore().refreshSummary().catch(() => undefined);
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
      await useMessageStore().refreshSummary().catch(() => undefined);
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
      await useMessageStore().refreshSummary().catch(() => undefined);
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
