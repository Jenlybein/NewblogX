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
import interWoff2 from "@fontsource-variable/inter/files/inter-latin-wght-normal.woff2";
import tokensCss from "~/assets/css/tokens.css?raw";
import { themeBodyClass, themeHtmlClass, themeRootClass } from "~/utils/themeClass";
import { buildThemeInitScript } from "~/utils/themePreference";
import { buildReaderThemeOverrides, parseReaderThemePalette, parseReaderThemeScale } from "~/utils/readerTheme";
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
    {
      // 正文/UI 的 Inter 是可变的：一条 woff2 覆盖 100–900，首屏只需预加载这一条。
      rel: "preload",
      as: "font",
      type: "font/woff2",
      href: interWoff2,
      crossorigin: "anonymous",
    },
  ],
  // html 与 body 同一组类：首屏脚本在水合前先钉一次，水合后这里跟着 store 更新，
  // 保证用户切换后 html 上的旧类不会残留（否则 CSS 变量会一直停在暗/浅）。
  htmlAttrs: {
    class: themeHtmlClass(uiStore.theme),
  },
  bodyAttrs: {
    class: themeBodyClass(uiStore.theme),
  },
  // 首屏前置脚本：SSR 只认得 cookie、看不到系统偏好。没有 cookie 的新访客若系统是暗色，
  // 服务端会先渲染浅色、水合后才变暗（闪白）。这段脚本在水合前把根类钉成最终值，
  // 颜色由 CSS 变量瞬间生效，不改字号/间距。
  script: [{ key: "jelly-theme-init", innerHTML: buildThemeInitScript() }],
  // 无 JS（或脚本被拦）时兜底：入场动画的 initial 会把元素内联成 opacity:0，
  // 这条 noscript 样式用 !important 盖过内联样式，保证内容仍然可读 ——
  // 博客内容不能因为动效在 JS 之前就凭空消失。
  noscript: [
    {
      innerHTML: "<style>.motion-stagger-item{opacity:1!important;transform:none!important}</style>",
    },
  ],
}));

// 主题覆盖：主色墨色（CTA 实心胶囊）+ info 功能蓝 + 圆角阶梯 + 中文 fallback 字体栈。
// 取值来自 tokens.css（颜色只有一个来源）；明暗各取同一组语义 token的第二份取值，
// 随当前主题切换（否则静态浅色值会在暗色下变成白块）。
const readerThemeScale = parseReaderThemeScale(tokensCss);
const themeOverrides = computed<GlobalThemeOverrides>(() =>
  buildReaderThemeOverrides(parseReaderThemePalette(tokensCss, uiStore.theme), readerThemeScale),
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
                <NuxtLoadingIndicator color="var(--color-ink)" />
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
