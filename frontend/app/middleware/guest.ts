export default defineNuxtRouteMiddleware(async () => {
  const authStore = useAuthStore();

  if (!authStore.initialized) {
    await authStore.initializeSession();
  }

  if (authStore.isLoggedIn) {
    return navigateTo("/");
  }
});
