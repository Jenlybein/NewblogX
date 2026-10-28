import type { SetupAdminPayload, SetupStatusData } from '~/types/api'

// 首次初始化相关的请求。
//
// 单独一个文件而不是塞进 admin.ts：这里两个接口是**公开**的（还没有账号
// 可以登录），和 admin.ts 里那些都要管理员令牌的不是一类东西。
// 混在一起以后很容易有人照着旁边的写法给它加个"需要登录"的假设。

function api() {
  return useNuxtApp().$api
}

/** 查询站点是否还需要初始化。 */
export function getSetupStatus() {
  return api().request<SetupStatusData>('/setup/status')
}

/** 创建站点第一个管理员。初始化完成后该接口永久失效。 */
export function createFirstAdmin(payload: SetupAdminPayload) {
  return api().request<{ id: string; username: string }, SetupAdminPayload>('/setup/admin', {
    method: 'POST',
    body: payload,
  })
}
