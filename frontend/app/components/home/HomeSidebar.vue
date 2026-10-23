<script setup lang="ts">
import { computed, ref } from "vue";
import type { StyleValue } from "vue";
import { NButton, NSkeleton, NTag } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import HomeAiChatDialog from "~/components/home/HomeAiChatDialog.vue";
import { useAuthStore } from "~/stores/auth";
import { useUiStore } from "~/stores/ui";
import type { SearchArticleItem, SiteAiInfo, SiteRuntimeConfig } from "~/types/api";
import { resolveAvatarInitial, resolveAvatarUrl } from "~/utils/avatar";

const props = withDefaults(defineProps<{
  pending?: boolean;
  runtimeConfig: SiteRuntimeConfig | null;
  aiInfo: SiteAiInfo | null;
  articles: SearchArticleItem[];
  sticky?: boolean;
  stickyTop?: number | string;
}>(), {
  sticky: false,
  stickyTop: "0.5rem",
});

const sidebarClasses = computed(() => ({
  "profile-sidebar": true,
  "home-sidebar": true,
  "page-sidebar--sticky": props.sticky,
}));

const sidebarStyle = computed<StyleValue>(() => {
  if (!props.sticky) {
    return undefined;
  }

  return {
    "--page-sidebar-sticky-top": typeof props.stickyTop === "number" ? `${props.stickyTop}px` : props.stickyTop,
  };
});

const enabledModules = computed(() => {
  const source = props.runtimeConfig?.index_right?.list || [];
  return new Set(
    source
      .map((item) => String(item.title || "").trim())
      .filter(Boolean),
  );
});

const loginModes = computed(() => {
  const login = props.runtimeConfig?.login;
  const result: string[] = [];

  if (login?.pwd) result.push("密码登录");
  if (login?.email) result.push("邮箱登录");
  if (login?.qq) result.push("QQ 登录");

  return result;
});

const hotTags = computed(() => {
  const tagMap = new Map<string, { id: string; title: string; count: number }>();

  for (const article of props.articles) {
    for (const tag of article.tags || []) {
      const current = tagMap.get(tag.id);
      if (current) {
        current.count += 1;
      } else {
        tagMap.set(tag.id, {
          id: tag.id,
          title: tag.title,
          count: 1,
        });
      }
    }
  }

  return [...tagMap.values()]
    .sort((left, right) => right.count - left.count || left.id.localeCompare(right.id))
    .slice(0, 8);
});

const recommendedAuthors = computed(() => {
  const authorMap = new Map<
    string,
    {
      id: string;
      nickname: string;
      avatar: string;
      articleCount: number;
      totalViews: number;
    }
  >();

  for (const article of props.articles) {
    const current = authorMap.get(article.author.id);
    if (current) {
      current.articleCount += 1;
      current.totalViews += article.view_count;
      continue;
    }

    authorMap.set(article.author.id, {
      id: article.author.id,
      nickname: article.author.nickname,
      avatar: resolveAvatarUrl(article.author.avatar),
      articleCount: 1,
      totalViews: article.view_count,
    });
  }

  return [...authorMap.values()]
    .sort((left, right) => right.articleCount - left.articleCount || right.totalViews - left.totalViews)
    .slice(0, 4);
});

const showNotice = computed(() => enabledModules.value.size === 0 || enabledModules.value.has("site_notice"));
const showTags = computed(() => enabledModules.value.size === 0 || enabledModules.value.has("hot_tags"));
const showAuthors = computed(() => enabledModules.value.size === 0 || enabledModules.value.has("recommended_authors"));

const projectTitle = computed(() => props.runtimeConfig?.project?.title || props.runtimeConfig?.site_info?.title || "BlogX");
const projectAbstract = computed(
  () => props.runtimeConfig?.project?.abstract || props.runtimeConfig?.site_info?.subtitle || "为开发者写作、搜索与知识整理提供稳定的内容入口。",
);
const aiAvatarUrl = computed(() => resolveAvatarUrl(props.aiInfo?.avatar ?? ""));
const authStore = useAuthStore();
const uiStore = useUiStore();
const aiDialogOpen = ref(false);
const aiDisplayName = computed(() => props.aiInfo?.nickname || "BlogX 助手");
const aiDescription = computed(
  () => props.aiInfo?.abstract || "可以直接提问文章搜索、写作改写、结构诊断与内容整理问题。",
);
const aiCapabilities = ["站内搜索", "写作建议", "结构诊断", "Markdown 回复"];

async function handleOpenAiDialog() {
  if (!props.aiInfo?.enable) {
    return;
  }

  const isLoggedIn = await authStore.initializeSession();
  if (!isLoggedIn) {
    uiStore.openAuthModal();
    return;
  }

  aiDialogOpen.value = true;
}
</script>

