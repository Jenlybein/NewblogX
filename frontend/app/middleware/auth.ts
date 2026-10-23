export default defineNuxtRouteMiddleware(async () => {
  const authStore = useAuthStore();
  const uiStore = useUiStore();

  if (!authStore.initialized) {
    await authStore.initializeSession();
  }

  if (!authStore.isLoggedIn) {
    uiStore.openAuthModal();
    return navigateTo("/");
  }
});
