import { useNuxtApp } from "#imports";
import type {
  ArticleAuthorInfo,
  ArticleCreateResult,
  ArticleDetail,
  ArticleDiggResult,
  ArticleFavoriteResult,
  ArticleHotTopListData,
  ArticleWritePayload,
} from "~/types/api";
import { pickArticlePrivateState, type ArticlePrivateState } from "~/utils/readerPrivateFields";

/**
 * 首页「热门置顶」运营位（公开读）。
 *
 * 顺序由后端按管理员手动排的顺序返回，前端不排序。旧实现调 `/articles/top` 并塞
 * `type: 2`（v1 的全站运营置顶参数，v2 后端根本不认），所以这个区块一直是空的（#67）。
 */
export function getHotTopArticles() {
  return useNuxtApp().$api.request<ArticleHotTopListData>("/api/articles/hot-top", {
    auth: false,
  });
}

export function getArticleDetail(id: string | number) {
  return useNuxtApp().$api.request<ArticleDetail>(`/api/articles/${id}`);
}

/**
 * 文章详情的**匿名**取数（ADR-0019 决策 5 / 约束 2）。
 *
 * 返回体里可能带 `is_digg`（"我有没有点过赞"）。服务端渲染的 HTML 要进按 URL 键的
 * 共享缓存，所以 SSR 取数必须显式匿名（`auth: false`），再叠加
 * `withoutPrivateFields` 把用户态字段整个剔掉；客户端首屏/跳转仍走
 * {@link getArticleDetail}，用真实登录态补齐。
 */
export function getArticleDetailAnonymous(id: string | number) {
  return useNuxtApp().$api.request<ArticleDetail>(`/api/articles/${id}`, { auth: false });
}

/**
 * 文章页的**用户私有态**取数（ADR-0024 决策 2）。
 *
 * 复用带鉴权的详情接口，但只投影 `is_digg` / `is_favor` / `relation`：
 * - 显式调用 `getArticleDetail`（默认 auth，不传 `auth: false`），否则匿名视角会
 *   把"未赞"当成用户态；
 * - 结果不进 SSR payload（调用方 `server: false`），也不进 30s SWR；
 * - 只投影私有字段，正文/计数/作者资料继续由各自匿名源负责，避免两份数据打架。
 */
export function getArticlePrivateState(id: string | number): Promise<ArticlePrivateState> {
  return getArticleDetail(id).then((detail) => pickArticlePrivateState(detail as unknown as object));
}

export function getArticleAuthorInfo(authorId: string | number) {  return useNuxtApp().$api.request<ArticleAuthorInfo>("/api/articles/author_info", {
    auth: false,
    query: {
      author_id: String(authorId),
    },
  });
}

export function markArticleViewed(id: string | number) {
  return useNuxtApp().$api.request("/api/articles/view", {
    method: "POST",
    body: {
      article_id: String(id),
    },
  });
}

/**
 * 设置点赞目标态（ADR-0024 决策 3）。
 *
 * 旧接口是 toggle（不带请求体）：按钮显示"未赞"时再点一次会把真实点赞取消掉。
 * 现在传 `{liked}`，服务端按目标收敛、重复同态无操作；响应带回权威 `digg_count`，
 * 前端直接 patch，不再本地 +1 / 等 30 秒刷新。
 */
export function setArticleDigg(id: string | number, liked: boolean) {
  return useNuxtApp().$api.request<ArticleDiggResult>(`/api/articles/${id}/digg`, {
    method: "PUT",
    body: { liked },
  });
}

/**
 * 收藏文章（写响应回填权威 `favor_count` / `is_favor`，见 ADR-0024 决策 4）。
 */
export function favoriteArticle(articleId: string | number, favorId?: string | number) {
  return useNuxtApp().$api.request<ArticleFavoriteResult>("/api/articles/favorite", {
    method: "POST",
    body: {
      article_id: String(articleId),
      ...(favorId ? { favor_id: String(favorId) } : {}),
    },
  });
}

export function createArticle(payload: ArticleWritePayload) {
  return useNuxtApp().$api.request<ArticleCreateResult>("/api/articles", {
    method: "POST",
    body: payload,
  });
}

export function updateArticle(
  id: string | number,
  payload: Partial<Omit<ArticleWritePayload, "status">> & {
    status?: 1 | 2;
  },
) {
  return useNuxtApp().$api.request(`/api/articles/${id}`, {
    method: "PUT",
    body: payload,
  });
}

export function deleteOwnArticle(id: string | number) {
  return useNuxtApp().$api.request(`/api/articles/${id}`, {
    method: "DELETE",
  });
}
