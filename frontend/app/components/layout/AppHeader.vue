<script setup lang="ts">
import { computed, h, ref, watch } from "vue";
import {
  IconHome2,
  IconLogin2,
  IconPencilPlus,
  IconMoonStars,
  IconSearch,
  IconSunHigh,
  IconUserCircle,
} from "@tabler/icons-vue";
import { NButton, NDropdown, NInput, NSkeleton } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const siteStore = useSiteStore();
const uiStore = useUiStore();
const { openWriteEntry } = useWriteEntry();

const searchKeyword = ref("");
const authActionWidthClass = "w-[132px] md:w-[148px]";

watch(
  () => route.query.key,
  (value) => {
    searchKeyword.value = typeof value === "string" ? value : "";
  },
  { immediate: true },
);

const userMenuOptions = computed(() => [
  {
    label: "个人中心",
    key: "studio",
    icon: () => h(IconHome2, { size: 16 }),
  },
  {
    label: "我的主页",
    key: "profile",
    icon: () => h(IconUserCircle, { size: 16 }),
  },
  {
    label: "退出登录",
    key: "logout",
    icon: () => h(IconLogin2, { size: 16 }),
  },
]);

async function submitSearch() {
  await router.push({
    path: "/search",
    query: searchKeyword.value ? { key: searchKeyword.value } : {},
  });
}

async function handleUserMenuSelect(key: string | number) {
  if (key === "studio") {
    await navigateTo("/studio/dashboard");
    return;
  }

  if (key === "profile" && authStore.profileId) {
    await navigateTo(`/users/${authStore.profileId}`);
    return;
  }

  if (key === "logout") {
    await authStore.logout();
  }
}
</script>

<template>
  <header class="page-shell pb-0">
    <div class="surface-card surface-card--strong flex flex-wrap items-center gap-4 px-5 py-4 md:px-6">
      <NuxtLink to="/" class="flex items-center gap-3">
        <div class="flex h-12 w-12 items-center justify-center rounded-2xl bg-teal-800 text-white shadow-lg">
          <span class="text-lg font-bold">BX</span>
        </div>
        <div class="min-w-0">
          <div class="truncate text-lg font-semibold">{{ siteStore.seo?.project_title || "BlogX" }}</div>
          <div class="truncate text-sm muted">
            {{ siteStore.runtimeConfig?.project?.abstract || "为开发者写作而生的内容平台" }}
          </div>
        </div>
      </NuxtLink>

      <nav class="ml-2 hidden items-center gap-2 md:flex">
        <NuxtLink to="/" class="soft-tab" :class="{ 'is-active': route.path === '/' }">
          <IconHome2 :size="16" />
          <span>首页</span>
        </NuxtLink>
        <NuxtLink to="/search" class="soft-tab" :class="{ 'is-active': route.path.startsWith('/search') }">
          <IconSearch :size="16" />
          <span>搜索</span>
        </NuxtLink>
        <button
          type="button"
          class="soft-tab"
          :class="{ 'is-active': route.path.startsWith('/studio/write') }"
          @click="openWriteEntry()">
          <IconPencilPlus :size="16" />
          <span>创作</span>
        </button>
      </nav>

      <div class="ml-auto flex w-full items-center justify-end gap-3 md:w-auto">
        <div class="w-full max-w-[420px]">
          <NInput
            v-model:value="searchKeyword"
            round
            clearable
            name="site-search"
            autocomplete="off"
            aria-label="站点搜索"
            placeholder="搜索文章、标签、作者…"
            @keydown.enter.prevent="submitSearch"
          >
            <template #suffix>
              <NButton text type="primary" @click="submitSearch">
                <IconSearch :size="18" />
              </NButton>
            </template>
          </NInput>
        </div>

        <ClientOnly>
          <template #fallback>
            <div :class="['flex shrink-0 justify-end', authActionWidthClass]">
              <NSkeleton round height="42px" width="100%" />
            </div>
          </template>

          <div :class="['flex shrink-0 justify-end', authActionWidthClass]">
            <NSkeleton
              v-if="authStore.isLoggedIn && (!authStore.initialized || !authStore.currentUser)"
              round
              height="42px"
              width="100%"
            />

            <NDropdown v-else-if="authStore.isLoggedIn" :options="userMenuOptions" @select="handleUserMenuSelect">
              <button
                type="button"
                aria-label="打开个人菜单"
                class="inline-flex w-full min-w-0 flex-nowrap items-center justify-center gap-2 rounded-full border border-slate-200/80 bg-white/70 px-3 py-2 text-sm font-medium text-slate-700 transition hover:border-teal-200 hover:text-teal-700 dark:border-slate-700 dark:bg-slate-900/70 dark:text-slate-100"
              >
                <AppAvatar
                  :key="authStore.profileAvatar || authStore.profileName"
                  class="shrink-0"
                  :size="30"
                  :src="authStore.profileAvatar"
                  :name="authStore.profileName"
                  fallback="我" />
                <span class="hidden min-w-0 max-w-[96px] truncate whitespace-nowrap leading-none md:inline">
                  {{ authStore.profileName }}
                </span>
              </button>
            </NDropdown>

            <NButton v-else quaternary class="w-full" @click="uiStore.openAuthModal()">登录 / 注册</NButton>
          </div>
        </ClientOnly>

        <NButton quaternary circle aria-label="切换明暗主题" @click="uiStore.toggleTheme()">
          <template #icon>
            <component :is="uiStore.theme === 'dark' ? IconSunHigh : IconMoonStars" :size="18" />
          </template>
        </NButton>
      </div>
    </div>
  </header>
</template>
