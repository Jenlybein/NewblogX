export default defineNuxtRouteMiddleware(async (to) => {
  if (to.path === '/login') return

  const auth = useAuthStore()
  const ok = await auth.initializeSession()
  if (!ok) {
    return navigateTo({ path: '/login', query: { redirect: to.fullPath } })
  }
})
