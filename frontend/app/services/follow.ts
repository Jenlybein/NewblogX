import type { RelationCounts, RelationListData } from "~/types/api";

/**
 * 关系接口（票 327）。
 *
 * 读只有一处：`/api/relations?type=follow|fans|mutual`。三种视图返回同一种条目形状，
 * 互关由服务端过滤——前端不再自己筛、也不再为了凑够一页向后扫页。
 *
 * 写分两类，因为它们是两件事：
 *   - 我对别人的动作：`/api/follow/{id}` POST / DELETE（关注 / 取关）；
 *   - 我对"关注我的人"的动作：`/api/relations/fans/{id}` DELETE（移除粉丝，单向）。
 */
export type RelationKind = "follow" | "fans" | "mutual";

export function followUser(id: string | number) {
  return useNuxtApp().$api.request(`/api/follow/${id}`, {
    method: "POST",
  });
}

export function unfollowUser(id: string | number) {
  return useNuxtApp().$api.request(`/api/follow/${id}`, {
    method: "DELETE",
  });
}

/** 移除粉丝：单向删除"他关注我"这一条，不通知对方。 */
export function removeFan(id: string | number) {
  return useNuxtApp().$api.request(`/api/relations/fans/${id}`, {
    method: "DELETE",
  });
}

export function getRelations(params: {
  type: RelationKind;
  userId?: string | number;
  page?: number;
  limit?: number;
}) {
  return useNuxtApp().$api.request<RelationListData>("/api/relations", {
    query: {
      type: params.type,
      page: params.page ?? 1,
      limit: params.limit ?? 30,
      ...(params.userId ? { user_id: String(params.userId) } : {}),
    },
  });
}

/** 三个视图的计数（顶栏徽标）；省略 userId 时统计自己。 */
export function getRelationCounts(params: { userId?: string | number } = {}) {
  return useNuxtApp().$api.request<RelationCounts>("/api/relations/counts", {
    query: params.userId ? { user_id: String(params.userId) } : {},
  });
}
