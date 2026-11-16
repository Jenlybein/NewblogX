<script setup lang="ts">
/**
 * 审核 → 关键词风险（`/review/content-filter`）：命中记录只读页（ADR-0032 决策 7 / 票 396）。
 *
 * 这一票**只读**：能看、能按文本面 / 状态 / 用户筛、能分页，不能处置。
 * 处置动作（删除内容 / 忽略 / 封禁 / 禁言）是下一票（票 398），届时在本页加按钮。
 */
import { h } from 'vue'
import { NButton, NCard, NDataTable, NEmpty, NInput, NSelect, NSpace, NTag, useMessage } from 'naive-ui'
import { getContentFilterHits } from '~/services/admin'
import type { ContentFilterHitItem, ContentFilterHitSnippet, ContentFilterTopUser } from '~/types/api'
import {
  CONTENT_FILTER_STATUS_OPTIONS,
  CONTENT_FILTER_SURFACE_OPTIONS,
  contentFilterCategoryLabel,
  contentFilterStatusLabel,
  contentFilterStatusTagType,
  contentFilterSurfaceLabel,
  contentFilterTierMeta,
  splitContentFilterSnippet,
} from '~/utils/contentFilterRisk'
import { formatDateTime, shortId } from '~/utils/format'

const message = useMessage()

// 初始为 true：首屏还没请求时不要先闪一下空态。
const loading = ref(true)
const page = ref(1)
const limit = ref(10)
const total = ref(0)
const surface = ref<string | null>(null)
// 默认只看待处理：风险页的第一用途是"把没看过的看一遍"。
const status = ref<string | null>('pending')
const userId = ref('')
const rows = ref<ContentFilterHitItem[]>([])
const topUsers = ref<ContentFilterTopUser[]>([])

// as const 的选项表要复制成可变数组：NSelect 的 options 是 SelectOption[]，
// 只读元组直接传会因 readonly 而编译不过。
const surfaceOptions = CONTENT_FILTER_SURFACE_OPTIONS.map(option => ({ ...option }))
const statusOptions = CONTENT_FILTER_STATUS_OPTIONS.map(option => ({ ...option }))

/** 一段上下文：命中词按档位高亮（拦截红底、观察黄底）。 */
function renderSnippet(tier: string, snippet: ContentFilterHitSnippet) {
  const meta = contentFilterTierMeta(tier)
  const { before, hit, after } = splitContentFilterSnippet(snippet.text, snippet.start, snippet.end)
  return h('div', { class: 'text-xs leading-relaxed break-all' }, [
    before,
    h('mark', { class: `${meta.highlightClass} rounded px-0.5` }, hit),
    after,
  ])
}

const columns = [
  {
    title: '时间',
    key: 'created_at',
    width: 165,
    render: (row: ContentFilterHitItem) => formatDateTime(row.created_at),
  },
  {
    title: '文本面',
    key: 'surface',
    width: 90,
    render: (row: ContentFilterHitItem) =>
      h(NTag, { size: 'small', bordered: false, type: 'info' }, {
        default: () => contentFilterSurfaceLabel(row.surface),
      }),
  },
  {
    title: '内容',
    key: 'content_id',
    width: 170,
    render: (row: ContentFilterHitItem) =>
      h('div', null, [
        h('div', null, `#${shortId(row.content_id)}`),
        h('div', { class: 'muted text-xs' }, `用户 ${row.user_nickname || shortId(row.user_id)}`),
      ]),
  },
  {
    title: '命中词',
    key: 'words',
    minWidth: 170,
    render: (row: ContentFilterHitItem) =>
      h(NSpace, { size: 4, wrap: true }, {
        default: () =>
          row.words.map((word) => {
            const meta = contentFilterTierMeta(word.tier)
            return h(NTag, { size: 'small', bordered: false, type: meta.type }, {
              default: () => `${word.word}（${meta.label}·${contentFilterCategoryLabel(word.category)}）`,
            })
          }),
      }),
  },
  {
    title: '上下文（命中词前后 20 字）',
    key: 'snippets',
    minWidth: 320,
    render: (row: ContentFilterHitItem) =>
      h('div', { class: 'space-y-1' },
        row.words.flatMap(word => word.snippets.map(snippet => renderSnippet(word.tier, snippet)))),
  },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row: ContentFilterHitItem) =>
      h(NTag, { size: 'small', bordered: false, type: contentFilterStatusTagType(row.status) }, {
        default: () => contentFilterStatusLabel(row.status),
      }),
  },
  { title: '命中处数', key: 'hit_count', width: 95 },
]

