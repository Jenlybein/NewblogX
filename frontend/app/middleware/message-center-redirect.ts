import { resolveMessageCenterRedirect } from "~/utils/messageCenter";

/**
 * 消息中心入口重定向（#239）。
 *
 * 两个入口共用一个中间件：
 *   - 旧 `/studio/inbox`：按 `tab` / `group` 的语义映射到新子项，
 *     并保留私信草稿 `draft_*` query（从作者主页点「发私信」会带过来）；
 *   - `/messages`：自身没有内容，落到默认的「评论与回复」。
 *
 * 为什么放在页面中间件而不是页面 setup 里：客户端 SPA（`ssr: false`）页面在
 * setup 里 await 路由替换会打断初始挂起（`#83` 的教训，见 `/articles` 的中件）。
 */
export default defineNuxtRouteMiddleware((to) => {
  const target = resolveMessageCenterRedirect(to.path, to.query);
  if (!target) {
    return;
  }
  return navigateTo(target, { replace: true });
});
