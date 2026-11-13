<script setup lang="ts">
/**
 * 目录树编辑器（决定清单 §8）：合集与项目共用，弹窗抽屉里挂载。
 *
 * 从原来的独立页 `pages/articles/outline.vue` 抽出来，行为不变：
 * 本地扁平行 → 点「保存」整树一次性提交（ADR-0019 约束 7：不得造第二套顺序真相）。
 *
 * **树的渲染与拖拽都交给 Naive UI 的 `NTree`**（验收反馈：手写的 `<ul>` 看不出层级，
 * 每行还摊开六个按钮）。它正对着痛点：树线（`show-line`）、缩进（`indent`）、折叠箭头、
 * 整行命中、内置拖拽与落点提示都是现成的，键盘也能操作。我们只做两件事：
 *   1. 扁平行 ↔ 树节点互转（`treeData` 由 `rows` 派生；拖拽落点经 `outlineDropIndex`
 *      换回下标，再交给 `moveOutlineRowToIndex` —— 父级仍由"落点上方那行"推导）；
 *   2. 行内操作收进一个「⋯」下拉（上移 / 下移 / 缩进 / 移出 / 重命名 / 移除），
 *      不再每行摊开六个按钮；`NTree` 的拖拽与这些按钮改的都是同一份本地行。
 *
 * 仍然只改本地行、仍然只有「保存」落地库。
 */
import { computed, h, nextTick, onMounted, ref, watch } from 'vue'
import { NButton, NDropdown, NInput, NSelect, NSpace, NTag, NTooltip, NTree, useMessage } from 'naive-ui'
import type { DropdownOption, TreeDropInfo, TreeOption } from 'naive-ui'
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
  outlineDropIndex,
  outlineIncludeTargetKey,
  outlineRowTitle,
  outlineStaleCount,
  removeOutlineRow,
  renameOutlineFolder,
  staleReasonLabel,
} from '~/utils/outlineArrange'
import type { OutlineArticleCandidate, OutlineFlatRow, OutlineMoveOp } from '~/utils/outlineArrange'

const props = withDefaults(defineProps<{
  /** 编排的是合集还是项目（两个实体共用同一棵树结构）。 */
  kind: 'collection' | 'project'
  /** 实体 ID；空串表示还没保存过基本信息，此时不允许编排。 */
  entityId: string
  /**
   * 嵌在编辑弹窗的两栏布局里（票 348）。
   *
   * 嵌入时**没有自己的保存按钮**：整份保存由弹窗的「保存修改」一次提交
   * （字段 + 目录树走原子 compose）。树上只留拖拽 / 改名 / 删除 / 清理失效 /
   * 批量收录这些编辑动作 —— 两个保存按钮会让"到底保存了什么"变成猜谜。
   */
  embedded?: boolean
}>(), {
  embedded: false,
})

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

/** 「⋯」下拉当前指向的行（NDropdown 需要 v-model:show + 位置信息）。 */
const menuRowKey = ref('')
const menuVisible = ref(false)
const menuX = ref(0)
const menuY = ref(0)

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

function selectRow(key: string) {
  selectedKey.value = key
  const row = selectedRow()
  renameTitle.value = row?.kind === 1 ? row.title : ''
}

/* ---------------- 扁平行 → NTree 数据 ---------------- */

/**
 * 树节点：`key` 用行 key，`kind`/`stale` 透传给标签渲染。
 * 文章（kind 2）永远没有子节点，标成 `isLeaf` 让 NTree 不画折叠箭头。
 */
interface OutlineTreeNode extends TreeOption {
  key: string
  label: string
  kind: 1 | 2
  stale: boolean
  staleReason: string | null
  children?: OutlineTreeNode[]
}

const treeData = computed<OutlineTreeNode[]>(() => {
  const children = new Map<string, OutlineFlatRow[]>()
  for (const row of rows.value) {
    const list = children.get(row.parentKey) ?? []
    list.push(row)
    children.set(row.parentKey, list)
  }
  const build = (parentKey: string): OutlineTreeNode[] =>
    (children.get(parentKey) ?? []).map((row) => {
      const node: OutlineTreeNode = {
        key: row.key,
        label: outlineRowTitle(row),
        kind: row.kind,
        stale: row.kind === 2 && row.stale,
        staleReason: row.staleReason ?? null,
      }
      if (row.kind === 1) {
        node.children = build(row.key)
        node.isLeaf = node.children.length === 0 ? false : undefined
      } else {
        node.isLeaf = true
      }
      return node
    })
  return build('')
})

function renderLabel({ option }: { option: TreeOption }) {
  const node = option as OutlineTreeNode
  return h('span', { class: 'outline-node', 'data-outline-row': node.key }, [
    h('span', { class: 'outline-node__kind', 'aria-hidden': 'true' }, node.kind === 1 ? '▣' : '·'),
    h('span', { class: 'outline-node__title' }, node.label),
    node.stale
      ? h(
          NTag,
          { size: 'small', type: 'warning', bordered: false, class: 'outline-node__stale' },
          { default: () => `已失效（${staleReasonLabel(node.staleReason)}）` },
        )
      : null,
  ])
}

