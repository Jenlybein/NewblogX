<script setup lang="ts">
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, shallowRef, watch } from "vue";
import { IconEye, IconHeart, IconMessageCircle2, IconShare2, IconThumbUp } from "@tabler/icons-vue";
import { NButton, NTag, useMessage } from "naive-ui";
import ArticleScoreModal from "~/components/article/ArticleScoreModal.vue";
import ArticleTocAnchor from "~/components/article/ArticleTocAnchor.vue";
import AppAvatar from "~/components/common/AppAvatar.vue";
import CommentComposer from "~/components/comment/CommentComposer.vue";
import CommentThread from "~/components/comment/CommentThread.vue";
import FavoriteFolderModal from "~/components/favorite/FavoriteFolderModal.vue";
import { useReadingProgress } from "~/composables/useReadingProgress";
import { followUser, unfollowUser } from "~/services/follow";
import { ApiBusinessError } from "~/services/http/errors";
import { getArticleScoreSummary } from "~/services/ai";
import { getArticleAuthorInfo, getArticleDetail, markArticleViewed, toggleArticleDigg } from "~/services/article";
import { createComment, diggComment, getReplyComments, getRootComments } from "~/services/comment";
import { getUserBaseInfo } from "~/services/user";
import type { CommentReplyItem } from "~/types/api";
import { formatCount, formatDateTimeLabel } from "~/utils/format";
import { getAuthorButtonLabel, isFollowing } from "~/utils/relation";
import { resolveAvatarInitial } from "~/utils/avatar";
import katexCssUrl from "katex/dist/katex.min.css?url";
import highlightCssUrl from "highlight.js/styles/github.min.css?url";
import githubMarkdownCssUrl from "github-markdown-css/github-markdown-light.css?url";
import githubMarkdownDarkCssUrl from "github-markdown-css/github-markdown-dark.css?url";

const route = useRoute();
const router = useRouter();
const ArticleMarkdownRenderer = defineAsyncComponent(() => import("~/components/common/MarkdownRenderSurface.vue"));
const articleId = computed(() => route.params.id as string);
const authStore = useAuthStore();
const uiStore = useUiStore();
const message = useMessage();
const shadowPreviewRef = ref<{
  scrollToHeading: (id: string) => boolean;
  getHeadingElement: (id: string) => HTMLElement | null;
} | null>(null);
const articleHeadings = ref<ArticleHeadingAnchor[]>([]);

const ROOT_COMMENT_PAGE_SIZE = 7;
const REPLY_COMMENT_PAGE_SIZE = 3;
const ARTICLE_LIKE_DEBOUNCE_MS = 600;
const favoriteModalOpen = ref(false);
const articleScoreModalOpen = ref(false);

const { data: article, error: articleError, refresh: refreshArticle } = await useAsyncData(
  () => `article-${articleId.value}`,
  () => getArticleDetail(articleId.value),
);
const articleLoadError = computed(() => {
  const error = articleError.value;
  if (!error) {
    return "";
  }

  if (error instanceof ApiBusinessError) {
    return error.message;
  }

  if (error instanceof Error) {
    return error.message;
  }

  return "文章详情暂时无法加载。";
});
const authorId = computed(() => article.value?.author_id || "");

const { data: authorInfo, refresh: refreshAuthorInfo } = await useAsyncData<Awaited<ReturnType<typeof getArticleAuthorInfo>> | null>(
  () => `article-author-info-${authorId.value || articleId.value}`,
  () => (authorId.value ? getArticleAuthorInfo(authorId.value) : Promise.resolve(null)),
  {
    watch: [authorId],
  },
);

const { data: authorProfile, refresh: refreshAuthorProfile } = await useAsyncData<Awaited<ReturnType<typeof getUserBaseInfo>> | null>(
  () => `article-author-base-${authorId.value || articleId.value}`,
  () => (authorId.value ? getUserBaseInfo(authorId.value).catch(() => null) : Promise.resolve(null)),
  {
    watch: [authorId],
    server: false,
    lazy: true,
  },
);

