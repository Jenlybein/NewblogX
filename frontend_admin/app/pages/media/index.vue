<script setup lang="ts">
/**
 * `/media` 旧地址的跳转页。
 *
 * 「媒体运营」拆成「轮播运营」与「图片资源库」两页后（ADR-0029），这个地址不再是
 * 一个真实页面：它只负责把旧书签与旧链接送到轮播运营。
 */
onMounted(() => {
  void navigateTo('/media/banners', { replace: true })
})
</script>

<template>
  <div class="admin-empty">正在跳转到轮播运营…</div>
</template>
