import { useNuxtApp } from "#imports";
import type { OptionItem, SearchArticleResponse } from "~/types/api";

export interface SearchArticlesParams {
  type?: 1 | 2 | 3 | 4 | 5;
  key?: string;
  page?: number;
  limit?: number;
  page_mode?: "has_more" | "count";
  sort?: 1 | 2 | 3 | 4 | 5 | 6;
  author_id?: string;
  category_id?: string;
  tag_ids?: string;
  /** Filters publish_status (1 draft, 2 pending, 3 published, 4 rejected). */
  status?: number;
  auth?: boolean;
}

export function searchArticles(params: SearchArticlesParams) {
  const { auth = false, ...query } = params;
  return useNuxtApp().$api.request<SearchArticleResponse>("/api/search/articles", {
    query: query as Record<string, unknown>,
    auth,
  });
}

export function searchArticlesWithAi(content: string) {
  return useNuxtApp().$api.request<SearchArticleResponse>("/api/ai/search/list", {
    method: "POST",
    body: {
      content,
    },
  });
}

export function getTagOptions() {
  return useNuxtApp()
    .$api.request<Array<{ id?: string; title?: string; value?: string; label?: string }>>("/api/articles/tags/options", {
      auth: false,
    })
    .then((list) =>
      list.map(
        (item) =>
          ({
            label: item.title ?? item.label ?? "",
            value: item.id ?? item.value ?? "",
          }) satisfies OptionItem,
      ),
    );
}

export function getCategoryOptions(userId: string | number) {
  return useNuxtApp()
    .$api.request<Array<{ id?: string; title?: string; value?: string; label?: string }>>("/api/articles/category/options", {
      auth: false,
      query: {
        user_id: String(userId),
      },
    })
    .then((list) =>
      list.map(
        (item) =>
          ({
            label: item.title ?? item.label ?? "",
            value: item.id ?? item.value ?? "",
          }) satisfies OptionItem,
      ),
    );
}

/**
 * createCategory 复用既有的分类创建接口（POST /api/articles/category）。
 *
 * 不新增端点：后端本来就有显式查重并返回可读业务错误，这里只负责把
 * 用户输入的标题送过去。标题是否为空、是否重名都由调用方与后端判定。
 */
export function createCategory(title: string) {
  return useNuxtApp().$api.request<unknown>("/api/articles/category", {
    method: "POST",
    body: {
      title,
    },
  });
}