const {
  data: articleScoreSummary,
  pending: articleScoreSummaryPending,
  refresh: refreshArticleScoreSummary,
} = await useAsyncData(
  () => `article-score-summary:${articleId.value}`,
  () => getArticleScoreSummary(articleId.value).catch(() => null),
  {
    watch: [articleId],
    lazy: true,
  },
);

const commentsPager = await usePagedResourceCache({
  cacheKey: () => `article-comments:${articleId.value}`,
  pageSize: () => ROOT_COMMENT_PAGE_SIZE,
  fetchPage: async (page, limit) => {
    try {
      const payload = await getRootComments(articleId.value, page, limit);
      return {
        items: payload.list,
        hasMore: payload.has_more,
      };
    } catch {
      return {
        items: [],
        hasMore: false,
      };
    }
  },
});

interface ReplyPagePayload {
  items: CommentReplyItem[];
  hasMore: boolean;
}

interface ReplyPagerState {
  currentPage: number;
  pages: Record<number, ReplyPagePayload>;
  pending: boolean;
  replyCount: number;
}

const replyStates = shallowRef<Record<string, ReplyPagerState>>({});
const replyTarget = ref<{ id: string; nickname: string } | null>(null);

function ensureReplyState(rootId: string) {
  const current = replyStates.value[rootId];
  if (current) {
    return current;
  }

  const nextState: ReplyPagerState = {
    currentPage: 1,
    pages: {},
    pending: false,
    replyCount: 0,
  };

  replyStates.value = {
    ...replyStates.value,
    [rootId]: nextState,
  };

  return nextState;
}

function resetReplyState(rootId: string) {
  const current = ensureReplyState(rootId);
  current.currentPage = 1;
  current.pages = {};
  current.pending = false;
  current.replyCount = 0;
  replyStates.value = {
    ...replyStates.value,
    [rootId]: current,
  };
}

const articleThemeHref = computed(() => (uiStore.theme === "dark" ? githubMarkdownDarkCssUrl : githubMarkdownCssUrl));
const markdownSupportStyleHrefs = [katexCssUrl, highlightCssUrl];
const { activeHeadingId, progressPercent } = useReadingProgress(
  computed(() => articleHeadings.value.map((heading) => heading.id)),
  (id) => shadowPreviewRef.value?.getHeadingElement(id) ?? null,
);
const authorInitial = computed(() => resolveAvatarInitial(article.value?.author_name, "A"));
const authorRelationText = computed(() => getAuthorButtonLabel(authorProfile.value?.relation));
const isSelfAuthor = computed(() => authStore.profileId != null && String(authStore.profileId) === authorId.value);
const articleScoreButtonLabel = computed(() => {
  if (articleScoreSummaryPending.value) {
    return "文章质量总分：读取中";
  }

  if (!articleScoreSummary.value?.has_score) {
    return "文章质量总分：--";
  }

  return `文章质量总分：${articleScoreSummary.value.total_score ?? "--"}`;
});
const articleIsDigged = shallowRef(false);
const articleDiggCount = shallowRef(0);
const articleDiggTouched = shallowRef(false);
const articleDiggStateArticleId = shallowRef("");
const articleLikePending = shallowRef(false);
const articleLikeDebouncing = shallowRef(false);
let articleLikeDebounceTimer: ReturnType<typeof setTimeout> | null = null;

watch(
  () => [articleId.value, article.value?.is_digg, article.value?.digg_count] as const,
  ([currentArticleId, isDigged, diggCount]) => {
    const articleChanged = currentArticleId !== articleDiggStateArticleId.value;
    if (articleChanged) {
      articleDiggStateArticleId.value = currentArticleId;
      articleDiggTouched.value = false;
    }

    if (articleDiggTouched.value) {
      return;
    }

    articleIsDigged.value = Boolean(isDigged);
    articleDiggCount.value = Math.max(Number(diggCount ?? 0), 0);
  },
  { immediate: true },
);

