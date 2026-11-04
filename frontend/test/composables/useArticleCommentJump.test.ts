import { describe, expect, it, vi } from "vitest";

import { useArticleCommentJump } from "~/composables/useArticleCommentJump";

/**
 * 评论按钮的落点（#236 / 决定清单第 2 节与第 3 节第 7 条）。
 *
 * 互动条的评论按钮做两件事：滚到 `#comments`，再把焦点放进评论框。但**未登录时
 * 一件都不能做**——先弹登录框就结束（否则游客会被滚到评论区、对着一个发不出去的
 * 输入框；更糟的是照着旧习点提交，把本地登录态打成"失效"）。
 *
 * 这是页面里唯一需要"先问登录再动 DOM"的路径，所以把顺序收进一个可注入依赖的
 * composable：滚动 / 聚焦是副作用，用假实现断言顺序与"没被调用"比挂整页容易得多。
 */

interface Recorder {
  calls: string[];
  deps: {
    ensureLoggedIn: () => Promise<boolean>;
    openAuthModal: () => void;
    scrollToComments: () => void;
    focusComposer: () => void;
  };
}

function recorder(options: { loggedIn: boolean }): Recorder {
  const calls: string[] = [];
  return {
    calls,
    deps: {
      ensureLoggedIn: async () => options.loggedIn,
      openAuthModal: () => calls.push("auth"),
      scrollToComments: () => calls.push("scroll"),
      focusComposer: () => calls.push("focus"),
    },
  };
}

describe("useArticleCommentJump：未登录先弹登录框，不动 DOM", () => {
  it("游客点击：只打开登录框，不滚动、不聚焦", async () => {
    const { calls, deps } = recorder({ loggedIn: false });

    const { jumpToComment } = useArticleCommentJump(deps);
    const outcome = await jumpToComment();

    expect(calls).toEqual(["auth"]);
    expect(outcome).toBe("auth");
  });
});

describe("useArticleCommentJump：已登录滚到评论区并聚焦", () => {
  it("先滚到 #comments，再把焦点放进评论框（顺序固定）", async () => {
    const { calls, deps } = recorder({ loggedIn: true });

    const { jumpToComment } = useArticleCommentJump(deps);
    const outcome = await jumpToComment();

    expect(calls).toEqual(["scroll", "focus"]);
    expect(outcome).toBe("focused");
  });

  it("登录校验必须真的跑一次：不能拿本地的 isLoggedIn 猜（令牌可能已被服务端否证）", async () => {
    const ensureLoggedIn = vi.fn(async () => true);
    const { deps } = recorder({ loggedIn: true });

    const { jumpToComment } = useArticleCommentJump({ ...deps, ensureLoggedIn });
    await jumpToComment();

    expect(ensureLoggedIn).toHaveBeenCalledTimes(1);
  });

  it("重复点击不会叠加副作用之外的东西：每次只是滚一次 + 聚焦一次", async () => {
    const { calls, deps } = recorder({ loggedIn: true });

    const { jumpToComment } = useArticleCommentJump(deps);
    await jumpToComment();
    await jumpToComment();

    expect(calls).toEqual(["scroll", "focus", "scroll", "focus"]);
  });
});
