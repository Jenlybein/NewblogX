<script setup lang="ts">
import { computed } from 'vue'
import FolderCardList from '~/components/common/FolderCardList.vue'
import type { CollectionCardView } from '~/utils/collection'
import type { GroupTreeData } from '~/types/api'

/**
 * 合集 / 项目列表的「按分组展示」（票 353）。
 *
 * 读者侧与后台的差别只有一条：**分组标题只是视觉分段**，不承担可见性语义：
 * 谁可见仍由列表端点决定，这里只决定"谁和谁在一起、谁在前"。
 *
 * 分组顺序与组内顺序都照后端给的来，前端不重排：顺序的唯一真相是归属表。
 *
 * 2026-11-12 重做：分组标题此前**一条样式都没有**（组件连 style 块都没有），
 * 渲染出来就是一行默认 h2；未分组的尾块又直接贴着上一组，两段糊成一片。
 * 现在标题是「记号 + 标题 + 发丝线 + 计数」的分段头，尾块用一条淡色细线
 * 居中托住「未分组」三字 —— 分隔线本身就是读者要的那条 `---- 未分组 ----`。
 */
const props = withDefaults(defineProps<{
  cards: CollectionCardView[]
  /** 分组读面；为空（还没加载 / 老数据）时退回"一整个平铺网格"。 */
  groups?: GroupTreeData | null
  label?: string
  /**
   * 列表容器的 data 标记（两个调用方各自的观测点）。
   *
   * 之前写死 `data-collection-list`，项目页因此静默丢了自己的标记 ——
   * 这种"共用组件把调用方的身份写死"的问题在 e2e 里才会暴露。
   */
  listRole?: string
}>(), {
  groups: null,
  label: '专题',
  listRole: 'collection',
})

/** 把卡片按分组归位；分组里没有卡片的段整段不渲染。 */
const sections = computed(() => {
  const byId = new Map(props.cards.map(card => [card.id, card]))
  const placed = new Set<string>()

  const grouped = (props.groups?.groups ?? [])
    .map(group => {
      const cards = (group.items ?? [])
        .map(item => byId.get(item.owner_id))
        .filter((card): card is CollectionCardView => Boolean(card))
      cards.forEach(card => placed.add(card.id))
      return { id: group.id, title: group.title, cards }
    })
    // 空分组在读者侧不渲染：只有标题、下面什么都没有的分段只会让页面看起来坏了。
    .filter(section => section.cards.length > 0)

  // 尾块：未分组 + 分组读面里没有的卡片（新建成还没进过分组）。
  const trailing: CollectionCardView[] = []
  for (const item of props.groups?.ungrouped ?? []) {
    const card = byId.get(item.owner_id)
    if (card && !placed.has(card.id)) {
      trailing.push(card)
      placed.add(card.id)
    }
  }
  for (const card of props.cards) {
    if (!placed.has(card.id)) trailing.push(card)
  }

  return { grouped, trailing }
})

/** 尾块的分隔线只在"上面确实有分组"时才画：整页都没有分组时它就是普通列表。 */
const showTrailingDivider = computed(() => sections.value.grouped.length > 0)
</script>

<template>
  <div class="grouped-reader-list">
    <section
      v-for="section in sections.grouped"
      :key="section.id"
      class="grouped-reader-list__section"
      :data-role="`reader-group-${section.id}`">
      <header class="grouped-reader-list__head">
        <span class="grouped-reader-list__mark" aria-hidden="true" />
        <h2 class="grouped-reader-list__title">{{ section.title }}</h2>
        <span class="grouped-reader-list__line" aria-hidden="true" />
        <span class="grouped-reader-list__count">{{ section.cards.length }} 个{{ label }}</span>
      </header>
      <FolderCardList :cards="section.cards" :data-collection-list="listRole === 'collection' ? 'true' : undefined" :data-project-list="listRole === 'project' ? 'true' : undefined" />
    </section>

    <!-- 尾块：淡色细线 + 居中标签，把"没归进任何分组"的内容与上一组隔开。 -->
    <section v-if="sections.trailing.length" class="grouped-reader-list__section" data-role="reader-group-trailing">
      <header v-if="showTrailingDivider" class="grouped-reader-list__divider">
        <span class="grouped-reader-list__divider-line" aria-hidden="true" />
        <h2 class="grouped-reader-list__divider-label">未分组</h2>
        <span class="grouped-reader-list__divider-line" aria-hidden="true" />
      </header>
      <FolderCardList :cards="sections.trailing" :data-collection-list="listRole === 'collection' ? 'true' : undefined" :data-project-list="listRole === 'project' ? 'true' : undefined" />
    </section>
  </div>
</template>

<style scoped>
/* 一组一段：段距比卡片行距大一档，分组之间才不会读成同一张网格。 */
.grouped-reader-list {
  display: flex;
  flex-direction: column;
  gap: clamp(2.5rem, 5vw, 3.75rem);
}

.grouped-reader-list__section {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 1.5rem;
}

/*
 * 分段头：记号 + 标题 + 一条吃掉剩余宽度的发丝线 + 计数。
 * 线不是装饰：它把"标题"和"计数"钉在同一行，读者一眼能看出这一行属于下面那组卡片。
 */
.grouped-reader-list__head {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 0.875rem;
}

.grouped-reader-list__mark {
  width: 0.5rem;
  height: 0.5rem;
  flex: 0 0 auto;
  background: var(--color-ink);
}

.grouped-reader-list__title {
  margin: 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: clamp(1.125rem, 1.6vw, 1.375rem);
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.2;
  text-wrap: balance;
}

.grouped-reader-list__line {
  flex: 1 1 auto;
  min-width: 1.5rem;
  height: 1px;
  background: var(--color-line);
}

.grouped-reader-list__count {
  flex: 0 0 auto;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.1em;
  white-space: nowrap;
}

/* 尾块分隔线：两侧淡色细线，中间是标签本身 —— 线一定要比分组头的更淡。 */
.grouped-reader-list__divider {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.grouped-reader-list__divider-line {
  flex: 1 1 auto;
  height: 1px;
  background: var(--color-line-faint);
}

.grouped-reader-list__divider-label {
  margin: 0;
  padding: 0 0.25rem;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  font-weight: 500;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  white-space: nowrap;
}

/* 窄屏：分段头允许计数掉到第二行，标题不被挤成竖排。 */
@media (max-width: 640px) {
  .grouped-reader-list__head {
    flex-wrap: wrap;
    gap: 0.5rem 0.75rem;
  }

  .grouped-reader-list__line {
    order: 3;
    flex-basis: 100%;
  }
}
</style>