function patchArticleDiggState(isDigged: boolean, diggCount: number) {
  articleIsDigged.value = isDigged;
  articleDiggCount.value = Math.max(diggCount, 0);

  if (article.value) {
    article.value = {
      ...article.value,
      is_digg: isDigged,
      digg_count: articleDiggCount.value,
    };
  }
}

function resolveArticleDiggMutation(resultMessage: string, wasDigged: boolean) {
  if (resultMessage.includes("取消")) {
    return {
      isDigged: false,
      countDelta: -1,
    };
  }

  if (resultMessage.includes("点赞")) {
    return {
      isDigged: true,
      countDelta: 1,
    };
  }

  return {
    isDigged: !wasDigged,
    countDelta: wasDigged ? -1 : 1,
  };
}

function startArticleLikeDebounce() {
  articleLikeDebouncing.value = true;
  if (articleLikeDebounceTimer) {
    clearTimeout(articleLikeDebounceTimer);
  }

  articleLikeDebounceTimer = setTimeout(() => {
    articleLikeDebouncing.value = false;
    articleLikeDebounceTimer = null;
  }, ARTICLE_LIKE_DEBOUNCE_MS);
}

async function handleLike() {
  const isLoggedIn = await authStore.initializeSession();
  if (!isLoggedIn) {
    uiStore.openAuthModal();
    return;
  }

  if (articleLikePending.value || articleLikeDebouncing.value || !article.value) {
    return;
  }

  const wasDigged = articleIsDigged.value;
  articleLikePending.value = true;

  try {
    const result = await toggleArticleDigg(articleId.value);
    const resultMessage = result.msg || (wasDigged ? "取消点赞成功" : "点赞成功");
    const mutation = resolveArticleDiggMutation(resultMessage, wasDigged);
    articleDiggTouched.value = true;
    patchArticleDiggState(mutation.isDigged, articleDiggCount.value + mutation.countDelta);
    message.success(resultMessage);
  } catch (error) {
    message.error(error instanceof Error ? error.message : "点赞失败");
  } finally {
    articleLikePending.value = false;
    startArticleLikeDebounce();
  }
}

async function handleFavorite() {
  const isLoggedIn = await authStore.initializeSession();
  if (!isLoggedIn) {
    uiStore.openAuthModal();
    return;
  }
  favoriteModalOpen.value = true;
}

async function resetAndRefreshRootComments() {
  commentsPager.reset(1);
  await commentsPager.loadPage(1, true);
}

async function handleCreateComment(content: string) {
  try {
    await createComment({
      article_id: String(articleId.value),
      content,
      ...(replyTarget.value ? { reply_id: replyTarget.value.id } : {}),
    });
    message.success(replyTarget.value ? "回复已发送" : "评论已发送");
    const repliedRootId = replyTarget.value?.id;
    replyTarget.value = null;
    await resetAndRefreshRootComments();
    if (repliedRootId) {
      resetReplyState(repliedRootId);
      await handleLoadReplies(repliedRootId, true);
    }
    await refreshArticle();
  } catch (error) {
    message.error(error instanceof Error ? error.message : "评论失败");
  }
}

async function handleCommentDigg(commentId: string, isDigg: boolean) {
  try {
    await diggComment(commentId);
    message.success(isDigg ? "已取消评论点赞" : "评论点赞成功");
    await commentsPager.refreshCurrentPage();
  } catch (error) {
    message.error(error instanceof Error ? error.message : "评论点赞失败");
  }
}

