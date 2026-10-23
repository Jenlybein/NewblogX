<script setup lang="ts">
import { computed } from "vue";
import { NButton, NTag, useMessage } from "naive-ui";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import ProfileCollectionShelf from "~/components/profile/ProfileCollectionShelf.vue";
import ProfileHeroCard from "~/components/profile/ProfileHeroCard.vue";
import ProfileRelationList from "~/components/profile/ProfileRelationList.vue";
import { getPublicFavoriteFolders, getOwnFavoriteFolders, getFavoriteFolderArticles } from "~/services/favorite";
import { getFansList, getFollowList, followUser, unfollowUser } from "~/services/follow";
import { getSelfUserDetail, getUserBaseInfo } from "~/services/user";
import type { InboxDraftSessionSeed } from "~/utils/chat";
import { formatCount } from "~/utils/format";
import { getAuthorButtonLabel, getRelationLabel, isFollowing } from "~/utils/relation";

type UserTab = "articles" | "favorites" | "follow" | "fans";
const USER_TABS: UserTab[] = ["articles", "favorites", "follow", "fans"];

function resolveUserTab(value: unknown): UserTab {
  return typeof value === "string" && USER_TABS.includes(value as UserTab) ? (value as UserTab) : "articles";
}

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const uiStore = useUiStore();
const message = useMessage();
const userId = computed(() => route.params.id as string);
const profileLoadFailed = shallowRef(false);

const activeTab = computed<UserTab>(() => resolveUserTab(route.query.tab));

function updateActiveTab(value: UserTab) {
  void router.replace({
    query: {
      ...route.query,
      ...(value === "articles" ? { tab: undefined } : { tab: value }),
    },
  });
}

const favoriteFolderId = computed<string>({
  get: () => (typeof route.query.folder === "string" ? route.query.folder : ""),
  set: (value) => {
    void router.replace({
      query: {
        ...route.query,
        ...(value ? { folder: value } : { folder: undefined }),
      },
    });
  },
});

const { data: profile, refresh: refreshProfile } = await useAsyncData(
  () => `user-base-${userId.value}`,
  () =>
    getUserBaseInfo(userId.value)
      .then((data) => {
        profileLoadFailed.value = false;
        return data;
      })
      .catch(() => {
        profileLoadFailed.value = true;
        return {
          id: userId.value,
          code_age: 0,
          avatar: "",
          nickname: "未知作者",
          abstract: "",
          view_count: 0,
          article_visited_count: 0,
          article_count: 0,
          fans_count: 0,
          follow_count: 0,
          favor_count: 0,
          digg_count: 0,
          comment_count: 0,
          favorites_visibility: true,
          followers_visibility: true,
          fans_visibility: true,
          home_style_id: null,
          relation: 0,
          place: "",
        };
      }),
  {
    watch: [userId],
    server: false,
    lazy: true,
  },
);

const isSelf = computed(() => authStore.profileId != null && String(authStore.profileId) === profile.value?.id);
const relationText = computed(() => getAuthorButtonLabel(profile.value?.relation));

const { data: selfDetail } = await useAsyncData(
  () => `self-detail:${authStore.profileId || "guest"}:${isSelf.value ? "self" : "other"}`,
  () => (isSelf.value ? getSelfUserDetail() : Promise.resolve(null)),
  {
    watch: [computed(() => authStore.profileId), isSelf],
  },
);

const profileAbstractText = computed(() => {
  if (isSelf.value) {
    return selfDetail.value?.abstract || "";
  }
  return profile.value?.abstract || "";
});

const articleQuery = computed(() => ({
  type: 3 as const,
  author_id: userId.value,
  page: 1,
  limit: 12,
  page_mode: "count" as const,
  sort: 2 as const,
}));

const { articles, pending: articlePending } = await useArticleSearch(articleQuery, {
  key: computed(() => `user-articles:${userId.value}`),
  server: false,
  lazy: true,
});

