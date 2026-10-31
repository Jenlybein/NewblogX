<script setup lang="ts">
import { h } from 'vue'
import { NButton, NCard, NDataTable, NInput, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import {
  addArticleHotTop,
  deleteAdminArticles,
  getAdminHotTop,
  removeArticleHotTop,
  searchAdminArticles,
  setArticleAdminVisibility,
} from '~/services/admin'
import type { SearchArticleItem } from '~/types/api'
import { isAdminHidden, publishLabel, visibilityLabel } from '~/utils/articleLabels'
import { formatDateTime } from '~/utils/format'
import { HOT_TOP_LIMIT, hotTopIds } from '~/utils/hotTop'
import { BRAND } from '~/utils/brand'

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const key = ref('')
const page = ref(1)
const limit = ref(10)
const total = ref(0)
const rows = ref<SearchArticleItem[]>([])
/**
 * 热门置顶的当前 ID 序列。
 *
 * **状态来自 `/articles/hot-top/admin`，不是 ES 字段**（ADR-0017 决策 5：热门置顶不进 ES、
 * 也不依赖任何 ES 派生字段）。失效项也算「已置顶」——置顶行还在，只是首页不展示。
 */
const hotTopIdList = ref<string[]>([])
const toppedIds = computed(() => new Set(hotTopIdList.value))
const hotTopTogglingId = ref('')

function renderArticleCover(row: SearchArticleItem) {
  return h('div', { class: 'admin-cover admin-article-cover' }, row.cover
    ? [h('img', { src: row.cover, alt: row.title })]
    : [
        h('div', { class: 'admin-article-cover__fallback', role: 'img', 'aria-label': `${row.title}暂无封面` }, [
          h('span', { class: 'admin-article-cover__badge' }, BRAND.name.en),
          h('strong', null, 'NO COVER'),
        ]),
      ])
}

const columns = [
  {
    title: '文章',
    key: 'title',
    minWidth: 280,
    render: (row: SearchArticleItem) =>
      h('div', { class: 'flex items-center gap-3' }, [
        renderArticleCover(row),
        h('div', { class: 'min-w-0' }, [
          h('strong', { class: 'block truncate' }, row.title),
          h('span', { class: 'muted text-xs line-clamp-2' }, row.abstract || '暂无摘要'),
        ]),
      ]),
  },
  {
    title: '作者',
    key: 'author',
    width: 150,
    render: (row: SearchArticleItem) => row.author?.nickname || '-',
  },
  {
    title: '发布态',
    key: 'publish_status',
    width: 110,
    render: (row: SearchArticleItem) => h(NTag, { size: 'small' }, { default: () => publishLabel(row.publish_status) }),
  },
  {
    title: '可见性',
    key: 'visibility_status',
    width: 120,
    render: (row: SearchArticleItem) =>
      h(
        NTag,
        { size: 'small', type: isAdminHidden(row.visibility_status) ? 'error' : 'success' },
        { default: () => visibilityLabel(row.visibility_status) },
      ),
  },
  {
    title: '热门置顶',
    key: 'hot_top',
    width: 110,
    render: (row: SearchArticleItem) =>
      h(
        NTag,
        { size: 'small', type: toppedIds.value.has(row.id) ? 'success' : 'default' },
        { default: () => (toppedIds.value.has(row.id) ? '已置顶' : '未置顶') },
      ),
  },
  {
    title: '更新时间',
    key: 'updated_at',
    width: 180,
    render: (row: SearchArticleItem) => formatDateTime(row.updated_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 280,
    render: (row: SearchArticleItem) =>
      h(NSpace, { size: 8 }, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              tertiary: true,
              type: toppedIds.value.has(row.id) ? 'warning' : 'primary',
              loading: hotTopTogglingId.value === row.id,
              onClick: () => void toggleHotTop(row),
            },
            { default: () => (toppedIds.value.has(row.id) ? '取消置顶' : '热门置顶') },
          ),
          h(
            NButton,
            {
              size: 'small',
              tertiary: true,
              type: isAdminHidden(row.visibility_status) ? 'primary' : 'warning',
              onClick: () => toggleVisibility(row),
            },
            { default: () => (isAdminHidden(row.visibility_status) ? '恢复' : '隐藏') },
          ),
          h(
            NButton,
            {
              size: 'small',
              tertiary: true,
              type: 'error',
              onClick: () => removeArticle(row),
            },
            { default: () => '删除' },
          ),
        ],
      }),
  },
]

