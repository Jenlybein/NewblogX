<script setup lang="ts">
import FolderCard from "~/components/common/FolderCard.vue";
import type { CollectionCardView } from "~/utils/collection";

/**
 * 合集 / 项目列表的文件夹卡网格（票 293）。
 *
 * 为什么网格也收进共用组件：两页原本各写一份同形的卡片与网格样式，本票把
 * 「卡片 + 网格」一起收敛成一处，页面只剩页头与空态。列宽公式保持不变
 * （`minmax(min(100%, 20rem), 1fr)`），卡片内部按列宽等比缩放。
 *
 * 卡片视图**复用既有的 `CollectionCardView`**：`ProjectCardView` 与它逐字段同形
 * （结构类型上互相兼容，两个列表页的 `cards` 都能直接传），本组件不再另立一份
 * 同形声明——多一份字段表，就多一处将来要跟着改的地方。
 */
defineProps<{ cards: CollectionCardView[] }>();
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
  /* 行距比列距宽：封面卡常态就比舞台高出一截（纸从文件夹里探出来），
     行距要留得下那段溢出，以及对焦/悬停时再上抽的 30px。 */
  gap: 1.75rem 1.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}
</style>
