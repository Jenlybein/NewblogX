<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";
import { useDialog, useMessage } from "naive-ui";
import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";
import ProfileCategoryExplorer from "~/components/profile/ProfileCategoryExplorer.vue";
import ProfileCategoryFormModal from "~/components/profile/ProfileCategoryFormModal.vue";
import ProfileCollectionShelf from "~/components/profile/ProfileCollectionShelf.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import ProfileHeroCard from "~/components/profile/ProfileHeroCard.vue";
import ProfileRelationList from "~/components/profile/ProfileRelationList.vue";
import { getProfileCategories } from "~/services/category";
import { getPublicFavoriteFolders, getOwnFavoriteFolders, getFavoriteFolderArticles } from "~/services/favorite";
import { getFansList, getFollowList, followUser, unfollowUser } from "~/services/follow";
import { searchArticles } from "~/services/search";
import { readListPayload } from "~/services/listData";
import { useDraftChatJump } from "~/composables/useDraftChatJump";
import { getSelfUserDetail, getUserBaseInfo } from "~/services/user";
import { formatCount } from "~/utils/format";
import { getAuthorButtonLabel, isFollowing } from "~/utils/relation";
import { errorMessage } from "~/utils/errorMessage";
import type { SearchArticleItem } from "~/types/api";

type UserTab = "articles" | "favorites" | "categories" | "follow" | "fans";
const USER_TABS: UserTab[] = ["articles", "favorites", "categories", "follow", "fans"];

function resolveUserTab(value: unknown): UserTab {
  return typeof value === "string" && USER_TABS.includes(value as UserTab) ? (value as UserTab) : "articles";
}

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const uiStore = useUiStore();
const { jumpToDraftChat } = useDraftChatJump();
const message = useMessage();
const dialog = useDialog();
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

