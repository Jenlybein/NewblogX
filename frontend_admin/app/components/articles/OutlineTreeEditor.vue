<script setup lang="ts">
/**
 * 目录树编辑器（决定清单 §8）：合集与项目共用，弹窗抽屉里挂载。
 *
 * 从原来的独立页 `pages/articles/outline.vue` 抽出来，行为不变：
 * 本地扁平行 → 点「保存」整树一次性提交（ADR-0023 约束 7：不得造第二套顺序真相）。
 *
 * 本轮新增的只有**拖拽**（ADR-0023「后续修订」）：用 sortablejs 做手势层，
 * 松手后把落点换算成 `(parent, index)` 交给纯函数 `moveOutlineRowToIndex`，
 * 仍然只改本地行、仍然只有「保存」落地库。按钮式（上移/下移/缩进/移出）一并保留，
 * 键盘与触屏不依赖拖拽也能完成同样的编排。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NInput, NSelect, NSpace, NTag, useMessage } from 'naive-ui'
import Sortable from 'sortablejs'
import {
  getCollectionOutline,
  getProjectOutline,
  saveCollectionOutline,
  saveProjectOutline,
  searchAdminArticles,
} from '~/services/admin'
import {
  addOutlineArticles,
  addOutlineFolder,
  buildOutlineNodes,
  canMoveOutlineRow,
  cleanupOutlineStaleRows,
  flattenOutlineTree,
  moveOutlineRow,
  moveOutlineRowToIndex,
  newOutlineFolderKey,
  outlineDepth,
  outlineIncludeTargetKey,
  outlineRowTitle,
  outlineStaleCount,
  removeOutlineRow,
  renameOutlineFolder,
  staleReasonLabel,
} from '~/utils/outlineArrange'
import type { OutlineArticleCandidate, OutlineFlatRow, OutlineMoveOp } from '~/utils/outlineArrange'

const props = defineProps<{
  /** 编排的是合集还是项目（两个实体共用同一棵树结构）。 */
  kind: 'collection' | 'project'
  /** 实体 ID；空串表示还没保存过基本信息，此时不允许编排。 */
  entityId: string
}>()

const emit = defineEmits<{ saved: [] }>()

const message = useMessage()

const rows = ref<OutlineFlatRow[]>([])
const selectedKey = ref('')
const loading = ref(false)
const saving = ref(false)
const dirty = ref(false)

const folderTitle = ref('')
const renameTitle = ref('')

const staleCount = computed(() => outlineStaleCount(rows.value))

const keyword = ref('')
const articleOptions = ref<Array<{ label: string; value: string }>>([])
const selectedArticleIds = ref<string[]>([])
const searching = ref(false)
const includedCount = ref(0)

const listRef = ref<HTMLElement | null>(null)
let sortable: Sortable | null = null

function selectedRow(): OutlineFlatRow | undefined {
  return rows.value.find(row => row.key === selectedKey.value)
}

function includeTargetKey(): string {
  return outlineIncludeTargetKey(rows.value, selectedKey.value)
}

function includeTargetLabel(): string {
  const key = includeTargetKey()
  if (!key) return '根层'
  const row = rows.value.find(item => item.key === key)
  return row ? outlineRowTitle(row) : '根层'
}

function rowIndentStyle(row: OutlineFlatRow) {
  return { paddingLeft: `${outlineDepth(rows.value, row.key) * 20}px` }
}

function selectRow(key: string) {
  selectedKey.value = key
  const row = selectedRow()
  renameTitle.value = row?.kind === 1 ? row.title : ''
}

async function load() {
  if (!props.entityId) {
    rows.value = []
    selectedKey.value = ''
    renameTitle.value = ''
    dirty.value = false
    return
  }

  loading.value = true
  try {
    const data = props.kind === 'collection'
      ? await getCollectionOutline(props.entityId)
      : await getProjectOutline(props.entityId)
    rows.value = flattenOutlineTree(data.nodes ?? [])
    selectedKey.value = ''
    renameTitle.value = ''
    dirty.value = false
    includedCount.value = 0
    await nextTick()
    initSortable()
  } catch (error) {
    rows.value = []
    message.error(error instanceof Error ? `目录树加载失败：${error.message}` : '目录树加载失败，请稍后重试')
  } finally {
    loading.value = false
  }
}

