<script setup lang="ts">
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref } from "vue";
import { IconEye, IconHeart, IconMessageCircle2, IconMessageCirclePlus, IconThumbUp } from "@tabler/icons-vue";
import { useMessage } from "naive-ui";
import ArticleActionsBar from "~/components/article/ArticleActionsBar.vue";
import ArticlePageSkeleton from "~/components/article/ArticlePageSkeleton.vue";
import ArticleOutlineAside from "~/components/article/ArticleOutlineAside.vue";
import ArticleScoreModal from "~/components/article/ArticleScoreModal.vue";
import ArticleTocAnchor from "~/components/article/ArticleTocAnchor.vue";
import AppAvatar from "~/components/common/AppAvatar.vue";
import CommentComposerSheet from "~/components/comment/CommentComposerSheet.vue";
import CommentThread from "~/components/comment/CommentThread.vue";
import FavoriteFolderModal from "~/components/favorite/FavoriteFolderModal.vue";
import { useArticleDigg } from "~/composables/useArticleDigg";
import { useArticleComments } from "~/composables/useArticleComments";
import { useArticleOutlineContext } from "~/composables/useArticleOutlineContext";
import { useArticlePageSources } from "~/composables/useArticlePageSources";
import { useArticlePrivateSession } from "~/composables/useArticlePrivateSession";
import { useArticleWritePolicy } from "~/composables/useArticleWritePolicy";
import { useDraftChatJump } from "~/composables/useDraftChatJump";
import { useMotionPreference } from "~/composables/useMotionPreference";
import { useParallax } from "~/composables/useParallax";
import { useReaderLoginGate } from "~/composables/useReaderLoginGate";
import { useReadingProgress } from "~/composables/useReadingProgress";
import { useSectionReveal } from "~/composables/useSectionReveal";
import { followUser, unfollowUser } from "~/services/follow";
import { ApiBusinessError } from "~/services/http/errors";
import { markArticleViewed, setArticleDigg } from "~/services/article";
import { createComment, diggComment, getReplyComments } from "~/services/comment";
import type { ArticleFavoriteResult } from "~/types/api";
import { resolveArticleEditEntry } from "~/utils/articleEdit";
import { formatCount, formatDateTimeLabel } from "~/utils/format";
import { getAuthorButtonLabel, isFollowing } from "~/utils/relation";
import { resolveAvatarInitial } from "~/utils/avatar";
import { brandAbsoluteAssetUrl, brandDescription, brandFullName } from "~/utils/brand";
import { articleJsonLd, breadcrumbJsonLd, canonicalArticleUrl, siteOrigin } from "~/utils/seo";
import { errorMessage } from "~/utils/errorMessage";
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

const ARTICLE_LIKE_DEBOUNCE_MS = 600;
const favoriteModalOpen = ref(false);
const articleScoreModalOpen = ref(false);
// 互动条（票 236）评论动作的两块落点：#comments 区块 = 滚动目标，评论框 = 聚焦目标；平滑滚动跟随动效偏好。
const commentsSectionRef = ref<HTMLElement | null>(null);

const { prefersReducedMotion } = useMotionPreference();

// 动效档位 C：文章页首屏（hero）做一个区块进场；正文 markdown 段落一律不进场。
// 装饰记号接一条 ≤10px 的视差（motion-v useScroll），减少动态下两者都归零。
const { target: revealTarget, revealStyle } = useSectionReveal();
const { parallaxStyle } = useParallax();

