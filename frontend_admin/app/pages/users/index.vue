<script setup lang="ts">
import { h } from 'vue'
import { NAvatar, NButton, NCard, NDataTable, NForm, NFormItem, NGrid, NGridItem, NInput, NModal, NSelect, NSpace, NTag, useMessage } from 'naive-ui'
import { createAdminUser, getAdminUsers, updateAdminUser } from '~/services/admin'
import type { UserListItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'

const message = useMessage()
const loading = ref(false)
const rows = ref<UserListItem[]>([])
const total = ref(0)
const selected = ref<UserListItem | null>(null)
const createVisible = ref(false)

const editForm = reactive({
  user_id: '',
  username: '',
  nickname: '',
  abstract: '',
  role: 1,
  status: 1,
})

const createForm = reactive({
  username: '',
  password: '',
  nickname: '',
  email: '',
})

const roleOptions = [
  { label: '普通用户', value: 1 },
  { label: '管理员', value: 2 },
  { label: '超级管理员', value: 3 },
]

const statusOptions = [
  { label: '正常', value: 1 },
  { label: '禁用', value: 2 },
  { label: '封禁', value: 3 },
]

const columns = [
  {
    title: '用户',
    key: 'name',
    minWidth: 220,
    render: (row: UserListItem) =>
      h(NSpace, { align: 'center', size: 10 }, {
        default: () => [
          h(NAvatar, { round: true, src: row.avatar }, { default: () => (row.nickname || row.username).slice(0, 1) }),
          h('div', null, [
            h('strong', { class: 'block' }, row.nickname || row.username),
            h('span', { class: 'muted text-xs' }, row.username),
          ]),
        ],
      }),
  },
  { title: 'IP', key: 'ip', width: 150 },
  { title: '地区', key: 'addr', width: 150 },
  {
    title: '角色',
    key: 'role',
    width: 110,
    render: (row: UserListItem) => h(NTag, { size: 'small' }, { default: () => roleLabel(row.role) }),
  },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row: UserListItem) =>
      h(NTag, { size: 'small', type: row.status === 2 || row.status === 3 ? 'error' : 'success' }, { default: () => statusLabel(row.status) }),
  },
  {
    title: '最近登录',
    key: 'last_login_at',
    width: 180,
    render: (row: UserListItem) => formatDateTime(row.last_login_at),
  },
  {
    title: '操作',
    key: 'action',
    width: 100,
    render: (row: UserListItem) => h(NButton, { size: 'small', tertiary: true, onClick: () => selectUser(row) }, { default: () => '编辑' }),
  },
]

function roleLabel(value?: number) {
  return ({ 1: '用户', 2: '管理员', 3: '超管' } as Record<number, string>)[value || 1] || '用户'
}

function statusLabel(value?: number) {
  return ({ 1: '正常', 2: '禁用', 3: '封禁' } as Record<number, string>)[value || 1] || '正常'
}

function selectUser(row: UserListItem) {
  selected.value = row
  editForm.user_id = row.id
  editForm.username = row.username
  editForm.nickname = row.nickname
  editForm.abstract = ''
  editForm.role = row.role || 1
  editForm.status = row.status || 1
}

async function refresh() {
  loading.value = true
  try {
    const data = await getAdminUsers()
    rows.value = data.list
    total.value = data.count
    if (!selected.value && rows.value[0]) selectUser(rows.value[0])
  } catch {
    message.error('用户列表加载失败')
  } finally {
    loading.value = false
  }
}

async function saveUser() {
  if (!editForm.user_id) return
  await updateAdminUser({
    user_id: editForm.user_id,
    username: editForm.username || null,
    nickname: editForm.nickname || null,
    abstract: editForm.abstract || null,
    role: editForm.role,
    status: editForm.status,
  })
  message.success('用户信息已保存')
  await refresh()
}

async function submitCreate() {
  if (!createForm.username || !createForm.password) {
    message.warning('请输入用户名和密码')
    return
  }

  await createAdminUser({
    username: createForm.username,
    password: createForm.password,
    nickname: createForm.nickname || null,
    email: createForm.email || null,
  })
  message.success('用户已创建')
  createVisible.value = false
  Object.assign(createForm, { username: '', password: '', nickname: '', email: '' })
  await refresh()
}

onMounted(refresh)
</script>

<template>
  <NSpace vertical :size="20">
    <NCard class="admin-card">
      <div class="admin-toolbar">
        <p class="muted m-0">共 {{ total }} 个用户</p>
        <NButton type="primary" @click="createVisible = true">新建用户</NButton>
      </div>
    </NCard>

    <NGrid :cols="24" :x-gap="20" :y-gap="20" responsive="screen">
      <NGridItem :span="15">
        <NCard class="admin-card" title="用户列表">
          <NDataTable :columns="columns" :data="rows" :loading="loading" :bordered="false" />
        </NCard>
      </NGridItem>

      <NGridItem :span="9">
        <NCard class="admin-card" title="用户资料">
          <NForm label-placement="top">
            <NFormItem label="用户名"><NInput v-model:value="editForm.username" /></NFormItem>
            <NFormItem label="昵称"><NInput v-model:value="editForm.nickname" /></NFormItem>
            <NFormItem label="简介"><NInput v-model:value="editForm.abstract" type="textarea" :autosize="{ minRows: 4, maxRows: 6 }" /></NFormItem>
            <NFormItem label="角色"><NSelect v-model:value="editForm.role" :options="roleOptions" /></NFormItem>
            <NFormItem label="状态"><NSelect v-model:value="editForm.status" :options="statusOptions" /></NFormItem>
            <NButton type="primary" block @click="saveUser">保存变更</NButton>
          </NForm>
        </NCard>
      </NGridItem>
    </NGrid>

    <NModal v-model:show="createVisible" preset="card" class="max-w-[520px]" title="新建用户">
      <NForm label-placement="top">
        <NFormItem label="用户名"><NInput v-model:value="createForm.username" /></NFormItem>
        <NFormItem label="密码"><NInput v-model:value="createForm.password" type="password" show-password-on="click" /></NFormItem>
        <NFormItem label="昵称"><NInput v-model:value="createForm.nickname" /></NFormItem>
        <NFormItem label="邮箱"><NInput v-model:value="createForm.email" /></NFormItem>
        <div class="flex justify-end gap-3">
          <NButton @click="createVisible = false">取消</NButton>
          <NButton type="primary" @click="submitCreate">创建</NButton>
        </div>
      </NForm>
    </NModal>
  </NSpace>
</template>
