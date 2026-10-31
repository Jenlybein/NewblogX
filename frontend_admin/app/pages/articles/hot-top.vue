<script setup lang="ts">
/**
 * 热门置顶（首页运营位）。
 *
 * 两件事：**看当前序列**、**调顺序**（上移 / 下移 / 移除），外加失效项标记与一键清理。
 *
 * 关键语义（ADR-0017 决策 5）：
 *   - 唯一真相是 `article_hot_top_models`，与 ES 无关；这里全部走 `/articles/hot-top/admin`。
 *   - 文章后来被隐藏 / 退回草稿 / 删除时，置顶行**保留**：首页 JOIN 过滤掉，后台标「已失效」。
 *   - `PATCH /articles/hot-top/order` 是**全量替换**，集合与库不一致会被后端拒绝，
 *     所以提交的是完整序列（含失效项），提交后再从服务端重新加载一次顺序。
 */
import { h } from 'vue'
import { NButton, NCard, NDataTable, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { getAdminHotTop, removeArticleHotTop, updateHotTopOrder } from '~/services/admin'
import type { HotTopAdminItem } from '~/types/api'
import { HOT_TOP_LIMIT, canMoveHotTop, hotTopIds, moveHotTop, staleHotTopIds } from '~/utils/hotTop'

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const ordering = ref(false)
const cleaning = ref(false)
const rows = ref<HotTopAdminItem[]>([])

const staleIds = computed(() => staleHotTopIds(rows.value))

function renderCover(row: HotTopAdminItem) {
  return h(
    'div',
    { class: 'admin-cover' },
    row.cover ? [h('img', { src: row.cover, alt: row.title || '置顶封面' })] : [],
  )
}

const columns: DataTableColumns<HotTopAdminItem> = [
  {
    title: '序号',
    key: 'sort_order',
    width: 72,
    // 序号按当前列表位置显示：库里的 sort_order 是压实过的，但展示顺序以返回列表为准。
    render: (_row, index) => index + 1,
  },
  {
    title: '封面',
    key: 'cover',
    width: 140,
    render: (row) => renderCover(row),
  },
  {
    title: '文章',
    key: 'title',
    minWidth: 240,
    render: (row) => h('strong', { class: 'block truncate' }, row.title || '（无标题）'),
  },
  {
    title: '状态',
    key: 'stale',
    width: 110,
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: row.stale ? 'warning' : 'success' },
        { default: () => (row.stale ? '已失效' : '展示中') },
      ),
  },
  {
    title: '操作',
    key: 'actions',
    width: 250,
    render: (row, index) =>
      h(NSpace, { size: 8 }, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              tertiary: true,
              disabled: ordering.value || !canMoveHotTop(index, rows.value.length, 'up'),
              onClick: () => void moveRow(index, 'up'),
            },
            { default: () => '上移' },
          ),
          h(
            NButton,
            {
              size: 'small',
              tertiary: true,
              disabled: ordering.value || !canMoveHotTop(index, rows.value.length, 'down'),
              onClick: () => void moveRow(index, 'down'),
            },
            { default: () => '下移' },
          ),
          h(
            NButton,
            { size: 'small', tertiary: true, type: 'error', onClick: () => removeRow(row) },
            { default: () => '移除' },
          ),
        ],
      }),
  },
]

async function refresh() {
  loading.value = true
  try {
    const data = await getAdminHotTop()
    rows.value = data.list || []
  } catch {
    message.error('热门置顶列表加载失败')
  } finally {
    loading.value = false
  }
}

/**
 * 上移 / 下移：先本地换位（点了立即有反馈），再整单提交，最后以服务端返回的顺序重载。
 * 提交失败时重载一次，避免界面停留在一个「看起来已保存、其实没保存」的顺序上。
 */
async function moveRow(index: number, direction: 'up' | 'down') {
  if (ordering.value) return
  const next = moveHotTop(rows.value, index, direction)
  if (hotTopIds(next).join('|') === hotTopIds(rows.value).join('|')) return

  ordering.value = true
  rows.value = next
  try {
    await updateHotTopOrder(hotTopIds(next))
    await refresh()
    message.success('顺序已保存')
  } catch (error) {
    message.error(error instanceof Error ? `顺序保存失败：${error.message}` : '顺序保存失败，请稍后重试')
    await refresh()
  } finally {
    ordering.value = false
  }
}

function removeRow(row: HotTopAdminItem) {
  dialog.warning({
    title: '移除热门置顶',
    content: `确认把「${row.title || '这篇文章'}」从热门置顶移除？首页会立即少一个位子。`,
    positiveText: '移除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await removeArticleHotTop(row.article_id)
        message.success('已移除')
      } catch (error) {
        message.error(error instanceof Error ? `移除失败：${error.message}` : '移除失败，请稍后重试')
      }
      await refresh()
    },
  })
}

/** 一键清理：失效项逐条删除（接口幂等），失败条数如实报出来，不假装全清。 */
function cleanupStale() {
  const ids = staleIds.value.slice()
  if (ids.length === 0) return

  dialog.warning({
    title: '清理失效置顶',
    content: `将移除 ${ids.length} 条已失效的置顶（文章已隐藏、退回草稿或删除）。展示中的置顶不受影响。`,
    positiveText: '清理',
    negativeText: '取消',
    async onPositiveClick() {
      cleaning.value = true
      try {
        const results = await Promise.allSettled(ids.map(id => removeArticleHotTop(id)))
        const failed = results.filter(result => result.status === 'rejected').length
        if (failed > 0) {
          message.warning(`已清理 ${ids.length - failed} 条，${failed} 条失败，可再点一次重试`)
        } else {
          message.success(`已清理 ${ids.length} 条失效置顶`)
        }
      } finally {
        cleaning.value = false
        await refresh()
      }
    },
  })
}

onMounted(refresh)
</script>

<template>
  <NCard class="admin-card" title="热门置顶">
    <template #header-extra>
      <NSpace>
        <NButton size="small" quaternary @click="refresh">刷新</NButton>
        <NButton
          size="small"
          type="error"
          :loading="cleaning"
          :disabled="staleIds.length === 0"
          @click="cleanupStale">
          清理失效（{{ staleIds.length }}）
        </NButton>
      </NSpace>
    </template>

    <p class="admin-hint">
      首页运营位：这里从上到下的顺序就是首页「热门置顶」的展示顺序，最多 {{ HOT_TOP_LIMIT }} 条（当前
      {{ rows.length }} 条）。文章后来被隐藏、退回草稿或删除时，置顶行会保留并标为「已失效」——首页
      已不展示，可在这里一键清理。
    </p>

    <NDataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :bordered="false"
      :row-key="(row: HotTopAdminItem) => row.article_id" />
  </NCard>
</template>
