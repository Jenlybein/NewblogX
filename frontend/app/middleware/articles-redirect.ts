// `/articles` → `/search` 的历史重定向（列表页已并入搜索页）。
//
// 为什么是中间件而不是页面 setup 里的 `await navigateTo()`：`/articles` 按
// ADR-0019 决策 1 改成 `ssr: false`（见 nuxt.config 的 routeRules），跳转只能
// 发生在客户端；在页面 setup 里 await 一个路由替换，会在客户端**初始挂起**
// （initial suspense）期间把正在进行的导航换掉，整页渲染卡死成空白（#83 实测：
// URL 变成 /search、`#__nuxt` 始终为空、无异常）。中间件在组件创建**之前**运行，
// 用返回重定向位置的标准写法，服务端与客户端都安全。
export default defineNuxtRouteMiddleware(() =>
  navigateTo("/search", {
    redirectCode: 301,
    replace: true,
  }),
);
