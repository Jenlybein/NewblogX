import { computed, ref } from "vue";
import { describe, expect, it, vi } from "vitest";

import { useArticleComments } from "~/composables/useArticleComments";
import type { CommentReplyItem, CommentRootItem } from "~/types/api";

/**
 * 评论区行为（#238）。
 *
 * 这块逻辑原本埋在 `pages/article/[id].vue` 里（`replyStates` / `replyTarget` /
 * 三个 handler 全靠页面闭包共享，零测试）。抽成 composable 的直接原因就是它错在
 * 了三个"看着对、用起来错"的地方：
 *
 *  1. 子评论的点赞按钮只是纯文本（后端其实早就支持二级点赞）；
 *  2. 点子评论的「回复」传的是**根评论 id**，于是回复落到根楼层；
 *  3. 点赞只刷根评论页，`replyStates` 从不重取 → 子评论计数不更新；
 *
 * 外加上 #238 决策 7：所有互动（点赞 / 评论提交）必须**先问登录态、未登录只弹框**，
 * 否则游客点一下就会把自己"已登录"打成失效。
 *
 * 这里不 mount 页面，只把取数与回调注入进来（与 `useArticleDigg.test.ts` 同一套
 * 假服务做法），断言"谁在什么时候被调用、参数是什么"。
 */

function makeRoot(overrides: Partial<CommentRootItem> = {}): CommentRootItem {
  return {
    id: "c1",
    user_id: "u1",
    user_nickname: "读者甲",
    user_avatar: "",
    content: "这是一条评论",
    created_at: "2026-10-30T10:00:00.000Z",
    digg_count: 3,
    is_digg: false,
    reply_count: 2,
    status: 1,
    ...overrides,
  } as CommentRootItem;
}

function makeReply(overrides: Partial<CommentReplyItem> = {}): CommentReplyItem {
  return {
    id: "r1",
    user_id: "u2",
    user_nickname: "读者乙",
    user_avatar: "",
    content: "楼中楼回复",
    created_at: "2026-10-30T11:00:00.000Z",
    digg_count: 1,
    is_digg: false,
    status: 1,
    reply_user_nickname: "读者甲",
    ...overrides,
  } as CommentReplyItem;
}

type ReplyPayload = { list: CommentReplyItem[]; has_more: boolean; reply_count: number };

function harness(options: { loggedIn?: boolean; replyPages?: Record<string, Record<number, ReplyPayload>> } = {}) {
  const rootItems = ref<CommentRootItem[]>([makeRoot()]);
  const pager = {
    currentPage: ref(1),
    currentItems: computed(() => rootItems.value),
    pending: ref(false),
    reset: vi.fn(),
    loadPage: vi.fn(async () => ({ items: rootItems.value, hasMore: false })),
    refreshCurrentPage: vi.fn(async () => ({ items: rootItems.value, hasMore: false })),
  };
  const replyPages = options.replyPages ?? {};
  const fetchReplies = vi.fn(async (_articleId: string, rootId: string, page: number): Promise<ReplyPayload> => {
    return replyPages[rootId]?.[page] ?? { list: [makeReply()], has_more: false, reply_count: 2 };
  });
  const createComment = vi.fn(async (): Promise<{ status?: number } | null> => ({ status: 2 }));
  const diggComment = vi.fn(async () => ({}) as unknown);
  const refreshArticle = vi.fn(async () => undefined);
  const openComposer = vi.fn();
  const notify = { success: vi.fn(), error: vi.fn() };
  const requireLogin = vi.fn(async () => options.loggedIn ?? true);
  const replyPageSize = 3;

  const section = useArticleComments({
    articleId: () => "a1",
    pager,
    requireLogin,
    fetchReplies,
    createComment,
    diggComment,
    refreshArticle,
    openComposer,
    notify,
    replyPageSize,
  });

  return {
    section,
    rootItems,
    pager,
    fetchReplies,
    createComment,
    diggComment,
    refreshArticle,
    openComposer,
    notify,
    requireLogin,
  };
}

describe("评论区：登录前置（未登录不发请求）", () => {
  it("未登录点评论点赞：一个接口都不打，也不提示成功", async () => {
    const { section, diggComment, pager, fetchReplies, notify, requireLogin } = harness({ loggedIn: false });

    await section.handleCommentDigg("c1", false);

    expect(requireLogin).toHaveBeenCalledTimes(1);
    expect(diggComment).not.toHaveBeenCalled();
    expect(fetchReplies).not.toHaveBeenCalled();
    expect(pager.refreshCurrentPage).not.toHaveBeenCalled();
    expect(notify.success).not.toHaveBeenCalled();
  });

  it("未登录提交评论：不发创建请求、不刷新任何列表", async () => {
    const { section, createComment, pager, refreshArticle, notify } = harness({ loggedIn: false });

    await section.handleCreateComment("游客想说话");

    expect(createComment).not.toHaveBeenCalled();
    expect(pager.reset).not.toHaveBeenCalled();
    expect(pager.loadPage).not.toHaveBeenCalled();
    expect(refreshArticle).not.toHaveBeenCalled();
    expect(notify.success).not.toHaveBeenCalled();
  });
});

