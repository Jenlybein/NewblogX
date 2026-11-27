<script setup lang="ts">
/**
 * 旧地址 `/site/content-filter` 的跳转页。
 *
 * 这一页搬到了「内容修改与发布 → 关键词过滤」（`/content/keywords`，ADR-0029 改写版）。
 * 留跳转页而不是直接删文件：这个地址在文档、书签和运营的手工记录里存在过，
 * 直接删会得到一个 404，而 404 是最难被解释的一种失败。
 *
 * 用 `replace`：跳转不该在浏览器历史里留一条"回不去的中间页"。
 */
onMounted(() => {
  void navigateTo('/content/keywords', { replace: true })
})
</script>

<template>
  <div class="admin-empty">正在跳转到关键词过滤…</div>
</template>
