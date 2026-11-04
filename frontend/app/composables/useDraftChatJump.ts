import { useRouter } from "vue-router";

import type { InboxDraftSessionSeed } from "~/utils/chat";
import { buildDraftChatQuery } from "~/utils/chat";

/**
 * 「发私信」跨页跳转（票 265 / spec 269 D-1）。
 *
 * 作者主页 `users/[id].vue` 与文章页 `article/[id].vue` 共用这一处：
 * 四个 `draft_*` 参数由 `buildDraftChatQuery` 单一来源生成，路由目标固定
 * `/messages/chat`。这样两个入口不会出现"一个接了、另一个还在弹 toast"的偏科。
 *
 * 登录前置、自我判断、资料是否加载成功仍留在各页面做（各自的资料源不同），
 * 这里只负责「已经可以发了 → 跳过去」这最后一步。
 */
export function useDraftChatJump() {
  const router = useRouter();

  function jumpToDraftChat(seed: InboxDraftSessionSeed) {
    return router.push({
      path: "/messages/chat",
      query: buildDraftChatQuery(seed),
    });
  }

  return { jumpToDraftChat };
}
