import { isMessageCenterTab } from "~/utils/messageCenter";

/**
 * `/messages/[tab]` 的子项校验（#239）。
 *
 * 路由是动态段，未知子项（`/messages/foo`）不该渲染成「评论与回复」的空壳还留在
 * 错误 URL 上：统一 replace 到默认子项。
 */
export default defineNuxtRouteMiddleware((to) => {
  if (isMessageCenterTab(to.params.tab)) {
    return;
  }
  return navigateTo("/messages/comments", { replace: true });
});
