<script setup lang="ts">
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, shallowRef } from "vue";
import { IconEye, IconHeart, IconMessageCircle2, IconThumbUp } from "@tabler/icons-vue";
import { useMessage } from "naive-ui";
import ArticleActionsBar from "~/components/article/ArticleActionsBar.vue";
import ArticlePageSkeleton from "~/components/article/ArticlePageSkeleton.vue";
import ArticleOutlineAside from "~/components/article/ArticleOutlineAside.vue";
import ArticleScoreModal from "~/components/article/ArticleScoreModal.vue";
import ArticleTocAnchor from "~/components/article/ArticleTocAnchor.vue";
import AppAvatar from "~/components/common/AppAvatar.vue";
import CommentComposer from "~/components/comment/CommentComposer.vue";
import CommentThread from "~/components/comment/CommentThread.vue";
import FavoriteFolderModal from "~/components/favorite/FavoriteFolderModal.vue";
import { useArticleCommentJump } from "~/composables/useArticleCommentJump";
import { useArticleDigg } from "~/composables/useArticleDigg";
import { useArticleOutlineContext } from "~/composables/useArticleOutlineContext";
import { useArticlePageSources } from "~/composables/useArticlePageSources";
import { useArticleWritePolicy } from "~/composables/useArticleWritePolicy";
import { useMotionPreference } from "~/composables/useMotionPreference";
import { useParallax } from "~/composables/useParallax";
import { useReadingProgress } from "~/composables/useReadingProgress";
import { useSectionReveal } from "~/composables/useSectionReveal";
import { followUser, unfollowUser } from "~/services/follow";
import { ApiBusinessError } from "~/services/http/errors";
import { markArticleViewed, setArticleDigg } from "~/services/article";
import { createComment, diggComment, getReplyComments } from "~/services/comment";
import type { ArticleFavoriteResult, CommentReplyItem } from "~/types/api";
import { resolveArticleEditEntry } from "~/utils/articleEdit";
import { formatCount, formatDateTimeLabel } from "~/utils/format";
import { getAuthorButtonLabel, isFollowing } from "~/utils/relation";
import { resolveAvatarInitial } from "~/utils/avatar";
import { brandDescription } from "~/utils/brand";
import { readerArticleCacheKey } from "~/utils/readerCacheKeys";
import { readerSwrCache } from "~/utils/readerSwrCache";
import katexCssUrl from "katex/dist/katex.min.css?url";

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

const REPLY_COMMENT_PAGE_SIZE = 3;
const ARTICLE_LIKE_DEBOUNCE_MS = 600;
const favoriteModalOpen = ref(false);
const articleScoreModalOpen = ref(false);
// 互动条（票 236）评论动作的两块落点：#comments 区块 = 滚动目标，评论框 = 聚焦目标；平滑滚动跟随动效偏好。
const commentsSectionRef = ref<HTMLElement | null>(null);
const commentComposerRef = ref<{ focus: () => void } | null>(null);
const { prefersReducedMotion } = useMotionPreference();

// 动效档位 C：文章页首屏（hero）做一个区块进场；正文 markdown 段落一律不进场。
// 装饰记号接一条 ≤10px 的视差（motion-v useScroll），减少动态下两者都归零。
const { target: revealTarget, revealStyle } = useSectionReveal();
const { parallaxStyle } = useParallax();

// 文章页取数编排（ADR-0019 决策 3）：正文 / 评论 / 评分在 composable 里同时发起，
// 作者资料依赖正文里的 author_id、作为第二波；客户端跳转不等数据，先出骨架。
const {
  article,
  articlePending,
  articleError,
  refreshArticle,
  refreshArticlePrivate,
  authorInfo,
  refreshAuthorInfo,
  authorProfile,
  refreshAuthorProfile,
  articleScoreSummary,
  articleScoreSummaryPending,
  refreshArticleScoreSummary,
  commentsPager,
} = await useArticlePageSources({ articleId: () => articleId.value });

