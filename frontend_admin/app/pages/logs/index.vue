<script setup lang="ts">
/**
 * 日志中心。
 *
 * 六类日志共用一张表，按 kind 换列：
 *   运行日志 runtime / 审计 action / 登录 login / CDC 事件 cdc / 回放 replay / AI 调用 ai
 *
 * 为什么做成"一个页面一个 kind 切换"而不是五个页面：五者的交互完全一样
 * （分页 + 关键字），只有列不同。五个页面等于把同一段分页逻辑抄五遍。
 *
 * 注意：这个页面之前**根本不存在** —— 侧边栏一直有「日志中心 → /logs」，
 * 点进去是 404。所以下面每一类日志都是第一次有界面。
 */
import { h } from 'vue'
import { NButton, NCard, NDataTable, NInput, NSpace, NTabPane, NTabs, NTag, useMessage } from 'naive-ui'
import { getLogs, getReplayLogs } from '~/services/admin'
import type { DataTableColumns } from 'naive-ui'
import { formatDateTime } from '~/utils/format'
import { replayResultTagType } from '~/utils/deadLetters'
import { aiResultLabel, sceneLabel } from '~/utils/aiTokens'

type LogKind = 'runtime' | 'action' | 'login' | 'cdc' | 'replay' | 'ai'

const PAGE_SIZE = 20

const message = useMessage()
const kind = ref<LogKind>('runtime')
const loading = ref(false)
const page = ref(1)
const total = ref(0)
const keyword = ref('')
const rows = ref<LogRow[]>([])

const kindOptions = [
  { label: '运行日志', value: 'runtime' },
  { label: '审计日志', value: 'action' },
  { label: '登录日志', value: 'login' },
  { label: 'CDC 事件', value: 'cdc' },
  { label: '回放日志', value: 'replay' },
  { label: 'AI 调用', value: 'ai' },
]

const kindHints: Record<LogKind, string> = {
  runtime: '接口调用的运行日志：耗时、状态码、错误栈。',
  action: '写操作的审计留痕：谁在什么时候改了哪个对象。',
  login: '登录事件：来源 IP/归属地、UA、失败原因。',
  cdc: 'MySQL → ES 的同步事件：成功/重试/进死信。',
  replay: '死信重投的时间线：每次人工重投的结果（含跳过与不支持）。',
  ai: '模型调用用量：场景、模型、token 数与耗时；失败会带错误原因。',
}

const levelTagType = (level: string) => {
  switch (String(level || '').toLowerCase()) {
    case 'error':
      return 'error'
    case 'warn':
      return 'warning'
    case 'info':
      return 'success'
    default:
      return 'default'
  }
}

const levelColumn: DataTableColumns<LogRow>[number] = {
  title: '级别',
  key: 'level',
  width: 90,
  render: row =>
    h(NTag, { size: 'small', type: levelTagType(text(row, 'level')) }, { default: () => text(row, 'level') || '-' }),
}

const tsColumn: DataTableColumns<LogRow>[number] = {
  title: '时间',
  key: 'ts',
  width: 180,
  render: row => formatDateTime(text(row, 'ts')),
}

const msgColumn: DataTableColumns<LogRow>[number] = {
  title: '内容',
  key: 'message',
  minWidth: 320,
  render: row => text(row, 'message') + (text(row, 'error_message') ? `（${text(row, 'error_message')}）` : ''),
}

/** 每类日志的列。只放"这类日志特有的信息"，公共列（时间/级别/内容）统一追加。 */
type LogRow = Record<string, unknown>

const text = (row: LogRow, key: string) => String(row[key] ?? '').trim()

