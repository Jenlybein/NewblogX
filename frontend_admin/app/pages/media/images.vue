<script setup lang="ts">
// 图片资源库（`/media/images`）：**只列已过审（pass）的图**，支持搜索与按日期翻旧图。
//
// 与「审核 → 图片审核」解耦（ADR-0034）：那一页处理 pending / review，是把关；
// 这一页是成品库，只该出现能直接用的图。混在一起时运营分不清"该处理的"和"已经能用的"。
//
// `/images` 是 page/count 分页（后端 `res.OkWithList` → `{list, count}`），没有 has_more，
// 所以"下一页"要按总数自己算。
import { NButton, NCard, NDataTable, NDatePicker, NInput, NSpace, NTag, useDialog, useMessage } from 'naive-ui'

import { deleteImages, getImages } from '~/services/admin'
import type { ImageListItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'
import { hasNextPage } from '~/utils/pagination'

const message = useMessage()
const dialog = useDialog()

const loadingImages = ref(false)
const IMAGE_PAGE_SIZE = 20
const imagePage = ref(1)
const hasMoreImages = ref(false)
const imageTotal = ref(0)
const images = ref<ImageListItem[]>([])
// 搜索：文件名或 object key（后端 `key` 两列都 LIKE）。
const keyword = ref('')
// 日期范围（毫秒时间戳）：空数组表示不限。
const dateRange = ref<[number, number] | null>(null)

/** 把毫秒时间戳格式化成后端要的 `YYYY-MM-DD`。 */
function toDay(ms: number): string {
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

const imageColumns = [
  { title: 'ID', key: 'id', minWidth: 200, render: (row: ImageListItem) => String(row.id || '-') },
  {
    title: '预览',
    key: 'url',
    width: 180,
    // 资源库只列 pass，所以这里就是读者看到的同一张图。
    render: (row: ImageListItem) => {
      const src = String(row.url || '')
      return h('div', { class: 'admin-cover' }, src ? [h('img', { src, alt: String(row.file_name || row.id || '') })] : [])
    },
  },
  { title: '文件名', key: 'file_name', minWidth: 180, render: (row: ImageListItem) => String(row.file_name || '-') },
  { title: '对象 key', key: 'object_key', minWidth: 260, render: (row: ImageListItem) => String(row.object_key || '-') },
  { title: '大小', key: 'size', width: 110, render: (row: ImageListItem) => `${Math.round((Number(row.size) || 0) / 1024)} KB` },
  {
    title: '状态',
    key: 'audit_status',
    width: 110,
    render: (row: ImageListItem) => h(
      NTag,
      { size: 'small', type: 'success' },
      { default: () => String(row.audit_label || '已通过') },
    ),
  },
  { title: '创建时间', key: 'created_at', width: 180, render: (row: ImageListItem) => formatDateTime(String(row.created_at || '')) },
  {
    title: '操作',
    key: 'actions',
    width: 110,
    render: (row: ImageListItem) => h(
      NButton,
      { size: 'small', tertiary: true, type: 'error', disabled: !row.id, onClick: () => removeImage(row) },
      { default: () => '删除' },
    ),
  },
]

async function refreshImages() {
  loadingImages.value = true
  try {
    const data = await getImages({
      page: imagePage.value,
      limit: IMAGE_PAGE_SIZE,
      // 资源库的口径写死在这里：只列已过审的图。要看未过审的去「审核 → 图片审核」。
      status: 'pass',
      key: keyword.value.trim() || undefined,
      start_at: dateRange.value ? toDay(dateRange.value[0]) : undefined,
      end_at: dateRange.value ? toDay(dateRange.value[1]) : undefined,
    })
    images.value = data.list || []
    imageTotal.value = data.count ?? 0
    hasMoreImages.value = hasNextPage(imagePage.value, IMAGE_PAGE_SIZE, imageTotal.value)
  } catch (error) {
    message.error(`图片资源加载失败：${(error as Error).message}`)
  } finally {
    loadingImages.value = false
  }
}

/**
 * 删除一张图。
 *
 * 后端按"先删对象、再清账本与行"执行，对象已不存在时按成功处理。这里仍然要 catch：
 * 其余失败必须以红色提示落地 —— 业务失败在协议里是 **HTTP 200 + code 非 0**，
 * 不 catch 的话确认框既不关闭也不提示，看起来就是"点了没反应"。
 */
function removeImage(row: ImageListItem) {
  if (!row.id) return
  dialog.warning({
    title: '删除图片',
    content: `将从存储与图片库中删除「${row.file_name || row.object_key || row.id}」。若它被文章当封面或正文引用，文章里会显示为破图。确定吗？`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await deleteImages([String(row.id)])
        message.success('图片已删除')
      } catch (error) {
        message.error(`删除失败：${(error as Error).message}`)
      }
      await refreshImages()
    },
  })
}

// 换筛选条件时回到第一页：否则会停在"第 3 页的空结果"，看起来像没数据。
watch([keyword, dateRange], () => {
  imagePage.value = 1
  void refreshImages()
})
watch(imagePage, refreshImages)
onMounted(() => {
  void refreshImages()
})
</script>

<template>
  <NCard class="admin-card" title="图片资源库">
    <NSpace class="mb-4" align="center">
      <NInput
        v-model:value="keyword"
        placeholder="搜文件名或对象 key"
        clearable
        class="admin-field--select"
        data-role="image-search" />
      <NDatePicker v-model:value="dateRange" type="daterange" clearable data-role="image-date-range" />
      <span class="text-sm opacity-70">共 {{ imageTotal }} 张已过审图片</span>
    </NSpace>
    <NDataTable :columns="imageColumns" :data="images" :loading="loadingImages" :bordered="false" />
    <div class="mt-4 flex justify-end gap-2">
      <NButton :disabled="imagePage <= 1" @click="imagePage--">
        上一页
      </NButton>
      <NButton :disabled="!hasMoreImages" @click="imagePage++">
        下一页
      </NButton>
    </div>
  </NCard>
</template>