async function loadReplyPage(rootId: string, page = 1, force = false) {
  const state = ensureReplyState(rootId);
  if (!force && state.pages[page]) {
    state.currentPage = page;
    replyStates.value = {
      ...replyStates.value,
      [rootId]: state,
    };
    return state.pages[page];
  }

  state.pending = true;
  replyStates.value = {
    ...replyStates.value,
    [rootId]: state,
  };

  try {
    const data = await getReplyComments(articleId.value, rootId, page, REPLY_COMMENT_PAGE_SIZE);
    state.pages = {
      ...state.pages,
      [page]: {
        items: data.list,
        hasMore: data.has_more,
      },
    };
    state.replyCount = data.reply_count;
    state.currentPage = page;
    replyStates.value = {
      ...replyStates.value,
      [rootId]: state,
    };
    return state.pages[page];
  } catch (error) {
    message.error(error instanceof Error ? error.message : "加载回复失败");
    return null;
  } finally {
    state.pending = false;
    replyStates.value = {
      ...replyStates.value,
      [rootId]: state,
    };
  }
}

async function handleLoadReplies(rootId: string, force = false) {
  const state = ensureReplyState(rootId);
  await loadReplyPage(rootId, state.currentPage || 1, force);
}

async function handleNextReplies(rootId: string) {
  const state = ensureReplyState(rootId);
  const current = state.pages[state.currentPage];
  if (!current?.hasMore && !state.pages[state.currentPage + 1]) {
    return;
  }
  await loadReplyPage(rootId, state.currentPage + 1);
}

async function handlePreviousReplies(rootId: string) {
  const state = ensureReplyState(rootId);
  if (state.currentPage <= 1) {
    return;
  }
  await loadReplyPage(rootId, state.currentPage - 1);
}

function handleReply(commentId: string) {
  const target = commentsPager.currentItems.value.find((item) => item.id === commentId);
  if (!target) return;
  replyTarget.value = {
    id: target.id,
    nickname: target.user_nickname,
  };
}

async function handleShare() {
  if (!import.meta.client) return;

  const shareUrl = `${window.location.origin}/article/${articleId.value}`;

  try {
    await navigator.clipboard.writeText(shareUrl);
    message.success("文章链接已复制");
  } catch {
    message.error("复制失败，请手动复制地址栏链接");
  }
}

async function handleFavoriteUpdated() {
  await refreshArticle();
}

function handleArticleScoreOpen() {
  articleScoreModalOpen.value = true;
  if (!articleScoreSummary.value) {
    void refreshArticleScoreSummary();
  }
}

async function handleAuthorFollow() {
  if (!authorId.value) {
    return;
  }

  const isLoggedIn = await authStore.initializeSession();
  if (!isLoggedIn) {
    uiStore.openAuthModal();
    return;
  }

  if (isSelfAuthor.value) {
    message.info("这是你自己发布的文章。");
    return;
  }

  try {
    if (isFollowing(authorProfile.value?.relation)) {
      await unfollowUser(authorId.value);
      message.success("已取消关注");
    } else {
      await followUser(authorId.value);
      message.success("已关注作者");
    }
    await refreshAuthorProfile();
    await refreshAuthorInfo();
  } catch (error) {
    message.error(error instanceof Error ? error.message : "关注操作失败");
  }
}

async function handlePrivateMessage() {
  const isLoggedIn = await authStore.initializeSession();
  if (!isLoggedIn) {
    uiStore.openAuthModal();
    return;
  }

  if (isSelfAuthor.value) {
    message.info("不能给自己发送私信。");
    return;
  }

  message.info("私信页面接入中，后续会直接跳转到与该作者的会话。");
}

function handleTocJump(id: string) {
  if (!id) {
    return;
  }

  const scrolled = shadowPreviewRef.value?.scrollToHeading(id);
  if (scrolled) {
    return;
  }

  if (!import.meta.client) {
    return;
  }

  document.getElementById(id)?.scrollIntoView({
    behavior: "smooth",
    block: "start",
  });
}

function handleArticleHeadingsChange(nextHeadings: ArticleHeadingAnchor[]) {
  articleHeadings.value = nextHeadings;
}

