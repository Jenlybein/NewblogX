<script setup lang="ts">
import { computed, h, ref, watch } from "vue";
import {
  IconHome2,
  IconLogin2,
  IconMenu2,
  IconMoonStars,
  IconSearch,
  IconSunHigh,
  IconUserCircle,
  IconX,
} from "@tabler/icons-vue";
import { NDropdown, NSkeleton } from "naive-ui";
import { navigateTo, useRoute, useRouter } from "#imports";
import AppAvatar from "~/components/common/AppAvatar.vue";
import BrandLogo from "~/components/brand/Logo.vue";
import BrandWordmark from "~/components/brand/Wordmark.vue";
import { useArticleWritePolicy } from "~/composables/useArticleWritePolicy";
import { usePointerSpecular } from "~/composables/usePointerSpecular";
import { useScrollChrome } from "~/composables/useScrollChrome";
import { useWriteEntry } from "~/composables/useWriteEntry";
import { useAuthStore } from "~/stores/auth";
import { useUiStore } from "~/stores/ui";
import { BRAND } from "~/utils/brand";
import {
  buildSearchLocation,
  headerNavItems,
  mobileMenuLabel,
  searchKeywordFromQuery,
  themeToggleLabel,
} from "~/utils/chrome";
import { headerNavIcon } from "~/utils/navIcon";

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const uiStore = useUiStore();
const { openWriteEntry } = useWriteEntry();
// 站点「发布与展示」开关：发布关闭时「写文章」入口不出现（ADR-0017 决策 2）。
// 服务端判定仍是唯一边界，这里只保证界面不说谎。
const { entryVisible } = useArticleWritePolicy();

const searchKeyword = ref("");
const menuOpen = ref(false);

// 吸顶玻璃导航：滚动方向/阈值判定与指针镜面高光都收在 composable 里（含降级）。
// 「减少动态」下 useScrollChrome 直接不启用 → 页头保持静止的不透明面、不隐藏。
const { scrolled, hidden } = useScrollChrome();
const { handlePointerMove } = usePointerSpecular();

const navItems = computed(() =>
  headerNavItems(route.path).filter((item) => item.key !== "write" || entryVisible.value),
);
const themeLabel = computed(() => themeToggleLabel(uiStore.theme));
const menuLabel = computed(() => mobileMenuLabel(menuOpen.value));

watch(
  () => route.query.key,
  (value) => {
    searchKeyword.value = searchKeywordFromQuery(value);
  },
  { immediate: true },
);