const favoriteLocked = computed(() => !isSelf.value && profile.value?.favorites_visibility === false);
const followLocked = computed(() => !authStore.isLoggedIn || (!isSelf.value && profile.value?.followers_visibility === false));
const fansLocked = computed(() => !authStore.isLoggedIn || (!isSelf.value && profile.value?.fans_visibility === false));

const { data: favoriteFolders, pending: favoriteFoldersPending } = await useAsyncData(
  () => `user-favorite-folders:${userId.value}:${isSelf.value ? "self" : "public"}:${favoriteLocked.value ? "locked" : "open"}`,
  async () => {
    if (favoriteLocked.value) {
      return { list: [], count: 0 };
    }

    return isSelf.value ? getOwnFavoriteFolders() : getPublicFavoriteFolders(userId.value);
  },
  {
    watch: [userId, isSelf, favoriteLocked],
  },
);

watch(
  () => [favoriteFolders.value?.list, favoriteFolderId.value] as const,
  ([list, current]) => {
    if (!list?.length) {
      if (current) {
        favoriteFolderId.value = "";
      }
      return;
    }

    if (!current || !list.some((item) => item.id === current)) {
      favoriteFolderId.value = list[0]?.id ?? "";
    }
  },
  { immediate: true },
);

const { data: favoriteArticles, pending: favoriteArticlesPending } = await useAsyncData(
  () => `user-favorite-articles:${favoriteFolderId.value || "empty"}:${favoriteLocked.value ? "locked" : "open"}`,
  async () => {
    if (favoriteLocked.value || !favoriteFolderId.value) {
      return { list: [], count: 0 };
    }

    return getFavoriteFolderArticles({
      favoriteId: favoriteFolderId.value,
      page: 1,
      limit: 18,
    });
  },
  {
    watch: [favoriteFolderId, favoriteLocked],
  },
);

const { data: followData, pending: followPending, refresh: refreshFollowData } = await useAsyncData(
  () => `user-follow:${userId.value}:${authStore.profileId || "guest"}:${followLocked.value ? "locked" : "open"}`,
  async () => {
    if (followLocked.value) {
      return { list: [], count: 0 };
    }

    return getFollowList({
      userId: userId.value,
      page: 1,
      limit: 30,
    });
  },
  {
    watch: [userId, computed(() => authStore.profileId), followLocked],
  },
);

const { data: fansData, pending: fansPending, refresh: refreshFansData } = await useAsyncData(
  () => `user-fans:${userId.value}:${authStore.profileId || "guest"}:${fansLocked.value ? "locked" : "open"}`,
  async () => {
    if (fansLocked.value) {
      return { list: [], count: 0 };
    }

    return getFansList({
      userId: userId.value,
      page: 1,
      limit: 30,
    });
  },
  {
    watch: [userId, computed(() => authStore.profileId), fansLocked],
  },
);

const favoriteSummaryText = computed(() => {
  if (favoriteLocked.value) return "收藏夹未公开";
  return `公开收藏夹 ${favoriteFolders.value?.count ?? 0} 个`;
});

const visibilityBadges = computed(() => [
  profile.value?.favorites_visibility ? "收藏公开" : "收藏私密",
  profile.value?.followers_visibility ? "关注公开" : "关注私密",
  profile.value?.fans_visibility ? "粉丝公开" : "粉丝私密",
]);

async function handleFollow() {
  if (!profile.value) return;
  if (profileLoadFailed.value) {
    message.error("作者资料加载失败，当前无法执行关注操作");
    return;
  }
  const isLoggedIn = await authStore.initializeSession();
  if (!isLoggedIn) {
    uiStore.openAuthModal();
    return;
  }

  try {
    if (isFollowing(profile.value.relation)) {
      await unfollowUser(profile.value.id);
      message.success("已取消关注");
    } else {
      await followUser(profile.value.id);
      message.success("已关注作者");
    }
    await Promise.all([refreshProfile(), refreshFollowData(), refreshFansData()]);
  } catch (error) {
    message.error(error instanceof Error ? error.message : "操作失败");
  }
}

