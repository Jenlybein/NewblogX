<script setup lang="ts">
import { IconLock, IconMail, IconUser } from '@tabler/icons-vue'
import { NButton, NCard, NInput, NSpace, useMessage } from 'naive-ui'
import { createFirstAdmin, getSetupStatus } from '~/services/setup'
import { isApiBusinessError } from '~/services/http/errors'

definePageMeta({ layout: 'auth' })

useHead({ title: '初始化' })

// 这个页面只在"库里还没有管理员"时可达。一旦初始化完成就跳回登录页 ——
// 不是靠隐藏入口，而是靠后端的闸门（接口会返回 setup.already_done）。
const router = useRouter()
const message = useMessage()

const checking = ref(true)
const submitting = ref(false)

const form = reactive({
  username: '',
  password: '',
  confirm: '',
  email: '',
})

onMounted(async () => {
  try {
    const status = await getSetupStatus()
    if (!status.needs_admin) {
      // 已经初始化过了：直接去登录页，别让人对着一个必然失败的按钮点。
      await router.replace('/login')
      return
    }
  } catch {
    message.error('无法确认站点初始化状态，请检查后端是否已启动')
  } finally {
    checking.value = false
  }
})

async function submit() {
  const username = form.username.trim()
  const email = form.email.trim()

  if (!username) {
    message.warning('请输入管理员用户名')
    return
  }
  if (form.password.length < 8) {
    message.warning('密码至少 8 位')
    return
  }
  if (form.password !== form.confirm) {
    message.warning('两次输入的密码不一致')
    return
  }

  submitting.value = true
  try {
    await createFirstAdmin({
      username,
      password: form.password,
      email: email || undefined,
    })
    message.success('管理员已创建，请用刚才的账号登录')
    await router.replace('/login')
  } catch (error) {
    message.error(isApiBusinessError(error) ? error.message : '创建失败，请稍后重试')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="admin-login-page">
    <div class="admin-login-shell">
      <section class="admin-login-hero">
        <div class="flex items-center gap-3">
          <AdminBrandLogo :size="52" />
          <div>
            <AdminBrandWordmark size="lg" layout="stack" />
            <p class="muted m-0 mt-1 text-sm">运营后台</p>
          </div>
        </div>

        <div class="admin-login-copy">
          <p class="eyebrow">First Run</p>
          <h1>初始化</h1>
          <p class="muted mt-4 max-w-[680px] leading-7">
            站点还没有管理员账号。设置好之后这个入口会永久关闭，
            后续新增账号请用管理员身份在「用户」页操作。
          </p>
        </div>
      </section>

      <NCard class="admin-login-card p-2" size="large">
        <template #header>
          <div>
            <h2 class="m-0 text-3xl font-semibold">创建管理员</h2>
            <p class="muted m-0 mt-2">这是唯一一次不需要登录就能建账号的机会。</p>
          </div>
        </template>

        <NSpace vertical :size="16">
          <NInput
            v-model:value="form.username"
            size="large"
            placeholder="管理员用户名"
            :disabled="checking"
            clearable
          >
            <template #prefix>
              <IconUser :size="16" />
            </template>
          </NInput>

          <NInput
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            size="large"
            placeholder="登录密码（至少 8 位）"
            :disabled="checking"
          >
            <template #prefix>
              <IconLock :size="16" />
            </template>
          </NInput>

          <NInput
            v-model:value="form.confirm"
            type="password"
            show-password-on="click"
            size="large"
            placeholder="再输一次密码"
            :disabled="checking"
            @keyup.enter="submit"
          >
            <template #prefix>
              <IconLock :size="16" />
            </template>
          </NInput>

          <NInput
            v-model:value="form.email"
            size="large"
            placeholder="邮箱（可选，用于找回密码）"
            :disabled="checking"
            @keyup.enter="submit"
          >
            <template #prefix>
              <IconMail :size="16" />
            </template>
          </NInput>

          <NButton
            type="primary"
            size="large"
            block
            :loading="submitting"
            :disabled="checking"
            @click="submit"
          >
            创建并进入登录页
          </NButton>
        </NSpace>
      </NCard>
    </div>
  </main>
</template>
