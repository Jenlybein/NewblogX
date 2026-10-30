<script setup lang="ts">
import { computed } from "vue";
import {
  darkTheme,
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  type GlobalTheme,
  type GlobalThemeOverrides,
} from "naive-ui";
import interWoff2 from "@fontsource-variable/inter/files/inter-latin-wght-normal.woff2";
import tokensCss from "~/assets/css/tokens.css?raw";
import { brandTitle } from "~/utils/brand";
import { buildNaiveThemeOverrides, parseThemePalette, parseThemeScale, parseThemeTriplets } from "~/utils/theme";
import { buildThemeInitScript, themeBodyClass, themeHtmlClass } from "~/utils/themePreference";

// 后台此前没有任何页面级头部信息。标题 / 语言 / 描述 / 图标 / OG 都在 nuxt.config
// 的 app.head 里（SPA 的骨架 HTML 也会带上静态 head）；这里只补标题的品牌后缀。
const themeStore = useThemeStore();

useHead(() => ({
  titleTemplate: (chunk?: string) => brandTitle(chunk),
  link: [
    {
      // 正文/UI 的 Inter 是可变的：一条 woff2 覆盖 100–900；后台没有 Sora 800 预加载。
      rel: "preload",
      as: "font",
      type: "font/woff2",
      href: interWoff2,
      crossorigin: "anonymous",
    },
  ],
  // html 与 body 同一组根类：首屏脚本在水合前先钉一次，水合后这里跟着 store 更新，
  // 保证用户切换后旧类不会残留（否则 CSS 变量会一直停在暗/浅）。
  htmlAttrs: { class: themeHtmlClass(themeStore.theme) },
  bodyAttrs: { class: themeBodyClass(themeStore.theme) },
  // 首屏前置脚本：SSR 只认得 cookie、看不到系统偏好；没有 cookie 的新访客若系统是
  // 暗色，会先渲染浅色再在水合后变暗（闪白）。这段脚本在水合前把根类钉成最终值。
  script: [{ key: "jelly-admin-theme-init", innerHTML: buildThemeInitScript() }],
}));

// Naive 基础主题（明暗那一层由 Naive 自己提供）与 token 覆盖：
// 覆盖对象从 tokens.css 的语义层解析（颜色只有一个来源），暗色读 `.dark` 块的第二份取值。
const naiveTheme = computed<GlobalTheme | null>(() => (themeStore.theme === "dark" ? darkTheme : null));
const themeScale = parseThemeScale(tokensCss);
const themeOverrides = computed<GlobalThemeOverrides>(() =>
  buildNaiveThemeOverrides(
    parseThemePalette(tokensCss, themeStore.theme),
    parseThemeTriplets(tokensCss, themeStore.theme),
    themeScale,
  ),
);
</script>

<template>
  <NConfigProvider :theme="naiveTheme" :theme-overrides="themeOverrides">
    <NMessageProvider>
      <NDialogProvider>
        <NuxtRouteAnnouncer />
        <NuxtLayout>
          <NuxtPage />
        </NuxtLayout>
      </NDialogProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>
