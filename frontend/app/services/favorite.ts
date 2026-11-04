import type {
  ArticleFavoriteRemoveResult,
  FavoriteArticleListData,
  FavoriteFolderWritePayload,
  FavoriteFolderItem,
  FavoriteFolderListData,
} from "~/types/api";

export function getOwnFavoriteFolders(articleId?: string | number) {
  return useNuxtApp().$api.request<FavoriteFolderListData>("/api/articles/favorite", {
    query: {
      type: 1,
      page: 1,
      limit: 100,
      ...(articleId ? { article_id: String(articleId) } : {}),
    },
  });
}

export function getPublicFavoriteFolders(userId: string | number) {
  // 与 getOwnFavoriteFolders 打的是同一个接口（只差 type 参数），所以响应类型也一样。
  // 之前这里手写了一份匿名的 4 字段类型，等于把 FavoriteFolderItem 抄了一遍还不完整，
  // 导致同一份数据在两个调用路径上是两个不同的类型。
  return useNuxtApp()
    .$api.request<FavoriteFolderListData>("/api/articles/favorite", {
      auth: false,
      query: {
        type: 2,
        user_id: String(userId),
        page: 1,
        limit: 100,
      },
    })
    .then((payload) => ({
      count: payload.count,
      list: payload.list.map<FavoriteFolderItem>((item) => ({
        id: item.id ?? "",
        user_id: item.user_id ?? String(userId),
        title: item.title ?? "未命名收藏夹",
        cover: item.cover ?? "",
        abstract: item.abstract ?? "",
        is_default: item.is_default ?? false,
        article_count: item.article_count ?? 0,
        has_article: item.has_article ?? false,
        nickname: item.nickname ?? "",
        avatar: item.avatar ?? "",
        created_at: item.created_at ?? "",
        updated_at: item.updated_at ?? "",
      })),
    }));
}

export function createFavoriteFolder(payload: FavoriteFolderWritePayload) {
  // 后端返回 `Envelope[EmptyData]`（只有消息，没有 data），所以这里不声明返回对象。
  return useNuxtApp().$api.request<unknown>("/api/articles/favorite", {
    method: "PUT",
    body: {
      title: payload.title,
      abstract: payload.abstract,
      cover: payload.cover ?? "",
    },
  });
}

export function updateFavoriteFolder(payload: { id: string; title: string; abstract: string; cover?: string }) {
  return useNuxtApp().$api.request("/api/articles/favorite", {
    method: "PUT",
    body: {
      id: payload.id,
      title: payload.title,
      abstract: payload.abstract,
      cover: payload.cover ?? "",
    },
  });
}

export function deleteFavoriteFolders(idList: string[]) {
  return useNuxtApp().$api.request("/api/articles/favorite", {
    method: "DELETE",
    body: {
      id_list: idList,
    },
  });
}

export function getFavoriteFolderArticles(payload: { favoriteId: string; page?: number; limit?: number }) {
  return useNuxtApp().$api.request<FavoriteArticleListData>("/api/articles/favorite/contents", {
    auth: false,
    query: {
      favorite_id: payload.favoriteId,
      page: payload.page ?? 1,
      limit: payload.limit ?? 12,
    },
  });
}

/**
 * 从收藏夹移除文章（ADR-0024 决策 6；评审票 248 的 P0-2 / P0-3）。
 *
 * 「已收藏 → 移除」必须走 DELETE：旧实现误发 POST，后端发现关系已存在后返回
 * `ErrItemExists`（"已收藏该文章"），用户点"移出"反而失败、关系也没动。
 *
 * 响应形状是**批量**口径：`{is_favor, favor_count, removed_count, failed_count}`。
 * 全部失败时后端返回非 0 业务码（不再是"成功 + 空 data"），调用方会进 catch；
 * 部分成功时靠 `failed_count > 0` 分辨，不能只看 toast。
 */
export function removeFavoriteFolderArticles(payload: { favoriteId: string; articleIds: string[] }) {
  return useNuxtApp().$api.request<ArticleFavoriteRemoveResult>("/api/articles/favorite/contents", {
    method: "DELETE",
    body: {
      favorite_id: payload.favoriteId,
      articles: payload.articleIds,
    },
  });
}
