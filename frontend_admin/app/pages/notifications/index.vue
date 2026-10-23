<script setup lang="ts">
import { h } from 'vue'
import { NButton, NCard, NDataTable, NDatePicker, NForm, NFormItem, NGrid, NGridItem, NInput, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import { createGlobalNotification, deleteGlobalNotifications, getGlobalNotifications } from '~/services/admin'
import type { GlobalNotifListItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const page = ref(1)
const hasMore = ref(false)
const rows = ref<GlobalNotifListItem[]>([])
const form = reactive({
  title: '',
  content: '',
  icon: '',
  href: '',
  expire_time: null as number | null,
})

const columns = [
  { title: '标题', key: 'title', minWidth: 180 },
  { title: '内容', key: 'content', minWidth: 260 },
  { title: '图标', key: 'icon', width: 100 },
  {
    title: '状态',
    key: 'is_read',
    width: 100,
    render: (row: GlobalNotifListItem) => h(NTag, { size: 'small', type: row.is_read ? 'default' : 'success' }, { default: () => (row.is_read ? '已读' : '未读') }),
  },
  {
    title: '创建时间',
    key: 'create_at',
    width: 180,
    render: (row: GlobalNotifListItem) => formatDateTime(row.create_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row: GlobalNotifListItem) => h(NButton, { size: 'small', tertiary: true, type: 'error', onClick: () => remove(row) }, { default: () => '删除' }),
  },
]

async function refresh() {
  loading.value = true
  try {
    const data = await getGlobalNotifications({ type: 2, page: page.value, limit: 10 })
    rows.value = data.list
    hasMore.value = data.has_more
  } catch {
    message.error('全局通知加载失败')
  } finally {
    loading.value = false
  }
}

async function submit() {
  if (!form.title || !form.content) {
    message.warning('标题和内容必填')
    return
  }

  try {
    await createGlobalNotification({
      title: form.title,
      content: form.content,
      icon: form.icon,
      href: form.href,
      expire_time: form.expire_time ? new Date(form.expire_time).toISOString() : null,
      user_visible_rule: 1,
    })
    message.success('全局通知已发布')
    Object.assign(form, { title: '', content: '', icon: '', href: '', expire_time: null })
    await refresh()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '全局通知发布失败')
  }
}

function remove(row: GlobalNotifListItem) {
  dialog.warning({
    title: '删除全局通知',
    content: row.title,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      await deleteGlobalNotifications([row.id])
      message.success('已删除')
      await refresh()
    },
  })
}

watch(page, refresh)
onMounted(refresh)
</script>

<template>
  <NGrid :cols="24" :x-gap="20" :y-gap="20" responsive="screen">
    <NGridItem :span="9">
      <NCard class="admin-card" title="发布通知">
        <NForm label-placement="top">
          <NFormItem label="标题"><NInput v-model:value="form.title" /></NFormItem>
          <NFormItem label="图标"><NInput v-model:value="form.icon" placeholder="info / warning / system" /></NFormItem>
          <NFormItem label="跳转地址"><NInput v-model:value="form.href" /></NFormItem>
          <NFormItem label="过期时间">
            <NDatePicker
              v-model:value="form.expire_time"
              type="datetime"
              clearable
              class="w-full"
              placeholder="选择通知过期时间" />
          </NFormItem>
          <NFormItem label="内容"><NInput v-model:value="form.content" type="textarea" :autosize="{ minRows: 5, maxRows: 8 }" /></NFormItem>
          <NButton type="primary" block @click="submit">发布通知</NButton>
        </NForm>
      </NCard>
    </NGridItem>

    <NGridItem :span="15">
      <NCard class="admin-card" title="通知列表">
        <NDataTable :columns="columns" :data="rows" :loading="loading" :bordered="false" />
        <div class="mt-4 flex justify-end gap-2">
          <NButton :disabled="page <= 1" @click="page--">上一页</NButton>
          <NButton :disabled="!hasMore" @click="page++">下一页</NButton>
        </div>
      </NCard>
    </NGridItem>
  </NGrid>
</template>