// 阅读页左栏目录树（票 220 / 决定清单 A9–A11）：从 `?from=<kind>:<id>` 找回读者
// 点进来的那棵树。树**只在客户端取数**（SSR 阶段不发请求，ADR-0019 的渲染边界与
// 匿名缓存策略不变），站内跳转先出与真实三栏版式同构的骨架；没有归属树时整栏
// 不出现，右栏标题大纲与阅读进度保持原样。
const outlineFrom = computed(() => String(route.query.from ?? ""));
const {
  context: articleOutlineContext,
  pending: articleOutlinePending,
  active: outlineColumnVisible,
} = useArticleOutlineContext({ articleId, from: outlineFrom });

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
// 读者侧封面固定居中（object-fit: cover），不再消费取景参数（票 204）：
// 上传时已定形成 16:9，未定形过的旧封面也按居中兜底，不出现空白或拉伸。
// 切页时（还没有正文）先出骨架；重试（正文已在）不闪骨架。
const articleSkeleton = computed(() => articlePending.value && !article.value);

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

// 正文主题由 MarkdownShadowPreview 内置（token 驱动，明暗自动跟随站点根类）；
// katex 只保留基线布局样式（分数/根号/字体），配色由自有主题覆盖。
const articleSupportStyleHrefs = [katexCssUrl];
const { activeHeadingId, progressPercent } = useReadingProgress(
  computed(() => articleHeadings.value.map((heading) => heading.id)),
  (id) => shadowPreviewRef.value?.getHeadingElement(id) ?? null,
);
const authorInitial = computed(() => resolveAvatarInitial(article.value?.author_name, "A"));
const authorRelationText = computed(() => getAuthorButtonLabel(authorProfile.value?.relation));
const isSelfAuthor = computed(() => authStore.profileId != null && String(authStore.profileId) === authorId.value);
// 「编辑」入口（票 213 / 决定清单 C4）：只给作者本人，且与页头「创作」共用同一个
// 站点发布开关（`entryVisible`）。判定收在纯函数里（`utils/articleEdit.ts`），
// 页面只负责把两个既有信号喂给它 —— 别人 / 游客 / 未登录都拿不到入口。
const { entryVisible: writeEntryVisible } = useArticleWritePolicy();
const articleEditEntry = computed(() =>
  resolveArticleEditEntry({
    articleId: articleId.value,
    isSelfAuthor: isSelfAuthor.value,
    writeEntryVisible: writeEntryVisible.value,
  }),
);
const articleScoreButtonLabel = computed(() => {
  if (articleScoreSummaryPending.value) {
    return "文章质量总分：读取中";
  }

  if (!articleScoreSummary.value?.has_score) {
    return "文章质量总分：--";
  }

  return `文章质量总分：${articleScoreSummary.value.total_score ?? "--"}`;
});
// 点赞状态机（乐观更新 + 脏标记 + 防抖 + 文案猜方向）见 composable。
// 这块混在页面里时没人愿意通读，所以单独成文并带测试。
const {
  articleIsDigged,
  articleDiggCount,
  articleLikePending,
  articleLikeDebouncing,
  articleDiggTouched,
  handleLike,
  dispose: disposeArticleDigg,
} = useArticleDigg({
  articleId: () => articleId.value,
  snapshot: () => ({ isDigg: article.value?.is_digg, diggCount: article.value?.digg_count }),
  patchArticle: (isDigg, diggCount) => {
    if (!article.value) {
      return;
    }
    article.value = { ...article.value, is_digg: isDigg, digg_count: diggCount };
    // 写响应已经是权威计数，客户端 30s SWR 里那份（匿名 payload 预热出的）旧值
    // 必须主动失效，否则返回/重进时会先渲染旧数字再等 TTL（ADR-0024 决策 4）。
    readerSwrCache.delete(readerArticleCacheKey(articleId.value));
  },
  // 点赞/收藏是写操作：用**目标态**接口（不是 toggle），写响应带回权威计数。
  setDigg: setArticleDigg,
  ensureLoggedIn: () => authStore.initializeSession(),
  openAuthModal: () => uiStore.openAuthModal(),
  notify: message,
  debounceMs: ARTICLE_LIKE_DEBOUNCE_MS,
});

