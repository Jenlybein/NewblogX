<script setup lang="ts">
// 图片资源库（`/media/images`）：列出站内上传的图片并支持删除。
//
// 拆页后本页只拉图片分页；轮播运营（`/media/banners`）有自己的列表与加载（ADR-0029）。
// `/images` 是 page/count 分页（后端 `res.OkWithList` → `{list, count}`），没有 has_more，
// 所以"下一页"要按总数自己算。
//
// 审核相关（ADR-0034）：状态列显示后端给的中文名（待审核 / 已通过 / 疑似违规 / 违规），
// 并可按状态筛选。待审核与疑似是**只有管理员能看到**的图 —— 读者侧一律不展示，
// 所以后台是这个队列唯一的处理入口。
import { h } from 'vue'
import { NButton, NCard, NDataTable, NSelect, NSpace, NTag, useDialog, useMessage } from 'naive-ui'

import { auditImage, deleteImages, getImages } from '~/services/admin'
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
// 空串表示不筛。
const statusFilter = ref('')

/** 筛选选项：与后端 ListRequest.status 的取值一一对应。 */
const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '待审核', value: 'pending' },
  { label: '疑似违规', value: 'review' },
  { label: '已通过', value: 'pass' },
  { label: '违规', value: 'block' },
]

/** 状态标签配色：不可展示的用醒目色，否则运营翻列表时看不出问题图。 */
function statusTagType(status: string) {
  switch (status) {
    case 'pass':
      return 'success' as const
    case 'pending':
      return 'warning' as const
    case 'review':
    case 'block':
      return 'error' as const
    default:
      return 'default' as const
  }
}

const imageColumns = [
  { title: 'ID', key: 'id', minWidth: 220, render: (row: ImageListItem) => String(row.id || '-') },
  {
    title: '预览',
    key: 'url',
    width: 180,
    render: (row: ImageListItem) => {
      // 预览用后端派生的地址：管理员能看到未过审的图（否则没法审），
      // 同时享受压缩样式。
      const src = String(row.url || '')
      return h('div', { class: 'admin-cover' }, src ? [h('img', { src, alt: String(row.file_name || row.id || '') })] : [])
    },
  },
  {
    title: '状态',
    key: 'audit_status',
    width: 140,
    render: (row: ImageListItem) =>
      h(
        NTag,
        { size: 'small', type: statusTagType(String(row.audit_status || '')) },
        { default: () => String(row.audit_label || row.audit_status || '未知') },
      ),
  },
  {
    title: '对象 key',
    key: 'object_key',
    minWidth: 240,
    // 探活、排查都要用 object key，列表里直接给出来，省得去数据库翻。
    render: (row: ImageListItem) => String(row.object_key || '-'),
  },
  {
    title: '创建时间',
    key: 'created_at',
    width: 180,
    render: (row: ImageListItem) => formatDateTime(String(row.created_at || '')),
  },
  {
    title: '操作',
    key: 'actions',
    width: 210,
    render: (row: ImageListItem) => {
      // 只有"还没有结论"的图能放行：违规是终局（对象已删），放行没有意义。
      const pending = row.audit_status === 'pending' || row.audit_status === 'review'
      const buttons = []
      if (pending) {
        buttons.push(
          h(
            NButton,
            { size: 'small', tertiary: true, type: 'primary', disabled: !row.id, onClick: () => handleAudit(row, 'approve') },
            { default: () => '放行' },
          ),
          h(
            NButton,
            { size: 'small', tertiary: true, type: 'warning', disabled: !row.id, onClick: () => handleAudit(row, 'reject') },
            { default: () => '驳回' },
          ),
        )
      }
      buttons.push(
        h(
          NButton,
          { size: 'small', tertiary: true, type: 'error', disabled: !row.id, onClick: () => removeImage(row) },
          { default: () => '删除' },
        ),
      )
      return h('div', { class: 'flex gap-2' }, buttons)
    },
  },
]

async function refreshImages() {
  loadingImages.value = true
  try {
    const data = await getImages({
      page: imagePage.value,
      limit: IMAGE_PAGE_SIZE,
      status: statusFilter.value || undefined,
    })
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

/**
 * 人工审核一张图：放行（→ 通过并在存储侧解禁）或驳回（→ 违规并删对象）。
 *
 * 驳回是不可逆的，所以走二次确认；放行只改状态，直接执行。
 *
 * 两条路径都必须 catch：业务失败在协议里是 **HTTP 200 + code 非 0**，不 catch 的话
 * naive-ui 的确认框会一直停在那里、也不弹任何提示，看起来就是"点了没反应"。
 */
function handleAudit(row: ImageListItem, action: 'approve' | 'reject') {
  if (!row.id) return
  const run = async () => {
    try {
      await auditImage(String(row.id), action)
      message.success(action === 'approve' ? '已放行' : '已驳回')
      await refreshImages()
    } catch (error) {
      message.error(`${action === 'approve' ? '放行' : '驳回'}失败：${(error as Error).message}`)
    }
  }
  if (action === 'approve') {
    void run()
    return
  }
  dialog.warning({
    title: '驳回图片',
    content: `驳回后「${row.file_name || row.id}」会被判定违规并删除存储里的文件，无法恢复。确定吗？`,
    positiveText: '驳回',
    negativeText: '取消',
    onPositiveClick: run,
  })
}

/**
 * 删除一张图。
 *
 * 后端按"先删对象、再清记账本与行"的顺序执行；对象已经不存在时按成功处理
 * （审核判违规时对象已被删过一次）。这里仍然要 catch：其它失败（权限、网络、
 * 存储异常）必须以红色提示落地，不能让确认框静默卡住。
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

// 换筛选条件时回到第一页：否则会停在"第 3 页的待审核"，看起来像没有数据。
watch(statusFilter, () => {
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
      <NSelect v-model:value="statusFilter" class="w-40" :options="statusOptions" />
      <span class="text-sm opacity-70">
        共 {{ imageTotal }} 张；待审核与疑似违规的图只有管理员能看到
      </span>
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
