import { computed, shallowRef } from "vue";
import type { ComputedRef, Ref } from "vue";
import type { CommentReplyItem, CommentRootItem } from "~/types/api";
import { errorMessage } from "~/utils/errorMessage";

/**
 * 评论区单片机（#238）。
 *
 * 为什么从 `pages/article/[id].vue` 抽出来：这块逻辑由 `replyStates` / `replyTarget`
 * 和四个 handler 靠页面闭包共享，之前零测试，而它正好错在三处"看着对、用起来错"：
 *
 *  1. 子评论点赞按钮只是纯文本（后端其实早就支持二级点赞），点完只刷根评论页，
 *     `replyStates` 从不重取 → 子评论计数永远不动；
 *  2. 子评论的「回复」传的是**根评论 id** → 回复落到根楼层，不是被回复的那条；
 *  3. 互动没有登录前置（游客点一下 → 请求 → `markPossiblyExpired()` 把登录态打假）。
 *
 * 现在的口径：
 *  - **回复目标精确到被回复的评论**（`replyTarget = {id, nickname, rootId}`），
 *    点「回复」顺带把页面交给输入面板（`openComposer`，见决定清单 §4）；
 *  - **点赞后只重取相关楼层**：子评论 → 该 root 的当前回复页；根评论 → 根评论页；
 *  - **所有互动先过登录闸门**（`requireLogin`）：未登录时一个请求都不发。
 *
 * 依赖全部注入（与 `useArticleDigg` 同一套做法），页面只负责接线，测试用假服务
 * 断言"谁在什么时候被调用、参数是什么"。
 */

/** 分页载荷（与 `usePagedResourceCache` 的页结构同形）。 */
export interface CommentPage<Item> {
  items: Item[];
  hasMore: boolean;
}

/** 根评论分页句柄（真实实现来自 `usePagedResourceCache`）。 */
export interface ArticleCommentsPager {
  currentPage: Ref<number>;
  currentItems: ComputedRef<CommentRootItem[]>;
  pending: Ref<boolean>;
  reset: (page?: number) => void;
  loadPage: (page?: number, force?: boolean) => Promise<CommentPage<CommentRootItem> | undefined>;
  refreshCurrentPage: () => Promise<CommentPage<CommentRootItem> | undefined>;
}

/** 回复页取数响应（`GET /api/comments/replies` 的投影）。 */
export interface ReplyListPayload {
  list: CommentReplyItem[];
  has_more: boolean;
  reply_count?: number;
}

/** 页面上被回复的那条评论：`rootId` 是它所属的楼层（根评论自己时等于 id）。 */
export interface CommentReplyTarget {
  id: string;
  nickname: string;
  rootId: string;
}

export interface ArticleCommentsDeps {
  articleId: () => string;
  pager: ArticleCommentsPager;
  /** 登录前置闸门：false 时调用方不得发任何互动请求。 */
  requireLogin: () => Promise<boolean>;
  fetchReplies: (articleId: string, rootId: string, page: number, limit: number) => Promise<ReplyListPayload>;
  createComment: (payload: { article_id: string; content: string; reply_id?: string }) => Promise<unknown>;
  diggComment: (commentId: string) => Promise<unknown>;
  /** 评论/回复成功后刷新正文（评论数在正文上）。 */
  refreshArticle: () => Promise<unknown> | unknown;
  /** 点「回复」后把握手权交给页面：打开底部评论输入面板（并聚焦）。 */
  openComposer: () => void;
  notify: { success: (message: string) => void; error: (message: string) => void };
  /** 回复每页条数，默认 3（与后端既有分页一致）。 */
  replyPageSize?: number;
}

interface ReplyPagerState {
  currentPage: number;
  pages: Record<number, CommentPage<CommentReplyItem>>;
  pending: boolean;
  replyCount: number;
}

interface CommentLocation {
  rootId: string;
  commentId: string;
  nickname: string;
  isReply: boolean;
}

