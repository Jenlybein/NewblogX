<script setup lang="ts">
import { computed, h } from 'vue'
import {
  IconArticle,
  IconBell,
  IconChartBar,
  IconDatabase,
  IconFileSearch,
  IconHome,
  IconPhoto,
  IconSettings,
  IconShieldCheck,
  IconUsers,
} from '@tabler/icons-vue'
import { NuxtLink } from '#components'
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

const route = useRoute()
const router = useRouter()
const config = useRuntimeConfig()
const auth = useAuthStore()
const message = useMessage()

const navItems: NavItem[] = [
  { label: '数据概览', path: '/', icon: IconChartBar },
  { label: '文章审核', path: '/review', icon: IconShieldCheck },
  { label: '文章管理', path: '/articles', icon: IconArticle },
  { label: '用户管理', path: '/users', icon: IconUsers },
  { label: '站点配置', path: '/site', icon: IconSettings },
  { label: '媒体运营', path: '/media', icon: IconPhoto },
  { label: '全局通知', path: '/notifications', icon: IconBell },
  { label: '日志中心', path: '/logs', icon: IconDatabase },
]

const fallbackNav = navItems[0]!
const routeMeta = computed<NavItem>(() => navItems.find(item => item.path === route.path) ?? fallbackNav)
const selectedKey = computed(() => routeMeta.value.path)

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
        <div class="admin-brand__mark">BX</div>
        <div class="min-w-0">
          <div class="admin-brand__name">BlogX Admin</div>
          <p class="m-0 truncate text-xs muted">内容运营与系统管理</p>
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
