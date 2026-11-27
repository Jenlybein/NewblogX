<script setup lang="ts">
/**
 * 旧地址 `/review` 的跳转页。
 *
 * 文章审核队列搬到了「内容修改与发布 → 文章审核」（`/content/review`，
 * ADR-0029 改写版）：它属于内容生产链的最后一环，与「关键词风险」那种"事后处置"
 * 不同。`/review` 这个名字还留着给旧书签用。
 *
 * 同级的 `/review/images` 与 `/review/content-filter` 仍是真实页面，不受影响。
 */
onMounted(() => {
  void navigateTo('/content/review', { replace: true })
})
</script>

<template>
  <div class="admin-empty">正在跳转到文章审核…</div>
</template>
