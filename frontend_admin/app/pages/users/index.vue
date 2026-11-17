<script setup lang="ts">
/**
 * 用户管理（管理员）。
 *
 * 四件事：**看列表**、**改资料**、**封禁 / 解封**、**解除禁言**（票 399）。
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
 *  4. **禁言**（票 399）：列表新增「禁言」列（到期时间 + 剩余天数），行内可
 *     「解除禁言」（提前放人；到期本来就会自动恢复，不需要人工操作）。
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
import { createAdminUser, getAdminUsers, unmuteAdminUser, updateAdminUser } from '~/services/admin'
import type { UserListItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'
import { contactText, loginPlaceText } from '~/utils/userContact'
import { muteStatusText } from '~/utils/mute'
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
          // 联系方式：手机号（登录主标识）与邮箱，两个都没有时给一句灰字而不是空行 ——
          // 空行会让"查不到"和"渲染坏了"看起来一样。
          h(
            'span',
            { class: 'muted text-xs admin-user-cell__contact', title: contactText(row) },
            contactText(row),
          ),
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
  {
    // 禁言（票 399）：只显示"禁言至某时（还有 N 天）"；到期自动恢复，不需要人工操作。
    title: '禁言',
    key: 'mute_until',
    width: 220,
    render: row => {
      const text = muteStatusText(row.mute_until)
      return text
        ? h('div', { class: 'text-xs' }, text)
        : h('div', { class: 'muted text-xs' }, '—')
    },
  },
  { title: '最近登录', key: 'last_login_at', width: 190, render: row => formatDateTime(row.last_login_at) },
  {
    title: '操作',
    key: 'action',
    width: 290,
    fixed: 'right',
    render: row => {
      // 「解除禁言」只在真有禁言时出现；已到期的行列里已经是 —，也就不该再有按钮。
      const muted = muteStatusText(row.mute_until) !== ''
      return h(NSpace, { size: 4, justify: 'end' }, {
        default: () => [
          h(NButton, { size: 'small', tertiary: true, onClick: () => openEditor(row) }, { default: () => '编辑' }),
          muted
            ? h(NButton, {
              size: 'small',
              tertiary: true,
              type: 'warning',
              disabled: isSelf(row),
              title: isSelf(row) ? '不能改自己的账号状态' : undefined,
              onClick: () => confirmUnmute(row),
            }, { default: () => '解除禁言' })
            : null,
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
        ].filter(Boolean),
      })
    },
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

/**
 * 删除用户头像。
 *
 * 传空串的 `avatar_image_id` 是后端的"明确清空"语义（见 user/app/admin.go）。
 * 这是独立动作、立刻提交，不跟「保存变更」一起走：删头像不该顺手把
 * 表单里还没保存的昵称/角色一起写进去。
 */
async function removeAvatar() {
  if (!selected.value || saving.value) return
  saving.value = true
  try {
    await updateAdminUser({ user_id: selected.value.id, avatar_image_id: '' })
    message.success('头像已删除')
    await refresh()
    // 列表刷新后把抽屉里的选中行换成最新的一份，头像预览才会跟着变。
    const fresh = rows.value.find(item => item.id === selected.value?.id)
    if (fresh) selected.value = fresh
  } catch (error) {
    message.error(error instanceof Error ? `删除头像失败：${error.message}` : '删除头像失败，请稍后重试')
  } finally {
    saving.value = false
  }
}

/** 封禁 / 解封只改 status：封禁 = 3，解封 = 1（禁用 2 走抽屉里的状态下拉）。 */async function applyStatus(row: UserListItem, status: number, successText: string) {
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

/**
 * 提前解除禁言（票 399）：清空 mute_until，发布能力立刻恢复。
 *
 * 与"到期自动恢复"是两条路：到期本来不需要任何操作，这里只是提前放人。
 */
async function confirmUnmute(row: UserListItem) {
  dialog.warning({
    title: '解除禁言',
    content: `确认解除「${displayName(row)}」的禁言？解除后该用户立刻可以发文 / 评论 / 私信。`,
    positiveText: '解除禁言',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await unmuteAdminUser(row.id)
        message.success('已解除禁言')
        await refresh()
      } catch (error) {
        message.error(error instanceof Error ? `解除禁言失败：${error.message}` : '解除禁言失败，请稍后重试')
      }
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
      <!-- 账号信息：只读。手机号与邮箱是登录主标识，只能由用户自己在读者站验证后绑定，
           后台不能直接改（直接改等于管理员能替任何人改绑登录凭证）。 -->
      <div v-if="selected" class="admin-detail-block" data-role="user-detail-block">
        <div class="admin-detail-row">
          <span class="admin-detail-label">手机号</span>
          <span data-role="user-detail-phone">{{ selected.phone || '未绑定' }}</span>
        </div>
        <div class="admin-detail-row">
          <span class="admin-detail-label">邮箱</span>
          <span data-role="user-detail-email">{{ selected.email || '未绑定' }}</span>
        </div>
        <div class="admin-detail-row">
          <span class="admin-detail-label">简介</span>
          <span data-role="user-detail-abstract">{{ selected.abstract || '未填写' }}</span>
        </div>
        <div class="admin-detail-row">
          <span class="admin-detail-label">注册时间</span>
          <span data-role="user-detail-created">{{ formatDateTime(selected.created_at) }}</span>
        </div>
        <div class="admin-detail-row">
          <span class="admin-detail-label">最近登录</span>
          <span data-role="user-detail-last-login">{{ formatDateTime(selected.last_login_at) }}</span>
        </div>
        <div class="admin-detail-row">
          <span class="admin-detail-label">登录地点</span>
          <span data-role="user-detail-addr">{{ loginPlaceText(selected) }}</span>
        </div>
      </div>

      <NForm label-placement="top" class="admin-field-form">
        <NFormItem label="头像">
          <div class="user-avatar-edit" data-role="user-avatar-edit">
            <AdminAvatar :src="selected?.avatar" :name="selected ? displayName(selected) : ''" :size="56" />
            <div class="user-avatar-edit__actions">
              <NButton
                size="small"
                :disabled="!selected?.avatar || saving"
                data-role="user-avatar-remove"
                @click="removeAvatar">
                删除头像
              </NButton>
              <p class="muted text-xs">{{ selected?.avatar ? '删除后回退到首字母头像。' : '当前没有头像。' }}</p>
            </div>
          </div>
        </NFormItem>
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
