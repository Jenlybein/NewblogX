<script setup lang="ts">
import FolderCard from "~/components/common/FolderCard.vue";

/**
 * 合集 / 项目列表的文件夹卡网格（票 293）。
 *
 * 为什么网格也收进共用组件：两页原本各写一份同形的卡片与网格样式，本票把
 * 「卡片 + 网格」一起收敛成一处，页面只剩页头与空态。列宽公式保持不变
 * （`minmax(min(100%, 20rem), 1fr)`），卡片内部按列宽等比缩放。
 */
interface FolderCardItem {
  /** 列表 key（后端契约里的雪花字符串）。 */
  id: string;
  /** 详情页路由。 */
  href: string;
  title: string;
  description: string;
  /** 封面 URL；空串走空占位。 */
  cover: string;
  /** 篇数文案。 */
  countLabel: string;
}

defineProps<{ cards: FolderCardItem[] }>();
</script>

<template>
  <ul class="folder-card-grid">
    <FolderCard
      v-for="card in cards"
      :key="card.id"
      :href="card.href"
      :title="card.title"
      :description="card.description"
      :cover="card.cover"
      :meta="card.countLabel"
    />
  </ul>
</template>

<style scoped>
.folder-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 20rem), 1fr));
  gap: 1.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}
</style>