<template>
  <aside :class="sidebarClasses" :style="sidebarStyle">
    <section v-if="pending" class="surface-card p-5 md:p-6">
      <NSkeleton text width="96px" />
      <NSkeleton text class="mt-3" :repeat="2" />
      <div class="mt-4 flex flex-wrap gap-2">
        <NSkeleton v-for="idx in 3" :key="`notice-tag-${idx}`" round height="24px" width="64px" />
      </div>
    </section>

    <section v-if="showNotice" class="surface-card p-5 md:p-6">
      <div class="text-base font-semibold">站点公告</div>
      <p class="mt-2 text-sm leading-7 muted">
        {{ projectAbstract }}
      </p>
      <div class="mt-3 flex flex-wrap gap-2">
        <NTag size="small" round :bordered="false">
          {{ projectTitle }}
        </NTag>
        <NTag
          v-for="mode in loginModes"
          :key="mode"
          size="small"
          round
          :bordered="false"
          type="success"
        >
          {{ mode }}
        </NTag>
      </div>
    </section>

    <section v-if="showTags && hotTags.length" class="surface-card p-5 md:p-6">
      <div class="text-base font-semibold">热门标签</div>
      <div class="mt-3 flex flex-wrap gap-2">
        <NuxtLink
          v-for="tag in hotTags"
          :key="tag.id"
          :to="{ path: '/search', query: { tag_ids: tag.id } }"
          class="inline-flex"
        >
          <NTag
            size="small"
            round
            :bordered="false"
          >
            {{ tag.title }}
          </NTag>
        </NuxtLink>
      </div>
    </section>

    <section v-if="showAuthors && recommendedAuthors.length" class="surface-card p-5 md:p-6">
      <div class="text-base font-semibold">推荐作者</div>
      <div class="mt-3 space-y-3">
        <NuxtLink
          v-for="author in recommendedAuthors"
          :key="author.id"
          :to="`/users/${author.id}`"
          class="flex items-center justify-between gap-3 rounded-2xl px-1 py-1 transition hover:bg-slate-100/70 dark:hover:bg-slate-800/70"
        >
          <div class="flex items-center gap-3">
            <AppAvatar :size="38" :src="author.avatar" :name="author.nickname" fallback="作" />
            <div>
              <div class="text-sm font-semibold">{{ author.nickname }}</div>
              <div class="text-xs muted">{{ author.articleCount }} 篇文章</div>
            </div>
          </div>
          <span class="text-xs muted">{{ author.totalViews }} 阅读</span>
        </NuxtLink>
      </div>
    </section>

    <section class="surface-card overflow-hidden p-0">
      <div class="home-ai-card">
        <div class="home-ai-card__glow" />
        <div class="home-ai-card__inner">
          <div class="flex items-start gap-4">
            <NSkeleton v-if="pending" circle height="60px" width="60px" />
            <div
              v-else
              class="home-ai-card__avatar"
            >
              <img
                v-if="aiAvatarUrl"
                :src="aiAvatarUrl"
                :alt="aiDisplayName"
                class="block h-full w-full object-cover object-center"
              />
              <div
                v-else
                class="flex h-full w-full items-center justify-center text-base font-semibold text-slate-700"
              >
                {{ resolveAvatarInitial(aiInfo?.nickname, "AI") }}
              </div>
            </div>

            <div class="min-w-0 flex-1">
              <template v-if="pending">
                <NSkeleton text width="72px" />
                <NSkeleton text width="128px" class="mt-3" />
                <NSkeleton text class="mt-3" :repeat="2" />
              </template>
              <template v-else>
                <div class="text-lg font-semibold text-slate-900">
                  {{ aiDisplayName }}
                </div>
                <p class="mt-2 text-sm leading-7 text-slate-600">
                  {{ aiDescription }}
                </p>
              </template>
            </div>
          </div>

          <div v-if="!pending" class="mt-5 flex flex-wrap gap-2">
            <span
              v-for="capability in aiCapabilities"
              :key="capability"
              class="home-ai-card__chip"
            >
              {{ capability }}
            </span>
          </div>

          <div class="mt-5 flex items-center justify-between gap-3">
            <NButton
              type="primary"
              class="ml-auto shrink-0"
              :disabled="pending || !aiInfo?.enable"
              @click="handleOpenAiDialog"
            >
              进行对话
            </NButton>
          </div>
        </div>
      </div>
    </section>

    <HomeAiChatDialog v-model:show="aiDialogOpen" :ai-info="aiInfo" />
  </aside>
</template>

<style scoped>
.home-ai-card {
  position: relative;
  overflow: hidden;
  background: #ffffff;
}

.home-ai-card__glow {
  position: absolute;
  inset: auto -14% -34% auto;
  width: 180px;
  height: 180px;
  border-radius: 999px;
  background: radial-gradient(circle, rgb(20 184 166 / 0.08), transparent 68%);
  pointer-events: none;
}

.home-ai-card__inner {
  position: relative;
  padding: 1.5rem;
}

.home-ai-card__avatar {
  display: flex;
  width: 60px;
  height: 60px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 1.25rem;
  border: 1px solid rgb(226 232 240 / 0.88);
  background: #ffffff;
  box-shadow: 0 20px 40px rgb(15 23 42 / 0.08);
}

.home-ai-card__chip {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  background: rgb(255 255 255 / 0.92);
  border: 1px solid rgb(226 232 240 / 0.86);
  padding: 0.42rem 0.72rem;
  font-size: 0.74rem;
  font-weight: 600;
  color: rgb(51 65 85 / 0.92);
}
</style>
