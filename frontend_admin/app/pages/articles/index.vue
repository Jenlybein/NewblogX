<script setup lang="ts">
import { h } from 'vue'
import { NButton, NCard, NDataTable, NInput, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import { deleteAdminArticles, searchAdminArticles, setArticleAdminVisibility } from '~/services/admin'
import type { SearchArticleItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const key = ref('')
const page = ref(1)
const limit = ref(10)
const total = ref(0)
const rows = ref<SearchArticleItem[]>([])

function renderArticleCover(row: SearchArticleItem) {
  return h('div', { class: 'admin-cover admin-article-cover' }, row.cover
    ? [h('img', { src: row.cover, alt: row.title })]
    : [
        h('div', { class: 'admin-article-cover__fallback', role: 'img', 'aria-label': `${row.title}暂无封面` }, [
          h('span', { class: 'admin-article-cover__badge' }, 'BlogX'),
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
    render: (row: SearchArticleItem) => row.author?.nickname || row.author?.username || '-',
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
        { size: 'small', type: row.visibility_status === 'admin_hidden' ? 'error' : 'success' },
        { default: () => visibilityLabel(row.visibility_status) },
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
    width: 190,
    render: (row: SearchArticleItem) =>
      h(NSpace, { size: 8 }, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              tertiary: true,
              type: row.visibility_status === 'admin_hidden' ? 'primary' : 'warning',
              onClick: () => toggleVisibility(row),
            },
            { default: () => (row.visibility_status === 'admin_hidden' ? '恢复' : '隐藏') },
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

function publishLabel(value?: number) {
  return ({ 1: '草稿', 2: '待审核', 3: '已发布', 4: '已驳回' } as Record<number, string>)[value || 0] || String(value ?? '-')
}

function visibilityLabel(value?: string) {
  return ({ visible: '公开', user_hidden: '作者隐藏', admin_hidden: '后台隐藏' } as Record<string, string>)[value || ''] || '公开'
}

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

function toggleVisibility(row: SearchArticleItem) {
  const next = row.visibility_status === 'admin_hidden' ? 'show' : 'hide'
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
onMounted(refresh)
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
