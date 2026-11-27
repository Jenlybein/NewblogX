<script setup lang="ts">
/**
 * 旧地址 `/content/review` 的跳转页（2026-11-27）。
 *
 * 「文章审核」从「内容修改与发布」搬回「审核」组时换了路由前缀 ——
 * 组变了，路径也跟着组走，否则菜单位置与地址长期不一致。
 * 留跳转页而不是直接删文件：这个地址在文档、书签与运营记录里存在过。
 */
onMounted(() => {
  void navigateTo('/review/article', { replace: true })
})
</script>

<template>
  <div class="admin-empty">正在跳转到文章审核…</div>
</template>
