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
import tokensCss from "~/assets/css/tokens.css?raw";
import { themeBodyClass, themeRootClass } from "~/utils/themeClass";
import { buildReaderThemeOverrides, parseReaderThemePalette } from "~/utils/readerTheme";
import {
  BRAND,
  brandAbsoluteAssetUrl,
  brandFullName,
  brandTitle,
} from "~/utils/brand";

const uiStore = useUiStore();

const theme = computed<GlobalTheme | null>(() => (uiStore.theme === "dark" ? darkTheme : null));

const runtimeConfig = useRuntimeConfig();
const ogImage = computed(() =>
  brandAbsoluteAssetUrl(String(runtimeConfig.public.siteUrl || ""), "og"),
);

useHead(() => ({
  titleTemplate: (chunk?: string) => brandTitle(chunk),
  meta: [
    { name: "description", content: BRAND.description },
    { property: "og:site_name", content: brandFullName() },
    { property: "og:type", content: "website" },
    { property: "og:title", content: brandFullName() },
    { property: "og:description", content: BRAND.description },
    { property: "og:image", content: ogImage.value },
    { property: "og:image:width", content: "1200" },
    { property: "og:image:height", content: "630" },
    { property: "og:image:alt", content: brandFullName() },
    { name: "twitter:card", content: "summary_large_image" },
    { name: "twitter:title", content: brandFullName() },
    { name: "twitter:description", content: BRAND.description },
    { name: "twitter:image", content: ogImage.value },
  ],
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
    class: themeBodyClass(uiStore.theme),
  },
  // 无 JS（或脚本被拦）时兜底：入场动画的 initial 会把元素内联成 opacity:0，
  // 这条 noscript 样式用 !important 盖过内联样式，保证内容仍然可读 ——
  // 博客内容不能因为动效在 JS 之前就凭空消失。
  noscript: [
    {
      innerHTML: "<style>.motion-stagger-item{opacity:1!important;transform:none!important}</style>",
    },
  ],
}));

// 主题覆盖：主色克莱因蓝 + 同色相派生明度 + 圆角 0 + 中文 fallback 字体栈。
// 取值来自 tokens.css（颜色只有一个来源）；只覆盖主色族/圆角/字体，中性纸面与
// 墨色交给 Naive 的 darkTheme 切换，否则静态浅色值会在暗色下变成白块。
const themeOverrides: GlobalThemeOverrides = buildReaderThemeOverrides(
  parseReaderThemePalette(tokensCss),
);
</script>

<template>
  <NConfigProvider :theme="theme" :theme-overrides="themeOverrides">
    <!-- 库级「减少动态」：reducedMotion="user" 跟随系统设置，退化为一次性淡入（不再位移/缩放）。 -->
    <MotionConfig reduced-motion="user">
      <NNotificationProvider>
        <NMessageProvider>
          <NDialogProvider>
            <NLoadingBarProvider>
              <div :class="['min-h-screen', themeRootClass(uiStore.theme)]">
                <NuxtLoadingIndicator color="var(--color-focus)" />
                <NuxtRouteAnnouncer />
                <NuxtLayout>
                  <NuxtPage />
                </NuxtLayout>
              </div>
            </NLoadingBarProvider>
          </NDialogProvider>
        </NMessageProvider>
      </NNotificationProvider>
    </MotionConfig>
  </NConfigProvider>
</template>