async function refresh() {
  loading.value = true
  try {
    const data = await searchAdminArticles({ page: page.value, limit: limit.value, key: key.value.trim() || undefined })
    rows.value = data.list
    total.value = data.pagination.total || 0
  } catch {
    message.error('文章列表加载失败')
  } finally {
    loading.value = false
  }
}

function search() {
  page.value = 1
  void refresh()
}

/**
 * 置顶状态单独加载：失败不连坐文章列表（列表照常可搜、可隐藏、可删除）。
 * 拿不到时全部按「未置顶」渲染，真正的判定仍在后端。
 */
async function refreshHotTop() {
  try {
    const data = await getAdminHotTop()
    hotTopIdList.value = hotTopIds(data.list || [])
  } catch {
    hotTopIdList.value = []
  }
}

/** 行内一键切换；成功后只重取置顶状态，本行按钮与标签立即变，不重载整张列表。 */
async function toggleHotTop(row: SearchArticleItem) {
  if (hotTopTogglingId.value) return

  const topped = toppedIds.value.has(row.id)
  if (!topped && hotTopIdList.value.length >= HOT_TOP_LIMIT) {
    message.warning(`热门置顶最多 ${HOT_TOP_LIMIT} 条，请先到「热门置顶」页移除一条`)
    return
  }

  hotTopTogglingId.value = row.id
  try {
    if (topped) {
      await removeArticleHotTop(row.id)
    } else {
      await addArticleHotTop(row.id)
    }
    await refreshHotTop()
    message.success(topped ? '已取消热门置顶' : '已加入热门置顶，可在「热门置顶」页调整顺序')
  } catch (error) {
    message.error(error instanceof Error ? `操作失败：${error.message}` : '操作失败，请稍后重试')
  } finally {
    hotTopTogglingId.value = ''
  }
}

function toggleVisibility(row: SearchArticleItem) {
  const next = isAdminHidden(row.visibility_status) ? 'show' : 'hide'
  dialog.warning({
    title: next === 'hide' ? '隐藏文章' : '恢复文章',
    content: `确认${next === 'hide' ? '隐藏' : '恢复'}「${row.title}」？`,
    positiveText: '确认',
    negativeText: '取消',
    async onPositiveClick() {
      await setArticleAdminVisibility(row.id, next)
      message.success('操作已完成')
      await refresh()
    },
  })
}

function removeArticle(row: SearchArticleItem) {
  dialog.error({
    title: '删除文章',
    content: `确认删除「${row.title}」？删除后文章将不再展示，相关作者统计也会回退。`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      await deleteAdminArticles([row.id])
      message.success('文章已删除')
      await refresh()
    },
  })
}

watch(page, refresh)
onMounted(() => {
  void refresh()
  void refreshHotTop()
})
</script>

<template>
  <NSpace vertical :size="20">
    <NCard class="admin-card">
      <div class="admin-toolbar">
        <NInput v-model:value="key" class="w-full md:max-w-[420px]" placeholder="搜索文章标题、摘要、作者" clearable @keyup.enter="search" />
        <NButton type="primary" :loading="loading" @click="search">搜索</NButton>
      </div>
    </NCard>

    <NCard class="admin-card" title="文章列表">
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :pagination="{ page, pageSize: limit, itemCount: total, onUpdatePage: (next: number) => (page = next) }"
        :bordered="false"
      />
    </NCard>
  </NSpace>
</template>
