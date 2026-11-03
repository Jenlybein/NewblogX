import type { ProjectDetail, ProjectListData } from "~/types/api";

/**
 * 项目（读者侧只读接口）。
 *
 * 项目与合集并列、页面形态复用，但**数据不共用**；读者侧同样只有两个动作：
 * 看列表、看详情。两个请求都 `auth: false`：项目是对匿名访客公开的内容——
 * 它受后端「展示开关」与「显示非管理员发布文章」两个开关约束，前端不做可见性
 * 过滤（过滤已在仓储 SQL 里，重复判断只会多一份会漂移的真相）。
 */

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
 * 项目详情（含按顺序排列的文章）。
 *
 * 未展示的项目后端按不存在处理：调用方拿到错误即可，不要用"空文章数组"
 * 来区分"项目隐藏"与"项目为空"。
 */
export function getProjectDetail(id: string | number) {
  return useNuxtApp().$api.request<ProjectDetail>(`/api/projects/${id}`, {
    auth: false,
  });
}
