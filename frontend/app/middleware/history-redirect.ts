// `/studio/history` → `/history` 的历史重定向（历史记录从创作区提升为顶级页，#241）。
//
// 与服务端 301 的关系：`routeRules` 里已经声明了 `/studio/history` → `/history` 的
// 301（见 shared/readerRenderingBoundary.ts），但那只覆盖**文档请求**；站内
// `router.push("/studio/history")`（旧书签、旧页面里的残留链接）走的是客户端路由，
// 由这个中间件兜底。
//
// 为什么是中间件而不是页面 setup 里的 `await navigateTo()`：与 articles-redirect
// 同一个理由 —— 在 setup 里 await 一次路由替换会打断客户端初始挂起，整页卡成
// 空白（#83 实测）。中间件在组件创建**之前**改路由，服务端与客户端都安全。
export default defineNuxtRouteMiddleware(() =>
  navigateTo("/history", {
    redirectCode: 301,
    replace: true,
  }),
);
