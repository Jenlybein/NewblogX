<script setup lang="ts">
/**
 * 项目编排（管理员）：批量收录 + 两级手动排序。
 *
 * 项目与合集并列、页面形态复用，但数据与表完全独立：本页只读写
 * `/projects` 下面的编排接口。
 *
 * 两套顺序各有落点，读者侧读的是同一份数据：
 *   - 项目之间：`project_models.sort_order`，读者「项目」列表页按它展示；
 *   - 项目之内：`project_article_models.sort_order`，读者项目详情按它展示。
 *
 * 两级都是**全量替换**：上移 / 下移只改本地数组，提交时带上完整有序集合
 * （项目内还要包含已失效的收录行）。后端在集合与库里不一致时整单拒绝，
 * 所以失败后一律从服务端重载，避免界面停留在一个"看起来已保存"的顺序上。
 *
 * 批量收录一次提交多篇，结局逐篇返回：已收录 / 已在项目中 / 不可收录 /
 * 文章不存在。重复项在这里被明确标出来，而不是只报一句"成功"。
 */
import { h } from 'vue'
import { NButton, NCard, NDataTable, NInput, NSelect, NSpace, NTag, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  batchIncludeProjectArticles,
  getAdminProjectItems,
  getAdminProjects,
  removeProjectArticle,
  searchAdminArticles,
  updateProjectItemOrder,
  updateProjectOrder,
} from '~/services/admin'
import type {
  ProjectAdminCard,
  ProjectAdminItem,
  ProjectBatchIncludeResponse,
  SearchArticleItem,
} from '~/types/api'
import {
  canMoveProjectArrangeItem,
  dedupeProjectArticleIds,
  moveProjectArrangeItem,
  projectBatchOutcomeLabel,
  projectItemIds,
  projectOrderIds,
  summarizeProjectBatchInclude,
} from '~/utils/projectArrange'

const message = useMessage()

const projects = ref<ProjectAdminCard[]>([])
const projectsLoading = ref(false)
const projectOrdering = ref(false)

/** 当前正在编排的项目 ID；空串表示还没选。 */
const selectedProjectId = ref('')
const items = ref<ProjectAdminItem[]>([])
const itemsLoading = ref(false)
const itemOrdering = ref(false)

/** 批量收录：关键词、可选文章、已勾选 ID、上次结果。 */
const keyword = ref('')
const articleOptions = ref<Array<{ label: string; value: string }>>([])
const selectedArticleIds = ref<string[]>([])
const searching = ref(false)
const batchLoading = ref(false)
const lastBatch = ref<ProjectBatchIncludeResponse | null>(null)
/** 文章 ID → 标题（搜索结果为主要来源；收录行补上历史条目的标题）。 */
const articleTitles = ref<Record<string, string>>({})

const projectColumns: DataTableColumns<ProjectAdminCard> = [
  {
    title: '序号',
    key: 'sort_order',
    width: 72,
    // 序号按当前列表位置显示：库里的 sort_order 是压实过的，但展示顺序以返回列表为准。
    render: (_row, index) => index + 1,
  },
  {
    title: '标题',
    key: 'title',
    minWidth: 220,
    render: row => h('strong', { class: 'block truncate' }, row.title || '（无标题）'),
  },
  { title: '收录', key: 'article_count', width: 100, render: row => `${row.article_count} 篇` },
  {
    title: '展示',
    key: 'displayed',
    width: 110,
    render: row =>
      h(NTag, { size: 'small', type: row.displayed ? 'success' : 'default' }, {
        default: () => (row.displayed ? '展示中' : '已隐藏'),
      }),
  },
  {
    title: '操作',
    key: 'actions',
    width: 180,
    render: (row, index) =>
      h(NSpace, { size: 8 }, {
        default: () => [
          h(NButton, {
            size: 'small',
            tertiary: true,
            disabled:
              projectOrdering.value ||
              !canMoveProjectArrangeItem(index, projects.value.length, 'up'),
            onClick: () => void moveProject(index, 'up'),
          }, { default: () => '上移' }),
          h(NButton, {
            size: 'small',
            tertiary: true,
            disabled:
              projectOrdering.value ||
              !canMoveProjectArrangeItem(index, projects.value.length, 'down'),
            onClick: () => void moveProject(index, 'down'),
          }, { default: () => '下移' }),
          h(NButton, {
            size: 'small',
            tertiary: true,
            type: 'primary',
            onClick: () => void chooseProject(row.id),
          }, { default: () => '编排收录' }),
        ],
      }),
  },
]

