<script setup lang="ts">
/**
 * 用户管理（管理员）。
 *
 * 三件事：**看列表**、**改资料**、**封禁 / 解封**。
 *
 * 本轮（决定清单 §6）修了三处：
 *
 *  1. **头像**：以前兜底直接取 `(nickname || username).slice(0, 1)`，而这里一半账号的
 *     用户名是邮箱，于是画出一个毫无意义的 `1`。现在走 `AdminAvatar` 的三层回退
 *     （图片 → 昵称首字符 → 人形图标）。
 *  2. **列表被裁**：7 列 minWidth 合计 1020px，以前塞进 `NGrid` 的 15/24 格且没有
 *     `scroll-x`，右侧「最近登录 / 操作」两列直接被切掉。现在表格占满内容区，
 *     并给 `scroll-x` 兜底窄视口。
 *  3. **封禁入口**：以前只能"先点编辑、再改右侧下拉"，路径绕。现在行内直接
 *     封禁 / 解封（二次确认，自己那一行禁用）；编辑从右栏常驻表单改成抽屉。
 *
 * 后端规则（`backend/internal/user/domain/rule.go`）：只有管理员能改，**不能改自己**，
 * 改状态会吊销该用户的全部会话。所以自己那一行必须在前端就先禁掉按钮，
 * 不能让人点了才吃一个 403。
 */
import { h } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NDrawer,
  NDrawerContent,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NSelect,
  NSpace,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import AdminAvatar from '~/components/admin/AdminAvatar.vue'
import { createAdminUser, getAdminUsers, updateAdminUser } from '~/services/admin'
import type { UserListItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'
import {
  ROLE_USER,
  STATUS_ACTIVE,
  STATUS_BANNED,
  roleLabel,
  roleOptions,
  statusLabel,
  statusOptions,
} from '~/utils/roles'

const message = useMessage()
const dialog = useDialog()
const auth = useAuthStore()

const loading = ref(false)
const saving = ref(false)
const rows = ref<UserListItem[]>([])
const total = ref(0)
const selected = ref<UserListItem | null>(null)
const createVisible = ref(false)
/** 资料编辑抽屉：不再常驻占右栏 9/24 的地。 */
const editorOpen = ref(false)

const editForm = reactive({
  user_id: '',
  username: '',
  nickname: '',
  abstract: '',
  role: ROLE_USER,
  status: STATUS_ACTIVE,
})

const createForm = reactive({
  username: '',
  password: '',
  nickname: '',
  email: '',
})

/** 当前登录的管理员 ID：自己那一行不允许改状态（后端也会拦）。 */
const selfUserId = computed(() => String(auth.currentUser?.id ?? ''))

function isSelf(row: UserListItem): boolean {
  return Boolean(selfUserId.value) && String(row.id) === selfUserId.value
}

function displayName(row: UserListItem): string {
  return row.nickname || row.username
}

const columns: DataTableColumns<UserListItem> = [
  {
    title: '用户',
    key: 'name',
    minWidth: 240,
    render: row =>
      h('div', { class: 'admin-user-cell' }, [
        h(AdminAvatar, { src: row.avatar, name: displayName(row), size: 36 }),
        h('div', { class: 'admin-user-cell__text' }, [
          h('strong', null, displayName(row)),
          // 用户名可能是邮箱，长到会把行撑破：交给 CSS 截断 + title 兜底可读。
          h('span', { class: 'muted text-xs admin-user-cell__username', title: row.username }, row.username),
        ]),
      ]),
  },
  { title: 'IP', key: 'ip', width: 150, render: row => row.ip || '—' },
  { title: '地区', key: 'addr', width: 150, render: row => row.addr || '—' },
  {
    title: '角色',
    key: 'role',
    width: 110,
    render: row => h(NTag, { size: 'small' }, { default: () => roleLabel(row.role) }),
  },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: row =>
      h(
        NTag,
        { size: 'small', type: row.status === 2 || row.status === 3 ? 'error' : 'success' },
        { default: () => statusLabel(row.status) },
      ),
  },
  { title: '最近登录', key: 'last_login_at', width: 190, render: row => formatDateTime(row.last_login_at) },
  {
    title: '操作',
    key: 'action',
    width: 200,
    fixed: 'right',
    render: row =>
      h(NSpace, { size: 4, justify: 'end' }, {
        default: () => [
          h(NButton, { size: 'small', tertiary: true, onClick: () => openEditor(row) }, { default: () => '编辑' }),
          row.status === STATUS_BANNED
            ? h(NButton, {
              size: 'small',
              tertiary: true,
              type: 'primary',
              disabled: isSelf(row),
              title: isSelf(row) ? '不能改自己的账号状态' : undefined,
              onClick: () => confirmUnban(row),
            }, { default: () => '解封' })
            : h(NButton, {
              size: 'small',
              tertiary: true,
              type: 'error',
              disabled: isSelf(row),
              title: isSelf(row) ? '不能改自己的账号状态' : undefined,
              onClick: () => confirmBan(row),
            }, { default: () => '封禁' }),
        ],
      }),
  },
]

