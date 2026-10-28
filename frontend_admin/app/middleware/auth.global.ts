// 后台的全局鉴权守卫。
//
// 只有两个路径不需要登录：
//   - /login  登录页
//   - /setup  首次初始化页（库里还没有管理员，无账号可登）
//
// 为什么不在这里顺便做"没有管理员就跳 /setup"：那会让每次路由切换都多发
// 一个 /setup/status 请求。这个判断放在登录页里做 —— 未登录的人最终都会
// 落到登录页，那里查一次就够。
const PUBLIC_PATHS = new Set(['/login', '/setup'])

export default defineNuxtRouteMiddleware(async (to) => {
  if (PUBLIC_PATHS.has(to.path)) return

  const auth = useAuthStore()
  const ok = await auth.initializeSession()
  if (!ok) {
    return navigateTo({ path: '/login', query: { redirect: to.fullPath } })
  }
})
