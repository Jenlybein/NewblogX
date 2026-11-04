import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from '~/stores/auth'

/**
 * 后台角色门禁（spec 269 / A-3，票 262）。
 *
 * 锁两件事：
 *   1. **行为**：拿着普通用户（role=2）的令牌进后台，`initializeSession()` 必须返回 false
 *      并把会话清干净 —— 否则界面会渲染出后台的壳（两个前端同源共享 localStorage，
 *      把读者站 token 复制到后台那个 key 就能绕过登录页）。
 *   2. **源码**：守卫链路里必须真的判角色。行为用例只能证明"当前实现如此"，
 *      源码门禁负责在别人重构掉分支时立刻变红。
 */

const { authApi } = vi.hoisted(() => ({
  authApi: {
    getSelfUserDetail: vi.fn(),
    loginWithPassword: vi.fn(),
    logoutCurrentSession: vi.fn(),
    refreshAccessToken: vi.fn(),
    getLoginCaptcha: vi.fn(),
    getPublicSiteLoginFlags: vi.fn(),
  },
}))

vi.mock('~/services/auth', () => authApi)

const TOKEN_KEY = 'blogx_admin_access_token'
const PROFILE_KEY = 'blogx_admin_profile_snapshot'

function adminProfile() {
  return { id: '1', username: 'jelly', nickname: 'Jelly', avatar: '', role: 1 }
}

function normalUserProfile() {
  return { id: '2', username: 'reader', nickname: '读者', avatar: '', role: 2 }
}

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  localStorage.clear()
  authApi.getSelfUserDetail.mockResolvedValue(adminProfile())
  authApi.loginWithPassword.mockResolvedValue('token-from-login')
  authApi.logoutCurrentSession.mockResolvedValue(undefined)
})

describe('后台会话初始化：只放管理员进来', () => {
  it('普通用户令牌：拿不到会话、令牌被清掉、记下原因', async () => {
    localStorage.setItem(TOKEN_KEY, 'reader-token')
    authApi.getSelfUserDetail.mockResolvedValue(normalUserProfile())

    const auth = useAuthStore()
    const ok = await auth.initializeSession()

    expect(ok).toBe(false)
    expect(auth.accessToken).toBe('')
    expect(localStorage.getItem(TOKEN_KEY)).toBeNull()
    expect(auth.currentUser).toBeNull()
    expect(auth.deniedReason).toBe('not-admin')
  })

  it('管理员令牌：会话成立，快照留在本地', async () => {
    localStorage.setItem(TOKEN_KEY, 'admin-token')

    const auth = useAuthStore()
    const ok = await auth.initializeSession()

    expect(ok).toBe(true)
    expect(auth.isAdmin).toBe(true)
    expect(auth.deniedReason).toBe('')
    expect(JSON.parse(localStorage.getItem(PROFILE_KEY) ?? '{}')).toMatchObject({ role: 1 })
  })

  it('没有任何令牌：不发请求，直接 false', async () => {
    const auth = useAuthStore()

    await expect(auth.initializeSession()).resolves.toBe(false)
    expect(authApi.getSelfUserDetail).not.toHaveBeenCalled()
  })

  it('登录成功后角色不是管理员：清会话并返回 false（纵深，不只依赖服务端拒绝）', async () => {
    authApi.getSelfUserDetail.mockResolvedValue(normalUserProfile())

    const auth = useAuthStore()
    const ok = await auth.login({ username: 'reader', password: 'secret' })

    expect(ok).toBe(false)
    expect(auth.accessToken).toBe('')
    expect(auth.deniedReason).toBe('not-admin')
  })

  it('刷新令牌后拿到的仍是普通用户：不保留会话（refresh 不带 scope）', async () => {
    localStorage.setItem(TOKEN_KEY, 'admin-token')
    authApi.refreshAccessToken.mockResolvedValue({ code: 0, data: 'fresh-token' })
    authApi.getSelfUserDetail.mockResolvedValue(normalUserProfile())

    const auth = useAuthStore()
    await expect(auth.refreshSession()).resolves.toBe(false)
    expect(auth.accessToken).toBe('')
  })
})

describe('源码门禁：守卫链路必须判角色', () => {
  const storeSource = readFileSync(join(process.cwd(), 'app/stores/auth.ts'), 'utf8')
  const middlewareSource = readFileSync(join(process.cwd(), 'app/middleware/auth.global.ts'), 'utf8')

  it('会话初始化里出现角色判断，且非管理员分支会清会话', () => {
    expect(storeSource).toContain('ADMIN_ROLE = 1')
    expect(storeSource).toMatch(/function isAdminUser\(profile: UserDetailResponseData \| null\): boolean \{[\s\S]{0,120}role === ADMIN_ROLE/u)

    // 非管理员分支：清会话 + 记原因。两处（初始化与登录）都要有。
    const clearCalls = storeSource.match(/clearSession\(\)/gu) ?? []
    expect(clearCalls.length).toBeGreaterThanOrEqual(3)
    expect(storeSource).toMatch(/if \(!isAdminUser\((?:profile|currentUser\.value)\)\) \{[\s\S]{0,200}clearSession\(\)/u)
  })

  it('全局守卫用的是 initializeSession（不是"有 token 就放行"）', () => {
    expect(middlewareSource).toContain('initializeSession()')
    expect(middlewareSource).toMatch(/if \(!ok\)[\s\S]{0,120}navigateTo/u)
    // 守卫不得自己读 localStorage 的 token 名来判断登录态。
    expect(middlewareSource).not.toContain('blogx_admin_access_token')
  })
})
