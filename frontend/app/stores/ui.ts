import type { ThemeMode } from "~/types/api";

export const useUiStore = defineStore("ui", () => {
  const themeCookie = useCookie<ThemeMode>("blogx-theme", {
    default: () => "light",
  });
  const authModalOpen = shallowRef(false);

  function openAuthModal() {
    authModalOpen.value = true;
  }

  function closeAuthModal() {
    authModalOpen.value = false;
  }

  function toggleTheme() {
    themeCookie.value = themeCookie.value === "dark" ? "light" : "dark";
  }

  return {
    authModalOpen,
    theme: themeCookie,
    openAuthModal,
    closeAuthModal,
    toggleTheme,
  };
});