const columnsByKind: Record<LogKind, DataTableColumns<LogRow>> = {
  runtime: [
    { title: '请求', key: 'method', width: 220, render: row => `${text(row, 'method')} ${text(row, 'path')}`.trim() || '-' },
    { title: '状态码', key: 'status_code', width: 90 },
    { title: '耗时(ms)', key: 'latency_ms', width: 100 },
  ],
  action: [
    { title: '动作', key: 'action_name', width: 160 },
    { title: '对象', key: 'target_type', width: 140, render: row => `${text(row, 'target_type') || '-'}#${text(row, 'target_id') || '-'}` },
    { title: '用户', key: 'user_id', width: 160 },
  ],
  login: [
    { title: '账号', key: 'username', width: 140 },
    { title: '方式', key: 'login_type', width: 100 },
    {
      title: '结果',
      key: 'success',
      width: 90,
      render: row =>
        h(NTag, { size: 'small', type: row.success ? 'success' : 'error' }, { default: () => (row.success ? '成功' : '失败') }),
    },
    { title: 'IP / 归属地', key: 'ip', width: 200, render: row => `${text(row, 'ip') || '-'} ${text(row, 'addr')}`.trim() },
  ],
  cdc: [
    { title: 'Stream', key: 'stream', width: 130 },
    { title: '源表', key: 'source_table', width: 200 },
    { title: '结果', key: 'result', width: 100 },
    { title: '重试', key: 'retry_count', width: 70 },
  ],
  replay: [
    { title: 'Stream', key: 'stream', width: 130 },
    { title: '源表', key: 'source_table', width: 180 },
    {
      title: '重投结果',
      key: 'result',
      width: 110,
      render: row =>
        h(NTag, { size: 'small', type: replayResultTagType(text(row, 'result')) }, { default: () => text(row, 'result') || '-' }),
    },
    { title: 'CDC 任务', key: 'cdc_job_id', minWidth: 240 },
  ],
  ai: [
    {
      title: '场景',
      key: 'scene',
      width: 120,
      render: row => h(NTag, { size: 'small', bordered: false }, { default: () => sceneLabel(text(row, 'scene')) }),
    },
    {
      title: '模型',
      key: 'model',
      minWidth: 220,
      render: row => `${text(row, 'vendor') || '-'} / ${text(row, 'model') || '-'}`,
    },
    {
      title: 'Token（提示/补全/合计）',
      key: 'total_tokens',
      width: 200,
      // 上游没回用量时显示 "-" 而不是 0：0 会被读成"这个模型不要钱"。
      render: row => (Number(row.usage_available) === 1
        ? `${text(row, 'prompt_tokens')} / ${text(row, 'completion_tokens')} / ${text(row, 'total_tokens')}`
        : '-'),
    },
    { title: '耗时(ms)', key: 'latency_ms', width: 100 },
    {
      title: '结果',
      key: 'success',
      width: 90,
      render: row =>
        h(
          NTag,
          { size: 'small', type: Number(row.success) === 1 ? 'success' : 'error' },
          { default: () => aiResultLabel(Number(row.success)) },
        ),
    },
  ],
}

const columns = computed<DataTableColumns<LogRow>>(() => [tsColumn, levelColumn, ...columnsByKind[kind.value], msgColumn])

/**
 * 取当前 kind 的一页数据。
 *
 * 用 switch 而不是 getLogs(kind.value, ...)：那个函数的重载按字面量返回不同的记录类型，
 * 传联合类型会报 "argument not assignable"。逐个分支写虽然啰嗦，但类型是真的被检查了。
 */
async function fetchPage(query: { page: number; limit: number }): Promise<{ list: LogRow[]; count: number }> {
  switch (kind.value) {
    case 'replay': {
      const data = await getReplayLogs(query)
      return { list: data.list as unknown as LogRow[], count: data.count }
    }
    case 'runtime': {
      const data = await getLogs('runtime', query)
      return { list: data.list as unknown as LogRow[], count: data.count }
    }
    case 'action': {
      const data = await getLogs('action', query)
      return { list: data.list as unknown as LogRow[], count: data.count }
    }
    case 'login': {
      const data = await getLogs('login', query)
      return { list: data.list as unknown as LogRow[], count: data.count }
    }
    case 'ai': {
      const data = await getLogs('ai', query)
      return { list: data.list as unknown as LogRow[], count: data.count }
    }
    default: {
      const data = await getLogs('cdc', query)
      return { list: data.list as unknown as LogRow[], count: data.count }
    }
  }
}

async function refresh() {
  loading.value = true
  try {
    const data = await fetchPage({ page: page.value, limit: PAGE_SIZE })
    rows.value = data.list
    total.value = data.count
  } catch {
    message.error('日志加载失败')
  } finally {
    loading.value = false
  }
}

function switchKind(value: LogKind) {
  kind.value = value
  page.value = 1
  keyword.value = ''
  refresh()
}

function search() {
  page.value = 1
  refresh()
}

const hasNext = computed(() => page.value * PAGE_SIZE < total.value)
const filteredRows = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) {
    return rows.value
  }
  // 关键字在前端过滤当前页：后端各类日志的关键字字段不一致，统一由前端做"当前页内查找"。
  return rows.value.filter((row) => JSON.stringify(row).toLowerCase().includes(kw))
})

watch(page, refresh)
onMounted(refresh)
</script>

<template>
  <NCard class="admin-card" title="日志中心">
    <template #header-extra>
      <span class="admin-hint">{{ kindHints[kind] }}</span>
    </template>

    <NTabs :value="kind" type="line" animated @update:value="switchKind">
      <NTabPane v-for="item in kindOptions" :key="item.value" :name="item.value" :tab="item.label" />
    </NTabs>

    <NSpace class="mb-3" align="center">
      <NInput v-model:value="keyword" clearable placeholder="在当前页内查找…" style="width: 260px" @keyup.enter="search" />
      <NButton type="primary" ghost @click="search">查找</NButton>
      <span class="admin-hint">共 {{ total }} 条</span>
    </NSpace>

    <NDataTable :columns="columns" :data="filteredRows" :loading="loading" :bordered="false" :row-key="(row: LogRow) => String(row.event_id || row.id || '')" />

    <div class="mt-4 flex justify-end gap-2">
      <NButton :disabled="page <= 1" @click="page--">上一页</NButton>
      <NButton :disabled="!hasNext" @click="page++">下一页</NButton>
    </div>
  </NCard>
</template>
