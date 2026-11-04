import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { findColorLiterals } from "../../../scripts/check-design-tokens.mjs";

/**
 * 阅读页评论区的接线（票 238）。
 *
 * 页面是 700+ 行的 SSR 路由页，整页 mount 会拖进整套取数 composable（既有先例
 * `test/pages/articleEditEntry.test.ts` / `articleOutlineColumn.test.ts` 都只断言
 * 页面源码里的接线）。行为由三处可单测的模块覆盖：
 *   - `test/composables/useArticleComments.test.ts`（点赞/回复/提交/登录前置）；
 *   - `test/composables/useReaderLoginGate.test.ts` + `test/utils/readerLoginGate.test.ts`；
 *   - `test/components/CommentThread.test.ts` / `CommentComposer.test.ts`。
 *
 * 这里只钉住"页面没有把逻辑长回自己身上"：
 *   1. 评论状态与交互都走 `useArticleComments`，页面不再自己维护 `replyStates`；
 *   2. 评论框拿到 ref 与登录态（聚焦由 composable 通过 `focusInput()` 触发）；
 *   3. 分享是纯前端 clipboard、不打接口，**不需要**登录前置（#247）；
 *      关注 / 收藏 / 评论提交仍先过统一登录闸门；
 *   4. 互动请求只在注入的服务里发生，页面里没有直接的调用点。
 */

const PAGE_FILE = "app/pages/article/[id].vue";

function pageSource(): string {
  return readFileSync(join(process.cwd(), PAGE_FILE), "utf8");
}

/** 截出某个函数体的源码（从 `function name(` 到下一个顶层 `}` 之前）。 */
function functionSource(source: string, name: string): string {
  const start = source.indexOf(`function ${name}(`);
  expect(start, `页面里找不到 function ${name}`).toBeGreaterThan(-1);
  const end = source.indexOf("\n}\n", start);
  expect(end, `function ${name} 没有闭合`).toBeGreaterThan(start);
  return source.slice(start, end + 2);
}

/** 截出评论面板模板（`id="comments"` 到收尾 `</section>`）。 */
function commentPanelSource(source: string): string {
  const start = source.indexOf('id="comments"');
  expect(start, "页面里找不到评论区 section").toBeGreaterThan(-1);
  const end = source.indexOf("</section>", start);
  expect(end, "评论区 section 没有闭合").toBeGreaterThan(start);
  return source.slice(start, end);
}

