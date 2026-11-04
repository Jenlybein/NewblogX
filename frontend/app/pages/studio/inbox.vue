<script setup lang="ts">
/**
 * 旧消息中心入口（第 239 票 已迁移到顶级页 `/messages`）。
 *
 * `/studio/inbox` 保留为兼容入口：页面中间件按 `tab` / `group` 的语义重定向到
 * `/messages/{comments,digg,system,global,chat}`，并保留私信草稿 `draft_*` query。
 * 页面本身不渲染内容 —— 重定向在组件创建之前完成。
 */
definePageMeta({
  layout: "default",
  middleware: ["auth", "message-center-redirect"],
});
</script>