// 换页后自动收起手机端面板：否则跳转过去还盖着一层菜单。
watch(
  () => route.fullPath,
  () => {
    menuOpen.value = false;
  },
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
  await router.push(buildSearchLocation(searchKeyword.value));
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
  <header
    class="site-header"
    :class="{ 'is-scrolled': scrolled, 'is-hidden': hidden, 'glass-thin': scrolled }"
    @pointermove="handlePointerMove"
  >
    <a class="site-skip" href="#main-content" data-chrome="skip-link">跳到正文</a>

    <div class="page-shell site-header__inner">
      <div class="site-header__brand">
        <NuxtLink to="/" class="site-brand" :aria-label="`${BRAND.name.en}（${BRAND.name.zh}）首页`">
          <!-- 品牌资产不随主题切换：亮暗两态共用同一张透底 PNG（ADR-0012 约束 12） -->
          <BrandLogo :size="40" decorative />
          <BrandWordmark size="md" />
        </NuxtLink>
      </div>

      <!-- 手机端折叠、平板/桌面常显：单一 DOM 靠 CSS 断点切换，避免复制一份控件 -->
      <div id="site-header-panel" class="site-header__panel" :class="{ 'is-open': menuOpen }">
        <nav class="site-nav" aria-label="主导航">
          <template v-for="item in navItems" :key="item.key">
            <NuxtLink
              v-if="item.key !== 'write'"
              :to="item.to"
              class="site-tab"
              :class="{ 'is-active': item.active }"
              :aria-current="item.active ? 'page' : undefined"
            >
              <!-- 图标走 utils/navIcon 的显式映射：旧的三元选择会让新入口静默拿到搜索图标 -->
              <component :is="headerNavIcon(item.key)" :size="16" />
              <span>{{ item.label }}</span>
            </NuxtLink>
            <button
              v-else
              type="button"
              class="site-tab"
              :class="{ 'is-active': item.active }"
              @click="openWriteEntry()"
            >
              <component :is="headerNavIcon(item.key)" :size="16" />
              <span>{{ item.label }}</span>
            </button>
          </template>
        </nav>

        <form class="site-search" role="search" aria-label="站点搜索" @submit.prevent="submitSearch">
          <label class="sr-only" for="site-search-input">搜索文章、标签、作者</label>
          <input
            id="site-search-input"
            v-model="searchKeyword"
            class="site-search__input"
            type="search"
            name="site-search"
            autocomplete="off"
            placeholder="搜索文章、标签、作者…"
          />
          <button type="submit" class="site-search__submit" aria-label="提交搜索">
            <IconSearch :size="18" />
          </button>
        </form>
      </div>

      <div class="site-header__actions">
        <button
          type="button"
          class="site-action"
          :aria-label="themeLabel"
          :title="themeLabel"
          @click="uiStore.toggleTheme()"
        >
          <component :is="uiStore.theme === 'dark' ? IconSunHigh : IconMoonStars" :size="18" />
        </button>

        <div data-chrome="account" class="site-header__account">
          <ClientOnly>
            <template #fallback>
              <NSkeleton round height="32px" width="96px" />
            </template>

            <NSkeleton
              v-if="authStore.isLoggedIn && (!authStore.initialized || !authStore.currentUser)"
              round
              height="32px"
              width="120px"
            />

            <NDropdown
              v-else-if="authStore.isLoggedIn"
              :options="userMenuOptions"
              @select="handleUserMenuSelect"
            >
              <button type="button" aria-label="打开账户菜单" class="site-action site-action--account">
                <AppAvatar
                  :key="authStore.profileAvatar || authStore.profileName"
                  :size="26"
                  :src="authStore.profileAvatar"
                  :name="authStore.profileName"
                  fallback="我"
                />
                <span class="site-header__account-name">{{ authStore.profileName }}</span>
              </button>
            </NDropdown>

            <button
              v-else
              type="button"
              class="site-action site-action--text"
              @click="uiStore.openAuthModal()"
            >
              登录 / 注册
            </button>
          </ClientOnly>
        </div>

        <button
          type="button"
          class="site-action site-header__menu"
          data-chrome="menu"
          :aria-expanded="menuOpen"
          aria-controls="site-header-panel"
          :aria-label="menuLabel"
          @click="menuOpen = !menuOpen"
        >
          <component :is="menuOpen ? IconX : IconMenu2" :size="18" />
        </button>
      </div>
    </div>
  </header>
</template>

<style scoped>
/*
 * 吸顶玻璃导航（ADR-0012 决策 6）：sticky + 高度 56–64px（--site-header-height 60px）。
 * 静止态是不透明纸面；滚动超过阈值后由 .glass-thin 接管材料（模糊 + 边缘带 + 噪点），
 * 并补一条底部 1px 边缘高光。滚动方向感知（下滚隐藏 / 上滚出现）由 useScrollChrome
 * 的 state → class 映射，位移只走 transform，不碰布局。
 * 「减少动态」下 composable 不启用 → 页头不转玻璃、不隐藏。
 * 布局策略（三档）不变：
 *   < 640px  手机：品牌 + 动作一排；导航/搜索折叠进面板（菜单按钮展开）
 *   640–1023 平板：品牌 + 动作一排；导航/搜索第二行常显
 *   ≥ 1024px 桌面：品牌 / 导航+搜索 / 动作各占一列的单行
 */
.site-header {
  position: sticky;
  top: 0;
  z-index: 30;
  min-height: var(--site-header-height);
  transition: transform var(--motion-base) var(--motion-ease);
}

/* 静止态（未滚动）才用不透明面 + 发丝结构线：滚动后材料交给 .glass-thin，
 * 两套背景不会互相覆盖。 */
.site-header:not(.is-scrolled) {
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-line-strong);
}

/* 转玻璃后：底部 1px 边缘高光（叠在玻璃自己的抬升阴影上）。 */
.site-header.is-scrolled {
  box-shadow:
    0 18px 44px -20px var(--color-shadow),
    0 1px 0 var(--glass-rim);
}

/* 下滚隐藏 / 上滚出现：只动 transform（不给 sticky 带来任何布局变化）。 */
.site-header.is-hidden {
  transform: translateY(-100%);
  transition: transform var(--motion-base) var(--motion-ease);
}

/* 减少动态：导航的隐藏/出现也不做过渡（全局 reduced-motion 已把时长归零，
 * 这里再兜一层，避免"归零后仍有一次 transform 跳变"）。 */
@media (prefers-reduced-motion: reduce) {
  .site-header,
  .site-header.is-hidden {
    transition: none;
  }
}

