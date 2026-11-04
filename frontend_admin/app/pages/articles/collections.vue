<script setup lang="ts">
/**
 * 合集管理（决定清单 §8 第 33–36 条）。
 *
 * 页面本身只做三件事：拉列表、把卡片网格铺开、把「编辑 / 删除 / 展示开关」三个动作
 * 接到接口上。表单、封面上传、目录树全在 `CollectionEditorOverlay` 里 ——
 * 页面不再自带一张常驻的大表单（旧形态把"看列表"挤到了屏幕下半截）。
 */
import { onMounted, ref } from 'vue'
import { NButton, NSpace, useDialog, useMessage } from 'naive-ui'
import CollectionCardGrid, { type CollectionCardItem } from '~/components/articles/CollectionCardGrid.vue'
import CollectionEditorOverlay from '~/components/articles/CollectionEditorOverlay.vue'
import { deleteCollection, getAdminCollections, setCollectionDisplay } from '~/services/admin'
import { applyDisplayResult } from '~/utils/collectionAdmin'

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
    const data = await getAdminCollections()
    rows.value = (data.list || []) as CollectionCardItem[]
  } catch {
    message.error('合集列表加载失败')
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
    await setCollectionDisplay(row.id, displayed)
    // 结果立即落回列表：开关不该等下一次整表刷新才变。
    rows.value = applyDisplayResult(rows.value, row.id, displayed)
    message.success(displayed ? '合集已展示给读者' : '合集已隐藏，读者侧不再可见')
  } catch (error) {
    message.error(error instanceof Error ? `展示开关保存失败：${error.message}` : '展示开关保存失败，请稍后重试')
  } finally {
    togglingId.value = ''
  }
}

function removeRow(row: CollectionCardItem) {
  dialog.warning({
    title: '删除合集',
    content: `确认删除「${row.title || '这个合集'}」？合集里的文章不会被删除，但读者侧会立即看不到这个合集。`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await deleteCollection(row.id)
        if (editing.value?.id === row.id) {
          editing.value = null
          editorOpen.value = false
        }
        message.success('合集已删除')
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
        <h1 class="admin-page__title">合集管理</h1>
        <p class="admin-hint">
          合集是管理员编排的文章系列。封面上传前自动裁成 16:9；不传封面时读者侧回退到首篇收录文章的封面。
          新建后信息卡保持打开，可以接着点「编排目录树」。
        </p>
      </div>
      <NButton type="primary" data-role="collection-create" @click="openCreate">新建合集</NButton>
    </div>

    <CollectionCardGrid
      kind="collection"
      :items="rows"
      :loading="loading"
      :toggling-id="togglingId"
      @edit="startEdit"
      @remove="removeRow"
      @toggle="toggleDisplay" />

    <CollectionEditorOverlay
      :show="editorOpen"
      kind="collection"
      :entity="editing"
      @close="editorOpen = false"
      @saved="handleSaved" />
  </NSpace>
</template>