// 文章页取数编排（ADR-0016 决策 3）：正文 / 评论 / 评分在 composable 里同时发起，
// 作者资料依赖正文里的 author_id、作为第二波；客户端跳转不等数据，先出骨架。
const {
  article,
  articlePending,
  articleError,
  refreshArticle,
  refreshArticlePrivate,
  reportArticleWrite,
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
// 点进来的那棵树。树**只在客户端取数**（SSR 阶段不发请求，ADR-0016 的渲染边界与
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

// 互动统一登录前置（票 238 范围 7）：收藏 / 关注 / 评论提交先问登录态，
// 未登录只弹登录框、不打接口 —— 否则请求层的 markPossiblyExpired() 会把用户的
// 登录态打成失效。分享不在此列：它是纯前端 clipboard、不打任何接口，
// 游客也应该能复制链接（票 247 的修正）。
const requireLogin = useReaderLoginGate();
const { jumpToDraftChat } = useDraftChatJump();
// 评论输入面板（决定清单 §4）：内嵌输入框退役，改成一枚「去发表评论」按钮 + 底部滑出面板。
// 登录前置由 `openCommentSheet` 统一收口，未登录只弹登录框、不打接口、不展开面板。
const commentSheetOpen = ref(false);

async function openCommentSheet() {
  if (!(await requireLogin())) {
    return;
  }
  commentSheetOpen.value = true;
}

async function handleCommentInvite() {
  await openCommentSheet();
}

function closeCommentSheet() {
  commentSheetOpen.value = false;
  // 关闭即清回复目标：下次打开默认是「发表评论」，而不是悄悄接着回复上一个人。
  clearReplyTarget();
}

// 评论区状态机（票 238）：回复分页、回复目标、点赞/提交的统一入口。
// 页面只负责接线与模板渲染，逻辑与登录前置都在 composable 里（可单测）。
const {
  comments,
  replyTarget,
  clearReplyTarget,
  replyLoading,
  replyPages,
  replyHasPrevious,
  replyHasNext,
  replyLoadedPages,
  handleReply,
  handleCommentDigg,
  handleCreateComment,
  handleLoadReplies,
  handleNextReplies,
  handlePreviousReplies,
} = useArticleComments({
  articleId: () => String(articleId.value),
  pager: commentsPager,
  requireLogin,
  fetchReplies: (id, rootId, page, limit) => getReplyComments(id, rootId, page, limit),
  createComment,
  diggComment,
  refreshArticle: () => refreshArticle(),
  openComposer: () => openCommentSheet(),
  notify: message,
});

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
/** 作者公开主页：作者卡上的头像 / 昵称 / @id 都指向它。 */
const authorProfileLink = computed(() => (authorId.value ? `/users/${authorId.value}` : ""));
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
    // 页面只上报「读者做了什么」：写响应回填权威计数 / 清客户端 30s SWR /
    // 私有态覆盖 / 缓存键拼接已收进阅读页取数模块（票 282、ADR-0016 决策 4）。
    reportArticleWrite({ kind: "digg", result: { is_digg: isDigg, digg_count: diggCount } });
  },
  // 点赞/收藏是写操作：用**目标态**接口（不是 toggle），写响应带回权威计数。
  setDigg: setArticleDigg,
  ensureLoggedIn: () => authStore.initializeSession(),
  openAuthModal: () => uiStore.openAuthModal(),
  notify: message,
  debounceMs: ARTICLE_LIKE_DEBOUNCE_MS,
});

// 登录态补取私有态（评审票 248 / P0-4）：
// 只在 onMounted 里判一次 initializeSession() 会漏掉「用户在文章页被弹登录框、
// 登录成功后才成为已登录」这条路径 —— 私有态永不请求，按钮继续显示未赞（库里其实已赞）。
// 这里同时接住两条触发（首屏初始化 / 登录态 false→true），单飞闸门保证一次登录只补一次。
const articlePrivateSession = useArticlePrivateSession({
  isLoggedIn: () => authStore.isLoggedIn,
  initializeSession: () => authStore.initializeSession(),
  refreshPrivate: () => refreshArticlePrivate(),
  // 用户已经手动点过赞时不覆盖乐观结果（ADR-0016 约束 3）。
  shouldSkip: () => articleDiggTouched.value,
});

