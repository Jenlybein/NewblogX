<script setup lang="ts">
/**
 * 文章审核队列（`/review/article`）。
 *
 * 2026-11-27 从 `/content/review` 搬到这里：它是**处置队列**，不是配置页，
 * 所以跟着「审核」组走（旧地址留了跳转页）。审核规则的配置在
 * 「站点配置 → 内容修改与发布 → 文章审核设置」（免审核 / 投稿配额 / 质量闸门 / 自动放行）。
 */
import { h } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NAlert,
  NEmpty,
  NInput,
  NModal,
  NSelect,
  NSpace,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import ReviewBodyPreview from '~/components/review/ReviewBodyPreview.vue'
import ForceDemoteModal from '~/components/articles/ForceDemoteModal.vue'
import {
  getArticleReviewReasons,
  getArticleReviewTasks,
  batchReviewArticleTasks,
  reviewArticleTask,
  getSiteRuntimeConfig,
} from '~/services/admin'
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
/**
 * 两个「机器判据开关」的本地快照（2026-11-28 / ADR-0037）。
 *
 * 管理端没有全局 site store，这两项只影响本页两列语义，所以在页面里读一次。
 * 读失败按"都开着"处理（与原行为一致）：拿不到配置不该让队列变成一片「未评估」。
 */
const scoreEnabled = ref(true)
const aiComplianceEnabled = ref(true)
/**
 * AI 不可用落点是否为「直接放行」。
 *
 * `release` 是临时兜底：那些文章的合规列也是「未判定」，但原因不是"还没判"，
 * 而是"判不了就放了"。审核员必须知道这件事，否则会把兜底放行的稿子当成
 * "检查过且没问题"。
 */
const aiReleaseFallback = ref(false)

async function loadJudgeSwitches() {
  try {
    const site = await getSiteRuntimeConfig()
    scoreEnabled.value = site.article?.score_enabled !== false
    aiComplianceEnabled.value = site.article?.ai_compliance_enabled !== false
    aiReleaseFallback.value = site.article?.ai_failure_action === "release"
  } catch {
    // 配置读不到只是少了两条提示，不该阻断审核队列本身。
  }
}
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

/**
 * 排序项：评分关闭时**风险排序置灰**（spec §5.4）。
 *
 * 风险排序按分数派生标签排（低分 / 维度塌陷 / 篇幅异常），评分一停就没有
 * 这个量纲可排 —— 留着可选项只会让人以为"按风险排好了"，而实际顺序仍是
 * 提交时间。禁用它，比排序后加一句解释诚实。
 */
const sortOptions = computed(() => [
  { label: '提交时间（默认）', value: 'created_at' },
  {
    label: scoreEnabled.value ? '风险优先' : '风险优先（评分已关闭，不可用）',
    value: 'risk',
    disabled: !scoreEnabled.value,
  },
])

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
      // 评分关掉时没有任何风险标签可派生：明确显示「未评估」而不是「无风险」
      // （后者会被读成"检查过了，没问题"）。
      if (!scoreEnabled.value) {
        return h(
          NTag,
          { size: 'small', type: 'default', bordered: false },
          { default: () => '未评估' },
        )
      }
      const meta = riskLevelMeta(row.risk_level)
      const labels = (row.risk_labels ?? []).map((code) =>
        h(
          NTag,
          { size: 'small', bordered: false, type: 'info' },
          { default: () => riskLabelText(code) },
        ),
      )
      return h(
        NSpace,
        { size: 4, wrap: true },
        {
          default: () => [
            h(NTag, { size: 'small', type: meta.type }, { default: () => meta.label }),
            ...labels,
          ],
        },
      )
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
        return h(
          NTag,
          { size: 'small', type: 'default', bordered: false },
          {
            default: () =>
              aiComplianceEnabled.value
                ? '未判定'
                : '未判定（未开启 AI 合规判定）',
          },
        )
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
    render: (row: ArticleReviewTaskItem) =>
      h(ReviewBodyPreview, { taskId: row.id, articleTitle: row.article_title }),
  },
  { title: '作者', key: 'author_name', width: 120 },
  {
    title: '来源',
    key: 'source',
    width: 110,
    render: (row: ArticleReviewTaskItem) =>
      h(NTag, { size: 'small' }, { default: () => sourceLabel(row.source) }),
  },
  {
    title: '状态',
    key: 'status',
    width: 120,
    render: (row: ArticleReviewTaskItem) =>
      h(
        NTag,
        { type: reviewStatusType(row.status), size: 'small' },
        { default: () => reviewStatusLabel(row.status) },
      ),
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
      h(
        NSpace,
        { size: 8 },
        {
          default: () => [
            h(
              NButton,
              {
                size: 'small',
                type: 'primary',
                disabled: row.status !== 'pending',
                onClick: () => approve(row),
              },
              { default: () => '通过' },
            ),
            h(
              NButton,
              {
                size: 'small',
                tertiary: true,
                type: 'error',
                disabled: row.status !== 'pending',
                onClick: () => openReject(row),
              },
              { default: () => '驳回' },
            ),
            // 「强制退回草稿」（票 481）也放在这里：审核任务的作用是把一篇稿子
            // 推向或拦下，而这条动作是"已经放行了、但事后判断不该放"。
            // 它不走审核状态机（不通过/不驳回），直接把文章复位成草稿并通知作者。
            row.article_id
              ? h(
                  NButton,
                  {
                    size: 'small',
                    tertiary: true,
                    onClick: () => openForceDemote(row),
                  },
                  { default: () => '退回草稿' },
                )
              : null,
          ],
        },
      ),
  },
]

