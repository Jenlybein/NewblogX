/**
 * 全局未读实时通道（票 #240）。
 *
 * 为什么是插件而不是某个布局 / 页面：WS 连接与未读订阅属于**整个登录会话**，
 * 不属于私信页。装在 app 启动时（`.client` 后缀，SSR 不执行），登录后自动建连、
 * 登出自动断开，任何页面（首页 / 文章页 / 消息中心）都能收到私信到达的未读 +1。
 *
 * 顺序：`01.auth-init.ts` 先初始化登录态，本插件随后订阅它的变化；账号已登录着
 * 进站时 watch 的 `immediate` 会立刻建连。
 */
export default defineNuxtPlugin(() => {
  useMessageRealtime();
});
