<script setup lang="ts">
import { h } from 'vue'
import { NButton, NCard, NDataTable, NInput, NModal, NSelect, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import { getArticleReviewTasks, reviewArticleTask } from '~/services/admin'
import type { ArticleReviewTaskItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const page = ref(1)
const limit = ref(10)
const total = ref(0)
const status = ref<'pending' | 'approved' | 'rejected' | 'canceled' | undefined>('pending')
const rows = ref<ArticleReviewTaskItem[]>([])
const rejectTarget = ref<ArticleReviewTaskItem | null>(null)
const rejectReason = ref('')
const rejectVisible = computed({
  get: () => !!rejectTarget.value,
  set: (value: boolean) => {
    if (!value) rejectTarget.value = null
  },
})

const statusOptions = [
  { label: '待处理', value: 'pending' },
  { label: '已通过', value: 'approved' },
  { label: '已驳回', value: 'rejected' },
  { label: '已取消', value: 'canceled' },
]

const columns = [
  { title: '文章', key: 'article_title', minWidth: 240 },
  { title: '作者', key: 'author_name', width: 140 },
  {
    title: '来源',
    key: 'source',
    width: 110,
    render: (row: ArticleReviewTaskItem) => h(NTag, { size: 'small' }, { default: () => sourceLabel(row.source) }),
  },
  {
    title: '状态',
    key: 'status',
    width: 120,
    render: (row: ArticleReviewTaskItem) =>
      h(NTag, { type: reviewStatusType(row.status), size: 'small' }, { default: () => reviewStatusLabel(row.status) }),
  },
  {
    title: '提交时间',
    key: 'created_at',
    width: 180,
    render: (row: ArticleReviewTaskItem) => formatDateTime(row.created_at),
  },
  {
    title: '处理',
    key: 'actions',
    width: 190,
    render: (row: ArticleReviewTaskItem) =>
      h(NSpace, { size: 8 }, {
        default: () => [
          h(
            NButton,
            { size: 'small', type: 'primary', disabled: row.status !== 'pending', onClick: () => approve(row) },
            { default: () => '通过' },
          ),
          h(
            NButton,
            { size: 'small', tertiary: true, type: 'error', disabled: row.status !== 'pending', onClick: () => openReject(row) },
            { default: () => '驳回' },
          ),
        ],
      }),
  },
]

function sourceLabel(value: string) {
  return ({ create: '新建', edit: '编辑', resubmit: '重提' } as Record<string, string>)[value] || value
}

function reviewStatusLabel(value: string) {
  return ({ pending: '待处理', approved: '已通过', rejected: '已驳回', canceled: '已取消' } as Record<string, string>)[value] || value
}

function reviewStatusType(value: string) {
  if (value === 'approved') return 'success'
  if (value === 'rejected') return 'error'
  if (value === 'canceled') return 'default'
  return 'warning'
}

async function refresh() {
  loading.value = true
  try {
    const data = await getArticleReviewTasks({ page: page.value, limit: limit.value, status: status.value })
    rows.value = data.list
    total.value = data.count
  } catch {
    message.error('审核队列加载失败')
  } finally {
    loading.value = false
  }
}

function approve(row: ArticleReviewTaskItem) {
  dialog.warning({
    title: '确认通过审核',
    content: `文章「${row.article_title}」将进入发布态。`,
    positiveText: '通过',
    negativeText: '取消',
    async onPositiveClick() {
      await reviewArticleTask(row.id, { status: 3 })
      message.success('已通过审核')
      await refresh()
    },
  })
}

function openReject(row: ArticleReviewTaskItem) {
  rejectTarget.value = row
  rejectReason.value = ''
}

async function reject() {
  if (!rejectTarget.value) return
  if (!rejectReason.value.trim()) {
    message.warning('请填写驳回原因')
    return
  }

  await reviewArticleTask(rejectTarget.value.id, { status: 4, reason: rejectReason.value.trim() })
  message.success('已驳回审核任务')
  rejectTarget.value = null
  await refresh()
}

watch([page, status], refresh)
onMounted(refresh)
</script>

<template>
  <NSpace vertical :size="20">
    <NCard class="admin-card">
      <div class="admin-toolbar">
        <NSelect v-model:value="status" :options="statusOptions" clearable class="w-full md:w-56" />
        <NButton secondary :loading="loading" @click="refresh">刷新队列</NButton>
      </div>
    </NCard>

    <NCard class="admin-card" title="审核任务">
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :pagination="{ page, pageSize: limit, itemCount: total, onUpdatePage: (next: number) => (page = next) }"
        :bordered="false"
      />
    </NCard>

    <NModal v-model:show="rejectVisible" preset="card" class="max-w-[560px]" title="驳回文章">
      <NSpace vertical>
        <p class="muted m-0">{{ rejectTarget?.article_title }}</p>
        <NInput v-model:value="rejectReason" type="textarea" :autosize="{ minRows: 4, maxRows: 6 }" placeholder="请输入驳回原因" />
        <div class="flex justify-end gap-3">
          <NButton @click="rejectTarget = null">取消</NButton>
          <NButton type="error" @click="reject">确认驳回</NButton>
        </div>
      </NSpace>
    </NModal>
  </NSpace>
</template>