function sourceLabel(value: string) {
  // 票 302 新规则新增的三个来源：同步风险直接拒、模型判存疑转人工、
  // 模型判定失败转人工。加上此前的合规自动拒，管理员要能一眼看清
  // "为什么这条在队列里"，不能显示机器值。
  return (
    (
      {
        create: '新建',
        edit: '编辑',
        resubmit: '重提',
        compliance: '合规自动拒',
        risk_check: '内容检查拒',
        model_suspect: '模型存疑',
        model_failure: '模型判定失败',
        // 管理员强制退回草稿（票 481）：队列里出现它说明"这篇文章被人工撤过"，
        // 复盘时要能一眼认出是运营动作而不是模型结论。
        force_demote: '管理员退回',
      } as Record<string, string>
    )[value] || value
  )
}

function reviewStatusLabel(value: string) {
  return (
    (
      {
        pending: '待处理',
        approved: '已通过',
        rejected: '已驳回',
        canceled: '已取消',
      } as Record<string, string>
    )[value] || value
  )
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

/**
 * 人工放行（票 481 的决定：管理员豁免质量闸门，但不静默）。
 *
 * 确认框必须把**分数与放行线**摆出来：管理员是终审，豁免不是"不知道就能过"，
 * 而是"知道自己越过了哪条线，并且愿意承担"。所以 47 分放行要显示
 * "当前 47 分（放行线 52 分）"，而不是只写一句"将进入发布态"。
 *
 * 分数为 null 与 0 分是两件事：null 表示取不到（AI 关闭 / 未评分 / 模型失败），
 * 这时如实写"分数未知"，不要拿 0 分冒充。
 * 留痕由后端写进 approve_note（"管理员以 X 分放行"），前端不重复记。
 */
function qualityLine(row: ArticleReviewTaskItem): string {
  const pass = row.quality_pass_score ?? 0
  const score = row.quality_score
  if (score === undefined || score === null) {
    return `当前分数未知（放行线 ${pass} 分）`
  }
  // naive 的 dialog 正文是纯文本，不渲染 Markdown —— 这里不能写 **加粗**，
  // 否则运营会看到字面的星号。
  const verdict = score >= pass ? '已过线' : '低于放行线'
  return `当前 ${score} 分（放行线 ${pass} 分，${verdict}）`
}

function approve(row: ArticleReviewTaskItem) {
  dialog.warning({
    title: '确认通过审核',
    content: `文章「${row.article_title}」将进入发布态。${qualityLine(row)}。`,
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

/** 「强制退回草稿」的目标（票 481）。 */
const demoteTarget = ref<{ id: string; title: string } | null>(null)
const demoteVisible = ref(false)

function openForceDemote(row: ArticleReviewTaskItem) {
  if (!row.article_id) return
  demoteTarget.value = {
    id: row.article_id,
    title: row.article_title || '该文章',
  }
  demoteVisible.value = true
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

/**
 * 批量通过：没有逐条的原因要填，但**必须**先把分数摆出来。
 *
 * 单条通过会显示"当前 X 分（放行线 Y 分）"，批量却是裸确认 —— 一次越过十几条
 * 质量线而屏幕上没有任何数字，这比逐条误放更难复盘。所以这里汇总一句：
 * 勾选里有多少条低于放行线、多少条取不到分数。
 */
function batchApprove() {
  if (checkedKeys.value.length === 0) return
  const picked = rows.value.filter((row) => checkedKeys.value.includes(row.id))
  const pass = picked.find((row) => row.quality_pass_score !== undefined)?.quality_pass_score ?? 0
  const below = picked.filter(
    (row) =>
      row.quality_score !== undefined && row.quality_score !== null && row.quality_score < pass,
  ).length
  const unknown = picked.filter(
    (row) => row.quality_score === undefined || row.quality_score === null,
  ).length

  const parts = [`将把勾选的 ${picked.length} 篇文章推向发布态。`]
  parts.push(`其中 ${below} 篇低于放行线 ${pass} 分，${unknown} 篇分数未知。`)
  dialog.warning({
    title: '确认批量通过',
    content: parts.join(''),
    positiveText: '通过',
    negativeText: '取消',
    async onPositiveClick() {
      const result = await batchReviewArticleTasks(checkedKeys.value, {
        status: 3,
      })
      reportBatch(result.succeeded, result.failed?.length ?? 0)
      checkedKeys.value = []
      await refresh()
    },
  })
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
  await loadJudgeSwitches()
  await refresh()
  // 原因清单由后端下发（前端不手抄中文）；拿不到时下拉为空，驳回按钮会被
  // 前置校验挡住——宁可让管理员看到"没得选"，也不发一条注定失败的请求。
  reasonOptions.value = await getArticleReviewReasons()
})
</script>

<template>
  <NSpace vertical :size="20">
    <!-- 判据开关的状态提示（spec §5.4）：这两条不是操作入口，只是让审核员
         别把"没检查"读成"检查过没问题"。 -->
    <NAlert v-if="!scoreEnabled" type="warning" :bordered="false">
      站点已关闭「文章质量评分」，风险列显示「未评估」且风险排序不可用：
      风险标签全部由分数派生，没有评分就没有任何标签可显示。
    </NAlert>
    <NAlert v-if="!aiComplianceEnabled" type="warning" :bordered="false">
      站点已关闭「AI 合规判定」，合规列显示「未判定（未开启 AI 合规判定）」：
      这些文章没有模型结论，质量过线即已对读者可见。
    </NAlert>
    <NAlert v-else-if="aiReleaseFallback" type="warning" :bordered="false">
      AI 不可用落点设为「直接放行」：判定的文章若显示「未判定」，可能是
      <strong>调不到模型就放行</strong>的兜底结果，而不是"还没判"。
    </NAlert>

    <NCard class="admin-card">
      <div class="admin-toolbar">
        <NSelect v-model:value="status" :options="statusOptions" clearable class="w-full md:w-56" />
        <NSelect v-model:value="sort" :options="sortOptions" class="w-full md:w-56" />
        <NButton secondary :loading="loading" @click="refresh">刷新队列</NButton>
        <NButton type="primary" :disabled="checkedKeys.length === 0" @click="batchApprove">
          批量通过{{ checkedKeys.length ? `（${checkedKeys.length}）` : '' }}
        </NButton>
        <NButton
          type="error"
          secondary
          :disabled="checkedKeys.length === 0"
          @click="openBatchReject"
        >
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
        :pagination="{
          page,
          pageSize: limit,
          itemCount: total,
          onUpdatePage: (next: number) => (page = next),
        }"
        :bordered="false"
      />
    </NCard>

    <NModal
      v-model:show="rejectVisible"
      preset="card"
      class="max-w-[560px]"
      :title="
        pendingReviewIDs.length > 1 ? `批量驳回 ${pendingReviewIDs.length} 条任务` : '驳回文章'
      "
    >
      <NSpace vertical>
        <p v-if="rejectTarget" class="muted m-0">
          {{ rejectTarget.article_title }}
        </p>
        <p v-else class="muted m-0">
          同一条原因会应用到勾选的全部 {{ pendingReviewIDs.length }} 条任务。
        </p>
        <NSelect
          v-model:value="rejectReasonCode"
          :options="reasonOptions"
          placeholder="请选择驳回原因（必选）"
        />
        <NInput
          v-model:value="rejectReason"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 6 }"
          placeholder="补充说明（可选；选『其他原因』时必填）"
        />
        <div class="flex justify-end gap-3">
          <NButton @click="rejectVisible = false">取消</NButton>
          <NButton type="error" @click="reject">确认驳回</NButton>
        </div>
      </NSpace>
    </NModal>

    <ForceDemoteModal
      v-if="demoteTarget"
      v-model:show="demoteVisible"
      :article-id="demoteTarget.id"
      :article-title="demoteTarget.title"
      @done="refresh"
    />
  </NSpace>
</template>
