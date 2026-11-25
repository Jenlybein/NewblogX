<script setup lang="ts">
/**
 * 审核 → 关键词风险（`/review/content-filter`）：命中记录与处置（ADR-0032 决策 7 / 票 396、398、399）。
 *
 * 能看、能按文本面 / 状态 / 用户筛、能分页；也能处置四类动作：
 * **删除命中的内容**、**忽略误报**、**封禁账号**（账号禁用）、**禁言账号**（带时长，默认 7 天）。
 *
 * 内容动作按文本面换文案（见 utils/contentFilterRisk.ts 的
 * CONTENT_FILTER_SURFACE_ACTIONS）：昵称 / 简介上是「重置资料」，私信面禁用删除。
 * 账号动作不删内容：内容早已被替换成 `*` 并发布，处罚对象是账号（封禁 / 禁言）。
 */
import { h } from 'vue'
import { NButton, NCard, NDataTable, NDatePicker, NDescriptions, NDescriptionsItem, NDrawer, NDrawerContent, NEmpty, NInput, NInputNumber, NPopconfirm, NSelect, NSpace, NTag, useMessage } from 'naive-ui'
import { getContentFilterHits, handleContentFilterHit, handleContentFilterHits } from '~/services/admin'
import type { ContentFilterHitItem, ContentFilterHitSnippet, ContentFilterTopUser } from '~/types/api'
import {
  CONTENT_FILTER_MUTE_DAYS_DEFAULT,
  CONTENT_FILTER_MUTE_DAYS_MAX,
  CONTENT_FILTER_MUTE_DAYS_MIN,
  CONTENT_FILTER_STATUS_OPTIONS,
  CONTENT_FILTER_SURFACE_ACTIONS,
  CONTENT_FILTER_SURFACE_OPTIONS,
  contentFilterActionLabel,
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
// 命中词（子串）与命中时间范围：排查"某个词打到了谁""那几天发生过什么"。
const word = ref('')
const dateRange = ref<[number, number] | null>(null)

/** 毫秒时间戳 → 后端要的 `YYYY-MM-DD`（右边界含当天，由后端 +1 天处理）。 */
function toDay(ms: number): string {
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
const rows = ref<ContentFilterHitItem[]>([])
// 详情抽屉：一行命中的全部信息（命中词明细、上下文、处置痕迹、关联内容入口）。
// 列表里只放得下摘要，处置时最需要的是"这条到底打在哪、上下文长什么样"。
const detailRow = ref<ContentFilterHitItem | null>(null)
const detailOpen = ref(false)

function openDetail(row: ContentFilterHitItem) {
  detailRow.value = row
  detailOpen.value = true
}

/**
 * 关联内容入口：按文本面拼到读者站的具体页面。
 *
 * 只给**确有其页**的面：文章 / 合集 / 用户名下的用户主页。评论与私信没有独立
 * 页面，硬拼一个只会 404，所以返回 null（抽屉里仍然显示内容 ID）。
 */
function contentLink(row: ContentFilterHitItem): { label: string, href: string } | null {
  const id = String(row.content_id || '')
  if (!id) return null
  switch (row.surface) {
    case 'article':
      return { label: '打开文章', href: `/article/${id}` }
    case 'collection':
      return { label: '打开合集', href: `/collections/${id}` }
    case 'nickname':
    case 'username':
    case 'abstract':
      return { label: '打开用户主页', href: `/users/${String(row.user_id || '')}` }
    default:
      return null
  }
}
const topUsers = ref<ContentFilterTopUser[]>([])
// 批量处置要的选中集合：naive-ui 的 NDataTable 用 checkedRowKeys 回传。
const checkedKeys = ref<string[]>([])
const handling = ref(false)
// 处置备注：同一批共用一个备注（逐条备注只会让审计表更难读）。
const note = ref('')
// 禁言天数（票 399）：单条「禁言」用它，后端再做 1–365 的最终校验（缺省 7 天）。
const muteDays = ref(CONTENT_FILTER_MUTE_DAYS_DEFAULT)

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
  {
    // 处置结果：昵称与用户名是**命中即拒绝**（补丁六），写都没写进去，所以这里
    // 显示"已拒绝"；其余面是"打星后照存"，显示"已替换"。两句话的差别正是运营
    // 判断"要不要找这个人"的依据 —— 拒绝说明他在试探，替换说明内容已经在库里了。
    title: '处置结果',
    key: 'disposal',
    width: 130,
    render: (row: ContentFilterHitItem) =>
      h(
        NTag,
        {
          size: 'small',
          bordered: false,
          type: row.surface === 'nickname' || row.surface === 'username' ? 'error' : 'default',
        },
        {
          default: () => (row.surface === 'nickname' || row.surface === 'username' ? '已拒绝写入' : '已替换'),
        },
      ),
  },
  {
    // 处置动作（票 398 / 399）：待处理的行才给按钮；已处置的行显示处理痕迹。
    // 二次确认放在按钮里（NPopconfirm）—— 删除不可逆、封禁/禁言是账号级处罚。
    title: '处置',
    key: 'actions',
    width: 380,
    render: (row: ContentFilterHitItem) => {
      if (row.status !== 'pending') {
        // 处理痕迹：谁、什么备注。已处置的行不给按钮，重复点只会是空操作。
        const who = row.handler_id ? `管理员 ${shortId(row.handler_id)}` : '已处置'
        return h('div', { class: 'text-xs muted' }, row.note ? `${who}：${row.note}` : who)
      }
      const meta = actionMeta(row)
      return h(NSpace, { size: 6, wrap: true }, {
        default: () => [
          h(NPopconfirm, {
            onPositiveClick: () => handleOne(row, 'delete'),
          }, {
            trigger: () => h(NButton, {
              size: 'tiny',
              type: 'error',
              secondary: true,
              disabled: !meta.deletable || handling.value,
            }, { default: () => meta.deleteLabel }),
            default: () =>
              meta.deletable
                ? `确认「${meta.deleteLabel}」？原文已被替换成 *，不可恢复。`
                : '私信没有可删除的内容，请改用「忽略」。',
          }),
          h(NPopconfirm, {
            onPositiveClick: () => handleOne(row, 'ignore'),
          }, {
            trigger: () => h(NButton, {
              size: 'tiny',
              secondary: true,
              disabled: handling.value,
            }, { default: () => '忽略' }),
            default: () => '确认把这条标记为误报？内容保持原样。',
          }),
          // 账号动作：内容保留，处罚落在内容作者账号上。
          h(NPopconfirm, {
            onPositiveClick: () => handleOne(row, 'ban'),
          }, {
            trigger: () => h(NButton, {
              size: 'tiny',
              secondary: true,
              type: 'error',
              disabled: handling.value,
            }, { default: () => '封禁' }),
            default: () => `确认封禁该用户「${row.user_nickname || shortId(row.user_id)}」？封禁后登录与发布都被拦，且不会自动恢复。`,
          }),
          h(NPopconfirm, {
            onPositiveClick: () => handleOne(row, 'mute'),
          }, {
            trigger: () => h(NButton, {
              size: 'tiny',
              secondary: true,
              type: 'warning',
              disabled: handling.value,
            }, { default: () => `禁言 ${muteDays.value} 天` }),
            default: () => `确认禁言该用户「${row.user_nickname || shortId(row.user_id)}」${muteDays.value} 天？账号仍可登录/浏览，发布类动作会被拒，到期自动恢复。`,
          }),
        ],
      })
    },
  },
  {
    // 详情独立成列而不是"点行打开"：这一页有勾选框（批量处置），点行会与勾选冲突。
    title: '详情',
    key: 'detail',
    width: 90,
    render: (row: ContentFilterHitItem) =>
      h(
        NButton,
        { size: 'tiny', quaternary: true, onClick: () => openDetail(row) },
        { default: () => '查看' },
      ),
  },
]


/**
 * 处置一条记录（票 398 / 399）。
 *
 * 二次确认不能省：删除是不可逆的（原文已被替换成 `*`，没有"恢复原文"这条路），
 * 误点一次就把一篇正常文章删了；封禁 / 禁言是账号级处罚，同样先确认。
 * 账号动作的错误文案来自后端（例如天数越界的 user.mute_days_invalid），
 * 这里原样展示，不自己翻译。
 */
async function handleOne(row: ContentFilterHitItem, action: 'delete' | 'ignore' | 'ban' | 'mute') {
  handling.value = true
  try {
    const got = await handleContentFilterHit(row.id, {
      action,
      mute_days: action === 'mute' ? muteDays.value : undefined,
      note: note.value.trim() || undefined,
    })
    if (got.already_handled) {
      message.info('这条记录已被处置过，未重复执行')
    } else {
      message.success(actionSuccessText(action))
    }
    await refresh()
  } catch (error) {
    message.error(error instanceof Error
      ? `${contentFilterActionLabel(action)}失败：${error.message}`
      : `${contentFilterActionLabel(action)}失败，请稍后重试`)
  } finally {
    handling.value = false
  }
}

/** 单条处置成功后的提示（账号动作要说清处罚落在账号上、内容保留）。 */
function actionSuccessText(action: 'delete' | 'ignore' | 'ban' | 'mute'): string {
  if (action === 'delete') return '已删除命中的内容'
  if (action === 'ignore') return '已忽略'
  if (action === 'ban') return '账号已封禁（登录与发布都被拦）'
  return `账号已禁言 ${muteDays.value} 天（到期自动恢复）`
}

/**
 * 批量处置：后端返回**逐条结果**，这里按 ok 汇总提示。
 *
 * 不把失败弹成一串错误框：一批里混着私信面（没有可删内容）是常态，
 * 给一句"N 条成功、M 条失败（原因见下）"比连弹三次更好用。
 */
async function handleSelected(action: 'delete' | 'ignore') {
  if (checkedKeys.value.length === 0) {
    message.warning('先勾选要处置的记录')
    return
  }
  handling.value = true
  try {
    const got = await handleContentFilterHits({
      ids: checkedKeys.value,
      action,
      note: note.value.trim() || undefined,
    })
    const failed = got.results.filter(item => !item.ok)
    if (failed.length === 0) {
      message.success(`已处置 ${got.results.length} 条`)
    } else {
      message.warning(`成功 ${got.results.length - failed.length} 条，失败 ${failed.length} 条：${failed[0]!.message}`)
    }
    checkedKeys.value = []
    await refresh()
  } catch {
    message.error('批量处置失败')
  } finally {
    handling.value = false
  }
}

/** 某个面的删除按钮文案与是否可用（昵称 / 简介是"重置资料"，私信禁删）。 */
function actionMeta(row: ContentFilterHitItem) {
  return CONTENT_FILTER_SURFACE_ACTIONS[row.surface] ?? { deleteLabel: '删除内容', deletable: true }
}

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
      word: word.value.trim() || undefined,
      start_at: dateRange.value ? toDay(dateRange.value[0]) : undefined,
      end_at: dateRange.value ? toDay(dateRange.value[1]) : undefined,
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
  word.value = ''
  dateRange.value = null
  page.value = 1
}

