<script setup lang="ts">
/**
 * 项目管理（决定清单 §8 第 37 条：与合集同一套方案）。
 *
 * 卡片网格 + hover 编辑/删除 + 居中的编辑弹窗（内含右侧滑出的目录树抽屉）。
 * 与合集页唯一的差别是服务函数与展示开关的 helper，界面结构完全一致。
 *
 * 收录与排序不在本页：项目的编排在弹窗里的「编排目录树」抽屉（整树编辑器）。
 */
import { onMounted, ref } from 'vue'
import { NButton, NSpace, useDialog, useMessage } from 'naive-ui'
import CollectionCardGrid, { type CollectionCardItem } from '~/components/articles/CollectionCardGrid.vue'
import CollectionEditorOverlay from '~/components/articles/CollectionEditorOverlay.vue'
import { deleteProject, getAdminProjects, setProjectDisplay } from '~/services/admin'
import { applyProjectDisplayResult } from '~/utils/projectAdmin'

const message = useMessage()
const dialog = useDialog()

const rows = ref<CollectionCardItem[]>([])
const loading = ref(false)
const togglingId = ref('')
const editorOpen = ref(false)
const editing = ref<CollectionCardItem | null>(null)

async function refresh() {
  loading.value = true
  try {
    const data = await getAdminProjects()
    rows.value = (data.list || []) as CollectionCardItem[]
  } catch {
    message.error('项目列表加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  editorOpen.value = true
}

function startEdit(row: CollectionCardItem) {
  editing.value = row
  editorOpen.value = true
}

async function handleSaved() {
  await refresh()
}

async function toggleDisplay(row: CollectionCardItem, displayed: boolean) {
  if (togglingId.value) return
  togglingId.value = row.id
  try {
    await setProjectDisplay(row.id, displayed)
    rows.value = applyProjectDisplayResult(rows.value, row.id, displayed)
    message.success(displayed ? '项目已展示给读者' : '项目已隐藏，读者侧不再可见')
  } catch (error) {
    message.error(error instanceof Error ? `展示开关保存失败：${error.message}` : '展示开关保存失败，请稍后重试')
  } finally {
    togglingId.value = ''
  }
}

function removeRow(row: CollectionCardItem) {
  dialog.warning({
    title: '删除项目',
    content: `确认删除「${row.title || '这个项目'}」？项目里的文章不会被删除，但读者侧会立即看不到这个项目。`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await deleteProject(row.id)
        if (editing.value?.id === row.id) {
          editing.value = null
          editorOpen.value = false
        }
        message.success('项目已删除')
        await refresh()
      } catch (error) {
        message.error(error instanceof Error ? `删除失败：${error.message}` : '删除失败，请稍后重试')
      }
    },
  })
}

onMounted(refresh)
</script>

<template>
  <NSpace vertical :size="16">
    <div class="admin-page__head">
      <div>
        <h1 class="admin-page__title">项目管理</h1>
        <p class="admin-hint">
          项目与合集同一套编排：封面上传前自动裁成 16:9，不传封面时读者侧回退到首篇收录文章的封面。
          收录与顺序都在「编排目录树」里做。
        </p>
      </div>
      <NButton type="primary" data-role="project-create" @click="openCreate">新建项目</NButton>
    </div>

    <CollectionCardGrid
      kind="project"
      :items="rows"
      :loading="loading"
      :toggling-id="togglingId"
      @edit="startEdit"
      @remove="removeRow"
      @toggle="toggleDisplay" />

    <CollectionEditorOverlay
      :show="editorOpen"
      kind="project"
      :entity="editing"
      @close="editorOpen = false"
      @saved="handleSaved" />
  </NSpace>
</template>
