<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import AdminBreadcrumb from '~/components/admin/AdminBreadcrumb.vue'
import AdminNavTree from '~/components/admin/AdminNavTree.vue'
import {
  ADMIN_NAV,
  ADMIN_NAV_TREE,
  activeGroupKeys,
  buildAdminBreadcrumb,
  buildAdminNavMenu,
  navItemByPath,
  reconcileExpandedGroups,
} from '~/utils/menu'
import { BRAND } from '~/utils/brand'
import {
  NAvatar,
  NButton,
  NDropdown,
  NLayout,
  NLayoutContent,
  NLayoutHeader,
  NLayoutSider,
  NSpace,
  useMessage,
} from 'naive-ui'

const route = useRoute()
const router = useRouter()
const config = useRuntimeConfig()
const auth = useAuthStore()
const message = useMessage()

const fallbackNav = ADMIN_NAV[0]!
// 高亮 / 标题的唯一来源是拍平视图：按精确 path 查激活项，子项命中后标题取子项名。
const routeMeta = computed(() => navItemByPath(route.path) ?? fallbackNav)
// 面包屑：祖先链与当前项都来自菜单拍平视图，深链接进入时与菜单高亮同源。
// 未知路由返回空数组，此时不渲染面包屑（不编造层级）。
const breadcrumbItems = computed(() => buildAdminBreadcrumb(route.path))
// 浏览器标签：具体页名由 app.vue 的 titleTemplate 统一补成「<页名> · JellyBin 管理后台」。
useHead({ title: computed(() => routeMeta.value.label) })

// 展开态：深链接到子项时含激活项的分组默认展开；同一路由下用户手动收起后保持收起。
const expandedKeys = ref<string[]>(activeGroupKeys(route.path))
watch(
  () => route.path,
  (to, from) => {
    expandedKeys.value = reconcileExpandedGroups(to, from, expandedKeys.value)
  },
)

function toggleGroup(key: string) {
  expandedKeys.value = expandedKeys.value.includes(key)
    ? expandedKeys.value.filter(item => item !== key)
    : [...expandedKeys.value, key]
}

const menuEntries = computed(() =>
  buildAdminNavMenu(ADMIN_NAV_TREE, route.path, expandedKeys.value),
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

      <AdminNavTree :entries="menuEntries" @toggle="toggleGroup" />
    </NLayoutSider>

    <NLayout>
      <NLayoutHeader class="admin-main pb-0">
        <header class="admin-header">
          <div>
            <AdminBreadcrumb v-if="breadcrumbItems.length > 0" :items="breadcrumbItems" />
            <p class="eyebrow">Admin Console</p>
            <h1 class="admin-page-title">{{ routeMeta.label }}</h1>
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