.site-header__inner {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-areas:
    "brand actions"
    "panel panel";
  align-items: center;
  gap: 0.5rem 1rem;
  padding-top: 0.5rem;
  padding-bottom: 0.5rem;
}

.site-header__brand {
  grid-area: brand;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  min-width: 0;
}

.site-brand {
  display: inline-flex;
  align-items: center;
  gap: 0.625rem;
  min-height: 2.75rem;
  min-width: 0;
  color: var(--color-ink);
  text-decoration: none;
}

/* 页头 = 语义纸面 + 发丝线；形状语义（黄三角）已随 #20 退役。
 * logo 旁的介绍小字按 ADR-0013 决策 6 删除：主旨句不再重复三处。 */
.site-header__panel {
  grid-area: panel;
  display: none;
  min-width: 0;
}

.site-header__panel.is-open {
  display: flex;
  flex-direction: column;
  gap: 0.625rem;
  padding: 0.625rem 0 0.75rem;
  border-top: 1px solid var(--color-line-faint);
}

.site-nav {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.site-tab {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  min-height: 2.75rem;
  padding: 0 0.75rem;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-ink-muted);
  font: inherit;
  font-size: 0.8125rem;
  letter-spacing: 0.04em;
  white-space: nowrap;
  cursor: pointer;
  transition: color var(--motion-fast) var(--motion-ease);
}

.site-tab:hover {
  color: var(--color-ink);
}

/* active 用 inset 阴影而不是改边框/下划线高度：不改变盒模型 → 切页不抖布局 */
.site-tab.is-active {
  color: var(--color-ink);
  box-shadow: inset 0 -2px 0 var(--color-line-strong);
}

.site-search {
  display: flex;
  align-items: stretch;
  width: 100%;
  border: 1px solid var(--color-line-strong);
  border-radius: var(--radius-md);
  overflow: hidden;
  background: var(--color-surface);
}

.site-search__input {
  flex: 1 1 auto;
  min-width: 0;
  height: 2.75rem;
  padding: 0 0.75rem;
  border: 0;
  background: transparent;
  color: var(--color-ink);
  font: inherit;
  font-size: 0.875rem;
}

.site-search__input::placeholder {
  color: var(--color-ink-muted);
}

.site-search__submit {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.75rem;
  border: 0;
  border-left: 1px solid var(--color-line);
  background: transparent;
  color: var(--color-ink);
  cursor: pointer;
  transition: color var(--motion-fast) var(--motion-ease);
}

.site-search__submit:hover {
  color: var(--color-ink-muted);
}

.site-header__actions {
  grid-area: actions;
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.site-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
  min-width: 2.75rem;
  min-height: 2.75rem;
  padding: 0 0.5rem;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-ink);
  font: inherit;
  font-size: 0.8125rem;
  cursor: pointer;
  transition:
    border-color var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease);
}

.site-action:hover {
  border-color: var(--color-line-strong);
}

.site-action--text {
  padding: 0 0.875rem;
}

.site-action--account {
  padding: 0 0.375rem;
}

.site-header__account-name {
  display: none;
  max-width: 6rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 焦点环统一：功能蓝只用于焦点环与文字链接，暗色下自动换成暗色功能蓝。 */
.site-header :is(a, button, input, [tabindex]):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

/* skip link：平时移出视口，聚焦时出现在左上角（键盘用户不必 Tab 过整个页头） */
.site-skip {
  position: absolute;
  top: 0.25rem;
  left: 0.5rem;
  z-index: 2;
  padding: 0.5rem 0.75rem;
  border-radius: var(--radius-sm);
  background: var(--color-surface-raised);
  border: 1px solid var(--color-line-strong);
  color: var(--color-ink);
  font-size: 0.8125rem;
  transform: translateY(-200%);
}

.site-skip:focus-visible {
  transform: translateY(0);
}

@media (min-width: 640px) {
  .site-header__panel,
  .site-header__panel.is-open {
    display: flex;
    flex-direction: row;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.75rem 1.5rem;
    padding: 0;
    border-top: 0;
  }

  .site-search {
    flex: 1 1 16rem;
    max-width: 26rem;
  }

  /* 平板起导航/搜索常显，折叠按钮退场 */
  .site-header__menu {
    display: none;
  }
}

@media (min-width: 1024px) {
  .site-header__inner {
    grid-template-columns: auto minmax(0, 1fr) auto;
    grid-template-areas: "brand panel actions";
    gap: 2rem;
    padding-top: 0.5rem;
    padding-bottom: 0.5rem;
  }

  .site-header__panel {
    flex-wrap: nowrap;
    gap: 2rem;
  }

  .site-header__account-name {
    display: inline;
  }
}
</style>

