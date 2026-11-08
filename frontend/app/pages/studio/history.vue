<script setup lang="ts">
// 历史记录已提升为顶级页 `/history`。旧路径 `/studio/history` 必须重定向：
// 服务端由 routeRules 的 301 承担（shared/readerRenderingBoundary.ts），客户端由
// 中间件兜底（app/middleware/history-redirect.ts）。
// 这里不留任何数据获取或版式 —— 旧页面已经被 `/history` 取代。
definePageMeta({
  middleware: ["history-redirect"],
});
useNoIndexPage();
</script>
