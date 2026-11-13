<script setup lang="ts">
import { h } from 'vue'
import { NButton, NCard, NDataTable, NEmpty, NInput, NModal, NSelect, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import ReviewBodyPreview from '~/components/review/ReviewBodyPreview.vue'
import { getArticleReviewReasons, getArticleReviewTasks, batchReviewArticleTasks, reviewArticleTask } from '~/services/admin'
import type { ArticleReviewReasonItem, ArticleReviewTaskItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'
import { complianceVerdictMeta, riskLabelText, riskLevelMeta } from '~/utils/reviewRisk'

const message = useMessage()
const dialog = useDialog()
// 初始为 true：首屏还没请求时不要先闪一下空态。
const loading = ref(true)
const page = ref(1)
const limit = ref(10)
const total = ref(0)
const status = ref<'pending' | 'approved' | 'rejected' | 'canceled' | undefined>('pending')
// 排序由服务端做：风险不落库，前端只能拿到排好序的一页（票 311）。
const sort = ref<'created_at' | 'risk'>('created_at')
const rows = ref<ArticleReviewTaskItem[]>([])
const rejectTarget = ref<ArticleReviewTaskItem | null>(null)
const rejectReason = ref('')
// 原因码必选：值来自后端固定词表，中文只在 option.label 里出现。
const rejectReasonCode = ref<ArticleReviewReasonItem['value'] | null>(null)
const reasonOptions = ref<ArticleReviewReasonItem[]>([])

// 批量操作的目标任务：既是"当前勾选的"，也是驳回弹窗要处理的那一批。
// 单条驳回也走这条路（ids 长度为 1），从而只有一套确认逻辑。
const checkedKeys = ref<string[]>([])
const pendingReviewIDs = ref<string[]>([])
const rejectVisible = computed({
  get: () => pendingReviewIDs.value.length > 0,
  set: (value: boolean) => {
    if (!value) {
      pendingReviewIDs.value = []
      rejectTarget.value = null
    }
  },
})

const statusOptions = [
  { label: '待处理', value: 'pending' },
  { label: '已通过', value: 'approved' },
  { label: '已驳回', value: 'rejected' },
  { label: '已取消', value: 'canceled' },
]

const sortOptions = [
  { label: '提交时间（默认）', value: 'created_at' },
  { label: '风险优先', value: 'risk' },
]

const columns = [
  // 勾选列：批量操作（spec 决策 32）的入口。行键是任务 ID。
  { type: 'selection' as const },
  { title: '文章', key: 'article_title', minWidth: 200 },
  {
    title: '风险',
    key: 'risk_level',
    minWidth: 230,
    // 机审前两层（词表 + 评分派生标签）的结论，每次读取现算（票 311）。
    render: (row: ArticleReviewTaskItem) => {
      const meta = riskLevelMeta(row.risk_level)
      const labels = (row.risk_labels ?? []).map((code) =>
        h(NTag, { size: 'small', bordered: false, type: 'info' }, { default: () => riskLabelText(code) }),
      )
      return h(NSpace, { size: 4, wrap: true }, {
        default: () => [
          h(NTag, { size: 'small', type: meta.type }, { default: () => meta.label }),
          ...labels,
        ],
      })
    },
  },
  {
    title: '合规（模型）',
    key: 'compliance',
    minWidth: 180,
    // 第③层模型结论（票 312 / ADR-0024）。
    //
    // null 与 clean 必须显示成两件不同的事：null 是"还没判定"（AI 关闭 /
    // 调用失败 / 任务刚建），clean 是"判定过且没问题"。合并成"无风险"会让
    // 审核员把没检查过的稿子当成检查过的放过去。
    render: (row: ArticleReviewTaskItem) => {
      const compliance = row.compliance
      if (!compliance) {
        return h(NTag, { size: 'small', type: 'default', bordered: false }, { default: () => '未判定' })
      }
      const meta = complianceVerdictMeta(compliance.verdict)
      return h(NTag, { size: 'small', type: meta.type }, { default: () => meta.label })
    },
  },
  {
    title: '正文',
    key: 'content',
    minWidth: 360,
    // 列表里直接给正文（长稿折叠、可展开）：审核员不跳文章页就能判断（票 307）。
    render: (row: ArticleReviewTaskItem) => h(ReviewBodyPreview, { content: row.content }),
  },
  { title: '作者', key: 'author_name', width: 120 },
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
  // 票 302 新规则新增的三个来源：同步风险直接拒、模型判存疑转人工、
  // 模型判定失败转人工。加上此前的合规自动拒，管理员要能一眼看清
  // "为什么这条在队列里"，不能显示机器值。
  return ({
    create: '新建', edit: '编辑', resubmit: '重提',
    compliance: '合规自动拒', risk_check: '内容检查拒',
    model_suspect: '模型存疑', model_failure: '模型判定失败',
  } as Record<string, string>)[value] || value
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
    const data = await getArticleReviewTasks({
      page: page.value,
      limit: limit.value,
      status: status.value,
      sort: sort.value,
    })
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
  pendingReviewIDs.value = [row.id]
  rejectReasonCode.value = null
  rejectReason.value = ''
}

/** 批量驳回：把勾选的任务放进同一个弹窗，一次填原因。 */
function openBatchReject() {
  if (checkedKeys.value.length === 0) return
  rejectTarget.value = null
  pendingReviewIDs.value = [...checkedKeys.value]
  rejectReasonCode.value = null
  rejectReason.value = ''
  rejectVisible.value = true
}

/** 批量通过：不需要原因，点一下确认即可。 */
async function batchApprove() {
  if (checkedKeys.value.length === 0) return
  const result = await batchReviewArticleTasks(checkedKeys.value, { status: 3 })
  reportBatch(result.succeeded, result.failed?.length ?? 0)
  checkedKeys.value = []
  await refresh()
}

async function reject() {
  // 原因码是**必选**的：后端 reason 只收枚举里的一条（票 308 / app.reviewReason），
  // 自由文本只作补充。少传 reason_code 会被判 article.review_reason_invalid。
  if (!rejectReasonCode.value) {
    message.warning('请选择驳回原因')
    return
  }
  const ids = pendingReviewIDs.value
  if (ids.length === 0) return

  if (ids.length === 1) {
    await reviewArticleTask(ids[0]!, {
      status: 4,
      reason_code: rejectReasonCode.value,
      reason: rejectReason.value.trim(),
    })
    message.success('已驳回审核任务')
  } else {
    const result = await batchReviewArticleTasks(ids, {
      status: 4,
      reason_code: rejectReasonCode.value,
      reason: rejectReason.value.trim(),
    })
    reportBatch(result.succeeded, result.failed?.length ?? 0)
  }
  rejectTarget.value = null
  pendingReviewIDs.value = []
  checkedKeys.value = []
  await refresh()
}

/**
 * 批量结果提示：逐条失败是正常结局（任务可能已被别人处理），
 * 所以"部分成功"要如实说，不能报"全部成功"。
 */
function reportBatch(succeeded: number, failed: number) {
  if (failed === 0) {
    message.success(`已处理 ${succeeded} 条`)
    return
  }
  message.warning(`成功 ${succeeded} 条，失败 ${failed} 条（可能已被其他管理员处理）`)
}

watch([page, status, sort], refresh)
onMounted(async () => {
  await refresh()
  // 原因清单由后端下发（前端不手抄中文）；拿不到时下拉为空，驳回按钮会被
  // 前置校验挡住——宁可让管理员看到"没得选"，也不发一条注定失败的请求。
  reasonOptions.value = await getArticleReviewReasons()
})
</script>

<template>
  <NSpace vertical :size="20">
    <NCard class="admin-card">
      <div class="admin-toolbar">
        <NSelect v-model:value="status" :options="statusOptions" clearable class="w-full md:w-56" />
        <NSelect v-model:value="sort" :options="sortOptions" class="w-full md:w-56" />
        <NButton secondary :loading="loading" @click="refresh">刷新队列</NButton>
        <NButton
          type="primary"
          :disabled="checkedKeys.length === 0"
          @click="batchApprove">
          批量通过{{ checkedKeys.length ? `（${checkedKeys.length}）` : '' }}
        </NButton>
        <NButton
          type="error"
          secondary
          :disabled="checkedKeys.length === 0"
          @click="openBatchReject">
          批量驳回{{ checkedKeys.length ? `（${checkedKeys.length}）` : '' }}
        </NButton>
      </div>
    </NCard>

    <NCard class="admin-card" title="审核任务">
      <!-- 空队列是正常读面，不是错误：给空态，不弹"加载失败"。 -->
      <NEmpty
        v-if="!loading && rows.length === 0"
        class="py-10"
        description="队列为空：没有符合条件的审核任务"
      />
      <NDataTable
        v-else
        v-model:checked-row-keys="checkedKeys"
        :row-key="(row: ArticleReviewTaskItem) => row.id"
        :columns="columns"
        :data="rows"
        :loading="loading"
        :pagination="{ page, pageSize: limit, itemCount: total, onUpdatePage: (next: number) => (page = next) }"
        :bordered="false"
      />
    </NCard>

    <NModal v-model:show="rejectVisible" preset="card" class="max-w-[560px]" :title="pendingReviewIDs.length > 1 ? `批量驳回 ${pendingReviewIDs.length} 条任务` : '驳回文章'">
      <NSpace vertical>
        <p v-if="rejectTarget" class="muted m-0">{{ rejectTarget.article_title }}</p>
        <p v-else class="muted m-0">同一条原因会应用到勾选的全部 {{ pendingReviewIDs.length }} 条任务。</p>
        <NSelect
          v-model:value="rejectReasonCode"
          :options="reasonOptions"
          placeholder="请选择驳回原因（必选）"
        />
        <NInput v-model:value="rejectReason" type="textarea" :autosize="{ minRows: 3, maxRows: 6 }" placeholder="补充说明（可选；选『其他原因』时必填）" />
        <div class="flex justify-end gap-3">
          <NButton @click="rejectVisible = false">取消</NButton>
          <NButton type="error" @click="reject">确认驳回</NButton>
        </div>
      </NSpace>
    </NModal>
  </NSpace>
</template>
