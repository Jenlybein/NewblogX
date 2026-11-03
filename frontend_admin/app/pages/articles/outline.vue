<script setup lang="ts">
/**
 * 目录编排（管理员）：合集与项目共用的最小树编辑器（票 217）。
 *
 * 一个界面服务两个实体：上方选「合集 / 项目」与具体条目，下方读回同一棵嵌套树。
 * 编排动作全部是按钮式：新建文件夹、批量收录、上移 / 下移 / 缩进 / 移出；
 * 改的是本地扁平行，点「保存」才把**整棵树**一次性提交（新建 / 收录 / 顺序 /
 * 层级一起落库），保存后以后端返回的树重新渲染。
 *
 * 为什么不做拖拽、也不做单步保存：整树提交让"中断或并发不会留下半套顺序"
 * （票 209 用户故事 30）；单步接口会制造第二套顺序真相（ADR-0023 当前约束 7）。
 *
 * 票 217 只做最小闭环；重命名 / 移除节点 / 清理失效行与旧排列页退役是票 218。
 */
import { NButton, NCard, NInput, NSelect, NSpace, NTag, useMessage } from 'naive-ui'
import {
  getAdminCollections,
  getAdminProjects,
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
  flattenOutlineTree,
  moveOutlineRow,
  newOutlineFolderKey,
  outlineDepth,
  outlineIncludeTargetKey,
  outlineRowTitle,
} from '~/utils/outlineArrange'
import type { OutlineArticleCandidate, OutlineFlatRow, OutlineMoveOp } from '~/utils/outlineArrange'

type OutlineEntityKind = 'collection' | 'project'

const message = useMessage()

const entityKind = ref<OutlineEntityKind>('collection')
const collections = ref<Array<{ id: string; title: string }>>([])
const projects = ref<Array<{ id: string; title: string }>>([])
const entityId = ref('')
const entitiesLoading = ref(false)

const rows = ref<OutlineFlatRow[]>([])
const selectedKey = ref('')
const loading = ref(false)
const saving = ref(false)
const dirty = ref(false)

/** 新建文件夹的标题输入。 */
const folderTitle = ref('')

/** 批量收录：关键词、搜索结果、已勾选文章。 */
const keyword = ref('')
const articleOptions = ref<Array<{ label: string; value: string }>>([])
const selectedArticleIds = ref<string[]>([])
const searching = ref(false)
const includedCount = ref(0)

function entityOptions(): Array<{ label: string; value: string }> {
  const source = entityKind.value === 'collection' ? collections.value : projects.value
  return source.map(item => ({ label: item.title || item.id, value: item.id }))
}

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
}

async function loadEntities() {
  entitiesLoading.value = true
  try {
    const [collectionData, projectData] = await Promise.all([getAdminCollections(), getAdminProjects()])
    collections.value = (collectionData.list ?? []).map(item => ({ id: item.id, title: item.title }))
    projects.value = (projectData.list ?? []).map(item => ({ id: item.id, title: item.title }))
    const first = entityOptions()[0]
    if (!entityId.value && first) {
      entityId.value = first.value
    }
  } catch {
    message.error('内容实体加载失败')
  } finally {
    entitiesLoading.value = false
  }
}

function chooseKind(kind: OutlineEntityKind) {
  entityKind.value = kind
  entityId.value = ''
  rows.value = []
  selectedKey.value = ''
  dirty.value = false
  const first = entityOptions()[0]
  if (first) {
    entityId.value = first.value
    void loadOutline()
  }
}

function chooseEntity(id: string) {
  entityId.value = id
  void loadOutline()
}

async function loadOutline() {
  if (!entityId.value) {
    rows.value = []
    return
  }
  loading.value = true
  try {
    const data =
      entityKind.value === 'collection'
        ? await getCollectionOutline(entityId.value)
        : await getProjectOutline(entityId.value)
    rows.value = flattenOutlineTree(data.nodes ?? [])
    selectedKey.value = ''
    dirty.value = false
    includedCount.value = 0
  } catch (error) {
    rows.value = []
    message.error(error instanceof Error ? `目录树加载失败：${error.message}` : '目录树加载失败，请稍后重试')
  } finally {
    loading.value = false
  }
}

