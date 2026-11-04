<script setup lang="ts">
import { IconLock, IconRefresh, IconShieldCheck, IconUser } from '@tabler/icons-vue'
import { NButton, NCard, NCheckbox, NInput, NSpace, useMessage } from 'naive-ui'
import { getLoginCaptcha, getPublicSiteLoginFlags } from '~/services/auth'
import { isApiBusinessError } from '~/services/http/errors'
import { getSetupStatus } from '~/services/setup'

definePageMeta({ layout: 'auth' })

useHead({ title: '后台登录' })

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const message = useMessage()

/** 后台登录失败的可读原因（A-2（票 261））。 */
const NOT_ADMIN_HINT = '该账号不是管理员，无法登录管理后台'

// 被守卫踢回登录页时带上原因：这不是"密码打错了"，文案不该含糊。
onMounted(() => {
  if (auth.deniedReason === 'not-admin') {
    message.warning(NOT_ADMIN_HINT)
    auth.deniedReason = ''
  }
})

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
  captchaCode: '',
  remember: true,
})

/**
 * 图形验证码（票 263）。
 *
 * 站点设置里开启 `login.captcha` 后，后端 `CaptchaMiddleware` 对 `captcha_id` /
 * `captcha_code` 都是 `required` —— 后台登录页以前不送这两个字段，于是"一开验证码，
 * 后台就把自己锁在外面"。这里按站点配置决定是否显示，并随登录请求带上。
 */
const captchaRequired = ref(false)
const captchaId = ref('')
const captchaImage = ref('')
const captchaLoading = ref(false)

async function refreshCaptcha() {
  captchaLoading.value = true
  try {
    const data = await getLoginCaptcha()
    captchaId.value = data.captcha_id
    captchaImage.value = data.base64
    form.captchaCode = ''
  } catch {
    // 拉不到验证码就不显示这一行：至少还能试密码登录，失败也会给出服务端原因。
    captchaRequired.value = false
  } finally {
    captchaLoading.value = false
  }
}

onMounted(async () => {
  try {
    const site = await getPublicSiteLoginFlags()
    if (site?.login?.captcha === true) {
      captchaRequired.value = true
      await refreshCaptcha()
    }
  } catch {
    // 读不到站点配置就当没开验证码：多显示一个填不对的输入框比不显示更糟。
  }
})

async function submit() {
  if (!form.username.trim() || !form.password) {
    message.warning('请输入账号和密码')
    return
  }
  if (captchaRequired.value && !form.captchaCode.trim()) {
    message.warning('请输入图形验证码')
    return
  }

  try {
    const ok = await auth.login({
      username: form.username.trim(),
      password: form.password,
      captchaId: captchaRequired.value ? captchaId.value : undefined,
      captchaCode: captchaRequired.value ? form.captchaCode.trim() : undefined,
    })
    if (!ok) {
      // 登录接口本身会拒非管理员（CodeAuthAdminRequired），走到这里说明
      // 令牌拿到了但账号不是管理员 —— 会话已被清掉，直接给结论。
      message.error(NOT_ADMIN_HINT)
      return
    }
    message.success('已进入后台')
    await router.push(String(route.query.redirect || '/'))
  } catch (error) {
    // 服务端对"密码对了但不是管理员"有专门文案（apperr.CodeAuthAdminRequired →
    // "该账号没有后台权限"），所以这里原样透出业务错误消息，别再统一盖成
    // "登录失败，请检查账号或网络" —— 那会让人以为密码打错了。
    // 验证码是一次性的：失败后换一张，避免用同一个已消费的 id 重试。
    if (captchaRequired.value) await refreshCaptcha()
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
            <AdminBrandWordmark size="lg" layout="stack" tone="inverse" />
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

          <div v-if="captchaRequired" class="flex items-center gap-3" data-role="login-captcha">
            <NInput
              v-model:value="form.captchaCode"
              size="large"
              placeholder="图形验证码"
              data-role="login-captcha-code"
              @keyup.enter="submit">
              <template #prefix>
                <IconShieldCheck :size="16" />
              </template>
            </NInput>
            <button
              type="button"
              class="admin-login-captcha"
              data-role="login-captcha-image"
              title="看不清？换一张"
              :disabled="captchaLoading"
              @click="refreshCaptcha">
              <img v-if="captchaImage" :src="captchaImage" alt="图形验证码" />
              <IconRefresh v-else :size="18" />
            </button>
          </div>

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
