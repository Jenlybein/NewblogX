<script setup lang="ts">
/**
 * 右栏卡片管理器。
 *
 * 五类操作（新增 / 删除 / 拖动排序 / 改标题 / 启停）都作用在同一份草稿上，
 * 点「保存右栏卡片」时整表提交：列表顺序就是读者站顺序，停用项也一起提交，
 * 所以停用只影响读者侧渲染、不会把配置删掉。
 *
 * 纯函数都在 `~/utils/sidebarCardsAdmin`：组件不自己推导顺序、标题回退与数量上限，
 * 保证后台表单与后端整表保存的校验口径只有一处。
 *
 * 右侧预览直接消费同一份 `drafts`（见 `SidebarCardsPreview`），未保存的改动即时可见。
 */
import { computed, onMounted, ref } from 'vue'
import {
  NAlert,
  NButton,
  NInput,
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  useMessage,
} from 'naive-ui'
import { getAdminSidebarCards, updateAdminSidebarCards } from '~/services/admin'
import {
  SIDEBAR_CARD_KIND_OPTIONS,
  SIDEBAR_CARD_MAX_COUNT,
  addSidebarCardDraft,
  effectiveSidebarCardTitle,
  moveSidebarCardDraft,
  removeSidebarCardDraft,
  renameSidebarCardDraft,
  sidebarCardDraftErrors,
  sidebarCardDrafts,
  sidebarCardKindLabel,
  toSidebarCardsSavePayload,
  toggleSidebarCardDraft,
} from '~/utils/sidebarCardsAdmin'
import type { SidebarCardDraft, SidebarCardKind } from '~/utils/sidebarCardsAdmin'
import SidebarCardsPreview from '~/components/settings/SidebarCardsPreview.vue'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
const drafts = ref<SidebarCardDraft[]>([])
const newKind = ref<SidebarCardKind>('hot_articles')
// 拖拽源的下标：只在一次拖拽期间有效，放下后清空。
const dragIndex = ref<number | null>(null)

const errors = computed(() => sidebarCardDraftErrors(drafts.value))