/**
 * 拖拽手势层：只换算落点，不改数据模型。
 *
 * sortablejs 会把 DOM 节点先自己挪一遍，我们随即用模型重排把它纠正回来
 * （`:key` 绑定 row.key，Vue 会按模型顺序移动节点）。所以"DOM 顺序 ≠ 模型顺序"
 * 只存在一帧，不会变成第二个真相。
 */
function initSortable() {
  destroySortable()
  if (!listRef.value) return

  // 落位动画跟随系统「减少动态」偏好（`prefers-reduced-motion`）：宁可没有动画，
  // 也不该给开了减少动态的人播 150ms 的落位动效。
  const reduceMotion = typeof window !== "undefined"
    && typeof window.matchMedia === "function"
    && window.matchMedia("(prefers-reduced-motion: reduce)").matches

  sortable = Sortable.create(listRef.value, {
    animation: reduceMotion ? 0 : 150,
    handle: '[data-role="outline-row-grip"]',
    draggable: '[data-row-key]',
    ghostClass: 'outline-tree__row--ghost',
    chosenClass: 'outline-tree__row--chosen',
    onEnd(event) {
      const key = (event.item as HTMLElement).dataset.rowKey ?? ''
      if (!key || typeof event.newIndex !== 'number') return
      const next = moveOutlineRowToIndex(rows.value, key, event.newIndex)
      const changed = next.some((row, index) => row.key !== rows.value[index]?.key || row.parentKey !== rows.value[index]?.parentKey)
      rows.value = next
      selectRow(key)
      if (changed) dirty.value = true
    },
  })
}

function destroySortable() {
  sortable?.destroy()
  sortable = null
}

function createFolder(parentKey: string) {
  const title = folderTitle.value.trim()
  if (!title) {
    message.warning('请先填写文件夹名称')
    return
  }
  const key = newOutlineFolderKey(rows.value)
  rows.value = addOutlineFolder(rows.value, parentKey, title)
  selectRow(key)
  folderTitle.value = ''
  dirty.value = true
}

function createRootFolder() {
  createFolder('')
}

function createChildFolder() {
  const current = selectedRow()
  if (!current || current.kind !== 1) {
    message.warning('请先选中一个文件夹')
    return
  }
  createFolder(current.key)
}

function renameSelectedFolder(key = selectedKey.value) {
  const title = renameTitle.value.trim()
  if (!title) {
    message.warning('请先填写新的文件夹名称')
    return
  }
  const before = rows.value.find(row => row.key === key)?.title ?? ''
  rows.value = renameOutlineFolder(rows.value, key, title)
  if ((rows.value.find(row => row.key === key)?.title ?? '') === before) return
  renameTitle.value = title
  dirty.value = true
}

function removeRow(row: OutlineFlatRow) {
  const next = removeOutlineRow(rows.value, row.key)
  if (next.length === rows.value.length) return
  rows.value = next
  if (selectedKey.value === row.key) selectRow('')
  dirty.value = true
}

function cleanupStale() {
  const next = cleanupOutlineStaleRows(rows.value)
  if (next.length === rows.value.length) return
  rows.value = next
  selectedKey.value = ''
  renameTitle.value = ''
  dirty.value = true
}

function moveRow(row: OutlineFlatRow, op: OutlineMoveOp) {
  if (!canMoveOutlineRow(rows.value, row.key, op)) return
  rows.value = moveOutlineRow(rows.value, row.key, op)
  selectRow(row.key)
  dirty.value = true
}

