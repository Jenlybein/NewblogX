import { computed, ref, shallowRef } from 'vue'
import { defineStore } from 'pinia'
import { getSelfUserDetail, loginWithPassword, logoutCurrentSession, refreshAccessToken } from '~/services/auth'
import { isAuthLikeError } from '~/services/http/errors'
import type { UserDetailResponseData } from '~/types/api'

export const useAuthStore = defineStore('auth', () => {
  const tokenStorageKey = 'blogx_admin_access_token'
  const profileStorageKey = 'blogx_admin_profile_snapshot'
  const canUseStorage = typeof window !== 'undefined' && typeof localStorage !== 'undefined'

  /**
   * 管理员的角色值（与库里 `user_models.role` 同口径：1 管理员 / 2 注册用户）。
   *
   * 后台必须自己判一次角色，而不是只判"有没有 token"：两个前端同源共享 localStorage，
   * 普通用户把自己的读者站 token 复制到后台这个 key 下，就能渲染出后台的空壳界面
   * （数据仍然拿不到 —— 真正的授权是后端 `AdminMiddleware` 每请求查库）。
   * 见 spec 269 / A-1 260。
   */
  const ADMIN_ROLE = 1
  /** 上一次被拒的原因，供登录页给出可读说明（A-2（票 261））。 */
  const deniedReason = ref<'not-admin' | ''>('')

  const accessToken = shallowRef(canUseStorage ? localStorage.getItem(tokenStorageKey) || '' : '')
  const currentUser = ref<UserDetailResponseData | null>(restoreProfile())
  const initialized = shallowRef(false)
  const pending = shallowRef(false)
  let initPromise: Promise<boolean> | null = null
  let refreshPromise: Promise<boolean> | null = null

  function isAdminUser(profile: UserDetailResponseData | null): boolean {
    return profile?.role === ADMIN_ROLE
  }

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
        const profile = await fetchCurrentUser().catch(() => null)
        // 刷新后同样要判角色：refresh 读的是 session 表、不带 scope，
        // 拿到的可能是普通用户令牌（ADR-0018 的边界）。
        if (!isAdminUser(profile)) {
          clearSession()
          deniedReason.value = 'not-admin'
          return false
        }
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
    if (initialized.value) return !!accessToken.value && isAdminUser(currentUser.value)
    if (initPromise) return initPromise

    initPromise = (async () => {
      if (!accessToken.value) {
        initialized.value = true
        initPromise = null
        return false
      }

      try {
        const profile = await fetchCurrentUser({ throwOnError: true })
        if (!isAdminUser(profile)) {
          // 有 token 但不是管理员：清掉会话，让守卫踢回登录页并说明原因。
          clearSession()
          deniedReason.value = 'not-admin'
          initialized.value = true
          initPromise = null
          return false
        }
      } catch (error) {
        if (isAuthLikeError(error)) clearSession()
      }

      initialized.value = true
      initPromise = null
      return !!accessToken.value && isAdminUser(currentUser.value)
    })()

    return initPromise
  }

  async function login(payload: {
    username: string
    password: string
    /** 站点开启图形验证码时由登录页传入（票 263）。 */
    captchaId?: string
    captchaCode?: string
  }) {
    pending.value = true
    try {
      const token = await loginWithPassword(payload)
      setAccessToken(token)
      const profile = await fetchCurrentUser()
      // 登录接口本身已拒非管理员（CodeAuthAdminRequired），这里再判一次是纵深：
      // 万一服务端校验被放宽，前端也不会把非管理员放进后台。
      if (!isAdminUser(profile)) {
        clearSession()
        deniedReason.value = 'not-admin'
        return false
      }
      deniedReason.value = ''
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
      deniedReason.value = ''
      initialized.value = true
    }
  }

  const isLoggedIn = computed(() => !!accessToken.value)
  const isAdmin = computed(() => isAdminUser(currentUser.value))
  const profileName = computed(() => currentUser.value?.nickname || currentUser.value?.username || '管理员')
  const profileAvatar = computed(() => currentUser.value?.avatar || '')

  return {
    accessToken,
    currentUser,
    initialized,
    pending,
    isLoggedIn,
    isAdmin,
    deniedReason,
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
