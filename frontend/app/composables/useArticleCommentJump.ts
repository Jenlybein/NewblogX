/**
 * 互动条「评论」按钮的落点（票 236 / 决定清单第 2 节、第 3 节第 7 条）。
 *
 * 点评论按钮要做两件事：滚到 `#comments`、把焦点放进评论框。但用户没登录时
 * **一件都不能做**——先弹登录框就结束：
 *
 *  - 滚过去会让游客对着一个发不出去的输入框；
 *  - 照着旧习点提交，会打接口拿到「请先登录」，`markClientAuthInvalid()` 再把
 *    `isLoggedIn` 打成假（`services/http/client.ts:170-172` 的老坑）。
 *
 * 顺序与副作用都收在这里而不是页面模板里：滚动 / 聚焦是 DOM 副作用，页面只提供
 * "怎么滚、怎么聚焦"两个闭包，这里负责"什么时候允许做"。测试可以用假闭包断言
 * 顺序与"没被调用"，不必挂载 700 行的 SSR 路由页。
 */

export interface ArticleCommentJumpDeps {
  /** 登录校验（页面传 `authStore.initializeSession`）：必须真查，不能拿本地标记猜。 */
  ensureLoggedIn: () => Promise<boolean>;
  /** 未登录时打开登录弹窗。 */
  openAuthModal: () => void;
  /** 滚到评论区（页面用 `#comments` 区块的 ref）。 */
  scrollToComments: () => void;
  /** 聚焦评论框（页面用 `CommentComposer` 暴露的 `focus()`）。 */
  focusComposer: () => void;
}

/** 点击结果：`auth` = 停在登录框；`focused` = 已滚到评论区并聚焦输入框。 */
export type ArticleCommentJumpOutcome = "auth" | "focused";

export function useArticleCommentJump(deps: ArticleCommentJumpDeps) {
  async function jumpToComment(): Promise<ArticleCommentJumpOutcome> {
    const loggedIn = await deps.ensureLoggedIn();
    if (!loggedIn) {
      deps.openAuthModal();
      return "auth";
    }

    // 先滚再聚焦：反过来浏览器会因为"把输入框拉进视野"再滚一次，落点会飘。
    deps.scrollToComments();
    deps.focusComposer();
    return "focused";
  }

  return { jumpToComment };
}