const itemColumns: DataTableColumns<ProjectAdminItem> = [
  { title: '顺序', key: 'sort_order', width: 72, render: (_row, index) => index + 1 },
  {
    title: '文章',
    key: 'title',
    minWidth: 240,
    render: row => h('strong', { class: 'block truncate' }, row.title || '（无标题）'),
  },
  {
    title: '状态',
    key: 'stale',
    width: 110,
    render: row =>
      h(NTag, { size: 'small', type: row.stale ? 'warning' : 'success' }, {
        default: () => (row.stale ? '已失效' : '展示中'),
      }),
  },
  {
    title: '操作',
    key: 'actions',
    width: 220,
    render: (row, index) =>
      h(NSpace, { size: 8 }, {
        default: () => [
          h(NButton, {
            size: 'small',
            tertiary: true,
            disabled:
              itemOrdering.value ||
              !canMoveProjectArrangeItem(index, items.value.length, 'up'),
            onClick: () => void moveItem(index, 'up'),
          }, { default: () => '上移' }),
          h(NButton, {
            size: 'small',
            tertiary: true,
            disabled:
              itemOrdering.value ||
              !canMoveProjectArrangeItem(index, items.value.length, 'down'),
            onClick: () => void moveItem(index, 'down'),
          }, { default: () => '下移' }),
          h(NButton, {
            size: 'small',
            tertiary: true,
            type: 'error',
            onClick: () => void removeItem(row),
          }, { default: () => '移除' }),
        ],
      }),
  },
]

async function refreshProjects() {
  projectsLoading.value = true
  try {
    const data = await getAdminProjects()
    projects.value = data.list || []
  } catch {
    message.error('项目列表加载失败')
  } finally {
    projectsLoading.value = false
  }
}

/** 项目之间的上移 / 下移：先本地换位，再整单提交，最后以服务端顺序重载。 */
async function moveProject(index: number, direction: 'up' | 'down') {
  if (projectOrdering.value) return
  const next = moveProjectArrangeItem(projects.value, index, direction)
  if (projectOrderIds(next).join('|') === projectOrderIds(projects.value).join('|')) return

  projectOrdering.value = true
  projects.value = next
  try {
    await updateProjectOrder(projectOrderIds(next))
    message.success('项目顺序已保存')
  } catch (error) {
    message.error(error instanceof Error ? `项目顺序保存失败：${error.message}` : '项目顺序保存失败，请稍后重试')
  } finally {
    projectOrdering.value = false
    await refreshProjects()
  }
}

async function chooseProject(id: string) {
  if (!id) return
  selectedProjectId.value = id
  lastBatch.value = null
  selectedArticleIds.value = []
  await loadItems()
}

async function loadItems() {
  const id = selectedProjectId.value
  if (!id) {
    items.value = []
    return
  }
  itemsLoading.value = true
  try {
    const data = await getAdminProjectItems(id)
    items.value = data.list || []
    for (const item of items.value) {
      articleTitles.value[item.article_id] = item.title
    }
  } catch {
    message.error('项目收录加载失败')
  } finally {
    itemsLoading.value = false
  }
}

/**
 * 项目内的上移 / 下移：提交完整集合（含失效行），失败则重载。
 *
 * 失效行也必须带上：后端校验的是"当前全部活跃收录行"的排列，
 * 用读者详情拼出来的列表会漏掉它们，提交就会被整单拒绝。
 */
async function moveItem(index: number, direction: 'up' | 'down') {
  const id = selectedProjectId.value
  if (!id || itemOrdering.value) return
  const next = moveProjectArrangeItem(items.value, index, direction)
  if (projectItemIds(next).join('|') === projectItemIds(items.value).join('|')) return

  itemOrdering.value = true
  items.value = next
  try {
    await updateProjectItemOrder(id, projectItemIds(next))
    message.success('顺序已保存')
  } catch (error) {
    message.error(error instanceof Error ? `顺序保存失败：${error.message}` : '顺序保存失败，请稍后重试')
  } finally {
    itemOrdering.value = false
    await loadItems()
  }
}

async function removeItem(row: ProjectAdminItem) {
  const id = selectedProjectId.value
  if (!id) return
  try {
    await removeProjectArticle(id, row.article_id)
    message.success('已移除')
    await loadItems()
  } catch (error) {
    message.error(error instanceof Error ? `移除失败：${error.message}` : '移除失败，请稍后重试')
  }
}

async function searchArticles() {
  if (searching.value) return
  searching.value = true
  try {
    const data = await searchAdminArticles({ page: 1, limit: 20, key: keyword.value.trim() || undefined })
    const list = (data.list || []) as SearchArticleItem[]
    articleOptions.value = list.map(item => ({
      label: item.title || item.id,
      value: item.id,
    }))
    for (const item of list) {
      articleTitles.value[item.id] = item.title || item.id
    }
    selectedArticleIds.value = []
  } catch {
    message.error('文章搜索失败')
  } finally {
    searching.value = false
  }
}

