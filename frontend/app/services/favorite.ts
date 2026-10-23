import type {
  FavoriteArticleListData,
  FavoriteFolderCreatePayload,
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
  return useNuxtApp()
    .$api.request<{
      list: Array<{
        id?: string;
        user_id?: string;
        title?: string;
        article_count?: number;
      }>;
      count: number;
    }>("/api/articles/favorite", {
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
      list: payload.list.map((item) => ({
        id: item.id ?? "",
        user_id: item.user_id ?? String(userId),
        title: item.title ?? "未命名收藏夹",
        cover: "",
        abstract: "",
        is_default: false,
        article_count: item.article_count ?? 0,
        has_article: false,
      })),
    }));
}

export function createFavoriteFolder(payload: { title: string; abstract: string; cover?: string }) {
  return useNuxtApp().$api.request<FavoriteFolderCreatePayload>("/api/articles/favorite", {
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

export function removeFavoriteFolderArticles(payload: { favoriteId: string; articleIds: string[] }) {
  return useNuxtApp().$api.request("/api/articles/favorite/contents", {
    method: "DELETE",
    body: {
      favorite_id: payload.favoriteId,
      articles: payload.articleIds,
    },
  });
}
