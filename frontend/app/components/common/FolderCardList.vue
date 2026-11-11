<script setup lang="ts">
import FolderCard from "~/components/common/FolderCard.vue";
import type { CollectionCardView } from "~/utils/collection";

/**
 * 合集 / 项目列表的文件夹卡网格（票 293）。
 *
 * 为什么网格也收进共用组件：两页原本各写一份同形的卡片与网格样式，本票把
 * 「卡片 + 网格」一起收敛成一处，页面只剩页头与空态。
 *
 * 列数（2026-11-12 调整）：桌面 **6 个一行**。读者要的是"一屏扫完所有专题"，
 * 4 列时每张卡约 310px 宽、封面抢戏，列表反而要滚很久；6 列后卡宽约 200px，
 * 文件夹的形态仍然成立（几何全部按容器宽度等比缩放），密度接近杂志目录。
 * 中间断点逐级收列（5 / 4 / 3 / 2 / 1），不靠 auto-fill 的列宽上界猜。
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
  /* 宽屏六列：每列等分剩余宽度，卡片随列宽等比缩放。 */
  grid-template-columns: repeat(6, minmax(0, 1fr));
  /* 行距比列距宽：封面卡常态就比舞台高出一截（纸从文件夹里探出来），
     行距要留得下那段溢出，以及对焦/悬停时再上抽的距离。 */
  gap: 2.25rem 1.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

/* 逐级收列：每一档都先保证列宽 ≥ 150px，再谈列数。 */
@media (max-width: 1279px) {
  .folder-card-grid {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }
}

@media (max-width: 1080px) {
  .folder-card-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (max-width: 860px) {
  .folder-card-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .folder-card-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1.75rem 1rem;
  }
}

@media (max-width: 420px) {
  .folder-card-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