const comments = computed(() =>
  commentsPager.currentItems.value.map((comment) => {
    const replyState = replyStates.value[comment.id];
    const currentReplies = replyState?.pages[replyState.currentPage]?.items || [];
    return {
    ...comment,
    replies: currentReplies,
    };
  }),
);

const commentsPending = computed(() => commentsPager.pending.value);
const commentPage = computed(() => commentsPager.currentPage.value);
const commentPagesLoaded = computed(() => Object.keys(commentsPager.pages.value).length);
const hasPreviousCommentPage = computed(() => commentsPager.hasPreviousPage.value);
const hasNextCommentPage = computed(() => commentsPager.hasNextPage.value);
const replyLoading = computed(() =>
  Object.fromEntries(Object.entries(replyStates.value).map(([rootId, state]) => [rootId, state.pending])),
);
const replyPages = computed(() =>
  Object.fromEntries(Object.entries(replyStates.value).map(([rootId, state]) => [rootId, state.currentPage])),
);
const replyHasPrevious = computed(() =>
  Object.fromEntries(Object.entries(replyStates.value).map(([rootId, state]) => [rootId, state.currentPage > 1])),
);
const replyHasNext = computed(() =>
  Object.fromEntries(
    Object.entries(replyStates.value).map(([rootId, state]) => [
      rootId,
      Boolean(state.pages[state.currentPage + 1]) || Boolean(state.pages[state.currentPage]?.hasMore),
    ]),
  ),
);
const replyLoadedPages = computed(() =>
  Object.fromEntries(Object.entries(replyStates.value).map(([rootId, state]) => [rootId, Object.keys(state.pages).length])),
);

async function handlePreviousCommentPage() {
  await commentsPager.goToPreviousPage();
}

async function handleNextCommentPage() {
  await commentsPager.goToNextPage();
}

onMounted(() => {
  markArticleViewed(articleId.value).catch(() => undefined);
  void authStore.initializeSession().then(async (isLoggedIn) => {
    if (!isLoggedIn || articleDiggTouched.value) {
      return;
    }
    await refreshArticle().catch(() => undefined);
  });
});

onUnmounted(() => {
  if (articleLikeDebounceTimer) {
    clearTimeout(articleLikeDebounceTimer);
  }
});

useSeoMeta({
  title: computed(() => article.value?.title || "文章详情"),
  description: computed(() => article.value?.abstract || "BlogX 文章详情页"),
  ogTitle: computed(() => article.value?.title || "文章详情"),
  ogDescription: computed(() => article.value?.abstract || "BlogX 文章详情页"),
  ogImage: computed(() => article.value?.cover || ""),
});
</script>

