import { useNuxtApp } from "#imports";
import type { GroupTreeData, ProjectDetail, ProjectListData, ProjectOutlineTree } from "~/types/api";

/**
 * 项目（读者侧只读接口）。
 *
 * 项目与合集并列、页面形态复用，但**数据不共用**；读者侧同样只有两个动作：
 * 看列表、看详情。两个请求都 `auth: false`：项目是对匿名访客公开的内容——
 * 它受后端「展示开关」与「显示非管理员发布文章」两个开关约束，前端不做可见性
 * 过滤（过滤已在仓储 SQL 里，重复判断只会多一份会漂移的真相）。
 */

/** 读者侧分组读面（与合集同一形状，见 getCollectionGroups）。 */
export function getProjectGroups() {
  return useNuxtApp().$api.request<GroupTreeData>("/api/projects/grouped", { auth: false });
}

/** 项目列表（分页）。 */
export function getProjects(page = 1, limit = 12) {
  return useNuxtApp().$api.request<ProjectListData>("/api/projects", {
    auth: false,
    query: {
      page,
      limit,
    },
  });
}

/**
 * 项目具名树（#276）：详情页与阅读页左栏从 `/projects/{id}/outline-tree`
 * 读回同一棵 `project_outline_node_models` 的树。
 *
 * 公开读（`auth: false`）：匿名访客看到的树与详情主表一样受后端「展示开关」
 * 与「显示非管理员发布文章」开关约束，前端不重复判一次可见性。
 */
export function getProjectOutlineTree(id: string | number) {
  return useNuxtApp().$api.request<ProjectOutlineTree>(
    `/api/projects/${id}/outline-tree`,
    { auth: false },
  );
}

/**
 * 项目详情（项目主表 + 项目具名树，#276）。
 *
 * 两个公开读并发：主表给标题 / 描述 / 创建时间（以及管理员显式上传的封面），
 * 目录树、篇数与封面回退只认项目具名端点。合成回既有 `ProjectDetail` 形状，
 * 所以页面、`projectOutlineView` 与共用呈现组件逐字不变；拆树前后读者看到的
 * 版式与链接一模一样，只有树的数据来源换成项目自己的那张表。
 *
 * 未展示的项目后端按不存在处理：调用方拿到错误即可，不要用"空文章数组"
 * 来区分"项目隐藏"与"项目为空"。
 */
export async function getProjectDetail(id: string | number): Promise<ProjectDetail> {
  const [detail, tree] = await Promise.all([
    useNuxtApp().$api.request<ProjectDetail>(`/api/projects/${id}`, { auth: false }),
    getProjectOutlineTree(id),
  ]);
  return {
    ...detail,
    outline: {
      ...detail.outline,
      nodes: tree?.nodes ?? [],
    },
    article_count: tree?.article_count ?? 0,
    cover: detail.cover || tree?.cover || "",
  };
}
