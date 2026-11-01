/**
 * 页面当前跑在哪种渲染环境里（ADR-0019 决策 3：并行取数 + 骨架优先）。
 *
 * 同一个页面代码要在三种环境里给出不同的取数时序，所以把它显式命名出来，
 * 而不是在页面里散落 `import.meta.server` / `isHydrating` 判断：
 *
 * - `server`：服务端渲染。必须等齐数据（HTML 里要有首屏内容，分享卡片与抓取器依赖它）。
 * - `hydration`：客户端首屏。路由数据在 payload 里（useAsyncData 直接命中），
 *   但 `usePagedResourceCache` 的分页数据不在 payload —— 它要**等**回来，否则
 *   SSR 已渲染的列表会被骨架替换，产生 hydration mismatch 与闪动。
 * - `navigation`：站内路由跳转。目标页没有 payload，setup 一律不等数据：
 *   先出骨架，请求在后台并行跑，数据到了自己填。
 */
export type ReaderRenderMode = "server" | "hydration" | "navigation";

export interface ReaderRenderModeEnv {
  server: boolean;
  client: boolean;
  isHydrating: boolean;
}

/** 纯函数版本（单测直接喂 env，不依赖 Nuxt 运行时）。 */
export function readerRenderMode(env: ReaderRenderModeEnv): ReaderRenderMode {
  if (env.server) {
    return "server";
  }

  if (env.client && env.isHydrating) {
    return "hydration";
  }

  return "navigation";
}

/** 运行时判定：页面/composable 的默认来源。 */
export function detectReaderRenderMode(): ReaderRenderMode {
  return readerRenderMode({
    server: import.meta.server,
    client: import.meta.client,
    isHydrating: import.meta.client ? Boolean(useNuxtApp().isHydrating) : false,
  });
}