async function handleFavorite() {
  const isLoggedIn = await authStore.initializeSession();
  if (!isLoggedIn) {
    uiStore.openAuthModal();
    return;
  }
  favoriteModalOpen.value = true;
}

// 互动条的唯一状态源（票 236 验收：右栏与 <1024px 底部条绑同一份 props，不可能不同步）：
// 点赞取 `useArticleDigg` 的乐观值（票 235），收藏 / 评论 / 已藏态取正文的权威字段。
const articleActionProps = computed(() => ({
  diggCount: articleDiggCount.value,
  favorCount: article.value?.favor_count ?? 0,
  commentCount: article.value?.comment_count ?? 0,
  isDigged: articleIsDigged.value,
  isFavor: Boolean(article.value?.is_favor),
  likePending: articleLikePending.value,
  likeDebouncing: articleLikeDebouncing.value,
}));

function scrollToComments() {
  commentsSectionRef.value?.scrollIntoView({ behavior: prefersReducedMotion.value ? "auto" : "smooth", block: "start" });
}

function focusCommentComposer() {
  commentComposerRef.value?.focus();
}

// 评论按钮：未登录先弹登录框、不滚动不聚焦；已登录才滚到 #comments 并聚焦输入框（顺序在 composable 里）。
const { jumpToComment } = useArticleCommentJump({
  ensureLoggedIn: () => authStore.initializeSession(),
  openAuthModal: () => uiStore.openAuthModal(),
  scrollToComments,
  focusComposer: focusCommentComposer,
});

function handleCommentJump() {
  void jumpToComment();
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

function handleFavoriteUpdated(payload: ArticleFavoriteResult) {
  // 写响应回填权威计数，直接 patch；不再为一次收藏重取整篇正文（ADR-0024 决策 4/6）。
  if (article.value) {
    article.value = {
      ...article.value,
      is_favor: payload.is_favor,
      favor_count: payload.favor_count,
    };
  }
  // 客户端 30s SWR 里那份正文仍是旧计数/旧私有态，主动失效。
  readerSwrCache.delete(readerArticleCacheKey(articleId.value));
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

  // 标题渲染在正文的 shadow DOM 里，跳转必须经由预览组件提供的入口。
  // 这里没有 document.getElementById 的回退：主文档里根本查不到这些 id，
  // 写了也永远是 null（原实现留下的死代码，随目录滚动一并清理）。
  shadowPreviewRef.value?.scrollToHeading(id);
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
  // 首屏 hydration 的 payload 是**匿名**的：点赞/收藏态不在里面（ADR-0019 约束 2）。
  // 登录初始化完成后必须补一次用户态请求（ADR-0024 决策 2/6），否则硬刷新后
  // 按钮永远显示"未赞"。这条请求不走 SWR、只投影私有字段，不污染共享缓存。
  // 用户已经手动点过赞时不覆盖（articleDiggTouched），避免把乐观结果刷回去。
  void authStore.initializeSession().then(async (isLoggedIn) => {
    if (!isLoggedIn || articleDiggTouched.value) {
      return;
    }
    await refreshArticlePrivate().catch(() => undefined);
  });
});

onUnmounted(() => {
  // 定时器归 composable 管，页面只负责通知它该收尾了。
  disposeArticleDigg();
});

useSeoMeta({
  title: computed(() => article.value?.title || "文章详情"),
  description: computed(() => brandDescription(article.value?.abstract)),
  ogTitle: computed(() => article.value?.title || "文章详情"),
  ogDescription: computed(() => brandDescription(article.value?.abstract)),
  ogImage: computed(() => article.value?.cover || ""),
});
</script>

