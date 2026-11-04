<script setup lang="ts">
/**
 * `/messages` 没有独立内容：中间件把它 replace 到默认子项
 * `/messages/comments`（保留非 tab/group 的 query）。页面本身不渲染。
 */
definePageMeta({
  middleware: ["auth", "message-center-redirect"],
});
</script>