describe("文章页：评论区接线", () => {
  it("评论状态与交互走 useArticleComments，页面不再自己维护 replyStates", () => {
    const source = pageSource();

    expect(source).toContain('import { useArticleComments } from "~/composables/useArticleComments";');
    expect(source).toMatch(/useArticleComments\(\{/u);
    // 旧实现把 replyStates / loadReplyPage / handleReply 等全写在页面里。
    expect(source).not.toMatch(/replyStates/u);
    expect(source).not.toMatch(/function handleReply\(/u);
    expect(source).not.toMatch(/function loadReplyPage\(/u);
  });

  it("注入的服务齐全：回复取数 / 发评论 / 点赞 / 登录闸门 / 打开输入面板", () => {
    const source = pageSource();
    const block = source.slice(source.indexOf("useArticleComments({"), source.indexOf("});", source.indexOf("useArticleComments({")));

    for (const dep of [
      "articleId:",
      "pager: commentsPager",
      "requireLogin",
      "fetchReplies:",
      "createComment",
      "diggComment",
      "refreshArticle",
      "openComposer:",
      "notify: message",
    ]) {
      expect(block, `useArticleComments 缺少依赖 ${dep}`).toContain(dep);
    }
  });

  it("内嵌输入框换成「去发表评论」按钮；面板挂在页面根、点它是开面板的唯一入口", () => {
    const source = pageSource();
    const panel = commentPanelSource(source);

    // 内嵌 textarea 退役：面板外只剩一枚入口按钮。
    expect(panel).not.toContain("<CommentComposer");
    expect(panel).toContain('data-role="comment-invite"');
    expect(panel).toContain("@click=\"handleCommentInvite\"");

    // 面板挂在页面根（与评分弹窗同级），提交走 handleCommentSubmit。
    expect(source).toContain("<CommentComposerSheet");
    expect(source).toContain('@submit="handleCommentSubmit"');
    expect(source).toContain("openComposer: () => openCommentSheet()");
    expect(source).not.toMatch(/document\.querySelector\(|document\.getElementById\(/u);
  });

  it("点赞（含子评论）与回复都接到 composable 的 handler", () => {
    const panel = commentPanelSource(pageSource());

    expect(panel).toContain("@digg=\"handleCommentDigg\"");
    expect(panel).toContain("@reply=\"handleReply\"");
    expect(panel).toContain("@load-replies=\"handleLoadReplies\"");
    expect(panel).toContain("@next-replies=\"handleNextReplies\"");
    expect(panel).toContain("@previous-replies=\"handlePreviousReplies\"");
    // 提交已搬进底部面板：面板外这块只剩点赞 / 回复 / 回复分页的接线。
    expect(panel).not.toContain("@submit=");
    expect(panel).not.toContain("@cancel=");
  });
});

describe("文章页：互动统一登录前置（未登录不打接口）", () => {
  it("页面用同一份 useReaderLoginGate", () => {
    const source = pageSource();

    expect(source).toContain('import { useReaderLoginGate } from "~/composables/useReaderLoginGate";');
    expect(source).toMatch(/const requireLogin = useReaderLoginGate\(\);/u);
  });

  it("分享：不需要登录，直接读剪贴板（纯前端操作、不打接口）", () => {
    const share = functionSource(pageSource(), "handleShare");

    // #247：游客也应该能复制文章链接。分享不调接口，登录前置只会挡住它。
    expect(share).not.toContain("requireLogin()");
    expect(share).not.toMatch(/initializeSession|openAuthModal/u);
    expect(share).toContain("clipboard");
  });

  it("收藏 / 关注：不再各自写一遍 initializeSession + openAuthModal", () => {
    const source = pageSource();

    for (const name of ["handleFavorite", "handleAuthorFollow"]) {
      const body = functionSource(source, name);
      expect(body, `${name} 没走统一闸门`).toContain("requireLogin()");
      expect(body, `${name} 仍在自己弹登录框`).not.toContain("openAuthModal");
      expect(body, `${name} 仍在自己初始化会话`).not.toContain("initializeSession");
    }
  });

  it("页面里没有直接的互动请求调用（请求只发生在注入的服务里）", () => {
    const source = pageSource();

    // 关注是页面上唯一直接调服务的互动：它在闸门之后才调（见下一条）。
    const follow = functionSource(source, "handleAuthorFollow");
    expect(follow.indexOf("requireLogin()")).toBeLessThan(follow.indexOf("await followUser("));
    // 评论点赞 / 发评论的调用点都在 composable 里：页面只是把服务注入进去。
    // （回复取数例外：页面把 `getReplyComments` 包成 `fetchReplies` 交给 composable。）
    expect(source).not.toMatch(/diggComment\(/u);
    expect(source).not.toMatch(/createComment\(/u);
    expect(source).toMatch(/fetchReplies: \(id, rootId, page, limit\) => getReplyComments\(id, rootId, page, limit\)/u);
  });

  it("评论提交的登录前置在 composable 里（页面只接线，不再自己判 isLoggedIn）", () => {
    const source = pageSource();

    expect(source).not.toMatch(/if \(!authStore\.isLoggedIn\)/u);
  });
});

describe("文章页：不为这次改造引入新的颜色来源", () => {
  it("页面没有裸色值", () => {
    expect(findColorLiterals(pageSource())).toEqual([]);
  });
});
