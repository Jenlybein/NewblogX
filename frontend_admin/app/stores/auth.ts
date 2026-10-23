import { computed, ref, shallowRef } from 'vue'
import { defineStore } from 'pinia'
import { getSelfUserDetail, loginWithPassword, logoutCurrentSession, refreshAccessToken } from '~/services/auth'
import { isAuthLikeError } from '~/services/http/errors'
import type { UserDetailResponseData } from '~/types/api'

export const useAuthStore = defineStore('auth', () => {
  const tokenStorageKey = 'blogx_admin_access_token'
  const profileStorageKey = 'blogx_admin_profile_snapshot'
  const canUseStorage = typeof window !== 'undefined' && typeof localStorage !== 'undefined'

  const accessToken = shallowRef(canUseStorage ? localStorage.getItem(tokenStorageKey) || '' : '')
  const currentUser = ref<UserDetailResponseData | null>(restoreProfile())
  const initialized = shallowRef(false)
  const pending = shallowRef(false)
  let initPromise: Promise<boolean> | null = null
  let refreshPromise: Promise<boolean> | null = null

  function restoreProfile() {
    if (!canUseStorage) return null
    const raw = localStorage.getItem(profileStorageKey)
    if (!raw) return null

    try {
      const parsed = JSON.parse(raw) as UserDetailResponseData
      return parsed?.id ? parsed : null
    } catch {
      localStorage.removeItem(profileStorageKey)
      return null
    }
  }

  function persistProfile(profile: UserDetailResponseData | null) {
    if (!canUseStorage) return
    if (profile) localStorage.setItem(profileStorageKey, JSON.stringify(profile))
    else localStorage.removeItem(profileStorageKey)
  }

  function setAccessToken(token: string) {
    accessToken.value = token
    if (!canUseStorage) return
    if (token) localStorage.setItem(tokenStorageKey, token)
    else localStorage.removeItem(tokenStorageKey)
  }

  function clearSession() {
    setAccessToken('')
    currentUser.value = null
    persistProfile(null)
  }

  async function fetchCurrentUser(options: { throwOnError?: boolean } = {}) {
    if (!accessToken.value) {
      currentUser.value = null
      persistProfile(null)
      return null
    }

    try {
      const profile = await getSelfUserDetail()
      currentUser.value = profile
      persistProfile(profile)
      return profile
    } catch (error) {
      if (options.throwOnError) throw error
      return null
    }
  }

  async function refreshSession() {
    if (refreshPromise) return refreshPromise

    refreshPromise = (async () => {
      try {
        const response = await refreshAccessToken()
        if (response.code !== 0 || !response.data) {
          clearSession()
          return false
        }

        setAccessToken(response.data)
        await fetchCurrentUser().catch(() => undefined)
        return true
      } catch {
        clearSession()
        return false
      } finally {
        refreshPromise = null
      }
    })()

    return refreshPromise
  }

  async function initializeSession() {
    if (initialized.value) return !!accessToken.value
    if (initPromise) return initPromise

    initPromise = (async () => {
      if (!accessToken.value) {
        initialized.value = true
        initPromise = null
        return false
      }

      try {
        await fetchCurrentUser({ throwOnError: true })
      } catch (error) {
        if (isAuthLikeError(error)) clearSession()
      }

      initialized.value = true
      initPromise = null
      return !!accessToken.value
    })()

    return initPromise
  }

  async function login(payload: { username: string; password: string }) {
    pending.value = true
    try {
      const token = await loginWithPassword(payload)
      setAccessToken(token)
      await fetchCurrentUser()
      initialized.value = true
      return true
    } finally {
      pending.value = false
    }
  }

  async function logout() {
    try {
      if (accessToken.value) await logoutCurrentSession()
    } finally {
      clearSession()
      initialized.value = true
    }
  }

  const isLoggedIn = computed(() => !!accessToken.value)
  const profileName = computed(() => currentUser.value?.nickname || currentUser.value?.username || '管理员')
  const profileAvatar = computed(() => currentUser.value?.avatar || '')

  return {
    accessToken,
    currentUser,
    initialized,
    pending,
    isLoggedIn,
    profileName,
    profileAvatar,
    setAccessToken,
    clearSession,
    fetchCurrentUser,
    refreshSession,
    initializeSession,
    login,
    logout,
  }
})
