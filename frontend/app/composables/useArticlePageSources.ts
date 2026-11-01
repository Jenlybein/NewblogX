import { computed, toValue } from "vue";
import type { MaybeRefOrGetter } from "vue";
import { usePagedResourceCache } from "~/composables/usePagedResourceCache";
import { getArticleScoreSummary } from "~/services/ai";
import { getArticleAuthorInfo, getArticleDetail, getArticleDetailAnonymous } from "~/services/article";
import { getRootComments } from "~/services/comment";
import { getUserBaseInfo } from "~/services/user";
import type { ArticleAuthorInfo, ArticleDetail, CommentRootItem } from "~/types/api";
import { detectReaderRenderMode, type ReaderRenderMode } from "~/utils/readerRenderMode";
import { anonymousArticleView, anonymousCommentView } from "~/utils/ssrAnonymity";

export interface ArticleCommentsPage {
  items: CommentRootItem[];
  hasMore: boolean;
}

type ArticleScoreSummary = Awaited<ReturnType<typeof getArticleScoreSummary>>;
type ArticleAuthorProfile = Awaited<ReturnType<typeof getUserBaseInfo>>;

export interface ArticlePageSourcesOptions {
  articleId: MaybeRefOrGetter<string | number>;
  /** 覆盖运行环境判定（单测注入）；默认按 import.meta + isHydrating 判定。 */
  mode?: ReaderRenderMode;
  /** 取数入口可注入（单测用假请求，断言"谁在什么时候发起"）。 */
  fetchArticle?: (articleId: string) => Promise<ArticleDetail>;
  fetchScore?: (articleId: string) => Promise<ArticleScoreSummary | null>;
  fetchComments?: (articleId: string, page: number, limit: number) => Promise<ArticleCommentsPage>;
  fetchAuthorInfo?: (authorId: string) => Promise<ArticleAuthorInfo>;
  fetchAuthorProfile?: (authorId: string) => Promise<ArticleAuthorProfile | null>;
}

const ROOT_COMMENT_PAGE_SIZE = 7;

/**
 * 文章页的取数编排（ADR-0019 决策 3）。
 *
 * 病根：以前是 `await 正文 → await 作者资料 → await 评论 → await 评分`，
 * 一次切页 4 个接口严格首尾相接；而且作者资料只能靠"正文的 author_id"才知道取谁，
 * 天然是两段式。这里把它拆成两波：
 *
 * - **第一波（正文优先）**：正文 / 评论 / 评分都以路由里的文章 id 为参数，
 *   在这里同时发起 —— 正文先回来就先渲染正文，评论与评分随后填；
 * - **第二波（作者资料）**：正文到达后拿到 author_id，才发起作者资料；
 *   首屏 hydration 时作者统计已在 payload 里，只补作者主页资料（它本来就不服务端渲染）。
 *
 * 三种环境下的等待口径：
 * - 服务端：第一波等齐 + 第二波（作者统计）也等 —— SSR 的 HTML 不能缺内容；
 * - 首屏 hydration：只等评论分页（不在 payload 里，等不到会和 SSR 的列表打架）；
 * - 客户端跳转：一个都不等，setup 立刻返回 → 页面先画骨架，数据并行到达后自己填。
 *
 * 服务端还有一个额外口径（ADR-0019 决策 5 / 约束 2）：SSR 的 HTML 会进按 URL 键的
 * 30s 共享缓存，所以服务端取到的正文与评论都要先过一层匿名化 —— 匿名取数（不携带
 * 登录身份）+ 剔除 `is_digg` / `is_favor` / `relation`。客户端照旧拿带鉴权的数据，
 * 登录用户的点赞态由它自己补齐（先默认态、后跳变，ADR 已接受这个观感）。
 */