describe("评论区：子评论点赞", () => {
  it("子评论点赞按被点的那条评论 id 发请求，并只重取**该楼层**的回复页（不整页重取）", async () => {
    const { section, diggComment, pager, fetchReplies } = harness();
    await section.handleLoadReplies("c1");

    await section.handleCommentDigg("r1", false);

    expect(diggComment).toHaveBeenCalledExactlyOnceWith("r1");
    // 第 1 次是展开时的首取，第 2 次是点赞后的重取：都是同一楼层、同一页。
    expect(fetchReplies).toHaveBeenCalledTimes(2);
    expect(fetchReplies.mock.calls[1]?.slice(0, 3)).toEqual(["a1", "c1", 1]);
    // 根评论页不重取：只刷新被点赞的那个楼层。
    expect(pager.refreshCurrentPage).not.toHaveBeenCalled();
  });

  it("多页时只重取当前页，其他已加载页保留（不把楼层打回第一页）", async () => {
    const { section, pager, fetchReplies, notify } = harness({
      replyPages: {
        c1: {
          1: { list: [makeReply({ id: "r1" })], has_more: true, reply_count: 4 },
          2: { list: [makeReply({ id: "r2" })], has_more: false, reply_count: 4 },
        },
      },
    });
    await section.handleLoadReplies("c1");
    await section.handleNextReplies("c1");

    expect(section.replyPages.value.c1).toBe(2);
    await section.handleCommentDigg("r1", true);

    expect(fetchReplies).toHaveBeenLastCalledWith("a1", "c1", 2, 3);
    expect(pager.refreshCurrentPage).not.toHaveBeenCalled();
    expect(notify.success).toHaveBeenCalledTimes(1);
    // 只重取第 2 页：第 1 页那份缓存仍在（加载页数保持 2，不回到 1）。
    expect(section.replyLoadedPages.value.c1).toBe(2);
    expect(section.replyPages.value.c1).toBe(2);
  });

  it("根评论点赞仍刷新根评论页，不动回复页", async () => {
    const { section, diggComment, pager, fetchReplies } = harness();

    await section.handleCommentDigg("c1", false);

    expect(diggComment).toHaveBeenCalledExactlyOnceWith("c1");
    expect(pager.refreshCurrentPage).toHaveBeenCalledTimes(1);
    expect(fetchReplies).not.toHaveBeenCalled();
  });

  it("点赞抛错时提示失败，不把错误抛给调用方", async () => {
    const { section, diggComment, notify } = harness();
    diggComment.mockRejectedValueOnce(new Error("评论不存在"));

    await section.handleCommentDigg("c1", false);

    expect(notify.error).toHaveBeenCalledWith("评论不存在");
    expect(notify.success).not.toHaveBeenCalled();
  });
});

describe("评论区：回复目标精确到被回复的那条评论", () => {
  it("点子评论的「回复」→ replyTarget 指该子评论（不是根评论），并把焦点交给评论框", async () => {
    const { section, openComposer } = harness();
    await section.handleLoadReplies("c1");

    section.handleReply("r1");

    expect(section.replyTarget.value).toEqual({ id: "r1", nickname: "读者乙", rootId: "c1" });
    expect(openComposer).toHaveBeenCalledTimes(1);
  });

  it("点根评论的「回复」→ replyTarget 仍是该根评论（rootId 与 id 相同）", () => {
    const { section, openComposer } = harness();

    section.handleReply("c1");

    expect(section.replyTarget.value).toEqual({ id: "c1", nickname: "读者甲", rootId: "c1" });
    expect(openComposer).toHaveBeenCalledTimes(1);
  });

  it("点到不存在的评论：不产生回复目标，也不去聚焦（避免标题空白）", () => {
    const { section, openComposer } = harness();

    section.handleReply("ghost");

    expect(section.replyTarget.value).toBeNull();
    expect(openComposer).not.toHaveBeenCalled();
  });
});

