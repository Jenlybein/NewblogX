<script setup lang="ts">
import { IconLock, IconUser } from '@tabler/icons-vue'
import { NButton, NCard, NCheckbox, NInput, NSpace, useMessage } from 'naive-ui'
import { isApiBusinessError } from '~/services/http/errors'
import { getSetupStatus } from '~/services/setup'

definePageMeta({ layout: 'auth' })

useHead({ title: '后台登录' })

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const message = useMessage()

// 库里还没有管理员时，登录必然失败 —— 直接把人送到初始化页。
// 判断放在这里而不是全局守卫里：只有未登录的人会落到这个页面，
// 所以整站只在必要时多发这一个请求。
onMounted(async () => {
  try {
    const status = await getSetupStatus()
    if (status.needs_admin) {
      await router.replace('/setup')
    }
  } catch {
    // 查不到就正常显示登录表单：后端没起来时，报"无法连接"比
    // 跳到一个同样连不上的初始化页更有用。
  }
})

const form = reactive({
  username: '',
  password: '',
  remember: true,
})

async function submit() {
  if (!form.username.trim() || !form.password) {
    message.warning('请输入账号和密码')
    return
  }

  try {
    await auth.login({ username: form.username.trim(), password: form.password })
    message.success('已进入后台')
    await router.push(String(route.query.redirect || '/'))
  } catch (error) {
    message.error(isApiBusinessError(error) ? error.message : '登录失败，请检查账号或网络')
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
          <p class="eyebrow">Admin Access</p>
          <h1>登录区</h1>
          <p class="muted mt-4 max-w-[680px] leading-7">
            统一处理内容审核、站点配置与运行观测。
          </p>
        </div>

      </section>

      <NCard class="admin-login-card p-2" size="large">
        <template #header>
          <div>
            <h2 class="m-0 text-3xl font-semibold">后台登录</h2>
            <p class="muted m-0 mt-2">请输入管理员账号与密码。</p>
          </div>
        </template>

        <NSpace vertical :size="16">
          <NInput v-model:value="form.username" size="large" placeholder="用户名或邮箱" clearable @keyup.enter="submit">
            <template #prefix>
              <IconUser :size="16" />
            </template>
          </NInput>

          <NInput v-model:value="form.password" type="password" show-password-on="click" size="large" placeholder="登录密码"
            @keyup.enter="submit">
            <template #prefix>
              <IconLock :size="16" />
            </template>
          </NInput>

          <div class="flex items-center justify-between gap-3">
            <NCheckbox v-model:checked="form.remember">记住当前设备</NCheckbox>
            <NuxtLink class="text-sm text-focus underline-offset-4 hover:underline" to="/">返回仪表盘</NuxtLink>
          </div>

          <NButton type="primary" size="large" block :loading="auth.pending" @click="submit">
            登录后台
          </NButton>
        </NSpace>
      </NCard>
    </div>
  </main>
</template>
