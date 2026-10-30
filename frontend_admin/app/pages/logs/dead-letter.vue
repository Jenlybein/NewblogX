<script setup lang="ts">
/**
 * 同步死信（DLQ）。
 *
 * 这是"同步失败之后怎么办"的闭环页面：
 *   1. 列出所有失败留证的记录（`cdc_dead_letter`）；
 *   2. 勾选后重投，逐条看结果；
 *   3. 右上有「回放日志」入口，看每次重投的时间线。
 *
 * 重投的语义与支持范围见 docs/adr/0008-dead-letter-replay.md：
 *   - 能重投：es_river（搜索同步）、outbox_relay（领域事件）
 *   - 不能重投：es_projection（没有主键）、image_ref_river（行数据类型还原不了）
 *     —— 这类会带着"该改用别的什么办法"一起返回，不会假装成功。
 */
import { h } from 'vue'
import { NButton, NCard, NDataTable, NSelect, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { getDeadLetters, replayDeadLetters } from '~/services/admin'
import type { DeadLetterItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'
import { canReplay, deadLetterStatusLabel, replayResultLabel, streamLabel } from '~/utils/deadLetters'
import { hasNextPage } from '~/utils/pagination'

const PAGE_SIZE = 10

const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const replaying = ref(false)
const page = ref(1)
const total = ref(0)
const rows = ref<DeadLetterItem[]>([])
const checked = ref<string[]>([])
const filterStream = ref<string | null>(null)
const filterStatus = ref<string | null>(null)

const streamOptions = [
  { label: '全部来源', value: '' },
  { label: streamLabel('es_river'), value: 'es_river' },
  { label: streamLabel('outbox_relay'), value: 'outbox_relay' },
  { label: streamLabel('es_projection'), value: 'es_projection' },
  { label: streamLabel('image_ref_river'), value: 'image_ref_river' },
]

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: deadLetterStatusLabel('pending'), value: 'pending' },
  { label: deadLetterStatusLabel('resolved'), value: 'resolved' },
]

const columns: DataTableColumns<DeadLetterItem> = [
  { type: 'selection', disabled: (row: DeadLetterItem) => !canReplay(row.status) },
  {
    title: '状态',
    key: 'status',
    width: 96,
    render: (row: DeadLetterItem) =>
      h(
        NTag,
        { size: 'small', type: row.status === 'resolved' ? 'success' : 'warning' },
        { default: () => deadLetterStatusLabel(row.status) },
      ),
  },
  { title: '来源', key: 'stream', width: 170, render: (row: DeadLetterItem) => streamLabel(row.stream) },
  { title: '源表', key: 'source_table', width: 190 },
  {
    title: 'CDC 任务',
    key: 'cdc_job_id',
    minWidth: 240,
    render: (row: DeadLetterItem) => h('span', { class: 'admin-mono' }, row.cdc_job_id),
  },
  { title: '重试', key: 'retry_count', width: 70 },
  {
    title: '最后错误',
    key: 'error_msg',
    minWidth: 240,
    render: (row: DeadLetterItem) => `${row.error_code || ''} ${row.error_msg || ''}`.trim() || '-',
  },
  {
    title: '时间',
    key: 'updated_at',
    width: 180,
    render: (row: DeadLetterItem) => formatDateTime(row.updated_at || row.created_at),
  },
]

async function refresh() {
  loading.value = true
  try {
    const data = await getDeadLetters({
      page: page.value,
      limit: PAGE_SIZE,
      stream: filterStream.value || undefined,
      status: filterStatus.value || undefined,
    })
    rows.value = data.list || []
    total.value = data.count
    // 刷新后清掉勾选：列表换了一页还留着旧 ID 会误点重投。
    checked.value = []
  } catch {
    message.error('死信列表加载失败')
  } finally {
    loading.value = false
  }
}

function applyFilter() {
  page.value = 1
  refresh()
}

async function doReplay() {
  const ids = checked.value.slice()
  if (ids.length === 0) {
    message.warning('请先勾选要重投的记录')
    return
  }
  dialog.warning({
    title: '重投死信',
    content: `将重投 ${ids.length} 条记录。已解决的会自动跳过；不支持的来源会告诉你该改用别的什么办法。`,
    positiveText: '开始重投',
    negativeText: '取消',
    async onPositiveClick() {
      replaying.value = true
      try {
        const data = await replayDeadLetters(ids)
        report(data.list || [])
        await refresh()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '重投失败')
      } finally {
        replaying.value = false
      }
    },
  })
}

/** 把逐条结果汇总成一句人话，再把"没成功"的详细原因单独提示。 */
function report(results: { result: string; message: string }[]) {
  const counts = results.reduce<Record<string, number>>((acc, item) => {
    acc[item.result] = (acc[item.result] || 0) + 1
    return acc
  }, {})
  const summary = Object.entries(counts)
    .map(([result, count]) => `${replayResultLabel(result)} ${count}`)
    .join(' / ')
  if (counts.failed) {
    message.warning(`重投完成：${summary}`)
  } else {
    message.success(`重投完成：${summary || '没有记录'}`)
  }
  const problem = results.find((item) => item.result === 'unsupported' || item.result === 'failed')
  if (problem) {
    dialog.info({
      title: replayResultLabel(problem.result),
      content: problem.message || '后端未返回原因',
      positiveText: '知道了',
    })
  }
}

const hasNext = computed(() => hasNextPage(page.value, PAGE_SIZE, total.value))

watch(page, refresh)
onMounted(refresh)
</script>

<template>
  <NCard class="admin-card" title="同步死信">
    <template #header-extra>
      <NSpace>
        <NuxtLink to="/logs"><NButton size="small" quaternary>回放日志</NButton></NuxtLink>
        <NButton size="small" quaternary @click="refresh">刷新</NButton>
      </NSpace>
    </template>

    <p class="admin-hint">
      数据同步（MySQL → 搜索/领域事件）重试耗尽后的失败记录。勾选后可以在这里重投一次，
      不必再进数据库手工补数据。
    </p>

    <NSpace class="mb-3" align="center">
      <NSelect v-model:value="filterStream" :options="streamOptions" placeholder="全部来源" clearable style="width: 200px" @update:value="applyFilter" />
      <NSelect v-model:value="filterStatus" :options="statusOptions" placeholder="全部状态" clearable style="width: 160px" @update:value="applyFilter" />
      <NButton type="primary" :loading="replaying" :disabled="checked.length === 0" @click="doReplay">
        重投选中（{{ checked.length }}）
      </NButton>
      <span class="admin-hint">共 {{ total }} 条</span>
    </NSpace>

    <NDataTable
      v-model:checked-row-keys="checked"
      :columns="columns"
      :data="rows"
      :loading="loading"
      :bordered="false"
      :row-key="(row: DeadLetterItem) => String(row.id)"
    />

    <div class="mt-4 flex justify-end gap-2">
      <NButton :disabled="page <= 1" @click="page--">上一页</NButton>
      <NButton :disabled="!hasNext" @click="page++">下一页</NButton>
    </div>
  </NCard>
</template>
