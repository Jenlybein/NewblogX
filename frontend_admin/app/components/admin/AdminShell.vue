<script setup lang="ts">
import { computed, h } from 'vue'
import {
  IconAlertTriangle,
  IconArticle,
  IconBell,
  IconChartBar,
  IconDatabase,
  IconPhoto,
  IconPin,
  IconSettings,
  IconShieldCheck,
  IconUsers,
} from '@tabler/icons-vue'
import { NuxtLink } from '#components'
import type { IconHome } from '@tabler/icons-vue'
import { ADMIN_NAV, navItemByPath } from '~/utils/menu'
import type { AdminNavIcon } from '~/utils/menu'
import { BRAND } from '~/utils/brand'
import {
  NAvatar,
  NButton,
  NDropdown,
  NIcon,
  NLayout,
  NLayoutContent,
  NLayoutHeader,
  NLayoutSider,
  NMenu,
  NSpace,
  useMessage,
} from 'naive-ui'
import type { MenuOption } from 'naive-ui'

interface NavItem {
  label: string
  path: string
  icon: typeof IconHome
}

// 图标在这里映射，清单在 ~/utils/menu.ts（唯一来源，另有单测对账页面文件）。
const ICONS: Record<AdminNavIcon, typeof IconHome> = {
  chart: IconChartBar,
  shield: IconShieldCheck,
  article: IconArticle,
  pin: IconPin,
  users: IconUsers,
  settings: IconSettings,
  photo: IconPhoto,
  bell: IconBell,
  database: IconDatabase,
  alert: IconAlertTriangle,
}

const navItems: NavItem[] = ADMIN_NAV.map(item => ({
  label: item.label,
  path: item.path,
  icon: ICONS[item.icon],
}))

const route = useRoute()
const router = useRouter()
const config = useRuntimeConfig()
const auth = useAuthStore()
const message = useMessage()

const fallbackNav = navItems[0]!
// 高亮 / 标题的唯一来源是拍平视图（#106）：当前仍按精确 path 匹配，行为不变；
// 两级化后只改 navItemByPath 的匹配语义，不用再动这里。
const routeMeta = computed<NavItem>(() => {
  const active = navItemByPath(route.path)
  return active?.path
    ? { label: active.label, path: active.path, icon: ICONS[active.icon] }
    : fallbackNav
})
const selectedKey = computed(() => routeMeta.value.path)
// 浏览器标签：具体页名由 app.vue 的 titleTemplate 统一补成「<页名> · JellyBin 管理后台」。
useHead({ title: computed(() => routeMeta.value.label) })

const menuOptions = computed<MenuOption[]>(() =>
  navItems.map(item => ({
    label: () =>
      h(
        NuxtLink,
        {
          to: item.path,
          class: 'menu-link',
        },
        { default: () => item.label },
      ),
    key: item.path,
    icon: () => h(NIcon, null, { default: () => h(item.icon) }),
  })),
)

const userOptions = [
  { label: '刷新资料', key: 'refresh' },
  { label: '打开 Web 前台', key: 'web' },
  { label: '退出登录', key: 'logout' },
]

async function handleUserCommand(key: string) {
  if (key === 'refresh') {
    await auth.fetchCurrentUser()
    message.success('资料已刷新')
    return
  }

  if (key === 'web') {
    window.open(String(config.public.webSiteUrl || 'http://localhost:3000'), '_blank', 'noopener,noreferrer')
    return
  }

  if (key === 'logout') {
    await auth.logout()
    message.success('已退出后台')
    await router.push('/login')
  }
}
</script>

<template>
  <NLayout has-sider class="admin-shell">
    <NLayoutSider :width="276" bordered collapse-mode="width" :native-scrollbar="false" class="admin-sider">
      <div class="admin-brand">
        <AdminBrandLogo :size="44" />
        <div class="min-w-0">
          <AdminBrandWordmark size="sm" />
          <p class="m-0 truncate text-xs muted">{{ BRAND.tagline }}</p>
        </div>
      </div>

      <NMenu :value="selectedKey" :options="menuOptions" />
    </NLayoutSider>

    <NLayout>
      <NLayoutHeader class="admin-main pb-0">
        <header class="admin-header">
          <div>
            <p class="eyebrow">Admin Console</p>
            <h1 class="admin-page-title">{{ routeMeta.label }}</h1>
            <p class="admin-page-subtitle m-0 mt-2">基于 OpenAPI 契约接入后台管理能力。</p>
          </div>

          <NSpace align="center">
            <AdminThemeToggle />
            <NButton secondary @click="router.go(0)">刷新</NButton>
            <NDropdown :options="userOptions" trigger="click" @select="handleUserCommand">
              <button class="admin-user-pill" type="button">
                <NAvatar round size="small" :src="auth.profileAvatar">
                  {{ auth.profileName.slice(0, 1) }}
                </NAvatar>
                <span>{{ auth.profileName }}</span>
              </button>
            </NDropdown>
          </NSpace>
        </header>
      </NLayoutHeader>

      <NLayoutContent class="admin-main">
        <slot />
      </NLayoutContent>
    </NLayout>
  </NLayout>
</template>
