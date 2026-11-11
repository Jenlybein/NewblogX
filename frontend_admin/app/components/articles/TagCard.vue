<script setup lang="ts">
/**
 * 标签卡（票 346）。从 TagManager 里抽出来的唯一理由：拖拽分支与"过滤时不可拖"
 * 分支渲染的是同一张卡，之前整段复制了两遍，改一处必漏另一处。
 *
 * 拖动把手只在可拖动时渲染：过滤状态下卡片不该显示一个按不动的 ⠿。
 */
import { NButton, NTag } from 'naive-ui'
import type { TagListItem } from '~/types/api'

withDefaults(defineProps<{
  row: TagListItem
  draggable?: boolean
  toggling?: boolean
}>(), {
  draggable: false,
  toggling: false,
})

const emit = defineEmits<{
  edit: []
  toggle: []
  remove: []
}>()
</script>

<template>
  <article
    class="tag-card"
    :class="{ 'tag-card--disabled': row.is_enabled === false }"
    :data-role="`tag-card-${row.id}`">
    <header class="tag-card__head">
      <span v-if="draggable" class="tag-card__grip" title="拖动调整顺序" aria-hidden="true">⠿</span>
      <strong class="tag-card__title" data-role="tag-title">{{ row.title || '（无标题）' }}</strong>
      <span class="tag-card__count" data-role="tag-article-count">{{ row.article_count ?? 0 }} 篇</span>
    </header>

    <p v-if="row.description" class="tag-card__desc">{{ row.description }}</p>

    <footer class="tag-card__foot">
      <NTag
        size="small"
        :type="row.is_enabled === false ? 'default' : 'success'"
        data-role="tag-status">
        {{ row.is_enabled === false ? '停用' : '启用' }}
      </NTag>
      <span class="tag-card__sort" data-role="tag-sort">#{{ row.sort ?? 0 }}</span>
      <div class="tag-card__actions">
        <NButton size="tiny" tertiary data-role="tag-edit" @click="emit('edit')">编辑</NButton>
        <NButton
          size="tiny"
          tertiary
          :type="row.is_enabled === false ? 'primary' : 'warning'"
          :loading="toggling"
          :data-role="row.is_enabled === false ? 'tag-enable' : 'tag-disable'"
          @click="emit('toggle')">
          {{ row.is_enabled === false ? '启用' : '停用' }}
        </NButton>
        <NButton size="tiny" tertiary type="error" data-role="tag-delete" @click="emit('remove')">删除</NButton>
      </div>
    </footer>
  </article>
</template>
