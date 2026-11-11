<script setup lang="ts">
import { computed } from 'vue'
import FolderCardList from '~/components/common/FolderCardList.vue'
import type { CollectionCardView } from '~/utils/collection'
import type { GroupTreeData } from '~/types/api'

/**
 * 合集 / 项目列表的「按分组展示」（票 353）。
 *
 * 读者侧与后台的差别只有两条，都在这里落地：
 *  1. **不渲染「未分组」标题** —— 没有分组的实体作为所有分组之后的尾块直接平铺，
 *     一个叫"未分组"的分段对访客没有意义（它描述的是后台的编排状态）；
 *  2. 分组标题只是视觉分段，不承担可见性语义：谁可见仍由列表端点决定，
 *     这里只决定"谁和谁在一起、谁在前"。
 *
 * 分组顺序与组内顺序都照后端给的来，前端不重排：顺序的唯一真相是归属表。
 */
const props = withDefaults(defineProps<{
  cards: CollectionCardView[]
  /** 分组读面；为空（还没加载 / 老数据）时退回"一整个平铺网格"。 */
  groups?: GroupTreeData | null
  label?: string
}>(), {
  groups: null,
  label: '专题',
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
</script>

<template>
  <div class="grouped-reader-list">
    <section
      v-for="section in sections.grouped"
      :key="section.id"
      class="grouped-reader-list__section"
      :data-role="`reader-group-${section.id}`">
      <header class="grouped-reader-list__head">
        <h2 class="grouped-reader-list__title">{{ section.title }}</h2>
        <span class="grouped-reader-list__count">{{ section.cards.length }} 个{{ label }}</span>
      </header>
      <FolderCardList :cards="section.cards" data-collection-list="true" />
    </section>

    <!-- 尾块没有标题：读者侧不出现「未分组」这种说法。 -->
    <section v-if="sections.trailing.length" class="grouped-reader-list__section" data-role="reader-group-trailing">
      <FolderCardList :cards="sections.trailing" data-collection-list="true" />
    </section>
  </div>
</template>
