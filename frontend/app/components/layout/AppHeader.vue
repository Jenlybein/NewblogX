<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import {
  IconBell,
  IconClockHour4,
  IconMenu2,
  IconMoonStars,
  IconSearch,
  IconSunHigh,
  IconX,
} from "@tabler/icons-vue";
import { NDropdown, NSkeleton } from "naive-ui";
import { useRoute, useRouter } from "#imports";
import AppAvatar from "~/components/common/AppAvatar.vue";
import BrandLogo from "~/components/brand/Logo.vue";
import BrandWordmark from "~/components/brand/Wordmark.vue";
import { useArticleWritePolicy } from "~/composables/useArticleWritePolicy";
import { useHeaderUserMenu } from "~/composables/useHeaderUserMenu";
import { useHeaderMessagesPanel } from "~/composables/useHeaderMessagesPanel";
import { usePointerSpecular } from "~/composables/usePointerSpecular";
import { useScrollChrome } from "~/composables/useScrollChrome";
import { useWriteEntry } from "~/composables/useWriteEntry";
import { useAuthStore } from "~/stores/auth";
import { useMessageStore } from "~/stores/message";
import { useUiStore } from "~/stores/ui";
import { BRAND } from "~/utils/brand";
import {
  buildSearchLocation,
  headerNavItems,
  mobileMenuLabel,
  searchKeywordFromQuery,
  themeToggleLabel,
} from "~/utils/chrome";
import { messageCenterNavItems, shouldShowUnreadCount } from "~/utils/messageCenter";
import { headerNavIcon } from "~/utils/navIcon";

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const uiStore = useUiStore();
const messageStore = useMessageStore();
const { openWriteEntry } = useWriteEntry();
// 站点「发布与展示」开关：发布关闭时「写文章」入口不出现（ADR-0017 决策 2）。
// 服务端判定仍是唯一边界，这里只保证界面不说谎。
const { entryVisible } = useArticleWritePolicy();

const searchKeyword = ref("");
const menuOpen = ref(false);

// 面板状态外移到 composable：200ms 宽限 + 透明桥接带（页头已撞 800 行上限）。
const panel = useHeaderMessagesPanel();
// ref 必须顶层解构：模板里的 `panel.messagesOpen` 是 Ref 对象（恒 truthy），
// 既会让 aria-expanded 拿到对象、也会让 v-if 永远成立。
const { messagesOpen, messagesTriggerRef } = panel;
const messageNavItems = computed(() => messageCenterNavItems(messageStore.summary));
const totalUnread = computed(() => messageStore.totalUnread);

// 吸顶玻璃导航：滚动方向/阈值判定与指针镜面高光都收在 composable 里（含降级）。
// 「减少动态」下 useScrollChrome 直接不启用 → 页头保持静止的不透明面、不隐藏。
const { scrolled, hidden } = useScrollChrome();
const { handlePointerMove } = usePointerSpecular();

// 「创作」仍在导航数据里（活动判定与路由契约依赖它），但渲染点已移到右侧动作区
// （第 198 票）：导航条只渲染纯链接，模板里不再有条件分支。
const navItems = computed(() => headerNavItems(route.path).filter((item) => item.key !== "write"));
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
    panel.closeMessages();
  },
);

// 历史入口的三档落位与账号菜单项外移到 composable（页头已撞 800 行上限）。
const { userMenuOptions, handleUserMenuSelect } = useHeaderUserMenu();

// 登录后刷新一次未读汇总：页头是读者站唯一常驻的未读消费点，
// 不能继续只靠 studio 布局兜底（消息中心已迁出个人中心）。
// onMounted 只在客户端执行，SSR 的首页 / 文章页不会带着登录身份发这个请求。
onMounted(() => {
  if (authStore.isLoggedIn && !messageStore.fetched) {
    void messageStore.refreshSummary().catch(() => undefined);
  }
});

