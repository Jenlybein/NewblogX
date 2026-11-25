<script setup lang="ts">
// 图片审核（`/review/images`，ADR-0034）：跑待审队列。
//
// 这一页处理的是 **pending / review** —— 还没结论、或被判定超时转人工的图。
// 「媒体运营 → 图片资源库」是成品库，只列已过审的图；两页解耦之后，
// 运营在这里"把关"，在那里"取用"，不会混。
//
// 放行 = 通过并在存储侧解禁；驳回 = 判定违规并删除存储对象（**不可逆**）。
// 业务失败在协议里是 HTTP 200 + code 非 0，所以每条动作都必须 catch 并给红色提示，
// 否则确认框既不关闭也不提示，看起来就是"点了没反应"（2026-11-25 踩过）。
import { NButton, NCard, NDataTable, NInput, NSelect, NSpace, NTag, useDialog, useMessage } from 'naive-ui'

import { auditImage, deleteImages, getImages } from '~/services/admin'
import type { ImageListItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'
import { hasNextPage } from '~/utils/pagination'

const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const PAGE_SIZE = 20
const page = ref(1)
const hasMore = ref(false)
const total = ref(0)
const images = ref<ImageListItem[]>([])
const keyword = ref('')
// 队列范围：待处理（pending + review）是默认；「已驳回」是为了能回看、也能删掉那条
// block 行 —— 判违规时对象已经被删了，行却还留在库里，不给入口就永远清不掉
// （2026-11-25 那句"删除没反应"，点的就是这种状态下的行）。
const queue = ref<'open' | 'blocked'>('open')
const queueOptions = [
  { label: '待处理（待审核 + 疑似）', value: 'open' },
  { label: '已驳回（违规）', value: 'blocked' },
]

/** 状态配色：待审黄、疑似红（不可展示的图要一眼看到）。 */
function statusTagType(status: string) {
  if (status === 'review') return 'error' as const
  return 'warning' as const
}

const columns = [
  {
    title: '预览',
    key: 'url',
    width: 160,
    // 未过审的图读者看不到，管理员必须能看到才谈得上审。
    render: (row: ImageListItem) => {
      const src = String(row.url || '')
      return h('div', { class: 'admin-cover' }, src ? [h('img', { src, alt: String(row.file_name || row.id || '') })] : [])
    },
  },
  { title: '文件名', key: 'file_name', minWidth: 160, render: (row: ImageListItem) => String(row.file_name || '-') },
  { title: '对象 key', key: 'object_key', minWidth: 240, render: (row: ImageListItem) => String(row.object_key || '-') },
  {
    title: '上传者',
    key: 'user_id',
    width: 190,
    render: (row: ImageListItem) => String(row.user_id || '-'),
  },
  {
    title: '状态',
    key: 'audit_status',
    width: 120,
    render: (row: ImageListItem) => h(
      NTag,
      { size: 'small', type: statusTagType(String(row.audit_status || '')) },
      { default: () => String(row.audit_label || row.audit_status || '未知') },
    ),
  },
  { title: '上传时间', key: 'created_at', width: 180, render: (row: ImageListItem) => formatDateTime(String(row.created_at || '')) },
  {
    title: '操作',
    key: 'actions',
    width: 220,
    render: (row: ImageListItem) => h('div', { class: 'flex gap-2' }, [
      // 已驳回（block）是终局状态：放行/驳回都会被后端拒绝，摆在这里只会又演一次
      // "点了没反应"，所以这一档只留删除。
      ...(queue.value === 'open'
        ? [
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
          ]
        : []),
      h(
        NButton,
        { size: 'small', tertiary: true, type: 'error', disabled: !row.id, onClick: () => removeImage(row) },
        { default: () => '删除' },
      ),
    ]),
  },
]

async function refresh() {
  loading.value = true
  try {
    const data = await getImages({
      page: page.value,
      limit: PAGE_SIZE,
      // 待审队列 = 还没结论 + 超时转人工。一条查询拿两种状态（后端支持逗号分隔）。
      status: queue.value === 'open' ? 'pending,review' : 'block',
      key: keyword.value.trim() || undefined,
    })
    images.value = data.list || []
    total.value = data.count ?? 0
    hasMore.value = hasNextPage(page.value, PAGE_SIZE, total.value)
  } catch (error) {
    message.error(`待审图片加载失败：${(error as Error).message}`)
  } finally {
    loading.value = false
  }
}

/** 放行或驳回；驳回走二次确认（不可逆）。 */
function handleAudit(row: ImageListItem, action: 'approve' | 'reject') {
  if (!row.id) return
  const run = async () => {
    try {
      await auditImage(String(row.id), action)
      message.success(action === 'approve' ? '已放行' : '已驳回')
    } catch (error) {
      message.error(`${action === 'approve' ? '放行' : '驳回'}失败：${(error as Error).message}`)
    }
    await refresh()
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

function removeImage(row: ImageListItem) {
  if (!row.id) return
  dialog.warning({
    title: '删除图片',
    content: `将从存储与图片库中删除「${row.file_name || row.object_key || row.id}」。确定吗？`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await deleteImages([String(row.id)])
        message.success('图片已删除')
      } catch (error) {
        message.error(`删除失败：${(error as Error).message}`)
      }
      await refresh()
    },
  })
}

watch([keyword, queue], () => {
  page.value = 1
  void refresh()
})
watch(page, refresh)
onMounted(() => {
  void refresh()
})
</script>

<template>
  <NCard class="admin-card" title="图片审核">
    <NSpace class="mb-4" align="center">
      <NSelect v-model:value="queue" :options="queueOptions" class="w-56" data-role="image-audit-queue" />
      <NInput
        v-model:value="keyword"
        placeholder="搜文件名或对象 key"
        clearable
        class="admin-field--select"
        data-role="image-audit-search" />
      <span class="text-sm opacity-70">{{ queue === 'open' ? '待处理' : '已驳回' }} {{ total }} 张</span>
    </NSpace>
    <NDataTable :columns="columns" :data="images" :loading="loading" :bordered="false" />
    <div class="mt-4 flex justify-end gap-2">
      <NButton :disabled="page <= 1" @click="page--">
        上一页
      </NButton>
      <NButton :disabled="!hasMore" @click="page++">
        下一页
      </NButton>
    </div>
  </NCard>
</template>



