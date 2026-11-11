import { useNuxtApp } from "#imports";
import type { CollectionDetail, CollectionListData, CollectionOutlineTree, GroupTreeData } from "~/types/api";

/**
 * 合集（读者侧只读接口）。
 *
 * 合集是管理员编排的一组有顺序的文章；读者侧只有两个动作：看列表、看详情。
 * 两个请求都 `auth: false`：合集是对匿名访客公开的内容——它受后端
 * 「展示开关」与「显示非管理员发布文章」两个开关约束，前端不做可见性过滤
 * （过滤已在仓储 SQL 里，重复判断只会多一份会漂移的真相）。
 *
 * 拆树迁移（#275）：详情页要展示的目录树改由**合集具名端点**
 * `/api/collections/<id>/outline-tree` 给出，标题 / 描述仍由详情端点给。
 * 这里把两份响应合成页面已经在消费的 `CollectionDetail` 形状，页面与共用
 * 呈现组件都不必知道树换了出处；篇数 / 封面回退读具名形状算好的值，
 * 合集自己上传的封面优先（与后端详情的显式封面规则一致）。
 */

/**
 * 读者侧分组读面（票 353）。
 *
 * 只回答"谁和谁在一起、谁在前"：只含展示中的合集，且**没有未分组标题** ——
 * 那些合集由页面作为尾块平铺。分组标题在读者侧不承担可见性语义，
 * 谁可见仍由列表端点决定。
 */
export function getCollectionGroups() {
  return useNuxtApp().$api.request<GroupTreeData>("/api/collections/grouped", { auth: false });
}

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
 * 合集详情：实体信息 + 合集具名目录树。
 *
 * 未展示的合集在详情端点按不存在处理：那次请求失败即整页按不存在渲染，
 * 不要用"空文章数组"来区分"合集隐藏"与"合集为空"。
 */
export async function getCollectionDetail(id: string | number): Promise<CollectionDetail> {
  const api = useNuxtApp().$api;
  const ownerId = String(id);
  const [meta, tree] = await Promise.all([
    api.request<CollectionDetail>(`/api/collections/${ownerId}`, { auth: false }),
    api.request<CollectionOutlineTree>(`/api/collections/${ownerId}/outline-tree`, { auth: false }),
  ]);
  return {
    ...meta,
    outline: {
      owner_kind: tree.owner_kind,
      owner_id: tree.owner_id,
      nodes: tree.nodes,
    },
    article_count: tree.article_count,
    cover: meta.cover || tree.cover,
  };
}