/**
 * 批量收录：一次请求提交全部选中项（去重后）。
 *
 * 结果逐篇标出来；重复项不会因为"已经是项目成员"被静默忽略。
 */
async function submitBatch() {
  const id = selectedProjectId.value
  const ids = dedupeProjectArticleIds(selectedArticleIds.value)
  if (!id || ids.length === 0 || batchLoading.value) {
    if (ids.length === 0) message.warning('请先勾选要收录的文章')
    return
  }

  batchLoading.value = true
  try {
    const result = await batchIncludeProjectArticles(id, ids)
    lastBatch.value = result
    selectedArticleIds.value = []
    const summary = summarizeProjectBatchInclude(result)
    if (result.included > 0) {
      message.success(summary)
    } else {
      message.warning(summary)
    }
    await loadItems()
  } catch (error) {
    message.error(error instanceof Error ? `批量收录失败：${error.message}` : '批量收录失败，请稍后重试')
  } finally {
    batchLoading.value = false
  }
}

/** 批量结果里显示文章标题：优先搜索结果，其次收录行，最后退回 ID。 */
function titleOf(articleId: string): string {
  return articleTitles.value[articleId] || articleId
}

onMounted(refreshProjects)
</script>

<template>
  <NSpace vertical :size="20">
    <NCard class="admin-card" title="项目之间的顺序">
      <p class="admin-hint">
        这里从上到下的顺序就是读者「项目」列表页的展示顺序。调整会整表提交，
        失败时自动从服务端重载，不会留下一个看起来已保存的顺序。
      </p>
      <NDataTable
        :columns="projectColumns"
        :data="projects"
        :loading="projectsLoading"
        :bordered="false"
        :row-key="(row: ProjectAdminCard) => row.id" />
    </NCard>

    <NCard class="admin-card" title="项目内收录与顺序">
      <p class="admin-hint">
        选中一个项目后可批量收录多篇、并手动排列项目内的顺序。文章后来被隐藏或删除时，
        收录行会保留并标为「已失效」；排序提交的是完整集合（含失效行），所以它们也要留在列表里。
      </p>

      <NSpace align="center" class="admin-collection-form">
        <NSelect
          data-role="arrange-project"
          class="admin-arrange-select"
          placeholder="选择项目"
          :value="selectedProjectId"
          :options="projects.map(item => ({ label: item.title || item.id, value: item.id }))"
          @update:value="chooseProject" />
      </NSpace>

      <NDataTable
        v-if="selectedProjectId"
        :columns="itemColumns"
        :data="items"
        :loading="itemsLoading"
        :bordered="false"
        :row-key="(row: ProjectAdminItem) => row.article_id" />

      <section v-if="selectedProjectId" class="admin-arrange-batch">
        <h3 class="admin-arrange-batch__title">批量收录文章</h3>
        <p class="admin-upload-panel__hint">
          先按关键词搜索，再按住 Ctrl / Shift 勾选多篇，一次提交。重复项与不可收录会在结果里逐篇标出。
        </p>

        <NSpace align="center">
          <NInput
            v-model:value="keyword"
            data-role="arrange-keyword"
            class="admin-arrange-keyword"
            placeholder="搜索文章标题、摘要、作者"
            @keyup.enter="searchArticles" />
          <NButton :loading="searching" @click="searchArticles">搜索</NButton>
          <NButton
            type="primary"
            data-role="batch-include"
            :loading="batchLoading"
            :disabled="selectedArticleIds.length === 0"
            @click="submitBatch">
            收录选中（{{ selectedArticleIds.length }}）
          </NButton>
        </NSpace>

        <NSelect
          v-if="articleOptions.length"
          data-role="arrange-articles"
          class="admin-arrange-articles"
          multiple
          placeholder="选择要收录的文章（可多选）"
          :value="selectedArticleIds"
          :options="articleOptions"
          @update:value="(value: string[]) => (selectedArticleIds = value)" />

        <ul v-if="lastBatch" class="admin-arrange-results" data-role="batch-results">
          <li v-for="result in lastBatch.results" :key="result.article_id">
            <span>{{ titleOf(result.article_id) }}</span>
            <NTag
              size="small"
              :type="result.outcome === 'included' ? 'success' : result.outcome === 'duplicated' ? 'warning' : 'error'">
              {{ projectBatchOutcomeLabel(result.outcome) }}
            </NTag>
          </li>
        </ul>
      </section>
    </NCard>
  </NSpace>
</template>
