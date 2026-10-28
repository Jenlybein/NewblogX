<script setup lang="ts">
import { h } from 'vue'
import { NButton, NDataTable, NPopconfirm, NTag, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { enableAiProvider, getAiProviders, getAiVendors, removeAiProvider } from '~/services/admin'
import type { AIProviderItem, VendorItem } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const rows = ref<AIProviderItem[]>([])
const vendors = ref<VendorItem[]>([])
const showForm = ref(false)
const editing = ref<AIProviderItem | null>(null)

function vendorLabel(id: string): string {
  return vendors.value.find(vendor => vendor.id === id)?.name ?? id
}

async function refresh() {
  loading.value = true
  try {
    const [list, specs] = await Promise.all([getAiProviders(), getAiVendors()])
    rows.value = list
    vendors.value = specs
  } catch {
    message.error('模型配置加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  showForm.value = true
}

function openEdit(row: AIProviderItem) {
  editing.value = row
  showForm.value = true
}

async function enable(row: AIProviderItem) {
  try {
    await enableAiProvider(row.id)
    message.success(`已启用「${row.name}」`)
    await refresh()
  } catch {
    message.error('启用失败')
  }
}

async function remove(row: AIProviderItem) {
  try {
    await removeAiProvider(row.id)
    message.success('配置已删除')
    await refresh()
  } catch {
    message.error('删除失败')
  }
}

const columns = computed<DataTableColumns<AIProviderItem>>(() => [
  {
    title: '名称',
    key: 'name',
    render: row => h('div', { class: 'flex items-center gap-2' }, [
      h('span', { class: 'font-medium' }, row.name),
      row.enabled ? h(NTag, { type: 'success', size: 'small', bordered: false }, { default: () => '启用中' }) : null,
    ]),
  },
  {
    title: '供应商',
    key: 'vendor',
    width: 150,
    render: row => vendorLabel(row.vendor),
  },
  {
    title: '模型',
    key: 'model',
    width: 200,
  },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    render: row => h('div', { class: 'flex items-center gap-2' }, [
      h(
        NButton,
        {
          size: 'small',
          type: 'primary',
          ghost: true,
          disabled: row.enabled,
          onClick: () => enable(row),
        },
        { default: () => (row.enabled ? '已启用' : '启用') },
      ),
      h(
        NButton,
        { size: 'small', ghost: true, onClick: () => openEdit(row) },
        { default: () => '编辑' },
      ),
      h(
        NPopconfirm,
        { onPositiveClick: () => remove(row) },
        {
          trigger: () => h(NButton, { size: 'small', ghost: true, type: 'error' }, { default: () => '删除' }),
          default: () => '删除后不影响已产生的评分记录，确定删掉这条配置？',
        },
      ),
    ]),
  },
])

onMounted(refresh)
</script>

<template>
  <div>
    <div class="mb-3 flex items-center justify-between">
      <div>
        <p class="m-0 font-medium">模型配置</p>
        <p class="muted m-0 text-xs">同一时刻只有一条处于启用状态；它决定站内 AI 实际调用哪家模型</p>
      </div>
      <NButton type="primary" ghost @click="openCreate">添加配置</NButton>
    </div>

    <NDataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :bordered="false"
      :row-key="(row: AIProviderItem) => row.id"
    />

    <p v-if="!loading && !rows.length" class="admin-empty">还没有模型配置</p>
    <p v-else-if="!loading && !rows.some(row => row.enabled)" class="muted m-0 mt-3 text-xs">
      当前没有启用的配置：站内 AI 会提示「还没有启用的模型配置」，点上方「启用」任意一条即可。
    </p>

    <AiProviderForm v-model:show="showForm" :vendors="vendors" :item="editing" @saved="refresh" />
  </div>
</template>