async function load() {
  loading.value = true
  try {
    const data = await getAdminSidebarCards()
    drafts.value = sidebarCardDrafts(data.list)
  } catch {
    message.error('右栏卡片加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)

function addCard() {
  drafts.value = addSidebarCardDraft(drafts.value, newKind.value)
}

function removeCard(key: string) {
  drafts.value = removeSidebarCardDraft(drafts.value, key)
}

function renameCard(key: string, title: string) {
  drafts.value = renameSidebarCardDraft(drafts.value, key, title)
}

function toggleCard(key: string, enabled: boolean) {
  drafts.value = toggleSidebarCardDraft(drafts.value, key, enabled)
}

function moveCard(from: number, to: number) {
  drafts.value = moveSidebarCardDraft(drafts.value, from, to)
}

/** 空标题时的占位提示：保存后读者侧看到的就是这个默认标题。 */
function placeholderOf(draft: SidebarCardDraft): string {
  return effectiveSidebarCardTitle(draft.kind, '')
}

function canMoveUp(index: number): boolean {
  return index > 0
}

function canMoveDown(index: number): boolean {
  return index < drafts.value.length - 1
}

function onDragStart(index: number, event: DragEvent) {
  dragIndex.value = index
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', String(index))
  }
}

function onDrop(index: number) {
  const from = dragIndex.value
  dragIndex.value = null
  if (from === null || from === index) return
  moveCard(from, index)
}

function onDragEnd() {
  dragIndex.value = null
}

async function save() {
  if (errors.value.length > 0) {
    message.error(errors.value[0]!)
    return
  }
  saving.value = true
  try {
    await updateAdminSidebarCards(toSidebarCardsSavePayload(drafts.value))
    message.success('右栏卡片已保存')
    // 回读服务端整表：标题回退、顺序号与 id 都以服务端为准，避免本地状态与服务端分叉。
    await load()
  } catch {
    message.error('右栏卡片保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="sidebar-cards-layout" data-role="sidebar-cards-editor">
    <div class="space-y-4">
      <NAlert type="info" :bordered="false">
        右栏卡片按这里的顺序在读者站首页渲染；停用的卡保留配置但读者侧不展示。
        调整顺序（拖动卡片，或用上移 / 下移）后点「保存右栏卡片」整表生效。
      </NAlert>

      <NSpace align="center" :size="12">
        <NSelect
          v-model:value="newKind"
          :options="SIDEBAR_CARD_KIND_OPTIONS"
          class="w-44"
          aria-label="新卡种类"
          data-role="kind-select" />
        <NButton
          type="primary"
          :disabled="drafts.length >= SIDEBAR_CARD_MAX_COUNT"
          data-role="add"
          @click="addCard">新增卡片</NButton>
        <NButton
          type="primary"
          :loading="saving"
          :disabled="loading"
          data-role="save"
          @click="save">保存右栏卡片</NButton>
        <span class="admin-hint">{{ drafts.length }} / {{ SIDEBAR_CARD_MAX_COUNT }} 张</span>
      </NSpace>

      <NAlert v-if="errors.length > 0" type="error" :bordered="false" data-role="errors">
        <ul class="list-disc pl-4">
          <li v-for="error in errors" :key="error">{{ error }}</li>
        </ul>
      </NAlert>

      <div v-if="drafts.length === 0" class="admin-hint" data-role="empty">
        还没有卡片，先新增一张。
      </div>

      <ul class="space-y-2">
        <li
          v-for="(draft, index) in drafts"
          :key="draft.key"
          data-card-row
          :data-key="draft.key"
          :data-kind="draft.kind"
          :data-enabled="draft.enabled ? 'true' : 'false'"
          :data-index="index"
          draggable="true"
          class="admin-card flex flex-wrap items-center gap-3 px-3 py-2"
          :class="{ 'opacity-60': !draft.enabled }"
          @dragstart="onDragStart(index, $event)"
          @dragover.prevent
          @drop.prevent="onDrop(index)"
          @dragend="onDragEnd">
          <span class="cursor-grab select-none" aria-hidden="true" data-role="drag-handle">⠿</span>
          <span class="w-6 text-center text-xs" data-role="sort-order">{{ index + 1 }}</span>
          <NTag size="small" :bordered="false">{{ sidebarCardKindLabel(draft.kind) }}</NTag>
          <NInput
            class="min-w-48 flex-1"
            :value="draft.title"
            :placeholder="placeholderOf(draft)"
            aria-label="卡片标题"
            data-role="title"
            @update:value="value => renameCard(draft.key, value)" />
          <span v-if="!draft.enabled" class="text-xs" data-role="disabled-hint">读者侧不展示</span>
          <NSpace :size="8" align="center">
            <NSwitch
              :value="draft.enabled"
              aria-label="启用这张卡"
              data-role="enabled"
              @update:value="value => toggleCard(draft.key, value)" />
            <NButton
              size="small"
              :disabled="!canMoveUp(index)"
              data-role="move-up"
              @click="moveCard(index, index - 1)">上移</NButton>
            <NButton
              size="small"
              :disabled="!canMoveDown(index)"
              data-role="move-down"
              @click="moveCard(index, index + 1)">下移</NButton>
            <NButton
              size="small"
              type="error"
              quaternary
              data-role="remove"
              @click="removeCard(draft.key)">删除</NButton>
          </NSpace>
        </li>
      </ul>
    </div>

    <aside class="sidebar-preview-column" data-role="sidebar-preview-column">
      <div class="preview-caption">
        <div class="eyebrow">预览</div>
        <p class="admin-hint">
          按首页右栏的固定宽度渲染当前草稿，未保存的改动也会立刻反映；
          内容为示例数据，只用于判断密度与顺序。
        </p>
      </div>
      <SidebarCardsPreview :drafts="drafts" />
    </aside>
  </div>
</template>

<style scoped>
/* 编辑器与预览并排；预览宽度自己固定，这里只决定它放在哪一列。 */
.sidebar-cards-layout {
  display: grid;
  gap: 1.5rem;
}

@media (min-width: 1280px) {
  .sidebar-cards-layout {
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: start;
  }
}

.sidebar-preview-column {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 0.75rem;
}

.preview-caption {
  max-width: 100%;
}

.preview-caption .admin-hint {
  margin: 0;
  line-height: 1.6;
}
</style>
