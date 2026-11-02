import type { CollectionDetail, CollectionListData } from "~/types/api";

/**
 * 合集（读者侧只读接口）。
 *
 * 合集是管理员编排的一组有顺序的文章；读者侧只有两个动作：看列表、看详情。
 * 两个请求都 `auth: false`：合集是对匿名访客公开的内容——它受后端
 * 「展示开关」与「显示非管理员发布文章」两个开关约束，前端不做可见性过滤
 * （过滤已在仓储 SQL 里，重复判断只会多一份会漂移的真相）。
 */

/** 合集列表（分页）。 */
export function getCollections(page = 1, limit = 12) {
  return useNuxtApp().$api.request<CollectionListData>("/api/collections", {
    auth: false,
    query: {
      page,
      limit,
    },
  });
}

/**
 * 合集详情（含按顺序排列的文章）。
 *
 * 未展示的合集后端按不存在处理：调用方拿到错误即可，不要用"空文章数组"
 * 来区分"合集隐藏"与"合集为空"。
 */
export function getCollectionDetail(id: string | number) {
  return useNuxtApp().$api.request<CollectionDetail>(`/api/collections/${id}`, {
    auth: false,
  });
}
