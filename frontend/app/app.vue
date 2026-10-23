<script setup lang="ts">
import { computed } from "vue";
import {
  darkTheme,
  NConfigProvider,
  NDialogProvider,
  NLoadingBarProvider,
  NMessageProvider,
  NNotificationProvider,
  type GlobalTheme,
  type GlobalThemeOverrides,
} from "naive-ui";
import sora400Woff2 from "@fontsource/sora/files/sora-latin-400-normal.woff2";
import sora600Woff2 from "@fontsource/sora/files/sora-latin-600-normal.woff2";
import sora700Woff2 from "@fontsource/sora/files/sora-latin-700-normal.woff2";

const uiStore = useUiStore();

const theme = computed<GlobalTheme | null>(() => (uiStore.theme === "dark" ? darkTheme : null));

useHead(() => ({
  link: [
    {
      rel: "preload",
      as: "font",
      type: "font/woff2",
      href: sora400Woff2,
      crossorigin: "anonymous",
    },
    {
      rel: "preload",
      as: "font",
      type: "font/woff2",
      href: sora600Woff2,
      crossorigin: "anonymous",
    },
    {
      rel: "preload",
      as: "font",
      type: "font/woff2",
      href: sora700Woff2,
      crossorigin: "anonymous",
    },
  ],
  bodyAttrs: {
    class: uiStore.theme === "dark" ? "theme-dark dark" : "theme-light",
  },
}));

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: "#0f766e",
    primaryColorHover: "#115e59",
    primaryColorPressed: "#0b5b55",
    borderRadius: "18px",
    borderRadiusSmall: "12px",
    fontFamily: '"Sora", "Sora Fallback", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans SC", sans-serif',
    fontFamilyMono: '"Cascadia Code", "JetBrains Mono", monospace',
  },
  Card: {
    borderRadius: "26px",
  },
  Button: {
    borderRadiusSmall: "14px",
    borderRadiusMedium: "16px",
  },
  Input: {
    borderRadius: "16px",
  },
};
</script>

<template>
  <NConfigProvider :theme="theme" :theme-overrides="themeOverrides">
    <NNotificationProvider>
      <NMessageProvider>
        <NDialogProvider>
          <NLoadingBarProvider>
            <div :class="['min-h-screen', uiStore.theme === 'dark' ? 'theme-dark' : 'theme-light']">
              <NuxtLoadingIndicator color="#0f766e" />
              <NuxtRouteAnnouncer />
              <NuxtLayout>
                <NuxtPage />
              </NuxtLayout>
            </div>
          </NLoadingBarProvider>
        </NDialogProvider>
      </NMessageProvider>
    </NNotificationProvider>
  </NConfigProvider>
</template>