/** 新建文件夹：parentKey 空串为根层，否则必须是一个文件夹行。 */
function createFolder(parentKey: string) {
  const title = folderTitle.value.trim()
  if (!title) {
    message.warning('请先填写文件夹名称')
    return
  }
  const key = newOutlineFolderKey(rows.value)
  rows.value = addOutlineFolder(rows.value, parentKey, title)
  selectedKey.value = key
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

function moveRow(row: OutlineFlatRow, op: OutlineMoveOp) {
  if (!canMoveOutlineRow(rows.value, row.key, op)) return
  rows.value = moveOutlineRow(rows.value, row.key, op)
  selectedKey.value = row.key
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

/** 批量收录：挂到当前选中的文件夹（选中文章行时退回其父文件夹）；已在树里的跳过。 */
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
  if (!entityId.value || saving.value) return
  saving.value = true
  try {
    const nodes = buildOutlineNodes(rows.value)
    const data =
      entityKind.value === 'collection'
        ? await saveCollectionOutline(entityId.value, nodes)
        : await saveProjectOutline(entityId.value, nodes)
    rows.value = flattenOutlineTree(data.nodes ?? [])
    selectedKey.value = ''
    dirty.value = false
    includedCount.value = 0
    message.success('目录树已保存')
  } catch (error) {
    message.error(error instanceof Error ? `保存失败：${error.message}` : '保存失败，请稍后重试')
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  await loadEntities()
  if (entityId.value) await loadOutline()
})
</script>

<template>
  <NSpace vertical :size="20">
    <NCard class="admin-card" title="选择内容实体">
      <p class="admin-hint">
        合集与项目共用这一套目录树。先选实体，再编排它的文件夹与文章；保存是整棵树一次性提交。
      </p>
      <NSpace align="center">
        <NSelect
          data-role="outline-kind"
          class="admin-arrange-select"
          :value="entityKind"
          :options="[
            { label: '合集', value: 'collection' },
            { label: '项目', value: 'project' },
          ]"
          @update:value="(value: OutlineEntityKind) => chooseKind(value)" />
        <NSelect
          data-role="outline-entity"
          class="admin-arrange-select"
          :loading="entitiesLoading"
          :value="entityId"
          :options="entityOptions()"
          placeholder="选择合集 / 项目"
          @update:value="chooseEntity" />
        <NButton :loading="loading" @click="loadOutline">重新读取</NButton>
        <NTag v-if="dirty" type="warning" size="small">有未保存的改动</NTag>
      </NSpace>
    </NCard>

    <NCard class="admin-card" title="目录树">
      <p class="admin-hint">
        文件夹只分组、不可阅读；文章节点挂在文件夹下。上移 / 下移调兄弟顺序，缩进挂到上一个兄弟文件夹，
        移出回到父文件夹的父级。所有改动先留在本地，点「保存」整棵树一次性提交。
      </p>

      <NSpace align="center" class="outline-toolbar">
        <NInput
          v-model:value="folderTitle"
          data-role="outline-folder-title"
          class="admin-arrange-keyword"
          placeholder="新文件夹名称"
          @keyup.enter="createRootFolder" />
        <NButton data-role="outline-create-root" @click="createRootFolder">根层新建文件夹</NButton>
        <NButton data-role="outline-create-child" :disabled="!selectedRow() || selectedRow()?.kind !== 1" @click="createChildFolder">
          在选中文件夹下新建
        </NButton>
        <NButton type="primary" data-role="outline-save" :loading="saving" :disabled="!entityId" @click="save">
          保存目录树
        </NButton>
      </NSpace>

      <ul v-if="rows.length" class="outline-tree" data-role="outline-rows">
        <li
          v-for="row in rows"
          :key="row.key"
          class="outline-tree__row"
          :class="{ 'outline-tree__row--selected': row.key === selectedKey }"
          :data-row-key="row.key"
          :style="rowIndentStyle(row)"
          @click="selectRow(row.key)">
          <span class="outline-tree__kind">{{ row.kind === 1 ? '文件夹' : '文章' }}</span>
          <strong class="outline-tree__title">{{ outlineRowTitle(row) }}</strong>
          <NTag v-if="row.kind === 2 && row.stale" size="small" type="warning">
            已失效{{ row.staleReason ? `（${row.staleReason}）` : '' }}
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
          </NSpace>
        </li>
      </ul>
      <p v-else data-role="outline-empty" class="admin-hint">
        这个专题还没有内容。先新建一个章节文件夹，或者直接把文章收录进根层。
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
            class="admin-arrange-keyword"
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
          class="admin-arrange-articles"
          multiple
          placeholder="选择要收录的文章（可多选）"
          :value="selectedArticleIds"
          :options="articleOptions"
          @update:value="(value: string[]) => (selectedArticleIds = value)" />

        <p v-if="includedCount > 0" class="admin-hint" data-role="outline-included-hint">
          本次已加入 {{ includedCount }} 篇，尚未保存。
        </p>
      </section>
    </NCard>
  </NSpace>
</template>