export function useArticleComments(deps: ArticleCommentsDeps) {
  const replyPageSize = deps.replyPageSize ?? 3;
  const replyStates = shallowRef<Record<string, ReplyPagerState>>({});
  const replyTarget = shallowRef<CommentReplyTarget | null>(null);

  function commitState(rootId: string, state: ReplyPagerState) {
    replyStates.value = { ...replyStates.value, [rootId]: state };
  }

  function ensureReplyState(rootId: string): ReplyPagerState {
    const current = replyStates.value[rootId];
    if (current) {
      return current;
    }

    const nextState: ReplyPagerState = { currentPage: 1, pages: {}, pending: false, replyCount: 0 };
    commitState(rootId, nextState);
    return nextState;
  }

  function resetReplyState(rootId: string): ReplyPagerState {
    const current = ensureReplyState(rootId);
    current.currentPage = 1;
    current.pages = {};
    current.pending = false;
    current.replyCount = 0;
    commitState(rootId, current);
    return current;
  }

  /**
   * 在"已渲染的根评论 + 该楼层已加载的回复页"里找被点的评论。
   *
   * 找到回复时一并带回 `rootId`：回复分页与"只重取该楼层"都靠它，
   * 单看子评论 id 是推不出所属楼层的。
   */
  function locateComment(commentId: string): CommentLocation | null {
    for (const root of deps.pager.currentItems.value) {
      if (root.id === commentId) {
        return { rootId: root.id || "", commentId: root.id || "", nickname: root.user_nickname || "", isReply: false };
      }
    }

    for (const [rootId, state] of Object.entries(replyStates.value)) {
      for (const page of Object.values(state.pages)) {
        const reply = page.items.find((item) => item.id === commentId);
        if (reply) {
          return { rootId, commentId: reply.id || "", nickname: reply.user_nickname || "", isReply: true };
        }
      }
    }

    return null;
  }

  async function loadReplyPage(rootId: string, page = 1, force = false) {
    const state = ensureReplyState(rootId);
    if (!force && state.pages[page]) {
      state.currentPage = page;
      commitState(rootId, state);
      return state.pages[page];
    }

    state.pending = true;
    commitState(rootId, state);

    try {
      const data = await deps.fetchReplies(deps.articleId(), rootId, page, replyPageSize);
      state.pages = { ...state.pages, [page]: { items: data.list, hasMore: data.has_more } };
      state.replyCount = data.reply_count ?? state.replyCount;
      state.currentPage = page;
      commitState(rootId, state);
      return state.pages[page];
    } catch (error) {
      deps.notify.error(errorMessage(error, "加载回复失败"));
      return null;
    } finally {
      state.pending = false;
      commitState(rootId, state);
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

  /**
   * 点某条评论的「回复」。
   *
   * 关键点：目标就是**被点的那条**（`location.commentId`），不是它所属的根评论。
   * 同时把"打开输入面板 + 聚焦"交给页面（`openComposer`），否则用户点了回复
   * 还得自己往下滚去找输入框。
   */
  function handleReply(commentId: string) {
    const location = locateComment(commentId);
    if (!location) {
      return;
    }

    replyTarget.value = {
      id: location.commentId,
      nickname: location.nickname,
      rootId: location.rootId,
    };
    deps.openComposer();
  }

  function clearReplyTarget() {
    replyTarget.value = null;
  }

  /**
   * 评论点赞（根评论与子评论共用，后端 `POST /comments/{id}/digg` 不区分层级）。
   *
   * 先过登录闸门：未登录时只弹登录框，**不发请求**。成功之后只重取相关楼层：
   * 子评论 → 它所属 root 的当前回复页（只一页，不把整片评论区重取一遍）；
   * 根评论 → 根评论当前页。
   */
  async function handleCommentDigg(commentId: string, isDigg: boolean) {
    if (!(await deps.requireLogin())) {
      return;
    }

    try {
      await deps.diggComment(commentId);
      deps.notify.success(isDigg ? "已取消评论点赞" : "评论点赞成功");
    } catch (error) {
      deps.notify.error(errorMessage(error, "评论点赞失败"));
      return;
    }

    const location = locateComment(commentId);
    if (location?.isReply) {
      const state = ensureReplyState(location.rootId);
      await loadReplyPage(location.rootId, state.currentPage || 1, true);
      return;
    }

    await deps.pager.refreshCurrentPage();
  }

  /**
   * 发表评论 / 回复。
   *
   * `reply_id` 必须是**被回复的那条评论**的 id（回复子评论时不能传根 id），
   * 刷新也回到那条评论所属楼层的回复页。未登录时不发请求，也不刷新任何列表。
   */
  async function handleCreateComment(content: string): Promise<boolean> {
    const trimmed = content.trim();
    if (!trimmed) {
      return false;
    }

    if (!(await deps.requireLogin())) {
      return false;
    }

    const target = replyTarget.value;
    try {
      await deps.createComment({
        article_id: deps.articleId(),
        content: trimmed,
        ...(target ? { reply_id: target.id } : {}),
      });
      deps.notify.success(target ? "回复已发送" : "评论已发送");
      replyTarget.value = null;
      deps.pager.reset(1);
      await deps.pager.loadPage(1, true);
      if (target) {
        resetReplyState(target.rootId);
        await loadReplyPage(target.rootId, 1, true);
      }
      await deps.refreshArticle();
      return true;
    } catch (error) {
      deps.notify.error(errorMessage(error, "评论失败"));
      return false;
    }
  }

  const comments = computed(() =>
    deps.pager.currentItems.value.map((comment) => {
      const state = replyStates.value[comment.id || ""];
      return {
        ...comment,
        replies: state?.pages[state.currentPage]?.items || [],
      };
    }),
  );

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
    Object.fromEntries(
      Object.entries(replyStates.value).map(([rootId, state]) => [rootId, Object.keys(state.pages).length]),
    ),
  );

  return {
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
  };
}