describe("评论区：提交", () => {
  it("提交新评论：不带 reply_id，刷新根评论页并清空回复目标", async () => {
    const { section, createComment, pager, refreshArticle, notify } = harness();

    await section.handleCreateComment("  我也来说两句  ");

    expect(createComment).toHaveBeenCalledExactlyOnceWith({ article_id: "a1", content: "我也来说两句" });
    expect(pager.reset).toHaveBeenCalledWith(1);
    expect(pager.loadPage).toHaveBeenCalledWith(1, true);
    expect(refreshArticle).toHaveBeenCalledTimes(1);
    expect(notify.success).toHaveBeenCalledWith("评论已发送");
    expect(section.replyTarget.value).toBeNull();
  });

  it("评论停在审核中时，提示不能是「已发送」（票 359）", async () => {
    // 机审观察词命中或 AI 判可疑时，后端返回 status=1（审核中）。
    // 这时列表里看不到这条评论，说"已发送"会让人以为丢了。
    const { section, createComment, notify } = harness();
    createComment.mockResolvedValueOnce({ status: 1 });

    await section.handleCreateComment("加微信聊");

    expect(notify.success).toHaveBeenCalledWith("评论已提交，审核通过后显示");
    expect(notify.success).not.toHaveBeenCalledWith("评论已发送");
  });

  it("回复根评论：reply_id 是该根评论的 id（落到它的楼层），刷新后清空回复目标", async () => {
    const { section, createComment, notify } = harness();
    section.handleReply("c1");

    await section.handleCreateComment("回复楼主");

    expect(createComment).toHaveBeenCalledExactlyOnceWith({ article_id: "a1", content: "回复楼主", reply_id: "c1" });
    expect(notify.success).toHaveBeenCalledWith("回复已发送");
    expect(section.replyTarget.value).toBeNull();
  });

  it("回复子评论：reply_id 是**子评论**的 id，且只重取该楼层的回复页", async () => {
    const { section, createComment, fetchReplies, notify } = harness();
    await section.handleLoadReplies("c1");
    section.handleReply("r1");

    await section.handleCreateComment("收到你的回复");

    expect(createComment).toHaveBeenCalledExactlyOnceWith({ article_id: "a1", content: "收到你的回复", reply_id: "r1" });
    expect(fetchReplies).toHaveBeenCalledTimes(2);
    // 第二次取数是"重置后重取该楼层第一页"，参数是根评论 id（不是子评论 id）。
    expect(fetchReplies.mock.calls[1]?.slice(0, 3)).toEqual(["a1", "c1", 1]);
    expect(notify.success).toHaveBeenCalledWith("回复已发送");
    expect(section.replyTarget.value).toBeNull();
  });

  it("提交失败只提示错误，不刷新列表、不丢回复目标（用户能重试）", async () => {
    const { section, createComment, pager, notify } = harness();
    section.handleReply("c1");
    createComment.mockRejectedValueOnce(new Error("内容太短"));

    await section.handleCreateComment("短");

    expect(pager.loadPage).not.toHaveBeenCalled();
    expect(notify.error).toHaveBeenCalledWith("内容太短");
    expect(section.replyTarget.value).toEqual({ id: "c1", nickname: "读者甲", rootId: "c1" });
  });
});

describe("评论区：回复分页与楼层合并", () => {
  it("展开回复后，当前页的子评论随楼层一起渲染", async () => {
    const { section } = harness();

    await section.handleLoadReplies("c1");

    expect(section.comments.value[0]?.replies?.map((reply) => reply.id)).toEqual(["r1"]);
  });

  it("下一页 / 上一页按页码取数，边界不越界", async () => {
    const { section, fetchReplies } = harness({ replyPages: {
      c1: {
        1: { list: [makeReply({ id: "r1" })], has_more: true, reply_count: 4 },
        2: { list: [makeReply({ id: "r2" })], has_more: false, reply_count: 4 },
      },
    } });

    await section.handleLoadReplies("c1");
    await section.handleNextReplies("c1");
    await section.handlePreviousReplies("c1");
    await section.handlePreviousReplies("c1");

    // 上一页命中已加载的第 1 页：不重复请求；第 1 页再往前是边界，直接不动作。
    expect(fetchReplies.mock.calls.map((call) => call[2])).toEqual([1, 2]);
    expect(section.replyPages.value.c1).toBe(1);
    expect(section.replyHasPrevious.value.c1).toBe(false);
    expect(section.replyHasNext.value.c1).toBe(true);
  });

  it("取回复失败时提示错误并结束加载态", async () => {
    const { section, fetchReplies, notify } = harness();
    fetchReplies.mockRejectedValueOnce(new Error("回复不可用"));

    await section.handleLoadReplies("c1");

    expect(notify.error).toHaveBeenCalledWith("回复不可用");
    expect(section.replyLoading.value.c1).toBe(false);
  });
});
