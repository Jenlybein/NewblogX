<script setup lang="ts">
/**
 * 申诉处置（ADR-0035）。
 *
 * 只做两件事：看申诉理由、给一个处置结论。**驳回必须填理由** —— 那句理由
 * 会原样进站内信给用户看，也是"为什么这条不算违规"的唯一解释来源。
 *
 * 通过不是"关掉单子"：后端会把事件置撤销、重算分数、必要时解禁（见
 * moderation/app.HandleAppeal）。所以这里不再单独提供"撤销扣分"按钮，
 * 否则就有两条能改分数的路径。
 */
import { h } from 'vue'
import { NButton, NCard, NDataTable, NInput, NSelect, NSpace, NTag, useDialog, useMessage } from 'naive-ui'

import { getModerationAppeals, handleModerationAppeal } from '~/services/admin'
import type { ModerationAppealItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'

const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const statusFilter = ref('pending')
const appeals = ref<ModerationAppealItem[]>([])
/** 驳回理由：按申诉 id 分开存，避免串味。 */
const notes = ref<Record<string, string>>({})

const statusOptions = [
  { label: '待处理', value: 'pending' },
  { label: '已通过', value: 'approved' },
  { label: '已驳回', value: 'rejected' },
  { label: '全部', value: '' },
]

function noteOf(id: string) {
  return notes.value[id] ?? ''
}

function setNote(id: string, value: string) {
  notes.value = { ...notes.value, [id]: value }
}

function statusTagType(status: string) {
  if (status === 'approved') return 'success' as const
  if (status === 'rejected') return 'error' as const
  return 'warning' as const
}

const columns = [
  { title: '申诉 ID', key: 'id', width: 200, render: (row: ModerationAppealItem) => String(row.id || '-') },
  { title: '事件 ID', key: 'event_id', width: 200, render: (row: ModerationAppealItem) => String(row.event_id || '-') },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row: ModerationAppealItem) =>
      h(NTag, { size: 'small', type: statusTagType(String(row.status)) }, { default: () => String(row.status) }),
  },
  {
    title: '理由',
    key: 'reason',
    minWidth: 240,
    render: (row: ModerationAppealItem) => String(row.reason || '-'),
  },
  {
    title: '提交时间',
    key: 'created_at',
    width: 180,
    render: (row: ModerationAppealItem) => formatDateTime(String(row.created_at || '')),
  },
  {
    title: '处置',
    key: 'actions',
    width: 320,
    render: (row: ModerationAppealItem) => {
      if (row.status !== 'pending') {
        return h('span', { class: 'text-sm opacity-70' }, String(row.note || '已处理'))
      }
      return h('div', { class: 'flex flex-col gap-2' }, [
        h(NInput, {
          size: 'small',
          value: noteOf(String(row.id)),
          placeholder: '驳回理由（通过可留空）',
          maxlength: 512,
          'onUpdate:value': (value: string) => setNote(String(row.id), value),
        }),
        h('div', { class: 'flex gap-2' }, [
          h(
            NButton,
            { size: 'small', type: 'primary', secondary: true, onClick: () => handle(row, true) },
            { default: () => '通过' },
          ),
          h(
            NButton,
            { size: 'small', type: 'warning', secondary: true, onClick: () => handle(row, false) },
            { default: () => '驳回' },
          ),
        ]),
      ])
    },
  },
]

async function refresh() {
  loading.value = true
  try {
    const data = await getModerationAppeals(statusFilter.value || undefined)
    appeals.value = data.list || []
  } catch {
    message.error('申诉列表加载失败')
  } finally {
    loading.value = false
  }
}

/**
 * 处置一条申诉。
 *
 * 两个方向都要确认：通过会把分数退回去、可能解除禁言；驳回的文案会直接
 * 发给用户。点错一次就是一次误判，值得多点一下。
 */
function handle(row: ModerationAppealItem, approve: boolean) {
  const id = String(row.id)
  const note = noteOf(id).trim()
  if (!approve && !note) {
    message.warning('驳回必须填写理由')
    return
  }
  dialog.warning({
    title: approve ? '通过申诉' : '驳回申诉',
    content: approve
      ? '通过后会撤销该条违规、重算风险分，分数落到处罚线以下时自动解除禁言。确定吗？'
      : `驳回理由将发给用户：${note}`,
    positiveText: approve ? '通过' : '驳回',
    negativeText: '取消',
    async onPositiveClick() {
      await handleModerationAppeal(id, { approve, note })
      message.success(approve ? '已通过' : '已驳回')
      setNote(id, '')
      await refresh()
    },
  })
}

watch(statusFilter, () => {
  void refresh()
})
onMounted(() => {
  void refresh()
})
</script>

<template>
  <NCard class="admin-card" title="申诉处置">
    <NSpace class="mb-4" align="center">
      <NSelect v-model:value="statusFilter" class="w-40" :options="statusOptions" />
      <span class="text-sm opacity-70">
        共 {{ appeals.length }} 条；通过会撤销该条违规并重算分数
      </span>
    </NSpace>
    <NDataTable :columns="columns" :data="appeals" :loading="loading" :bordered="false" />
  </NCard>
</template>