<template>
  <div class="article-page">
    <!-- 切页先出骨架：正文还没到时按文章页版式占位，不再让 Suspense 等"数据齐了才切页"。 -->
    <ArticlePageSkeleton v-if="articleSkeleton" :with-outline="outlineColumnVisible" />

    <section v-else-if="articleLoadError" class="article-paper article-paper--state">
      <p class="article-eyebrow">Article</p>
      <h1 class="article-state__title">文章详情暂不可用</h1>
      <p class="article-state__copy">
        {{ articleLoadError === "文章不存在" ? "当前文章详情暂不可用，请稍后重试或返回列表选择其他文章。" : articleLoadError }}
      </p>
      <div class="article-state__actions">
        <button type="button" class="article-action" @click="router.back()">返回上一页</button>
        <NuxtLink to="/search" class="article-action">返回搜索页</NuxtLink>
      </div>
    </section>

    <template v-else>
      <header ref="revealTarget" class="article-hero motion-reveal" :style="revealStyle">
        <div class="article-hero__main">
          <p class="article-eyebrow">
            <span class="article-eyebrow__mark motion-parallax" aria-hidden="true" :style="parallaxStyle" />
            Article / 阅读
          </p>
          <h1 class="article-hero__title">{{ article?.title }}</h1>
          <p class="article-hero__abstract">{{ article?.abstract }}</p>

          <div class="article-hero__taxonomy">
            <span v-if="article?.category_name" class="article-chip article-chip--mark">
              文章分类：{{ article.category_name }}
            </span>
            <span v-for="tag in article?.tags || []" :key="tag" class="article-chip">#{{ tag }}</span>
          </div>

          <dl class="article-meta">
            <div class="article-meta__item">
              <dt>发布</dt>
              <dd>{{ formatDateTimeLabel(article?.created_at) }}</dd>
            </div>
            <div class="article-meta__item">
              <dt><IconEye :size="16" /><span class="sr-only">阅读数</span></dt>
              <dd>{{ formatCount(article?.view_count || 0) }}</dd>
            </div>
            <div class="article-meta__item">
              <dt><IconThumbUp :size="16" /><span class="sr-only">点赞数</span></dt>
              <dd>{{ formatCount(articleDiggCount) }}</dd>
            </div>
            <div class="article-meta__item">
              <dt><IconHeart :size="16" /><span class="sr-only">收藏数</span></dt>
              <dd>{{ formatCount(article?.favor_count || 0) }}</dd>
            </div>
            <div class="article-meta__item">
              <dt><IconMessageCircle2 :size="16" /><span class="sr-only">评论数</span></dt>
              <dd>{{ formatCount(article?.comment_count || 0) }}</dd>
            </div>
          </dl>
        </div>

        <div class="article-actions">
          <button type="button" class="article-action" @click="handleArticleScoreOpen">
            {{ articleScoreButtonLabel }}
          </button>
          <!-- 「编辑」入口（票 213 门控：仅作者本人 + 站点允许发布）：票 236 起紧贴质量评分右侧。 -->
          <NuxtLink
            v-if="articleEditEntry"
            :to="articleEditEntry.href"
            class="article-action article-action--compact"
            :aria-label="articleEditEntry.ariaLabel"
            :title="articleEditEntry.ariaLabel"
          >编辑</NuxtLink>
        </div>
      </header>

      <div class="article-layout" :class="{ 'article-layout--with-outline': outlineColumnVisible }">
        <ArticleOutlineAside
          v-if="outlineColumnVisible"
          :context="articleOutlineContext"
          :pending="articleOutlinePending"
          :current-article-id="articleId"
        />

        <div class="article-main">
          <section v-if="article?.cover" class="article-paper article-paper--cover">
            <img :src="article.cover" :alt="article.title" class="article-cover" />
          </section>

          <section class="article-paper article-paper--body">
            <div class="article-body">
              <ArticleMarkdownRenderer
                ref="shadowPreviewRef"
                :source="article?.content || ''"
                :extra-style-hrefs="articleSupportStyleHrefs"
                article-class="markdown-body"
                @headings-change="handleArticleHeadingsChange"
              />
            </div>
          </section>

          <section id="comments" ref="commentsSectionRef" class="article-paper comment-panel">
            <div class="comment-panel__head">
              <h2 class="comment-panel__title">评论区</h2>
              <span class="comment-panel__count">{{ formatCount(article?.comment_count || 0) }} 条评论</span>
            </div>

            <CommentComposer
              ref="commentComposerRef"
              :loading="commentsPending"
              :title="replyTarget ? `回复 ${replyTarget.nickname}` : '发表评论'"
              :placeholder="replyTarget ? '补充你的回复内容…' : '写下你的看法，帮助更多读者补齐思路。'"
              :submit-label="replyTarget ? '发送回复' : '发表评论'"
              :can-cancel="Boolean(replyTarget)"
              @submit="handleCreateComment"
              @cancel="replyTarget = null"
            />

            <div class="comment-panel__thread">
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

            <div v-if="comments.length" class="comment-panel__pagination">
              <span>评论第 {{ commentPage }} 页，每页 7 条。</span>
              <div class="comment-panel__pagination-actions">
                <button
                  type="button"
                  class="article-action article-action--compact"
                  :disabled="!hasPreviousCommentPage"
                  @click="handlePreviousCommentPage"
                >
                  上一页
                </button>
                <button
                  type="button"
                  class="article-action article-action--compact"
                  :disabled="!hasNextCommentPage || commentsPending"
                  @click="handleNextCommentPage"
                >
                  下一页
                </button>
              </div>
            </div>
          </section>
        </div>

        <aside class="article-aside">
          <section class="article-paper article-author">
            <p class="article-eyebrow">
              <span class="article-eyebrow__mark" aria-hidden="true" />
              Author
            </p>

            <div class="article-author__head">
              <AppAvatar
                :size="64"
                :src="article?.author_avatar"
                :name="article?.author_name"
                :fallback="authorInitial"
              />
              <div class="article-author__identity">
                <div class="article-author__name-row">
                  <span class="article-author__name">{{ article?.author_name }}</span>
                  <span class="article-chip">文章作者</span>
                </div>
                <p class="article-author__handle">@{{ article?.author_username }}</p>
                <p class="article-author__joined">
                  加入于 {{ formatDateTimeLabel(article?.author_created_time) || "暂未公开" }}
                </p>
              </div>
            </div>

            <p class="article-author__abstract">
              {{ article?.author_abstract || authorProfile?.abstract || `${article?.author_name} 持续分享前端工程化、接口设计与页面组织相关内容。` }}
            </p>

            <dl class="article-author__stats">
              <div class="article-author__stat">
                <dt>文章</dt>
                <dd>{{ formatCount(authorInfo?.article_count || 0) }}</dd>
              </div>
              <div class="article-author__stat">
                <dt>阅读</dt>
                <dd>{{ formatCount(authorInfo?.article_visited_count || 0) }}</dd>
              </div>
              <div class="article-author__stat">
                <dt>粉丝</dt>
                <dd>{{ formatCount(authorInfo?.fans_count || 0) }}</dd>
              </div>
            </dl>

            <div class="article-author__actions">
              <button type="button" class="article-action" @click="handleAuthorFollow">
                {{ isSelfAuthor ? "这是你" : authorRelationText }}
              </button>
              <button type="button" class="article-action" @click="handlePrivateMessage">私信</button>
            </div>
          </section>

          <!-- 互动条（票 236）：右栏顺序固定为 作者简介 → 互动条 → 目录。
               props 与 <1024px 底部固定条共用 `articleActionProps`（同一状态源）。 -->
          <section class="article-paper article-actions-card">
            <ArticleActionsBar
              v-bind="articleActionProps"
              @like="handleLike"
              @favorite="handleFavorite"
              @share="handleShare"
              @comment="handleCommentJump"
            />
          </section>

          <ArticleTocAnchor
            :headings="articleHeadings"
            :active-heading-id="activeHeadingId"
            :progress-percent="progressPercent"
            @jump="handleTocJump"
          />
        </aside>
      </div>

      <!-- <1024px 兜底入口（票 236）：右栏互动条在此宽度由 CSS 让位，这里渲染同一组件 + 同一份 props。 -->
      <ArticleActionsBar
        v-bind="articleActionProps"
        variant="fixed"
        @like="handleLike"
        @favorite="handleFavorite"
        @share="handleShare"
        @comment="handleCommentJump"
      />
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
