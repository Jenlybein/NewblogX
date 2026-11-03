import type { HistoryArticleListData } from "~/types/api";

/**
 * 浏览历史（`/history` 独立页，见 #241）。
 *
 * 与 `services/studio.ts` 分开：历史已经不属于创作区（个人中心不再有入口），
 * 服务面跟着路由走，否则"删掉旧页面"之后还会留下一处指向 studio 的依赖。
 *
 * 分页是**游标**（before_updated_at + before_id），不是 page：历史页按天分组，
 * offset 分页在删除或重复浏览把某条顶到最前之后会让下一页整体位移，分组因此跨页断开。
 */
export function getHistoryArticles(
  params: {
    keyword?: string;
    beforeUpdatedAt?: string;
    beforeId?: string;
    limit?: number;
  } = {},
) {
  return useNuxtApp().$api.request<HistoryArticleListData>("/api/articles/history", {
    query: {
      keyword: params.keyword || undefined,
      before_updated_at: params.beforeUpdatedAt || undefined,
      before_id: params.beforeId || undefined,
      limit: params.limit ?? 20,
    },
  });
}

/** 删除若干条浏览历史（卡片右上角垃圾桶 / 批量删除共用）。 */
export function deleteHistoryArticles(idList: string[]) {
  return useNuxtApp().$api.request("/api/articles/history", {
    method: "DELETE",
    body: {
      id_list: idList,
    },
  });
}

/** 清空全部浏览历史（不可逆，页面必须二次确认）。 */
export function clearHistoryArticles() {
  return useNuxtApp().$api.request("/api/articles/history/all", {
    method: "DELETE",
  });
}