async function handlePrivateMessage() {
  if (!profile.value) return;
  if (profileLoadFailed.value) {
    message.error("作者资料加载失败，当前无法发起私信");
    return;
  }
  const isLoggedIn = await authStore.initializeSession();
  if (!isLoggedIn) {
    uiStore.openAuthModal();
    return;
  }

  const draftSessionSeed: InboxDraftSessionSeed = {
    receiverId: profile.value.id,
    receiverNickname: profile.value.nickname || "新会话",
    receiverAvatar: profile.value.avatar || "",
    relation: profile.value.relation ?? 0,
  };

  void router.push({
    path: "/studio/inbox",
    query: {
      tab: "chat",
      draft_receiver_id: draftSessionSeed.receiverId,
      draft_receiver_nickname: draftSessionSeed.receiverNickname,
      draft_receiver_avatar: draftSessionSeed.receiverAvatar || undefined,
      draft_relation: String(draftSessionSeed.relation ?? 0),
    },
  });
}

async function handleRelationToggle(targetId: string, relation: number) {
  const isLoggedIn = await authStore.initializeSession();
  if (!isLoggedIn) {
    uiStore.openAuthModal();
    return;
  }

  try {
    if (isFollowing(relation)) {
      await unfollowUser(targetId);
      message.success("已取消关注");
    } else {
      await followUser(targetId);
      message.success("已关注");
    }
    await Promise.all([refreshProfile(), refreshFollowData(), refreshFansData()]);
  } catch (error) {
    message.error(error instanceof Error ? error.message : "操作失败");
  }
}

const tabLabels: Record<UserTab, string> = {
  articles: "文章",
  favorites: "收藏",
  follow: "关注",
  fans: "粉丝",
};

useSeoMeta({
  title: computed(() => `${profile.value?.nickname || "作者"} - 个人主页`),
  description: computed(() => profile.value?.abstract || "开发者个人主页"),
});
</script>

