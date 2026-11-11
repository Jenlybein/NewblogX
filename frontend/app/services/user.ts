import { useNuxtApp } from "#imports";
import type { MessageSummary, UserBaseInfo, UserSelfDetail } from "~/types/api";

/**
 * 用户搜索的一行（公开字段）。
 *
 * 后端只回昵称 / 头像 / 简介 / 注册年数，没有邮箱与状态：搜索是匿名可用的读面。
 */
export type PublicUserSearchItem = {
  id: string;
  nickname: string;
  avatar: string;
  abstract: string;
  code_age: number;
};

export type UserSearchListData = {
  list: PublicUserSearchItem[];
  has_more: boolean;
};

/**
 * 按昵称 / 用户名搜索用户。
 *
 * 顶栏搜索进到结果页时，除了文章还会带一栏「相关用户」，这样输入作者名
 * 既能找到他的文章，也能直接进他的主页。
 */
export function searchUsers(params: { key: string; page?: number; limit?: number }) {
  return useNuxtApp().$api.request<UserSearchListData>("/api/users/search", {
    query: {
      key: params.key,
      page: params.page ?? 1,
      limit: params.limit ?? 6,
    },
  });
}

export function getUserBaseInfo(id: string | number) {
  return useNuxtApp().$api.request<UserBaseInfo>("/api/users/base", {
    query: {
      id: String(id),
    },
  });
}

/**
 * 取当前登录用户详情。
 *
 * `retryAuth: false` 用于"刷新令牌之后立刻校验新令牌"这一步：那时刷新流程本身
 * 还持有一个未完成的 Promise，若这里再触发一次自动刷新，就会 await 到自己。
 */
export function getSelfUserDetail(options: { retryAuth?: boolean } = {}) {
  return useNuxtApp().$api.request<UserSelfDetail>("/api/users/detail", {
    retryAuth: options.retryAuth,
  });
}

export function getMessageSummary() {
  return useNuxtApp().$api.request<MessageSummary>("/api/sitemsg/user");
}