async function submitSearch() {
  await router.push(buildSearchLocation(searchKeyword.value));
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
          <NuxtLink
            v-for="item in navItems"
            :key="item.key"
            :to="item.to"
            class="site-tab"
            :class="{ 'is-active': item.active }"
            :aria-current="item.active ? 'page' : undefined"
          >
            <!-- 图标走 utils/navIcon 的显式映射：旧的三元选择会让新入口静默拿到搜索图标 -->
            <component :is="headerNavIcon(item.key)" :size="16" />
            <span>{{ item.label }}</span>
          </NuxtLink>
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

        <!-- <640px：动作区放不下的「消息」收进折叠面板（同一份子项表）。
             ≥640 时这块整体隐藏，动作区用图标 + 红点 / ≥1024 带文字。 -->
        <nav v-if="authStore.isLoggedIn" class="site-messages-mobile" aria-label="消息">
          <span class="eyebrow">消息</span>
          <NuxtLink
            v-for="item in messageNavItems"
            :key="item.key"
            :to="item.to"
            class="site-messages-mobile__item"
          >
            <span>{{ item.label }}</span>
            <span v-if="shouldShowUnreadCount(item.count)" class="site-messages__count">{{ item.count }}</span>
          </NuxtLink>
        </nav>

        <!-- <640px：历史入口也收进折叠面板（与消息中心同一套窄屏规则）。 -->
        <nav v-if="authStore.isLoggedIn" class="site-history-mobile" aria-label="历史">
          <NuxtLink to="/history" class="site-history-mobile__item">
            <IconClockHour4 :size="16" aria-hidden="true" />
            <span>浏览历史</span>
          </NuxtLink>
        </nav>
      </div>

      <div class="site-header__actions">
        <button
          type="button"
          class="site-action"
          data-chrome="theme"
          :aria-label="themeLabel"
          :title="themeLabel"
          @click="uiStore.toggleTheme()"
        >
          <component :is="uiStore.theme === 'dark' ? IconSunHigh : IconMoonStars" :size="18" />
        </button>

        <!-- 创作：亮暗切换之后、个人信息之前；与动作区共用 .site-action 那套样式。
             发布门控关闭时整项不渲染（ADR-0017 决策 2）；窄屏只留图标。 -->
        <button
          v-if="entryVisible"
          type="button"
          class="site-action site-action--equal site-header__write"
          data-chrome="write"
          aria-label="创作"
          title="创作"
          @click="openWriteEntry()"
        >
          <component :is="headerNavIcon('write')" :size="18" />
          <span class="site-header__write-label">创作</span>
        </button>

        <!-- 消息（第 239 票）：hover / 键盘聚焦展开五子项；<1024 仅图标 + 红点，
             <640 整体移进折叠面板。子项未读数与页面内导航同源（messageCenterNavItems）。
             文案「消息」只改导航标签；`/messages` 页面标题与领域术语仍叫「消息中心」。 -->
        <div
          v-if="authStore.isLoggedIn"
          class="site-messages"
          data-chrome="messages"
          @pointerenter="panel.openMessages()"
          @pointerleave="panel.scheduleMessagesClose()"
          @focusin="panel.openMessages()"
          @focusout="panel.handleMessagesFocusOut"
          @keydown="panel.handleMessagesKeydown($event)"
        >
          <NuxtLink
            ref="messagesTriggerRef"
            to="/messages"
            class="site-action site-action--equal site-header__messages"
            aria-haspopup="menu"
            :aria-expanded="messagesOpen"
            :aria-label="totalUnread ? `消息（${totalUnread} 条未读）` : '消息'"
          >
            <IconBell :size="18" aria-hidden="true" />
            <span class="site-header__messages-label">消息</span>
            <span v-if="totalUnread" class="site-header__dot" aria-hidden="true"></span>
          </NuxtLink>

          <div v-if="messagesOpen" class="site-messages__panel" role="menu" aria-label="消息子项">
            <NuxtLink
              v-for="(item, index) in messageNavItems"
              :key="item.key"
              :to="item.to"
              class="site-messages__item"
              role="menuitem"
              @click="panel.closeMessages()"
              @keydown="panel.moveMessageFocus($event, index, messageNavItems.length)"
            >
              <span>{{ item.label }}</span>
              <span v-if="shouldShowUnreadCount(item.count)" class="site-messages__count">{{ item.count }}</span>
            </NuxtLink>
          </div>
        </div>

        <!-- 历史（第 241 票）：普通链接，hover / 聚焦都不展开面板。
             ≥1280 常显（CSS）；640–1279 由用户菜单承担；<640 由折叠面板承担。 -->
        <NuxtLink
          v-if="authStore.isLoggedIn"
          to="/history"
          class="site-action site-action--equal site-header__history"
          data-chrome="history"
          aria-label="浏览历史"
        >
          <IconClockHour4 :size="18" aria-hidden="true" />
          <span>历史</span>
        </NuxtLink>

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

/* 三枚动作按钮等宽：同一个 class、同一个宽度基准，不再各自覆写 padding。
 * 4.25rem（68px）刚好放得下 18px 图标 + gap + 两个汉字（0.8125rem），
 * 亮暗主题与三档断点几何不变；图标态（文字隐藏）也由这条声明给宽度。 */
.site-action--equal {
  min-width: 4.25rem;
  padding: 0 0.5rem;
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

/* 创作：窄屏（<640px）降级为纯图标，文字在 640px 断点起显示；
 * 图标态仍是 2.75rem 的可点方块，无障碍文案走 aria-label / title。 */
.site-header__write-label {
  display: none;
}

.site-header__account-name {
  display: none;
  max-width: 6rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ---- 消息中心入口（第 239 票；2026-11-05 修可交互性） ---------------------------
 * 面板与触发按钮同属 .site-messages。指针要从按钮走到面板，中间不能出现
 * "既不在按钮也不在面板"的空隙：面板贴着容器底部，并给容器补一条透明
 * 桥接带（::after），再配合 200ms 关闭宽限，鼠标就能安全移入并点子项。 */
.site-messages {
  position: relative;
  display: inline-flex;
}

.site-messages::after {
  content: "";
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  height: 0.75rem;
}

.site-header__messages-label {
  display: none;
}

.site-header__dot {
  position: absolute;
  top: 0.4375rem;
  right: 0.4375rem;
  width: 0.4375rem;
  height: 0.4375rem;
  border-radius: var(--radius-pill);
  background: var(--color-danger);
}

.site-messages__panel {
  position: absolute;
  top: 100%;
  right: 0;
  z-index: 40;
  min-width: 12rem;
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  padding: 0.375rem;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-surface-raised);
  box-shadow: 0 18px 44px -20px var(--color-shadow);
  animation: site-messages-reveal var(--motion-fast) var(--motion-ease) both;
}

@keyframes site-messages-reveal {
  from {
    opacity: 0;
    transform: translateY(-0.25rem);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .site-messages__panel {
    animation-duration: 0ms;
  }
}

.site-messages__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.5rem 0.625rem;
  border-radius: var(--radius-sm);
  color: var(--color-ink-muted);
  font-size: 0.8125rem;
}

.site-messages__item:hover,
.site-messages__item:focus-visible,
.site-messages-mobile__item:hover,
.site-messages-mobile__item:focus-visible {
  background: var(--color-line-faint);
  color: var(--color-ink);
}

.site-messages__count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.25rem;
  padding: 0 0.375rem;
  border-radius: var(--radius-pill);
  background: var(--color-line-faint);
  color: var(--color-ink);
  font-size: 0.6875rem;
  font-weight: 600;
}

/* <640px 才出现在折叠面板里；≥640px 动作区常显入口，这块整体隐藏。 */
.site-messages-mobile {
  display: none;
}

/* ---- 历史入口（第 241 票；2026-11-04 决定清单 §5 第 27 条）--------------------
 * 与「消息」同一套胶囊（`.site-action`：边框 + 图标 + 文字），不再是无边框裸链接。
 * 出现档位也与消息一致：≥1024 常显；640–1023 由头像菜单承担；<640 进折叠面板。
 * 三档只显示其中一处，避免同一入口在屏幕里出现两次。 */
.site-header__history {
  display: none;
}

.site-history-mobile {
  display: none;
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

  /* 创作从平板起显示文字（窄屏只有图标）；宽度由 .site-action--equal 统一，
   * 不再在这里额外加 padding，否则三枚动作按钮会不等宽。 */
  .site-header__write-label {
    display: inline;
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

  /* ≥1024px 消息才显示文字（640–1023 只留图标 + 红点）。 */
  .site-header__messages-label {
    display: inline;
  }

  /* 历史与消息同档：≥1024 常显（此前是 ≥1280，1024–1279 只能去头像菜单里找）。 */
  .site-header__history {
    display: inline-flex;
  }
}

/* <640px：动作区收不下「消息中心」，入口整体移进折叠面板。
 * 折叠面板本身只在菜单展开时可见，所以这里只负责"哪一份 DOM 显示"。 */
@media (max-width: 639px) {
  .site-messages {
    display: none;
  }

  .site-messages-mobile {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    padding-top: 0.5rem;
    border-top: 1px solid var(--color-line-faint);
  }

  .site-messages-mobile__item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    min-height: 2.75rem;
    padding: 0 0.5rem;
    border-radius: var(--radius-sm);
    color: var(--color-ink-muted);
    font-size: 0.8125rem;
  }

  /* 历史：<640 收进折叠面板（与消息中心同一套规则）。 */
  .site-history-mobile {
    display: flex;
    padding-top: 0.5rem;
    border-top: 1px solid var(--color-line-faint);
  }

  .site-history-mobile__item {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    min-height: 2.75rem;
    padding: 0 0.5rem;
    border-radius: var(--radius-sm);
    color: var(--color-ink-muted);
    font-size: 0.8125rem;
  }
}
</style>