function openEditor(row: UserListItem) {
  selected.value = row
  editForm.user_id = row.id
  editForm.username = row.username
  editForm.nickname = row.nickname
  editForm.abstract = ''
  // 缺省按注册用户，绝不默认成管理员（见 utils/roles.ts 的历史事故）。
  editForm.role = row.role || ROLE_USER
  editForm.status = row.status || STATUS_ACTIVE
  editorOpen.value = true
}

function closeEditor() {
  editorOpen.value = false
  selected.value = null
}

async function refresh() {
  loading.value = true
  try {
    const data = await getAdminUsers()
    rows.value = data.list
    total.value = data.count
  } catch {
    message.error('用户列表加载失败')
  } finally {
    loading.value = false
  }
}

async function saveUser() {
  if (!editForm.user_id || saving.value) return
  saving.value = true
  try {
    await updateAdminUser({
      user_id: editForm.user_id,
      username: editForm.username || null,
      nickname: editForm.nickname || null,
      abstract: editForm.abstract || null,
      role: editForm.role,
      status: editForm.status,
    })
    message.success('用户信息已保存')
    closeEditor()
    await refresh()
  } catch (error) {
    message.error(error instanceof Error ? `保存失败：${error.message}` : '保存失败，请稍后重试')
  } finally {
    saving.value = false
  }
}

/** 封禁 / 解封只改 status：封禁 = 3，解封 = 1（禁用 2 走抽屉里的状态下拉）。 */
async function applyStatus(row: UserListItem, status: number, successText: string) {
  try {
    await updateAdminUser({ user_id: row.id, status })
    message.success(successText)
    await refresh()
  } catch (error) {
    message.error(error instanceof Error ? `${successText}失败：${error.message}` : '操作失败，请稍后重试')
  }
}

function confirmBan(row: UserListItem) {
  dialog.warning({
    title: '封禁账号',
    content: `确认封禁「${displayName(row)}」？封禁后该用户的全部登录会话会被立即吊销，且不能自助解封。`,
    positiveText: '封禁',
    negativeText: '取消',
    async onPositiveClick() {
      await applyStatus(row, STATUS_BANNED, '账号已封禁')
    },
  })
}

function confirmUnban(row: UserListItem) {
  dialog.info({
    title: '解封账号',
    content: `确认解封「${displayName(row)}」？解封后账号恢复正常，可以重新登录。`,
    positiveText: '解封',
    negativeText: '取消',
    async onPositiveClick() {
      await applyStatus(row, STATUS_ACTIVE, '账号已解封')
    },
  })
}

async function submitCreate() {
  if (!createForm.username || !createForm.password) {
    message.warning('请输入用户名和密码')
    return
  }

  try {
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
  } catch (error) {
    message.error(error instanceof Error ? `创建失败：${error.message}` : '创建失败，请稍后重试')
  }
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

    <NCard class="admin-card" title="用户列表">
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :bordered="false"
        :scroll-x="1140"
        :row-key="(row: UserListItem) => row.id" />
    </NCard>
  </NSpace>

  <!-- 资料编辑：抽屉（宽 420，不铺满视口）。 -->
  <NDrawer v-model:show="editorOpen" :width="420" placement="right">
    <NDrawerContent title="用户资料" closable @close="closeEditor">
      <NForm label-placement="top" class="admin-field-form">
        <NFormItem label="用户名">
          <NInput v-model:value="editForm.username" class="admin-field--key" />
        </NFormItem>
        <NFormItem label="昵称">
          <NInput v-model:value="editForm.nickname" class="admin-field--key" />
        </NFormItem>
        <NFormItem label="简介">
          <NInput
            v-model:value="editForm.abstract"
            type="textarea"
            class="admin-field--long"
            :autosize="{ minRows: 4, maxRows: 8 }" />
        </NFormItem>
        <NFormItem label="角色">
          <NSelect v-model:value="editForm.role" class="admin-field--select" :options="roleOptions" />
        </NFormItem>
        <NFormItem label="状态">
          <NSelect v-model:value="editForm.status" class="admin-field--select" :options="statusOptions" />
        </NFormItem>
        <p v-if="selected && isSelf(selected)" class="muted text-xs">
          这是你自己的账号：后端不允许改自己的角色与状态，保存时会被拒绝。
        </p>
      </NForm>

      <template #footer>
        <NSpace justify="end">
          <NButton :disabled="saving" @click="closeEditor">取消</NButton>
          <NButton type="primary" :loading="saving" @click="saveUser">保存变更</NButton>
        </NSpace>
      </template>
    </NDrawerContent>
  </NDrawer>

  <NModal v-model:show="createVisible" preset="card" class="max-w-[520px]" title="新建用户">
    <NForm label-placement="top" class="admin-field-form">
      <NFormItem label="用户名"><NInput v-model:value="createForm.username" class="admin-field--key" /></NFormItem>
      <NFormItem label="密码">
        <NInput v-model:value="createForm.password" class="admin-field--key" type="password" show-password-on="click" />
      </NFormItem>
      <NFormItem label="昵称"><NInput v-model:value="createForm.nickname" class="admin-field--key" /></NFormItem>
      <NFormItem label="邮箱"><NInput v-model:value="createForm.email" class="admin-field--key" /></NFormItem>
      <div class="flex justify-end gap-3">
        <NButton @click="createVisible = false">取消</NButton>
        <NButton type="primary" @click="submitCreate">创建</NButton>
      </div>
    </NForm>
  </NModal>
</template>