async function handleFavorite() {
  if (!(await requireLogin())) {
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

/** 「底部」按钮：滚到评论区起点（= 正文末尾）。页脚不是内容，不作为落点。 */
function handleScrollToEnd() {
  scrollToComments();
}

/** 「到顶」按钮：与「到底」成对，回到正文开头。 */
function handleScrollToTop() {
  if (!import.meta.client) return;
  window.scrollTo({ top: 0, behavior: prefersReducedMotion.value ? "auto" : "smooth" });
}

/** 互动条的「评论」：直接开输入面板（未登录先弹登录框），不再滚动 + 聚焦。 */
function handleCommentJump() {
  void openCommentSheet();
}

/** 面板提交：成功后收起面板（失败时 composer 已经弹了错误提示，面板保留输入）。 */
async function handleCommentSubmit(content: string) {
  const sent = await handleCreateComment(content);
  if (sent) {
    closeCommentSheet();
  }
}

async function handleShare() {
  // 分享是纯前端 clipboard、不打任何接口，因此**不需要**登录前置（票 247）：
  // 游客复制文章链接和登录用户没有任何区别，弹登录框只会伤害体验。
  if (!import.meta.client) return;

  const shareUrl = `${window.location.origin}/article/${articleId.value}`;

  try {
    await navigator.clipboard.writeText(shareUrl);
    message.success("文章链接已复制");
  } catch {
    message.error("复制失败，请手动复制地址栏链接");
  }
}

function handleFavoriteUpdated(payload: ArticleFavoriteResult | null) {
  // 页面只上报收藏写响应（含空值）：权威计数回填、空值防御与客户端 30s SWR
  // 主动失效都由阅读页取数模块统一管（票 282、ADR-0016 决策 4/6）。
  reportArticleWrite({ kind: "favorite", result: payload });
}

function handleArticleScoreOpen() {
  articleScoreModalOpen.value = true;
  if (!articleScoreSummary.value) {
    void refreshArticleScoreSummary();
  }
}

async function handleAuthorFollow() {
  if (!(await requireLogin())) {
    return;
  }

  if (!authorId.value) {
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
    message.error(errorMessage(error, "关注操作失败"));
  }
}

async function handlePrivateMessage() {
  if (!(await requireLogin())) {
    return;
  }

  if (isSelfAuthor.value) {
    message.info("不能给自己发送私信。");
    return;
  }

  // 作者 id 来自正文（第一波就到），昵称与头像也在正文里；relation 来自第二波
  // 作者资料源，没到就按 0（陌生人）处理，不阻塞跳转。
  if (!authorId.value) {
    message.error("作者信息尚未就绪，请稍后再试");
    return;
  }

  void jumpToDraftChat({
    receiverId: authorId.value,
    receiverNickname: article.value?.author_name || "新会话",
    receiverAvatar: article.value?.author_avatar || "",
    relation: authorProfile.value?.relation ?? 0,
  });
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

const commentsPending = computed(() => commentsPager.pending.value);
// 页码不再上屏（决定清单 §3 第 16 条删掉了分页的页码与条数话术），只在翻页按钮的
// 可用态里用到「有没有上一页 / 下一页」。
const hasPreviousCommentPage = computed(() => commentsPager.hasPreviousPage.value);
const hasNextCommentPage = computed(() => commentsPager.hasNextPage.value);

async function handlePreviousCommentPage() {
  await commentsPager.goToPreviousPage();
}

async function handleNextCommentPage() {
  await commentsPager.goToNextPage();
}

onMounted(() => {
  markArticleViewed(articleId.value).catch(() => undefined);
  // 首屏 hydration 的 payload 是**匿名**的：点赞/收藏态不在里面（ADR-0016 约束 2）。
  // 登录初始化完成后必须补一次用户态请求（ADR-0016 决策 2/6），否则硬刷新后
  // 按钮永远显示"未赞"。这条请求不走 SWR、只投影私有字段，不污染共享缓存。
  // 页内登录成功那一路由 `useArticlePrivateSession` 的 watch 兜住（单飞去重）。
  void articlePrivateSession.initialize();
});

onUnmounted(() => {
  // 定时器归 composable 管，页面只负责通知它该收尾了。
  disposeArticleDigg();
});

// 机器读数（票 314 交付 4–6）：规范地址 / 分享卡片 / JSON-LD。
//
// 三样都只写 head，不动任何页面行为：地址栏仍然是 `?from=collection:5`，
// `route.query.from`（第 89 行）照常决定左侧抽屉显示哪棵树。
//
// 规范地址的域名取运行时 `siteUrl` 而不是从请求头反推：同一篇文章的
// `?from=` / `?utm_source=` 变体各自命中不同的 SSR 缓存键，但它们必须输出
// **同一条** canonical，否则"给搜索引擎去重"这件事本身会去重失败。
const runtimeConfig = useRuntimeConfig();
const siteUrl = computed(() => String(runtimeConfig.public.siteUrl || ""));
const canonicalUrl = computed(() => canonicalArticleUrl(siteUrl.value, articleId.value));

useSeoMeta({
  title: computed(() => article.value?.title || "文章详情"),
  description: computed(() => brandDescription(article.value?.abstract)),
  ogTitle: computed(() => article.value?.title || "文章详情"),
  ogDescription: computed(() => brandDescription(article.value?.abstract)),
  ogImage: computed(() => article.value?.cover || ""),
  // 票 314 交付 5：分享卡片声明成"一篇文章"（微信 / X / Discord 才会按文章卡片渲染，
  // 而不是当成"一个网站"）。og:url 用规范地址，与 canonical 同源。
  ogType: "article",
  ogUrl: canonicalUrl,
  // 发布时间取 `created_at`：库里没有 `published_at` 列（本票零 DDL 的硬约束），
  // 记录时间就是读者能看到的那一个"发布"时刻。空值不输出空标签。
  articlePublishedTime: computed(() => article.value?.created_at || undefined),
  // `article:author` 在 OG 里是可以重复的字段（多作者），所以这里给数组；空值时给
  // undefined 而不是空数组 —— 空数组会让 unhead 输出一条 `content` 为空的标签。
  articleAuthor: computed(() => (article.value?.author_name ? [article.value.author_name] : undefined)),
  articleTag: computed(() => (article.value?.tags?.length ? article.value.tags : undefined)),
});

// 票 314 交付 4/6：canonical 是 head 里的一行，不是一次跳转；Article 与 BreadcrumbList
// 埋成两条独立的 application/ld+json（带 key，水合与路由切换时原地替换，不叠加）。
useHead(() => {
  const current = article.value;

  return {
    link: [{ rel: "canonical", href: canonicalUrl.value }],
    script: current
      ? [
          {
            key: "seo-article-jsonld",
            type: "application/ld+json",
            innerHTML: JSON.stringify(
              articleJsonLd({
                url: canonicalUrl.value,
                siteUrl: siteUrl.value,
                siteName: brandFullName(),
                siteLogoUrl: brandAbsoluteAssetUrl(siteUrl.value, "logo"),
                title: current.title,
                abstract: current.abstract,
                cover: current.cover,
                createdAt: current.created_at,
                updatedAt: current.updated_at,
                authorName: current.author_name,
                authorId: current.author_id,
                tags: current.tags,
                categoryName: current.category_name,
              }),
            ),
          },
          {
            key: "seo-breadcrumb-jsonld",
            type: "application/ld+json",
            innerHTML: JSON.stringify(
              breadcrumbJsonLd({
                homeName: "首页",
                homeUrl: `${siteOrigin(siteUrl.value)}/`,
                articleTitle: current.title,
                articleUrl: canonicalUrl.value,
              }),
            ),
          },
        ]
      : [],
  };
});
</script>

<template>
  <div class="article-page">
    <!-- 切页先出骨架：正文还没到时按文章页版式占位，不再让 Suspense 等"数据齐了才切页"。 -->
    <ArticlePageSkeleton v-if="articleSkeleton" />

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

      <!-- <1024px 的互动条（决定清单 §3 第 13 条）：挂在 hero 下方，正文开头就能看见。
           ≥1024 由 CSS 让位给右栏那一份；两份不会同时可见，底部永远没有第三条。 -->
      <section class="article-paper article-actions-card article-actions-card--hero">
        <ArticleActionsBar
          v-bind="articleActionProps"
          variant="hero"
          @like="handleLike"
          @favorite="handleFavorite"
          @share="handleShare"
          @comment="handleCommentJump"
          @bottom="handleScrollToEnd"
          @top="handleScrollToTop"
        />
      </section>

      <!-- 目录不再占文章布局的一列（验收反馈：不要挤正文）：它是贴在窗口左边缘的
           可收起抽屉，见页面末尾的 ArticleOutlineAside。正文布局恒定两列。 -->
      <div class="article-layout">
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

            <!-- 评论输入面板的入口（决定清单 §4 第 18 条）：内嵌 textarea 退役，
                 这里只留一枚高对比按钮；点它（或互动条的「评论」、某个楼层的「回复」）都会
                 从窗口底部居中滑出同一个面板。 -->
            <div class="comment-panel__invite-row">
              <button
                type="button"
                class="article-action article-action--primary comment-panel__invite"
                data-role="comment-invite"
                @click="handleCommentInvite"
              >
                <IconMessageCirclePlus :size="18" aria-hidden="true" />
                <span>去发表评论</span>
              </button>
            </div>

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

            <!-- 分页行（决定清单 §3 第 16 条）：不显示分页的页码与条数话术（读者不需要"内部实现"），
                 翻页按钮保留，但只有第一页且没有下一页时整行都不渲染（不留一排点不动的按钮）。 -->
            <div
              v-if="comments.length && (hasPreviousCommentPage || hasNextCommentPage)"
              class="comment-panel__pagination comment-panel__pagination--bare"
            >
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
              <!-- 头像 / 昵称 / @id 三处都指向作者的公开主页：之前整张卡是死的，
                   读者看到作者想点进去，点哪儿都没反应（2026-11-12）。 -->
              <NuxtLink
                v-if="authorProfileLink"
                :to="authorProfileLink"
                class="article-author__avatar-link"
                :aria-label="`查看 ${article?.author_name} 的公开主页`">
                <AppAvatar
                  :size="64"
                  :src="article?.author_avatar"
                  :name="article?.author_name"
                  :fallback="authorInitial"
                />
              </NuxtLink>
              <AppAvatar
                v-else
                :size="64"
                :src="article?.author_avatar"
                :name="article?.author_name"
                :fallback="authorInitial"
              />
              <div class="article-author__identity">
                <div class="article-author__name-row">
                  <NuxtLink
                    v-if="authorProfileLink"
                    :to="authorProfileLink"
                    class="article-author__name article-author__name--link">
                    {{ article?.author_name }}
                  </NuxtLink>
                  <span v-else class="article-author__name">{{ article?.author_name }}</span>
                  <span class="article-chip">文章作者</span>
                </div>
                <p class="article-author__handle">
                  <NuxtLink
                    v-if="authorProfileLink"
                    :to="authorProfileLink"
                    class="article-author__handle-link">
                    @{{ article?.author_username }}
                  </NuxtLink>
                  <template v-else>@{{ article?.author_username }}</template>
                </p>
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
              @bottom="handleScrollToEnd"
              @top="handleScrollToTop"
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

      <!-- 窄屏兜底那条固定条已删除（决定清单 §3 第 12 条）：互动条只在右栏与 hero 下方各一份，
           宽度由 CSS 断点互斥，任何视口都只出现一份可见。 -->
    </template>

    <!-- 目录抽屉：贴在窗口左边缘，展开时覆盖正文（不占列、不挤正文），收起后只剩一条窄标。
         只有文章真的有归属树时才出现。 -->
    <ArticleOutlineAside
      v-if="outlineColumnVisible"
      :context="articleOutlineContext"
      :pending="articleOutlinePending"
      :current-article-id="articleId"
    />

    <ArticleScoreModal
      v-model:show="articleScoreModalOpen"
      title="文章质量评分摘要"
      :loading="articleScoreSummaryPending"
      :data="articleScoreSummary ?? null"
      empty-text="当前文章还没有公开评分摘要。"
    />

    <!-- 底部评论输入面板（决定清单 §4）：宽 min(92vw,640px)、居中、无暗色遮罩、
         点面板外 / Esc / × 都可关闭；提交成功后面板自动收起。 -->
    <CommentComposerSheet
      v-model:show="commentSheetOpen"
      :loading="commentsPending"
      :title="replyTarget ? `回复 ${replyTarget.nickname}` : '发表评论'"
      :placeholder="replyTarget ? '补充你的回复内容…' : '写下你的看法，帮助更多读者补齐思路。'"
      :submit-label="replyTarget ? '发送回复' : '发表评论'"
      @submit="handleCommentSubmit" />

    <FavoriteFolderModal
      v-model:show="favoriteModalOpen"
      :article-id="String(articleId)"
      :article-title="article?.title || '当前文章'"
      @updated="handleFavoriteUpdated"
    />
  </div>
</template>
