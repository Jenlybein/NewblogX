<script setup lang="ts">
/**
 * 评论管理（`/articles/comments`）：管理端处置读者站评论（2026-11-27）。
 *
 * 为什么要有这一页：读者站作者只能删"自己文章下"的评论，管理员虽然后端权限
 * （`domain.CanDelete`）早就有，却没有界面 —— 遇到刷广告的账号只能靠删库或
 * 逐篇文章点进去。这一页把三件事收进一个地方：
 *
 *   1. **找到它**：内容关键词 / 用户 / 文章 / 时间 / 状态五维筛选；
 *   2. **删掉它**：单条与批量删除（走与作者删除同一套业务逻辑：级联同根回复、
 *      清点赞、扣文章评论数，不碰计数以外的字段）；
 *   3. **处理发布人**：禁言（1/7/30 天或永久）与封禁（账号状态置封禁并踢下线）。
 *
 * 「删除」与「封禁」是两件事，不合并：删评论解决这一次，封人才解决这个人。
 * 禁言只拦发布类动作（登录、浏览、点赞照旧），封禁才是不能登录。
 */
import { h } from 'vue'

import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NDatePicker,
  NEmpty,
  NFormItem,
  NInput,
  NModal,
  NSelect,
  NSpace,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'

import {
  banCommentUser,
  getAdminComments,
  muteCommentUser,
  removeAdminComments,
} from '~/services/admin'
import type { AdminCommentItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'
import { buildCommentAdminQuery } from '~/utils/commentAdmin'
import type { CommentAdminFilters } from '~/utils/commentAdmin'

const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const removing = ref(false)
const page = ref(1)
const limit = ref(20)
const hasMore = ref(false)
const rows = ref<AdminCommentItem[]>([])
const checkedKeys = ref<string[]>([])

// 筛选态的"待提交值"与"已提交值"分开：改输入框不应立刻发请求，
// 否则每敲一个字都打一次库；点「查询」才落到已提交值上。
const keyword = ref('')
const userInput = ref('')
const articleInput = ref('')
const status = ref<'all' | 'published' | 'deleted'>('all')
const range = ref<[number, number] | null>(null)

/** 已提交的筛选态：决定下一次请求长什么样，与输入框里的待提交值分开。 */
const applied = ref<CommentAdminFilters>({ keyword: '', userInput: '', articleInput: '', status: 'all', range: null })

const STATUS_OPTIONS = [
  { label: '全部状态', value: 'all' },
  { label: '已发布', value: 'published' },
  { label: '已删除', value: 'deleted' },
]

/** 把页面上"已提交"的筛选态翻译成请求参数（规则见 utils/commentAdmin）。 */
function buildQuery() {
  return buildCommentAdminQuery(applied.value, page.value, limit.value)
}
async function refresh() {
  loading.value = true
  try {
    const data = await getAdminComments(buildQuery())
    rows.value = data.list ?? []
    hasMore.value = data.has_more ?? false
  } catch (error) {
    message.error(error instanceof Error ? `评论加载失败：${error.message}` : '评论加载失败')
    rows.value = []
    hasMore.value = false
  } finally {
    loading.value = false
  }
}

function search() {
  applied.value = {
    keyword: keyword.value,
    userInput: userInput.value,
    articleInput: articleInput.value,
    status: status.value,
    range: range.value,
  }
  page.value = 1
  checkedKeys.value = []
  void refresh()
}

function reset() {
  keyword.value = ''
  userInput.value = ''
  articleInput.value = ''
  status.value = 'all'
  range.value = null
  search()
}

function changePage(next: number) {
  page.value = next
  checkedKeys.value = []
  void refresh()
}

/** 批量删除：确认框里写清楚"级联"与"不可恢复"，与读者站作者侧的文案同一口径。 */
function confirmRemove(ids: string[]) {
  if (ids.length === 0) return
  dialog.warning({
    title: '删除评论',
    content: ids.length === 1
      ? '删除这条评论会连同它下面的全部回复一起删除，且不可恢复。'
      : `删除选中的 ${ids.length} 条评论：一级评论会连同它下面的全部回复一起删除，且不可恢复。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: () => {
      void performRemove(ids)
    },
  })
}

async function performRemove(ids: string[]) {
  removing.value = true
  try {
    const result = await removeAdminComments(ids)
    const failed = result.failed ?? []
    if (failed.length > 0) {
      // 部分失败必须逐条说清楚：整单报"删除失败"会让运营以为一条都没删。
      message.warning(`已删除 ${result.succeeded ?? 0} 条；${failed.length} 条失败：${failed.map(item => item.message).join('；')}`)
    } else {
      message.success(`已删除 ${result.succeeded ?? 0} 条评论`)
    }
    checkedKeys.value = []
    await refresh()
  } catch (error) {
    message.error(error instanceof Error ? `删除失败：${error.message}` : '删除失败')
  } finally {
    removing.value = false
    // 审计里"已删除"标记刷新后才会出现，这里不本地假删。
  }
}

// ── 禁言 / 封禁 ────────────────────────────────────────────
const MUTE_DAYS_OPTIONS = [
  { label: '1 天', value: 1 },
  { label: '7 天', value: 7 },
  { label: '30 天', value: 30 },
  { label: '永久', value: 0 },
]

const muteTarget = ref<AdminCommentItem | null>(null)
const muteDays = ref<number>(7)
const muteReason = ref('')
const muting = ref(false)

function openMute(row: AdminCommentItem) {
  muteTarget.value = row
  muteDays.value = 7
  muteReason.value = ''
}

async function submitMute() {
  const target = muteTarget.value
  if (!target?.user_id) return
  const reason = muteReason.value.trim()
  if (!reason) {
    message.warning('请填写禁言理由（这句话会写进处置记录）')
    return
  }
  muting.value = true
  try {
    const result = await muteCommentUser({ user_id: target.user_id, days: muteDays.value as 1 | 7 | 30 | 0, reason })
    const until = result.muted_until ? formatDateTime(result.muted_until) : ''
    message.success(until ? `已禁言 ${target.user_nickname ?? target.user_id}，到 ${until}` : `已禁言 ${target.user_nickname ?? target.user_id}`)
    muteTarget.value = null
  } catch (error) {
    message.error(error instanceof Error ? `禁言失败：${error.message}` : '禁言失败')
  } finally {
    muting.value = false
  }
}

const banTarget = ref<AdminCommentItem | null>(null)
const banReason = ref('')
const banning = ref(false)

function openBan(row: AdminCommentItem) {
  banTarget.value = row
  banReason.value = ''
}

async function submitBan() {
  const target = banTarget.value
  if (!target?.user_id) return
  banning.value = true
  try {
    await banCommentUser({ user_id: target.user_id, reason: banReason.value.trim() || undefined })
    message.success(`已封禁 ${target.user_nickname ?? target.user_id}（该账号被踢下线，不能登录）`)
    banTarget.value = null
  } catch (error) {
    message.error(error instanceof Error ? `封禁失败：${error.message}` : '封禁失败')
  } finally {
    banning.value = false
  }
}

const columns = [
  { type: 'selection' as const },
  {
    title: '评论',
    key: 'content',
    minWidth: 320,
    render: (row: AdminCommentItem) => {
      const text = row.content ?? ''
      return h('div', { class: 'comment-admin__content' }, [
        row.deleted
          ? h(NTag, { size: 'small', type: 'default', bordered: false }, { default: () => '已删除' })
          : null,
        h('p', { class: 'comment-admin__text' }, text.length > 200 ? `${text.slice(0, 200)}…` : text),
      ])
    },
  },
  {
    title: '文章',
    key: 'article_title',
    minWidth: 200,
    render: (row: AdminCommentItem) => h('div', { class: 'comment-admin__article' }, [
      h('p', { class: 'comment-admin__text' }, row.article_title || '（文章已删除）'),
      row.article_id
        ? h('a', {
          class: 'comment-admin__link',
          href: `/article/${row.article_id}`,
          target: '_blank',
          rel: 'noopener noreferrer',
        }, { default: () => '查看文章' })
        : null,
    ]),
  },
  {
    title: '用户',
    key: 'user_nickname',
    minWidth: 180,
    render: (row: AdminCommentItem) => h('div', { class: 'comment-admin__user' }, [
      h('p', { class: 'comment-admin__text' }, row.user_nickname || '（未留下昵称）'),
      row.user_id ? h('p', { class: 'comment-admin__sub' }, `ID ${row.user_id}`) : null,
    ]),
  },
  {
    title: '时间',
    key: 'created_at',
    minWidth: 160,
    render: (row: AdminCommentItem) => formatDateTime(row.created_at),
  },
  {
    title: '操作',
    key: 'actions',
    minWidth: 200,
    render: (row: AdminCommentItem) => h(NSpace, { size: 4 }, {
      default: () => [
        h(NButton, {
          size: 'tiny',
          type: 'error',
          quaternary: true,
          disabled: row.deleted,
          onClick: () => confirmRemove([row.id!]),
        }, { default: () => '删除' }),
        h(NButton, {
          size: 'tiny',
          quaternary: true,
          disabled: !row.user_id,
          onClick: () => openMute(row),
        }, { default: () => '禁言' }),
        h(NButton, {
          size: 'tiny',
          quaternary: true,
          disabled: !row.user_id,
          onClick: () => openBan(row),
        }, { default: () => '封禁' }),
      ],
    }),
  },
]


/** naive-ui 的勾选回调给的是「当前页全部被选中的行键」，直接存即可。 */
function onChecked(keys: Array<string | number>) {
  checkedKeys.value = keys.map(String)
}

onMounted(refresh)
</script>

<template>
  <div class="admin-settings-panel">
    <NAlert type="info" :bordered="false" class="mb-4">
      这里可以找到并处置读者站的全部评论：删除解决这一次，禁言 / 封禁处理这个人。
      删除与作者在文章页点「删除」走同一套逻辑（级联同根回复、扣文章评论数），不可恢复。
    </NAlert>

    <NCard class="admin-card mb-4" size="small">
      <NSpace vertical :size="12">
        <NSpace wrap :size="12" align="center">
          <NInput v-model:value="keyword" placeholder="评论内容关键词" clearable style="width: 200px" data-role="comment-admin-keyword" />
          <NInput v-model:value="userInput" placeholder="用户 ID 或昵称" clearable style="width: 170px" data-role="comment-admin-user" />
          <NInput v-model:value="articleInput" placeholder="文章 ID" clearable style="width: 140px" data-role="comment-admin-article" />
          <NDatePicker v-model:value="range" type="daterange" clearable style="width: 260px" data-role="comment-admin-range" />
          <NSelect v-model:value="status" :options="STATUS_OPTIONS" style="width: 140px" data-role="comment-admin-status" />
          <NButton type="primary" @click="search" data-role="comment-admin-search">查询</NButton>
          <NButton quaternary @click="reset">重置</NButton>
        </NSpace>

        <NSpace align="center">
          <NButton
            type="error"
            :loading="removing"
            :disabled="checkedKeys.length === 0"
            data-role="comment-admin-batch-delete"
            @click="confirmRemove(checkedKeys)"
          >
            批量删除（{{ checkedKeys.length }}）
          </NButton>
          <NButton quaternary :loading="loading" @click="refresh">刷新</NButton>
          <span class="comment-admin__hint">共 {{ rows.length }} 条{{ hasMore ? '（还有更多，用分页继续）' : '' }}</span>
        </NSpace>
      </NSpace>
    </NCard>

    <NCard class="admin-card" size="small">
      <NDataTable
        v-if="rows.length > 0"
        :row-key="(row: AdminCommentItem) => row.id!"
        :columns="columns"
        :data="rows"
        :loading="loading"
        :checked-row-keys="checkedKeys"
        :scroll-x="1100"
        @update:checked-row-keys="onChecked"
      />
      <NEmpty v-else :description="loading ? '加载中…' : '没有符合条件的评论'" />

      <!-- 上一页 / 下一页而不是页码器：后端只回 `has_more`（没有总数），
           页码器要靠猜总数来渲染，猜错就出现"第 3 页是空的"。 -->
      <div v-if="rows.length > 0 || page > 1" class="comment-admin__pager">
        <NButton quaternary size="small" :disabled="page === 1" @click="changePage(page - 1)">上一页</NButton>
        <span class="comment-admin__hint">第 {{ page }} 页</span>
        <NButton quaternary size="small" :disabled="!hasMore" @click="changePage(page + 1)">下一页</NButton>
      </div>
    </NCard>

    <!-- 禁言：档位 + 理由（理由必填，会写进处置记录，将来也会发给用户） -->
    <NModal
      :show="Boolean(muteTarget)"
      preset="card"
      title="禁言评论发布人"
      style="width: 460px"
      @update:show="(show: boolean) => { if (!show) muteTarget = null }"
    >
      <NSpace vertical :size="12">
        <p class="comment-admin__hint">
          将对 {{ muteTarget?.user_nickname || muteTarget?.user_id }} 禁言。禁言只拦发表评论 / 回复，
          登录、浏览、点赞照旧；到期自动解除。
        </p>
        <NFormItem label="时长">
          <NSelect v-model:value="muteDays" :options="MUTE_DAYS_OPTIONS" />
        </NFormItem>
        <NFormItem label="理由（必填）">
          <NInput v-model:value="muteReason" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" placeholder="例如：连续发布广告评论" maxlength="200" show-count />
        </NFormItem>
        <div class="flex justify-end gap-3">
          <NButton @click="muteTarget = null">取消</NButton>
          <NButton type="primary" :loading="muting" @click="submitMute">确认禁言</NButton>
        </div>
      </NSpace>
    </NModal>

    <!-- 封禁：账号状态置封禁 + 吊销会话，用户立刻掉线且不能再登录 -->
    <NModal
      :show="Boolean(banTarget)"
      preset="card"
      title="封禁评论发布人"
      style="width: 460px"
      @update:show="(show: boolean) => { if (!show) banTarget = null }"
    >
      <NSpace vertical :size="12">
        <NAlert type="warning" :bordered="false">
          封禁会让该账号<b>立即掉线且无法登录</b>（与「用户管理」里的封禁是同一个动作）。
          只是这一次不想让他评论，请改用「禁言」。
        </NAlert>
        <NFormItem label="理由（可选）">
          <NInput v-model:value="banReason" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" placeholder="例如：批量发布违法广告" maxlength="200" show-count />
        </NFormItem>
        <div class="flex justify-end gap-3">
          <NButton @click="banTarget = null">取消</NButton>
          <NButton type="error" :loading="banning" @click="submitBan">确认封禁</NButton>
        </div>
      </NSpace>
    </NModal>
  </div>
</template>

<style scoped>
/* 这一页的列宽与文字密度按"一眼扫过去能分清评论 / 文章 / 用户"来定：
   评论正文截断到 200 字（完整内容点「查看文章」去看），文章与用户各占一列窄卡。 */
.comment-admin__content,
.comment-admin__article,
.comment-admin__user {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.comment-admin__text {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
}

.comment-admin__sub,
.comment-admin__hint {
  margin: 0;
  color: var(--color-ink-muted);
  font-size: 12px;
}

.comment-admin__link {
  color: var(--color-ink);
  font-size: 12px;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.comment-admin__pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
</style>