<template>
  <div class="page-stack">
    <div class="content-grid">
      <div class="page-stack">
        <ProfileHeroCard
          v-if="profile"
          :profile="profile"
          :abstract-text="profileAbstractText"
          :is-self="isSelf"
          :relation-text="relationText"
          :action-disabled="profileLoadFailed"
          :action-active="isFollowing(profile?.relation)"
          @follow="handleFollow"
          @message="handlePrivateMessage"
        />

        <section class="surface-card p-5 md:p-6">
          <div v-if="profileLoadFailed" class="mb-5 rounded-3xl border border-amber-200 bg-amber-50/85 px-4 py-3 text-sm text-amber-700">
            作者资料暂时加载失败，部分关系操作已临时禁用。
          </div>

          <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
            <div class="flex flex-wrap gap-2">
              <button
                v-for="item in [
                  { label: '文章', value: 'articles' },
                  { label: '收藏', value: 'favorites' },
                  { label: '关注', value: 'follow' },
                  { label: '粉丝', value: 'fans' },
                ]"
                :key="item.value"
                type="button"
                class="soft-tab"
                :class="{ 'is-active': activeTab === item.value }"
                @click="updateActiveTab(item.value as UserTab)"
              >
                {{ item.label }}
              </button>
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <span class="glass-badge">{{ tabLabels[activeTab] }}</span>
              <span class="glass-badge">{{ favoriteSummaryText }}</span>
            </div>
          </div>

          <template v-if="activeTab === 'articles'">
            <div v-if="articles.length" class="space-y-4">
              <ArticleFeedItem
                v-for="article in articles"
                :key="article.id"
                :article="article"
                compact
                :show-author="false"
              />
            </div>
            <div v-else class="surface-section flex min-h-[220px] items-center justify-center p-6 text-sm muted">
              {{ articlePending ? "正在加载作者文章…" : "这位作者还没有公开文章。" }}
            </div>
          </template>

          <ProfileCollectionShelf
            v-else-if="activeTab === 'favorites'"
            :folders="favoriteFolders?.list || []"
            :active-folder-id="favoriteFolderId"
            :articles="favoriteArticles?.list || []"
            :pending="favoriteFoldersPending || favoriteArticlesPending"
            :locked="favoriteLocked"
            locked-title="收藏夹暂未公开"
            locked-description="这位作者把收藏夹设成了私密状态，所以目前只能查看文章流与基础成就。"
            @update:active-folder-id="favoriteFolderId = $event"
          />

          <ProfileRelationList
            v-else-if="activeTab === 'follow'"
            title="关注列表"
            :items="followData?.list || []"
            :pending="followPending"
            :locked="followLocked"
            :locked-title="authStore.isLoggedIn ? '关注列表暂未公开' : '登录后查看关注列表'"
            :locked-description="
              authStore.isLoggedIn
                ? '这位作者关闭了关注列表公开，当前登录用户无权查看。'
                : '关注/粉丝列表接口是登录态接口，先登录后才能继续查看。'
            "
            @toggle-follow="handleRelationToggle"
          />

          <ProfileRelationList
            v-else-if="activeTab === 'fans'"
            title="粉丝列表"
            :items="fansData?.list || []"
            :pending="fansPending"
            :locked="fansLocked"
            :locked-title="authStore.isLoggedIn ? '粉丝列表暂未公开' : '登录后查看粉丝列表'"
            :locked-description="
              authStore.isLoggedIn
                ? '这位作者关闭了粉丝列表公开，当前登录用户无权查看。'
                : '粉丝列表属于登录态接口，先登录后才能继续查看。'
            "
            @toggle-follow="handleRelationToggle"
          />

          <div v-else class="surface-section flex min-h-[280px] items-center justify-center p-6 text-sm muted">
            当前分栏不可用，请切换到其他内容继续浏览。
          </div>
        </section>
      </div>

      <aside class="profile-sidebar">
        <section class="surface-card p-5 md:p-6">
          <div class="section-title">个人成就</div>
          <div class="mt-4 line-divider">
            <div class="sidebar-list-row">
              <span class="muted">关注了</span>
              <strong>{{ profile?.follow_count || 0 }}</strong>
            </div>
            <div class="sidebar-list-row">
              <span class="muted">关注者</span>
              <strong>{{ profile?.fans_count || 0 }}</strong>
            </div>
            <div class="sidebar-list-row">
              <span class="muted">文章数</span>
              <strong>{{ profile?.article_count || 0 }}</strong>
            </div>
            <div class="sidebar-list-row">
              <span class="muted">累计阅读</span>
              <strong>{{ formatCount(profile?.view_count || 0) }}</strong>
            </div>
          </div>
        </section>

        <section class="surface-card p-5 md:p-6">
          <div class="section-title">公开设置</div>
          <div class="mt-4 flex flex-wrap gap-2">
            <NTag v-for="badge in visibilityBadges" :key="badge" size="small" round :bordered="false">
              {{ badge }}
            </NTag>
          </div>
          <div class="mt-4 line-divider">
            <div class="sidebar-list-row">
              <span class="muted">加入信息</span>
              <strong>{{ profile?.place || "未填写地区" }}</strong>
            </div>
            <div class="sidebar-list-row line-divider">
              <span class="muted">代码年龄</span>
              <strong>{{ profile?.code_age || 0 }} 年</strong>
            </div>
            <div class="sidebar-list-row line-divider">
              <span class="muted">关系状态</span>
              <NTag size="small" round :bordered="false">{{ relationText }}</NTag>
            </div>
          </div>
        </section>

        <section class="surface-card p-5 md:p-6">
          <div class="section-title">资料摘要</div>
          <p class="mt-4 text-sm leading-7 muted">
            {{
              profileAbstractText ||
              "这位作者暂时还没有补充个人简介。你可以先从文章流、收藏夹或关系分栏了解他的内容偏好。"
            }}
          </p>
          <div class="mt-4 flex flex-wrap gap-2">
            <NuxtLink to="/search" class="glass-badge">回到文章流</NuxtLink>
            <NuxtLink v-if="isSelf" to="/studio/dashboard" class="glass-badge">进入个人中心</NuxtLink>
          </div>
        </section>
      </aside>
    </div>
  </div>
</template>
