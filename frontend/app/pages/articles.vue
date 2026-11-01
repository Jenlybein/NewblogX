<script setup lang="ts">
// `/articles` 是历史列表入口，已由 `/search` 取代；ADR-0019 决策 1 把它移出 SSR
// 白名单（`ssr: false`，见 nuxt.config 的 routeRules）。重定向放在页面中间件里：
// 组件创建之前改路由，不在 setup 里 await 导航（那样会把客户端整页卡成空白，见
// app/middleware/articles-redirect.ts 的说明）。
definePageMeta({
  middleware: ["articles-redirect"],
});
</script>