const { data: profile, pending: profilePending, refresh: refreshProfile } = useAsyncData(
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
          // ctype.ID 的零值序列化成空串（不是 null），契约里 home_style_id 就是 string。
          home_style_id: "",
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

const { data: selfDetail } = useAsyncData(
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

// 文章流与下面几个源一起在 setup 里**并行发起**（ADR-0019 约束 6：不得串行 await）：
// `useArticleSearch` 内部已经开始取数，这里只把它的句柄等回来，不阻塞 setup。
const articleSearchPromise = useArticleSearch(articleQuery, {
  key: computed(() => `user-articles:${userId.value}`),
  server: false,
  lazy: true,
});
type UserArticlesState = Awaited<ReturnType<typeof useArticleSearch>>;
const articleSearch = shallowRef<UserArticlesState | null>(null);
void articleSearchPromise
  .then((state) => {
    articleSearch.value = state;
  })
  .catch(() => undefined);
const articles = computed(() => articleSearch.value?.articles.value ?? []);
const articlePending = computed(() => articleSearch.value?.pending.value ?? true);

const favoriteLocked = computed(() => !isSelf.value && profile.value?.favorites_visibility === false);
const followLocked = computed(() => !authStore.isLoggedIn || (!isSelf.value && profile.value?.followers_visibility === false));
const fansLocked = computed(() => !authStore.isLoggedIn || (!isSelf.value && profile.value?.fans_visibility === false));

const { data: favoriteFolders, pending: favoriteFoldersPending } = useAsyncData(
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

const { data: favoriteArticles, pending: favoriteArticlesPending } = useAsyncData(
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

const { data: followData, pending: followPending, refresh: refreshFollowData } = useAsyncData(
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

const { data: fansData, pending: fansPending, refresh: refreshFansData } = useAsyncData(
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

// ── 分类栏（票 321）：只读。分类列表公开可读，点分类再按 author_id + category_id 取文章 ──
const categoriesLoadFailed = shallowRef(false);

const { data: categoriesData, pending: categoriesPending, refresh: refreshCategories } = useAsyncData(
  () => `user-categories:${userId.value}`,
  () =>
    getProfileCategories(userId.value)
      .then((data) => {
        categoriesLoadFailed.value = false;
        return data;
      })
      .catch(() => {
        // 读不到分类不能连累整页：退回空列表，由 failed 标记让分类栏说明原因。
        categoriesLoadFailed.value = true;
        return { list: [], count: 0 };
      }),
  {
    watch: [userId],
    server: false,
    lazy: true,
  },
);

const categories = computed(() => categoriesData.value?.list ?? []);

const activeCategoryId = shallowRef("");
const categoryArticles = shallowRef<SearchArticleItem[]>([]);
const categoryArticlesPending = shallowRef(false);
const categoryArticlesFailed = shallowRef(false);

/** 就地展开：点一个分类 → 取该作者该分类下的公开文章，不跳页。 */
async function loadCategoryArticles(categoryId: string) {
  const targetUser = userId.value;
  categoryArticlesPending.value = true;
  categoryArticlesFailed.value = false;

  try {
    const payload = await searchArticles({
      type: 3,
      author_id: targetUser,
      category_id: categoryId,
      page: 1,
      limit: 12,
      page_mode: "count",
      sort: 2,
    });

    // 请求返回时用户可能已经换分类 / 换主页：丢弃过期结果，别把旧分类的文章画上去。
    if (activeCategoryId.value !== categoryId || userId.value !== targetUser) return;

    categoryArticles.value = readListPayload(payload).list;
  } catch {
    if (activeCategoryId.value !== categoryId || userId.value !== targetUser) return;

    categoryArticles.value = [];
    categoryArticlesFailed.value = true;
  } finally {
    if (activeCategoryId.value === categoryId && userId.value === targetUser) {
      categoryArticlesPending.value = false;
    }
  }
}

watch(activeCategoryId, (categoryId) => {
  categoryArticles.value = [];
  categoryArticlesFailed.value = false;

  if (!categoryId) {
    categoryArticlesPending.value = false;
    return;
  }

  void loadCategoryArticles(categoryId);
});

// 换一个人看主页时收起已展开的分类，避免把上一位作者的文章留在栏里。
watch(userId, () => {
  activeCategoryId.value = "";
});

// ── 分类栏的写操作（票 325）：只有自己的主页才有，他人主页与未登录一律只读 ──
//
// 页面的职责只剩两件："是不是我自己"和"把按钮放进插槽"。新建 / 改名 / 删除的
// 流程（空名校验、二次确认 + 受影响篇数、失败原样展示、写完重拉列表）全在
// useProfileCategoryManage 里，那里的用例不依赖这个页面。
const canManageCategories = computed(() => isSelf.value);

const {
  formOpen: categoryFormOpen,
  formTitle: categoryFormTitle,
  formMode: categoryFormMode,
  saving: categorySaving,
  openCreate: openCreateCategory,
  openRename: openRenameCategory,
  submit: submitCategory,
  remove: removeCategory,
} = useProfileCategoryManage({
  message,
  // 二次确认交给 naive-ui 的 dialog：删除不可逆，且要先说清楚影响几篇文章。
  confirm: ({ title, content, onConfirm }) =>
    dialog.warning({
      title,
      content,
      positiveText: "删除",
      negativeText: "取消",
      onPositiveClick: onConfirm,
    }),
  reload: () => refreshCategories(),
  onDeleted: (id) => {
    // 删掉正在展开的那个分类时收起内容区，别把它的文章留在栏里。
    if (activeCategoryId.value === id) {
      activeCategoryId.value = "";
    }
  },
});

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
    message.error(errorMessage(error, "操作失败"));
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

  // 四个 draft_* 参数由 useDraftChatJump / buildDraftChatQuery 单一来源生成，
  // 与文章页共用同一份跨页契约（票 265）。
  void jumpToDraftChat({
    receiverId: profile.value.id,
    receiverNickname: profile.value.nickname || "新会话",
    receiverAvatar: profile.value.avatar || "",
    relation: profile.value.relation ?? 0,
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
    message.error(errorMessage(error, "操作失败"));
  }
}

const tabLabels: Record<UserTab, string> = {
  articles: "文章",
  favorites: "收藏",
  categories: "分类",
  follow: "关注",
  fans: "粉丝",
};

const tabSummaryText = computed(() => {
  if (activeTab.value === "articles") return `${articles.value.length} 篇文章`;
  if (activeTab.value === "favorites") return favoriteSummaryText.value;
  if (activeTab.value === "categories") return `${categories.value.length} 个分类`;
  if (activeTab.value === "follow") return `${followData.value?.list?.length || 0} 个关注`;
  return `${fansData.value?.list?.length || 0} 个粉丝`;
});

useSeoMeta({
  title: computed(() => `${profile.value?.nickname || "作者"} - 个人主页`),
  description: computed(() => profile.value?.abstract || "开发者个人主页"),
});
</script>

<template>
  <div class="page-stack">
    <div class="content-grid">
      <div class="page-stack">
        <!-- 骨架优先（ADR-0019 约束 6）：所有源并行发起、setup 不等待；资料未到时先占位。 -->
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
        <LoadingSkeleton
          v-else-if="profilePending"
          :rows="3"
          variant="card"
          label="作者主页正在加载"
        />

        <section class="blueprint-panel">
          <div v-if="profileLoadFailed" class="notice mb-5">
            作者资料暂时加载失败，部分关系操作已临时禁用。刷新页面或稍后再试。
          </div>

          <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
            <div class="flex flex-wrap gap-2">
              <button
                v-for="item in [
                  { label: '文章', value: 'articles' },
                  { label: '收藏', value: 'favorites' },
                  { label: '分类', value: 'categories' },
                  { label: '关注', value: 'follow' },
                  { label: '粉丝', value: 'fans' },
                ]"
                :key="item.value"
                type="button"
                class="blueprint-tab"
                :class="{ 'is-active': activeTab === item.value }"
                :aria-pressed="activeTab === item.value"
                @click="updateActiveTab(item.value as UserTab)"
              >
                {{ item.label }}
              </button>
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <span class="blueprint-chip">{{ favoriteSummaryText }}</span>
              <span class="blueprint-chip">{{ followData?.list?.length || 0 }} 关注</span>
              <span class="blueprint-chip">{{ fansData?.list?.length || 0 }} 粉丝</span>
            </div>
          </div>

          <div class="mb-4 flex flex-wrap items-center justify-between gap-3 border-t border-line-faint pt-4">
            <span class="text-sm text-ink-muted">{{ tabLabels[activeTab] }} · {{ tabSummaryText }}</span>
            <NuxtLink to="/search" class="blueprint-link">搜索更多文章</NuxtLink>
          </div>

          <template v-if="activeTab === 'articles'">
            <!-- 结果区骨架优先：文章流还在路上时先出占位，不再用「加载中…」空态代替版式。 -->
            <div v-if="articlePending && !articles.length" class="space-y-4">
              <LoadingSkeleton :rows="3" variant="card" label="作者文章正在加载" />
            </div>
            <div v-else-if="articles.length" class="space-y-4">
              <ArticleFeedItem
                v-for="article in articles"
                :key="article.id"
                :article="article"
                variant="list"
                :show-author="false"
              />
            </div>
            <ProfileEmptyState
              v-else
              size="md"
              :pending="articlePending"
              :title="articlePending ? '正在加载作者文章' : '这位作者还没有公开文章'"
              :description="
                articlePending
                  ? '文章流正在读取，稍候会自动出现。'
                  : '作者还没有发布公开文章，可以先看收藏架或关注/粉丝列表。'
              "
              :hints="articlePending ? [] : ['公开文章才会出现在这里，草稿与私密文章不会展示。']"
            >
              <NuxtLink to="/search" class="blueprint-link">去搜索其他人的文章</NuxtLink>
            </ProfileEmptyState>
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

          <ProfileCategoryExplorer
            v-else-if="activeTab === 'categories'"
            v-model:active-category-id="activeCategoryId"
            :categories="categories"
            :articles="categoryArticles"
            :pending="categoriesPending"
            :articles-pending="categoryArticlesPending"
            :articles-failed="categoryArticlesFailed"
            :failed="categoriesLoadFailed"
          >
            <!-- 写入口只在自己的主页出现：他人主页与未登录看到的仍是纯只读视图。 -->
            <template v-if="canManageCategories" #toolbar>
              <button
                type="button"
                class="profile-category-action is-primary"
                data-role="category-create"
                @click="openCreateCategory"
              >
                新建分类
              </button>
            </template>
            <template v-if="canManageCategories" #category-actions="{ category }">
              <div class="profile-category-card__actions">
                <button
                  type="button"
                  class="profile-category-action"
                  :data-role="`category-rename-${category.id}`"
                  @click="openRenameCategory(category)"
                >
                  改名
                </button>
                <button
                  type="button"
                  class="profile-category-action is-danger"
                  :data-role="`category-delete-${category.id}`"
                  @click="removeCategory(category)"
                >
                  删除
                </button>
              </div>
            </template>
          </ProfileCategoryExplorer>

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

          <ProfileEmptyState
            v-else
            size="md"
            title="当前分栏不可用"
            description="这个分栏暂时没有内容，请切换到其他内容继续浏览。"
            :hints="['文章、收藏、分类、关注、粉丝五个分栏互不影响。']"
          />
        </section>
      </div>

      <aside class="profile-sidebar">
        <section class="blueprint-panel">
          <div class="eyebrow">Profile</div>
          <div class="section-title mt-2">个人成就</div>
          <div class="mt-4 blueprint-divider">
            <div class="sidebar-list-row">
              <span class="text-ink-muted">关注了</span>
              <strong class="text-ink">{{ formatCount(profile?.follow_count || 0) }}</strong>
            </div>
            <div class="sidebar-list-row blueprint-divider">
              <span class="text-ink-muted">关注者</span>
              <strong class="text-ink">{{ formatCount(profile?.fans_count || 0) }}</strong>
            </div>
            <div class="sidebar-list-row blueprint-divider">
              <span class="text-ink-muted">文章数</span>
              <strong class="text-ink">{{ formatCount(profile?.article_count || 0) }}</strong>
            </div>
            <div class="sidebar-list-row blueprint-divider">
              <span class="text-ink-muted">累计阅读</span>
              <strong class="text-ink">{{ formatCount(profile?.view_count || 0) }}</strong>
            </div>
          </div>
        </section>

        <section class="blueprint-panel">
          <div class="eyebrow">Visibility</div>
          <div class="section-title mt-2">公开设置</div>
          <div class="mt-4 flex flex-wrap gap-2">
            <span v-for="badge in visibilityBadges" :key="badge" class="blueprint-chip">{{ badge }}</span>
          </div>
          <div class="mt-4 blueprint-divider">
            <div class="sidebar-list-row">
              <span class="text-ink-muted">加入信息</span>
              <strong class="text-ink">{{ profile?.place || "未填写地区" }}</strong>
            </div>
            <div class="sidebar-list-row blueprint-divider">
              <span class="text-ink-muted">代码年龄</span>
              <strong class="text-ink">{{ profile?.code_age || 0 }} 年</strong>
            </div>
            <div class="sidebar-list-row blueprint-divider">
              <span class="text-ink-muted">关系状态</span>
              <span class="blueprint-chip">{{ relationText }}</span>
            </div>
          </div>
        </section>

        <section class="blueprint-panel">
          <div class="eyebrow">About</div>
          <div class="section-title mt-2">资料摘要</div>
          <p class="mt-4 text-sm leading-prose text-ink-muted">
            {{
              profileAbstractText ||
              "这位作者暂时还没有补充个人简介。你可以先从文章流、收藏夹或关系分栏了解他的内容偏好。"
            }}
          </p>
          <div class="mt-4 flex flex-wrap items-center gap-3">
            <NuxtLink to="/search" class="blueprint-link">回到文章流</NuxtLink>
            <NuxtLink v-if="isSelf" to="/studio/dashboard" class="blueprint-link">进入个人中心</NuxtLink>
          </div>
        </section>
      </aside>
    </div>

    <!-- 新建 / 改名弹窗：只有自己的主页会打开它（按钮本身也只在 isSelf 时渲染）。 -->
    <ProfileCategoryFormModal
      v-model:show="categoryFormOpen"
      :mode="categoryFormMode"
      :initial-title="categoryFormMode === 'rename' ? categoryFormTitle : ''"
      :pending="categorySaving"
      @submit="submitCategory"
    />
  </div>
</template>