<template>
  <div class="page-stack">
    <section v-if="articleLoadError" class="surface-card p-6 md:p-8">
      <div class="eyebrow">Article</div>
      <h1 class="section-title mt-2">文章详情暂不可用</h1>
      <p class="mt-4 text-sm leading-7 muted">
        {{ articleLoadError === "文章不存在" ? "当前文章详情暂不可用，请稍后重试或返回列表选择其他文章。" : articleLoadError }}
      </p>
      <div class="mt-5 flex flex-wrap gap-3">
        <NButton secondary round @click="router.back()">返回上一页</NButton>
        <NuxtLink to="/search" class="glass-badge">返回搜索页</NuxtLink>
      </div>
    </section>

    <template v-else>
    <section class="surface-card p-6 md:p-8">
      <div class="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
        <div class="max-w-3xl flex-1">
          <h1 class="page-title">{{ article?.title }}</h1>
          <p class="mt-4 text-base leading-8 muted">{{ article?.abstract }}</p>
          <div class="mt-5 flex flex-col gap-4">
            <div class="flex flex-wrap items-center gap-2.5 text-sm muted">
              <span
                v-if="article?.category_name"
                class="inline-flex items-center rounded-full bg-teal-50 px-3 py-1 text-sm font-medium text-teal-700 dark:bg-teal-500/10 dark:text-teal-200"
              >
                文章分类：{{ article.category_name }}
              </span>
              <NTag v-for="tag in article?.tags || []" :key="tag" round size="small" :bordered="false">{{ tag }}</NTag>
            </div>
            <div class="flex flex-wrap items-center gap-5 text-sm muted">
              <span>{{ formatDateTimeLabel(article?.created_at) }}</span>
              <span class="inline-flex items-center gap-1.5"><IconEye :size="16" /> {{ formatCount(article?.view_count || 0) }}</span>
              <span class="inline-flex items-center gap-1.5"><IconThumbUp :size="16" /> {{ formatCount(articleDiggCount) }}</span>
              <span class="inline-flex items-center gap-1.5"><IconHeart :size="16" /> {{ formatCount(article?.favor_count || 0) }}</span>
              <span class="inline-flex items-center gap-1.5"><IconMessageCircle2 :size="16" /> {{ formatCount(article?.comment_count || 0) }}</span>
            </div>
          </div>
        </div>

        <div class="flex shrink-0 flex-wrap items-center gap-3 lg:justify-end">
          <NButton round size="large" color="#0f172a" secondary @click="handleArticleScoreOpen">
            {{ articleScoreButtonLabel }}
          </NButton>
          <NButton
            round
            size="large"
            color="#0f766e"
            :loading="articleLikePending"
            :disabled="articleLikeDebouncing"
            @click="handleLike"
          >
            <template #icon>
              <IconThumbUp :size="18" />
            </template>
            {{ articleIsDigged ? "已点赞" : "点赞" }}
          </NButton>
          <NButton round size="large" color="#b45309" @click="handleFavorite">
            <template #icon>
              <IconHeart :size="18" />
            </template>
            收藏 {{ formatCount(article?.favor_count || 0) }}
          </NButton>
          <NButton round size="large" color="#475569" @click="handleShare">
            <template #icon>
              <IconShare2 :size="18" />
            </template>
            分享
          </NButton>
        </div>
      </div>
    </section>

    <div class="content-grid">
      <div class="page-stack">
        <section class="surface-card overflow-hidden">
          <img v-if="article?.cover" :src="article.cover" :alt="article.title" class="h-[320px] w-full object-cover md:h-[420px]" />
          <div class="p-6 md:p-8">
            <ArticleMarkdownRenderer
              ref="shadowPreviewRef"
              :source="article?.content || ''"
              :theme-href="articleThemeHref"
              :extra-style-hrefs="markdownSupportStyleHrefs"
              article-class="markdown-body"
              @headings-change="handleArticleHeadingsChange"
            />
          </div>
        </section>

        <section class="surface-card p-5 md:p-6">
          <div class="mb-5 flex items-center justify-between">
            <div class="section-title">评论区</div>
            <div class="glass-badge">{{ formatCount(article?.comment_count || 0) }} 条评论</div>
          </div>

          <CommentComposer
            :loading="commentsPending"
            :title="replyTarget ? `回复 ${replyTarget.nickname}` : '发表评论'"
            :placeholder="replyTarget ? '补充你的回复内容…' : '写下你的看法，帮助更多读者补齐思路。'"
            :submit-label="replyTarget ? '发送回复' : '发表评论'"
            :can-cancel="Boolean(replyTarget)"
            @submit="handleCreateComment"
            @cancel="replyTarget = null"
          />

          <div class="mt-6">
            <CommentThread
              :comments="comments"
              :loading-replies="replyLoading"
              :reply-pages="replyPages"
              :reply-has-previous="replyHasPrevious"
              :reply-has-next="replyHasNext"
              :reply-loaded-pages="replyLoadedPages"
              @reply="handleReply"
              @digg="handleCommentDigg"
              @load-replies="handleLoadReplies"
              @next-replies="handleNextReplies"
              @previous-replies="handlePreviousReplies"
            />
          </div>

          <div
            v-if="comments.length"
            class="mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-white/60 pt-5 text-sm muted"
          >
            <span>评论第 {{ commentPage }} 页，每页 7 条，已加载 {{ commentPagesLoaded }} 页。</span>
            <div class="flex items-center gap-3">
              <NButton quaternary :disabled="!hasPreviousCommentPage" @click="handlePreviousCommentPage">
                上一页
              </NButton>
              <NButton
                type="primary"
                ghost
                :disabled="!hasNextCommentPage"
                :loading="commentsPending"
                @click="handleNextCommentPage"
              >
                下一页
              </NButton>
            </div>
          </div>
        </section>
      </div>

      <aside class="profile-sidebar home-sidebar page-sidebar--sticky" style="--page-sidebar-sticky-top: 88px;">
        <div class="article-sidebar-stack">
          <section class="surface-card p-5 md:p-6">
            <div class="eyebrow">Author</div>
            <div class="mt-3 flex items-start gap-4">
              <AppAvatar :size="72" :src="article?.author_avatar" :name="article?.author_name" :fallback="authorInitial" />
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <div class="truncate text-[22px] font-semibold tracking-[-0.02em]">{{ article?.author_name }}</div>
                  <span class="glass-badge">文章作者</span>
                </div>
                <p class="mt-1 truncate text-sm muted">@{{ article?.author_username }}</p>
                <p class="mt-1 text-xs muted">
                  加入于 {{ formatDateTimeLabel(article?.author_created_time) || "暂未公开" }}
                </p>
              </div>
            </div>

            <p class="mt-4 text-sm leading-7 muted">
              {{ article?.author_abstract || authorProfile?.abstract || `${article?.author_name} 持续分享前端工程化、接口设计与页面组织相关内容。` }}
            </p>

            <div class="mt-5 grid grid-cols-3 gap-3 rounded-[24px] border border-white/60 bg-white/50 px-4 py-4 text-center dark:border-slate-700/70 dark:bg-slate-900/40">
              <div>
                <div class="text-[24px] font-semibold tracking-[-0.03em]">{{ formatCount(authorInfo?.article_count || 0) }}</div>
                <div class="mt-1 text-xs muted">文章</div>
              </div>
              <div>
                <div class="text-[24px] font-semibold tracking-[-0.03em]">{{ formatCount(authorInfo?.article_visited_count || 0) }}</div>
                <div class="mt-1 text-xs muted">阅读</div>
              </div>
              <div>
                <div class="text-[24px] font-semibold tracking-[-0.03em]">{{ formatCount(authorInfo?.fans_count || 0) }}</div>
                <div class="mt-1 text-xs muted">粉丝</div>
              </div>
            </div>

            <div class="mt-5 grid grid-cols-2 gap-3">
              <NButton :type="isFollowing(authorProfile?.relation) ? 'default' : 'primary'" :secondary="isFollowing(authorProfile?.relation)" round block @click="handleAuthorFollow">
                {{ isSelfAuthor ? "这是你" : authorRelationText }}
              </NButton>
              <NButton secondary round block @click="handlePrivateMessage">
                私信
              </NButton>
            </div>
          </section>

          <ArticleTocAnchor
            :headings="articleHeadings"
            :active-heading-id="activeHeadingId"
            :progress-percent="progressPercent"
            @jump="handleTocJump"
          />
        </div>
      </aside>
    </div>
    </template>

    <ArticleScoreModal
      v-model:show="articleScoreModalOpen"
      title="文章质量评分摘要"
      :loading="articleScoreSummaryPending"
      :data="articleScoreSummary ?? null"
      empty-text="当前文章还没有公开评分摘要。"
    />

    <FavoriteFolderModal
      v-model:show="favoriteModalOpen"
      :article-id="String(articleId)"
      :article-title="article?.title || '当前文章'"
      @updated="handleFavoriteUpdated"
    />
  </div>
</template>
