<template>
  <NConfigProvider :theme-overrides="themeOverrides">
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

<script setup lang="ts">
import { NConfigProvider, NDialogProvider, NMessageProvider } from 'naive-ui'
import tokensCss from '~/assets/css/tokens.css?raw'
import { brandTitle } from '~/utils/brand'
import { buildNaiveThemeOverrides, parseRawPalette } from '~/utils/theme'

// 后台此前没有任何页面级头部信息。标题 / 语言 / 描述 / 图标 / OG 都在 nuxt.config
// 的 app.head 里（SPA 的骨架 HTML 也会带上静态 head）；这里只补标题的品牌后缀。
useHead(() => ({
  titleTemplate: (chunk?: string) => brandTitle(chunk),
}))

// Naive 主题覆盖：主色克莱因蓝 + 同色相派生明度、危险珊瑚橙、圆角 0、字体栈含中文
// fallback。取值全部来自 tokens.css（后台唯一颜色来源）——这里从 `?raw` 的 token
// 源码里解析真实色值，因为 Naive 的 changeColor/composite 不接受 CSS 变量。
const themeOverrides = buildNaiveThemeOverrides(parseRawPalette(tokensCss))
</script>