/** 「⋯」菜单的内容只取决于当前指向的那一行，所以做成 computed，模板里不用写表达式。 */
const menuOptions = computed<DropdownOption[]>(() => {
  const row = rows.value.find(item => item.key === menuRowKey.value)
  if (!row) return []
  return [
    { label: '上移', key: 'up', disabled: !canMoveOutlineRow(rows.value, row.key, 'up') },
    { label: '下移', key: 'down', disabled: !canMoveOutlineRow(rows.value, row.key, 'down') },
    { label: '缩进（成为上一行的子项）', key: 'indent', disabled: !canMoveOutlineRow(rows.value, row.key, 'indent') },
    { label: '移出（升一级）', key: 'outdent', disabled: !canMoveOutlineRow(rows.value, row.key, 'outdent') },
    { type: 'divider', key: 'd1' },
    { label: '重命名', key: 'rename', disabled: row.kind !== 1 },
    { label: '移除', key: 'remove' },
  ]
})

function renderSuffix({ option }: { option: TreeOption }) {
  const node = option as OutlineTreeNode
  return h(
    NButton,
    {
      size: 'tiny',
      quaternary: true,
      circle: true,
      class: 'outline-node__more',
      'data-role': 'outline-row-more',
      title: '更多操作（上移 / 下移 / 缩进 / 移出 / 重命名 / 移除）',
      onClick: (event: MouseEvent) => {
        event.stopPropagation()
        menuRowKey.value = node.key
        menuX.value = event.clientX
        menuY.value = event.clientY
        menuVisible.value = true
      },
    },
    { default: () => '⋯' },
  )
}

/**
 * 拖拽落点：`before | inside | after` → 扁平行下标 → 交给纯函数。
 * 文章不接受 "inside"（它没有子节点）；其余情况一律放行，由 `moveOutlineRowToIndex`
 * 把"落到自己块里"这类无意义移动变成 no-op。
 */
function allowDrop(info: { dropPosition: 'before' | 'inside' | 'after'; node: TreeOption }) {
  if (info.dropPosition === 'inside' && (info.node as OutlineTreeNode).kind !== 1) return false
  return true
}

function handleDrop(info: TreeDropInfo) {
  const dragKey = String(info.dragNode.key)
  const targetKey = String(info.node.key)
  const targetIndex = outlineDropIndex(rows.value, targetKey, info.dropPosition)
  const next = moveOutlineRowToIndex(rows.value, dragKey, targetIndex)
  const changed = next.some(
    (row, index) => row.key !== rows.value[index]?.key || row.parentKey !== rows.value[index]?.parentKey,
  )
  rows.value = next
  selectRow(dragKey)
  if (changed) dirty.value = true
}

function handleSelectedKeys(keys: Array<string | number>) {
  selectRow(keys.length > 0 ? String(keys[keys.length - 1]) : '')
}

/* ---------------- 行操作 ---------------- */

function onMenuSelect(action: string) {
  menuVisible.value = false
  const row = rows.value.find(item => item.key === menuRowKey.value)
  if (!row) return
  if (action === 'rename') {
    selectRow(row.key)
    return
  }
  if (action === 'remove') {
    removeRow(row)
    return
  }
  moveRow(row, action as OutlineMoveOp)
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
  } catch (error) {
    rows.value = []
    message.error(error instanceof Error ? `目录树加载失败：${error.message}` : '目录树加载失败，请稍后重试')
  } finally {
    loading.value = false
  }
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

/** 供嵌入方取当前树（compose 要把节点一起提交）。 */
function nodes() {
  return buildOutlineNodes(rows.value)
}

/**
 * 供嵌入方在整份保存成功后复位脏标记。
 *
 * 两栏弹窗走的是 compose（字段 + 树一次提交），不经过树自己的 save()，
 * 所以脏标记要由调用方显式复位 —— 否则「有未保存的改动」会在保存成功后
 * 一直挂着，用户以为没存上。
 */
function markSaved() {
  dirty.value = false
  includedCount.value = 0
}

defineExpose({ save, dirty, nodes, markSaved, hasDirty: () => dirty.value })
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
        <!-- 树只保留编辑动作（拖拽 / 改名 / 删除 / 清理失效 / 批量收录）：
             保存由外层弹窗的「保存修改」一次提交，树里不再有第二个保存按钮。 -->
      </NSpace>

      <NTree
        v-if="rows.length"
        class="outline-tree"
        data-role="outline-rows"
        block-line
        show-line
        :data="treeData"
        :indent="16"
        :default-expand-all="true"
        :selectable="true"
        :selected-keys="selectedKey ? [selectedKey] : []"
        :draggable="true"
        :allow-drop="allowDrop"
        :render-label="renderLabel"
        :render-suffix="renderSuffix"
        @update:selected-keys="handleSelectedKeys"
        @drop="handleDrop" />
      <p v-else data-role="outline-empty" class="admin-hint">
        这个{{ kind === 'collection' ? '合集' : '项目' }}还没有内容。先新建一个章节文件夹，或者直接把文章收录进根层。
      </p>

      <!-- 下拉挂在编辑器根上：每行只留一个「⋯」，菜单内容随 menuRowKey 变化。 -->
      <NDropdown
        v-model:show="menuVisible"
        trigger="manual"
        placement="bottom-start"
        :x="menuX"
        :y="menuY"
        :options="menuOptions"
        @select="onMenuSelect" />

      <!-- 长说明收进 tooltip（票 348）：正文区只留操作本身，
           需要"怎么用"的人点一下问号就有，不需要的人不必每次读三行。 -->
      <NTooltip placement="bottom-start" style="max-width: 420px">
        <template #trigger>
          <NButton size="tiny" quaternary data-role="outline-help">操作说明</NButton>
        </template>
        拖动节点可调整顺序与层级（落在文件夹上是它的第一个子项，落在节点间隙是同级）。
        每行右侧「⋯」里有上移 / 下移 / 缩进 / 移出 / 重命名 / 移除，键盘与触屏也走得通。
        所有改动先留在本地，由「保存修改」一次性提交。
      </NTooltip>

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