// 翻页只重取；改筛选回到第一页（否则会停在一个空页上）。
watch([page, limit], refresh)
watch([surface, status, userId, word, dateRange], () => {
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
        <NInput v-model:value="word" placeholder="按命中词筛选" clearable class="w-full md:w-40" data-role="risk-word" />
        <NDatePicker v-model:value="dateRange" type="daterange" clearable data-role="risk-date-range" />
        <NButton secondary @click="resetFilters">重置筛选</NButton>
        <NButton secondary :loading="loading" @click="refresh">刷新</NButton>
      </div>
    </NCard>

    <NCard class="admin-card" title="批量处置">
      <p class="muted m-0 mb-3">
        勾选上方「待处理」的行后选择动作。备注会写到每条命中记录上（最多 200 字），
        用于事后追溯。
      </p>
      <div class="admin-toolbar">
        <NInput
          v-model:value="note"
          placeholder="处置备注（可选）"
          maxlength="200"
          show-count
          class="w-full md:w-72" />
        <!-- 禁言天数（票 399）：单条「禁言」按钮用它；缺省 7 天，范围 1–365。 -->
        <span class="muted text-xs">禁言天数</span>
        <NInputNumber
          v-model:value="muteDays"
          :min="CONTENT_FILTER_MUTE_DAYS_MIN"
          :max="CONTENT_FILTER_MUTE_DAYS_MAX"
          :step="1"
          class="w-28" />
        <span class="muted text-xs">已选 {{ checkedKeys.length }} 条</span>
        <NButton
          type="error"
          secondary
          :disabled="checkedKeys.length === 0 || handling"
          :loading="handling"
          @click="handleSelected('delete')">
          删除内容
        </NButton>
        <NButton
          secondary
          :disabled="checkedKeys.length === 0 || handling"
          :loading="handling"
          @click="handleSelected('ignore')">
          忽略
        </NButton>
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
        v-model:checked-row-keys="checkedKeys"
        :row-key="(row: ContentFilterHitItem) => row.id"
        :row-selection="{
          checks: (row: ContentFilterHitItem) => row.status === 'pending',
        }"
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
      删除不可逆：原文已被替换成等长 <code>*</code>，没有「恢复原文」这条路。昵称 / 简介上的「重置资料」会把昵称改成
      「问题昵称 + uid 末 4 位」并清空简介；私信没有可删内容，只能用「忽略」。封禁 = 账号禁用（登录与发布都被拦，
      不会自动恢复）；禁言只拦发布类动作，到期自动恢复，也可在账号管理页提前解除。命中词与上下文不外泄到任何公开端点。
    </p>
  </NSpace>

  <!-- 详情抽屉：把"这条命中到底打在什么上下文、后来怎么处置的"一次说清。 -->
  <NDrawer v-model:show="detailOpen" placement="right" :width="520">
    <NDrawerContent v-if="detailRow" title="命中详情" closable>
      <NDescriptions :column="1" label-placement="left" bordered size="small">
        <NDescriptionsItem label="时间">
          {{ formatDateTime(String(detailRow.created_at || '')) }}
        </NDescriptionsItem>
        <NDescriptionsItem label="文本面">
          {{ contentFilterSurfaceLabel(detailRow.surface) }}
        </NDescriptionsItem>
        <NDescriptionsItem label="用户">
          {{ detailRow.user_nickname || '（无昵称）' }} · {{ detailRow.user_id }}
        </NDescriptionsItem>
        <NDescriptionsItem label="内容 ID">
          {{ detailRow.content_id || '-' }}
        </NDescriptionsItem>
        <NDescriptionsItem label="命中处数">
          {{ detailRow.hit_count }}
        </NDescriptionsItem>
        <NDescriptionsItem label="状态">
          {{ contentFilterStatusLabel(detailRow.status) }}
        </NDescriptionsItem>
        <NDescriptionsItem label="处置结果">
          {{ detailRow.surface === 'nickname' || detailRow.surface === 'username' ? '改名被拒（未写入）' : '文本已打星入库' }}
        </NDescriptionsItem>
        <NDescriptionsItem label="处置痕迹">
          <template v-if="detailRow.handled_at">
            {{ detailRow.handler_id ? `管理员 ${shortId(detailRow.handler_id)}` : '已处置' }}
            · {{ formatDateTime(String(detailRow.handled_at)) }}
            <template v-if="detailRow.note">
              ：{{ detailRow.note }}
            </template>
          </template>
          <template v-else>
            未处置
          </template>
        </NDescriptionsItem>
      </NDescriptions>

      <NSpace class="mt-4" :size="8" align="center">
        <a
          v-if="contentLink(detailRow)"
          :href="contentLink(detailRow)!.href"
          target="_blank"
          rel="noopener noreferrer"
          class="text-sm">
          {{ contentLink(detailRow)!.label }} ↗
        </a>
        <span v-else class="text-xs muted">该文本面没有独立页面（评论 / 私信）</span>
      </NSpace>

      <h4 class="mt-4 mb-2">
        命中词（{{ (detailRow.words || []).length }}）
      </h4>
      <div v-for="hitWord in detailRow.words || []" :key="hitWord.word" class="mb-3">
        <NSpace :size="6" align="center">
          <NTag size="small" :type="hitWord.tier === 'block' ? 'error' : 'warning'">
            {{ hitWord.word }}
          </NTag>
          <span class="text-xs muted">{{ hitWord.tier === 'block' ? '拦截词' : '观察词' }} · {{ hitWord.category }}</span>
        </NSpace>
        <div v-for="(snippet, i) in hitWord.snippets || []" :key="i" class="mt-1 text-xs muted">
          …{{ (snippet.text || '').slice(0, snippet.start ?? 0) }}<mark class="risk-hit">{{ (snippet.text || '').slice(snippet.start ?? 0, snippet.end ?? 0) }}</mark>{{ (snippet.text || '').slice(snippet.end ?? 0) }}…
        </div>
      </div>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
/* 命中词在上下文里高亮：一眼看到打的是哪几个字，而不是自己数位置。 */
.risk-hit {
  background: var(--color-danger-soft);
  color: inherit;
  padding: 0 1px;
  border-radius: 2px;
}
</style>