export async function useArticlePageSources(options: ArticlePageSourcesOptions) {
  const mode = options.mode ?? detectReaderRenderMode();
  // 第二波取数发生在 `await` 之后（正文回来才知道 author_id），那时已经不在 setup 的同步段里：
  // 服务端渲染中直接调用 `useNuxtApp()`（所有 service 内部都这么取 client）会丢 Nuxt 上下文。
  // 所以在同步段先抓住 nuxtApp，第二波用 runWithContext 把取数包回上下文里。
  const nuxtApp = useNuxtApp();
  const inNuxtContext = <T>(run: () => T) => nuxtApp.runWithContext(run);
  const articleId = computed(() => String(toValue(options.articleId)));

  // 服务端显式匿名取数：缓存键只有 URL，SSR 请求绝不能带登录身份（哪怕后端将来从
  // cookie 里认出用户，进 HTML 的也只能是匿名内容）。
  const fetchArticle =
    options.fetchArticle ??
    ((id: string) => (mode === "server" ? getArticleDetailAnonymous(id) : getArticleDetail(id)));
  const fetchScore =
    options.fetchScore ?? ((id: string) => getArticleScoreSummary(id).catch(() => null));
  const fetchComments =
    options.fetchComments ??
    (async (id: string, page: number, limit: number) => {
      const payload = await getRootComments(id, page, limit);
      return {
        items: payload.list,
        hasMore: payload.has_more,
      };
    });
  const fetchAuthorInfo = options.fetchAuthorInfo ?? ((authorId: string) => getArticleAuthorInfo(authorId));
  const fetchAuthorProfile =
    options.fetchAuthorProfile ?? ((authorId: string) => getUserBaseInfo(authorId).catch(() => null));

  // 用户态只在服务端剔除；客户端（跳转 / 首屏 hydration）要原样保留，靠它渲染"已点赞"。
  const articleForRender = (article: ArticleDetail) => (mode === "server" ? anonymousArticleView(article) : article);

  // 第一波：三个请求只依赖路由里的文章 id，**同时发起**。
  const articleSource = useAsyncData(() => `article-${articleId.value}`, () =>
    fetchArticle(articleId.value).then(articleForRender),
  );
  const scoreSource = useAsyncData<ArticleScoreSummary | null>(
    () => `article-score-summary:${articleId.value}`,
    () => fetchScore(articleId.value),
    { watch: [articleId], lazy: true },
  );

  const authorId = computed(() => articleSource.data.value?.author_id || "");

  // 第二波的两个源在**同一个同步段里建好**（`immediate: false` → 现在不取数，等正文回来再 refresh）。
  // 为什么必须建在这里、不能挪到下面 await 之后：`useAsyncData` 要求 setup/Nuxt 上下文，
  // 而跨过 `usePagedResourceCache` 的 await 再调用，服务端渲染里会丢上下文（实测 500）。
  // 另外它们都 `immediate: false`：key 依赖正文里的 author_id，让 key 变化/监听自动取数
  // 会和下面的显式第二波重复发请求（dedupe=cancel 只是把前一个取消掉）。
  const authorInfoSource = useAsyncData<ArticleAuthorInfo | null>(
    () => `article-author-info-${authorId.value || articleId.value}`,
    () => (authorId.value ? fetchAuthorInfo(authorId.value) : Promise.resolve(null)),
    { immediate: false },
  );
  const authorProfileSource = useAsyncData<ArticleAuthorProfile | null>(
    () => `article-author-base-${authorId.value || articleId.value}`,
    () => (authorId.value ? fetchAuthorProfile(authorId.value) : Promise.resolve(null)),
    { server: false, lazy: true, immediate: false },
  );

  // 评论分页首屏：客户端跳转时后台取（先出骨架），服务端与首屏 hydration 等它（SSR 内容 / 不闪）。
  // 服务端的评论条目同样要匿名化：评论的"我点过赞""和作者的关系"都是登录用户私有状态。
  const commentsPager = await usePagedResourceCache({
    cacheKey: () => `article-comments:${articleId.value}`,
    pageSize: () => ROOT_COMMENT_PAGE_SIZE,
    initialLoad: mode === "navigation" ? "background" : "block",
    fetchPage: async (page, limit) => {
      try {
        const payload = await fetchComments(articleId.value, page, limit);
        return mode === "server"
          ? { items: payload.items.map((comment) => anonymousCommentView(comment)), hasMore: payload.hasMore }
          : payload;
      } catch {
        // 评论区失败不抛错：评论是次要数据，不能把整页带崩。
        return { items: [], hasMore: false };
      }
    },
  });

  const authorWave = (async () => {
    await articleSource;
    if (!authorId.value) {
      return;
    }

    const tasks: Array<Promise<unknown>> = [];
    if (mode !== "server") {
      // 作者主页资料本来就不进 SSR（server: false），首屏与切页都在客户端补齐。
      tasks.push(inNuxtContext(() => authorProfileSource.refresh()));
    }
    if (mode !== "hydration") {
      // 统计三项由服务端渲染 / 由第二波补：hydrating 时 payload 里已经有，不重复请求。
      tasks.push(inNuxtContext(() => authorInfoSource.refresh()));
    }
    await Promise.allSettled(tasks);
  })();

  if (mode === "server") {
    await Promise.all([articleSource, scoreSource, commentsPager.ready, authorWave]);
  }

  return {
    article: articleSource.data,
    articlePending: articleSource.pending,
    articleError: articleSource.error,
    refreshArticle: articleSource.refresh,
    authorInfo: authorInfoSource.data,
    refreshAuthorInfo: authorInfoSource.refresh,
    authorProfile: authorProfileSource.data,
    refreshAuthorProfile: authorProfileSource.refresh,
    articleScoreSummary: scoreSource.data,
    articleScoreSummaryPending: scoreSource.pending,
    refreshArticleScoreSummary: scoreSource.refresh,
    commentsPager,
  };
}