async function searchArticles() {
  if (searching.value) return
  searching.value = true
  try {
    const data = await searchAdminArticles({ page: 1, limit: 20, key: keyword.value.trim() || undefined })
    const list = (data.list ?? []) as Array<{ id: string; title?: string }>
    articleOptions.value = list.map(item => ({ label: item.title || item.id, value: item.id }))
    selectedArticleIds.value = []
  } catch {
    message.error('文章搜索失败')
  } finally {
    searching.value = false
  }
}

function includeSelectedArticles() {
  if (selectedArticleIds.value.length === 0) {
    message.warning('请先勾选要收录的文章')
    return
  }
  const candidates: OutlineArticleCandidate[] = selectedArticleIds.value.map(id => ({
    id,
    title: articleOptions.value.find(option => option.value === id)?.label ?? id,
  }))
  const before = rows.value.length
  rows.value = addOutlineArticles(rows.value, includeTargetKey(), candidates)
  const added = rows.value.length - before
  includedCount.value += added
  selectedArticleIds.value = []
  if (added > 0) dirty.value = true
  if (added === 0) {
    message.info('勾选的文章都已在目录树里')
  } else {
    message.success(`已加入 ${added} 篇，保存后生效`)
  }
}

async function save() {
  if (!props.entityId || saving.value) return
  saving.value = true
  try {
    const nodes = buildOutlineNodes(rows.value)
    const data = props.kind === 'collection'
      ? await saveCollectionOutline(props.entityId, nodes)
      : await saveProjectOutline(props.entityId, nodes)
    rows.value = flattenOutlineTree(data.nodes ?? [])
    selectedKey.value = ''
    renameTitle.value = ''
    dirty.value = false
    includedCount.value = 0
    message.success('目录树已保存')
    emit('saved')
    await nextTick()
    initSortable()
  } catch (error) {
    message.error(error instanceof Error ? `保存失败：${error.message}` : '保存失败，请稍后重试')
  } finally {
    saving.value = false
  }
}

watch(() => [props.kind, props.entityId], () => void load(), { immediate: false })

onMounted(async () => {
  await load()
})

onBeforeUnmount(destroySortable)

defineExpose({ save, dirty })
</script>