async function refresh() {
  loading.value = true
  try {
    const data = await getContentFilterHits({
      page: page.value,
      limit: limit.value,
      // 空串 / null 一律不传：后端把"不筛"与"筛一个空值"当成两件事。
      surface: surface.value || undefined,
      status: status.value || undefined,
      user_id: userId.value.trim() || undefined,
    })
    rows.value = data.list
    topUsers.value = data.top_users
    total.value = data.count
  } catch {
    message.error('关键词风险加载失败')
  } finally {
    loading.value = false
  }
}

/** 点 Top 榜里的某个用户 = 按该用户筛选；再点一次同一人取消筛选。 */
function filterByUser(user: ContentFilterTopUser) {
  userId.value = userId.value === user.user_id ? '' : user.user_id
}

function resetFilters() {
  surface.value = null
  status.value = 'pending'
  userId.value = ''
  page.value = 1
}

// 翻页只重取；改筛选回到第一页（否则会停在一个空页上）。
watch([page, limit], refresh)
watch([surface, status, userId], () => {
  if (page.value !== 1) {
    page.value = 1
    return
  }
  refresh()
})

onMounted(refresh)
</script>

<template>
  <NSpace vertical :size="20">
    <NCard class="admin-card" title="命中用户 Top">
      <p class="muted m-0 mb-3">最近 90 天命中处数最多的用户。点一下按该用户筛选，再点一次取消。</p>
      <NEmpty v-if="topUsers.length === 0" description="最近 90 天没有命中记录" />
      <NSpace v-else :size="8">
        <NButton
          v-for="user in topUsers"
          :key="user.user_id"
          size="small"
          secondary
          :type="userId === user.user_id ? 'primary' : 'default'"
          @click="filterByUser(user)">
          {{ user.user_nickname || shortId(user.user_id) }} · {{ user.hit_count }} 处
        </NButton>
      </NSpace>
    </NCard>

    <NCard class="admin-card">
      <div class="admin-toolbar">
        <NSelect
          v-model:value="surface"
          :options="surfaceOptions"
          clearable
          placeholder="全部文本面"
          class="w-full md:w-48" />
        <NSelect
          v-model:value="status"
          :options="statusOptions"
          clearable
          placeholder="全部状态"
          class="w-full md:w-48" />
        <NInput v-model:value="userId" placeholder="按用户 ID 筛选" class="w-full md:w-44" />
        <NButton secondary @click="resetFilters">重置筛选</NButton>
        <NButton secondary :loading="loading" @click="refresh">刷新</NButton>
      </div>
    </NCard>

    <NCard class="admin-card" title="命中记录">
      <!-- 空列表是正常读面，不是错误：给空态，不弹"加载失败"。 -->
      <NEmpty
        v-if="!loading && rows.length === 0"
        class="py-10"
        description="没有符合条件的命中记录" />
      <NDataTable
        v-else
        :row-key="(row: ContentFilterHitItem) => row.id"
        :columns="columns"
        :data="rows"
        :loading="loading"
        :pagination="{
          page,
          pageSize: limit,
          itemCount: total,
          showSizePicker: true,
          pageSizes: [10, 20, 50],
          onUpdatePage: (next: number) => (page = next),
          onUpdatePageSize: (size: number) => {
            limit = size
            page = 1
          },
        }"
        :bordered="false" />
    </NCard>

    <p class="muted m-0 text-xs">
      这一页只读：处置动作（删除内容 / 忽略 / 封禁 / 禁言）在下一票提供。命中词与上下文不外泄到任何公开端点。
    </p>
  </NSpace>
</template>
