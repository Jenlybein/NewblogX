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
import type { GlobalThemeOverrides } from 'naive-ui'
import { brandTitle } from '~/utils/brand'

// 后台此前没有任何页面级头部信息。标题 / 语言 / 描述 / 图标 / OG 都在 nuxt.config
// 的 app.head 里（SPA 的骨架 HTML 也会带上静态 head）；这里只补标题的品牌后缀。
useHead(() => ({
  titleTemplate: (chunk?: string) => brandTitle(chunk),
}))

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#0f766e',
    primaryColorHover: '#115e59',
    primaryColorPressed: '#134e4a',
    borderRadius: '8px',
    fontFamily:
      '"Sora", "Sora Fallback", "Noto Sans SC", "Noto Sans SC Fallback", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
  },
  Button: {
    borderRadiusMedium: '8px',
    borderRadiusLarge: '8px',
  },
  Card: {
    borderRadius: '8px',
  },
  Input: {
    borderRadius: '8px',
  },
  Select: {
    peers: {
      InternalSelection: {
        borderRadius: '8px',
      },
    },
  },
}
</script>