<template>
  <div class="outline-editor">
    <p v-if="!entityId" class="admin-hint" data-role="outline-needs-save">
      先保存基本信息，这个{{ kind === 'collection' ? '合集' : '项目' }}才有 ID；有了 ID 才能编排目录树。
    </p>

    <template v-else>
      <NSpace align="center" class="outline-toolbar">
        <NInput
          v-model:value="folderTitle"
          data-role="outline-folder-title"
          class="admin-field--key"
          placeholder="新文件夹名称"
          @keyup.enter="createRootFolder" />
        <NButton data-role="outline-create-root" @click="createRootFolder">根层新建文件夹</NButton>
        <NButton
          data-role="outline-create-child"
          :disabled="!selectedRow() || selectedRow()?.kind !== 1"
          @click="createChildFolder">
          在选中文件夹下新建
        </NButton>
      </NSpace>

      <NSpace align="center" class="outline-toolbar">
        <NInput
          v-model:value="renameTitle"
          data-role="outline-rename-title"
          class="admin-field--key"
          placeholder="选中文件夹的新名称"
          :disabled="selectedRow()?.kind !== 1"
          @keyup.enter="renameSelectedFolder()" />
        <NButton
          data-role="outline-rename"
          :disabled="selectedRow()?.kind !== 1"
          @click="renameSelectedFolder()">
          重命名
        </NButton>
        <NButton
          data-role="outline-cleanup-stale"
          :disabled="staleCount === 0"
          @click="cleanupStale">
          清理失效（{{ staleCount }}）
        </NButton>
        <NTag v-if="dirty" type="warning" size="small">有未保存的改动</NTag>
        <NButton type="primary" data-role="outline-save" :loading="saving" @click="save">
          保存目录树
        </NButton>
      </NSpace>

      <ul v-if="rows.length" ref="listRef" class="outline-tree" data-role="outline-rows">
        <li
          v-for="row in rows"
          :key="row.key"
          class="outline-tree__row"
          :class="{ 'outline-tree__row--selected': row.key === selectedKey }"
          :data-row-key="row.key"
          :style="rowIndentStyle(row)"
          @click="selectRow(row.key)">
          <span
            class="outline-tree__grip"
            data-role="outline-row-grip"
            title="拖动可调整顺序与层级"
            aria-hidden="true">⋮⋮</span>
          <span class="outline-tree__kind">{{ row.kind === 1 ? '文件夹' : '文章' }}</span>
          <strong class="outline-tree__title">{{ outlineRowTitle(row) }}</strong>
          <NTag v-if="row.kind === 2 && row.stale" size="small" type="warning">
            已失效（{{ staleReasonLabel(row.staleReason) }}）
          </NTag>
          <NSpace :size="4" class="outline-tree__actions">
            <NButton size="small" tertiary :disabled="!canMoveOutlineRow(rows, row.key, 'up')" @click.stop="moveRow(row, 'up')">
              上移
            </NButton>
            <NButton size="small" tertiary :disabled="!canMoveOutlineRow(rows, row.key, 'down')" @click.stop="moveRow(row, 'down')">
              下移
            </NButton>
            <NButton size="small" tertiary :disabled="!canMoveOutlineRow(rows, row.key, 'indent')" @click.stop="moveRow(row, 'indent')">
              缩进
            </NButton>
            <NButton size="small" tertiary :disabled="!canMoveOutlineRow(rows, row.key, 'outdent')" @click.stop="moveRow(row, 'outdent')">
              移出
            </NButton>
            <NButton v-if="row.kind === 1" size="small" tertiary data-role="outline-row-rename" @click.stop="renameSelectedFolder(row.key)">
              重命名
            </NButton>
            <NButton size="small" tertiary type="error" data-role="outline-row-remove" @click.stop="removeRow(row)">
              移除
            </NButton>
          </NSpace>
        </li>
      </ul>
      <p v-else data-role="outline-empty" class="admin-hint">
        这个{{ kind === 'collection' ? '合集' : '项目' }}还没有内容。先新建一个章节文件夹，或者直接把文章收录进根层。
      </p>

      <p class="admin-hint">
        拖动左侧把手可调整顺序与层级：落在文件夹上成为它的第一个子项，落在文章上成为同级兄弟。
        按钮式（上移 / 下移 / 缩进 / 移出）保留给键盘与触屏。所有改动先留在本地，点「保存目录树」一次性提交。
      </p>

      <section class="admin-arrange-batch">
        <h3 class="admin-arrange-batch__title">批量收录文章到「{{ includeTargetLabel() }}」</h3>
        <p class="admin-upload-panel__hint">
          先按关键词搜索，再按住 Ctrl / Shift 勾选多篇，一次加入当前文件夹。已在树里的文章会自动跳过。
        </p>
        <NSpace align="center">
          <NInput
            v-model:value="keyword"
            data-role="outline-keyword"
            class="admin-field--key"
            placeholder="搜索文章标题、摘要、作者"
            @keyup.enter="searchArticles" />
          <NButton :loading="searching" @click="searchArticles">搜索</NButton>
          <NButton
            type="primary"
            data-role="outline-include"
            :disabled="selectedArticleIds.length === 0"
            @click="includeSelectedArticles">
            收录选中（{{ selectedArticleIds.length }}）
          </NButton>
        </NSpace>

        <NSelect
          v-if="articleOptions.length"
          data-role="outline-articles"
          class="admin-field--long"
          multiple
          placeholder="选择要收录的文章（可多选）"
          :value="selectedArticleIds"
          :options="articleOptions"
          @update:value="(value: string[]) => (selectedArticleIds = value)" />

        <p v-if="includedCount > 0" class="admin-hint" data-role="outline-included-hint">
          本次已加入 {{ includedCount }} 篇，尚未保存。
        </p>
      </section>
    </template>
  </div>
</template>
