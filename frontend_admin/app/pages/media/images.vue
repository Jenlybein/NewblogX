<script setup lang="ts">
// 图片资源库（`/media/images`）：列出站内上传的图片并支持删除。
//
// 拆页后本页只拉图片分页；轮播运营（`/media/banners`）有自己的列表与加载（ADR-0029）。
// `/images` 是 page/count 分页（后端 `res.OkWithList` → `{list, count}`），没有 has_more，
// 所以"下一页"要按总数自己算。
import { h } from 'vue'
import { NButton, NCard, NDataTable, useDialog, useMessage } from 'naive-ui'

import { deleteImages, getImages } from '~/services/admin'
import type { ImageListItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'
import { hasNextPage } from '~/utils/pagination'

const message = useMessage()
const dialog = useDialog()

const loadingImages = ref(false)
const IMAGE_PAGE_SIZE = 10
const imagePage = ref(1)
const hasMoreImages = ref(false)
const imageTotal = ref(0)
const images = ref<ImageListItem[]>([])

const imageColumns = [
  { title: 'ID', key: 'id', minWidth: 220, render: (row: ImageListItem) => String(row.id || '-') },
  {
    title: '预览',
    key: 'url',
    width: 180,
    render: (row: ImageListItem) => {
      const src = String(row.url || '')
      return h('div', { class: 'admin-cover' }, src ? [h('img', { src, alt: String(row.file_name || row.id || '') })] : [])
    },
  },
  { title: '状态', key: 'status', width: 120, render: (row: ImageListItem) => h('span', {}, String(row.status ?? '-')) },
  {
    title: '创建时间',
    key: 'created_at',
    width: 180,
    render: (row: ImageListItem) => formatDateTime(String(row.created_at || '')),
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row: ImageListItem) =>
      h(
        NButton,
        { size: 'small', tertiary: true, type: 'error', disabled: !row.id, onClick: () => removeImage(row) },
        { default: () => '删除' },
      ),
  },
]

async function refreshImages() {
  loadingImages.value = true
  try {
    const data = await getImages({ page: imagePage.value, limit: IMAGE_PAGE_SIZE })
    images.value = data.list || []
    // `/images` 是 page/count 分页（后端 `res.OkWithList` → `{list, count}`），没有 has_more。
    // 这里曾经写 `Boolean(data.has_more)`，字段恒为 undefined → 按钮永久禁用、只能看第一页。
    imageTotal.value = data.count ?? 0
    hasMoreImages.value = hasNextPage(imagePage.value, IMAGE_PAGE_SIZE, imageTotal.value)
  } catch {
    message.error('图片资源加载失败')
  } finally {
    loadingImages.value = false
  }
}

function removeImage(row: ImageListItem) {
  if (!row.id) return
  dialog.warning({
    title: '删除图片',
    content: String(row.id),
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      await deleteImages([String(row.id)])
      message.success('图片已删除')
      await refreshImages()
    },
  })
}

watch(imagePage, refreshImages)
onMounted(() => {
  void refreshImages()
})
</script>

<template>
  <NCard class="admin-card" title="图片资源库">
    <NDataTable :columns="imageColumns" :data="images" :loading="loadingImages" :bordered="false" />
    <div class="mt-4 flex justify-end gap-2">
      <NButton :disabled="imagePage <= 1" @click="imagePage--">上一页</NButton>
      <NButton :disabled="!hasMoreImages" @click="imagePage++">下一页</NButton>
    </div>
  </NCard>
</template>
