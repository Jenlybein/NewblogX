<script setup lang="ts">
// 登录与发码：登录开关、验证码与发码/登录失败限流（ADR-0029 拆页后的第二页）。
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInputNumber,
  NSpace,
  NSwitch,
  useMessage,
} from 'naive-ui'

import { getSiteRuntimeConfig, updateSiteRuntimeConfig } from '~/services/admin'
import type { SiteRuntimeConfig } from '~/types/api'
import { applySiteSectionPatch } from '~/utils/siteSectionSave'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
const config = ref({} as SiteRuntimeConfig)
const loaded = ref(false)

const OWNED_BLOCKS = ['login'] as const

async function load() {
  loading.value = true
  try {
    config.value = await getSiteRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('登录配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const latest = await getSiteRuntimeConfig()
    const merged = applySiteSectionPatch(latest, config.value, OWNED_BLOCKS)
    await updateSiteRuntimeConfig(merged)
    message.success('登录与发码配置已保存')
  } catch {
    message.error('保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="admin-settings-panel">
    <NAlert type="warning" :bordered="false" class="mb-4">
      「邮箱验证码登录」打开前，必须先在「邮件」页把腾讯云 SES 配完；
      「手机号登录」打开前，必须先在「短信」页把阿里云 AccessKey、签名与模板填完。
      没配就打开，用户点「获取验证码」会直接报错。
    </NAlert>

    <NAlert type="info" :bordered="false" class="mb-4">
      本网站的注册入口是「手机号登录」：它同时承担注册（首次验证即建档）。
      「邮箱注册」默认关闭；关掉它不影响邮箱登录与「绑定邮箱」，后两者各有自己的开关与用途。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="登录方式" size="small">
        <NFormItem label="用户名密码登录">
          <NSwitch v-model:value="config.login.username_pwd_login" />
        </NFormItem>
        <NFormItem label="手机号登录（首次登录即注册）">
          <NSwitch v-model:value="config.login.phone_login" />
        </NFormItem>
        <NFormItem label="邮箱验证码登录">
          <NSwitch v-model:value="config.login.email_login" />
        </NFormItem>
        <!--
          邮箱注册与邮箱登录分开：绑定邮箱要用发码接口，混成一个开关会连带把
          「给已注册用户发绑定验证码」一起关掉。本站点默认注册只走手机号。
        -->
        <NFormItem label="邮箱注册（默认关闭：注册只走手机号）">
          <NSwitch v-model:value="config.login.email_register" />
        </NFormItem>
        <NFormItem label="QQ 登录">
          <NSwitch v-model:value="config.login.qq_login" />
        </NFormItem>
        <NFormItem label="启用图形验证码（总开关）">
          <NSwitch v-model:value="config.login.captcha" data-role="login-captcha-switch" />
        </NFormItem>
        <!--
          三个入口闸门：总开关打开后，逐个决定哪条路由真的校验。
          登录与发码被滥用的方式不同（发码烧钱、登录被撞库），一刀切会让
          「只想给发码加验证码」做不到。它们必须对读者公开 —— 前端要靠它们
          决定每个表单画不画验证码框；图片参数在「站点 → 图形验证码」页。
        -->
        <NFormItem label="登录接口要求图形验证码">
          <NSwitch
            v-model:value="config.login.captcha_login"
            :disabled="!config.login.captcha"
            data-role="login-captcha-gate-login"
          />
          <span v-if="!config.login.captcha" class="admin-hint">总开关关闭时不生效</span>
        </NFormItem>
        <NFormItem label="发邮箱验证码要求图形验证码">
          <NSwitch
            v-model:value="config.login.captcha_email_verify"
            :disabled="!config.login.captcha"
            data-role="login-captcha-gate-email"
          />
        </NFormItem>
        <NFormItem label="注册接口要求图形验证码">
          <NSwitch
            v-model:value="config.login.captcha_register"
            :disabled="!config.login.captcha"
            data-role="login-captcha-gate-register"
          />
          <span class="admin-hint">只作用于专门的注册路由；手机号注册的验证码在发码那一步</span>
        </NFormItem>
        <NFormItem label="发短信验证码要求图形验证码">
          <NSwitch
            v-model:value="config.login.captcha_phone_verify"
            :disabled="!config.login.captcha"
            data-role="login-captcha-gate-phone"
          />
          <NuxtLink to="/site/captcha" class="admin-link ml-2">
            图片样式与有效期 →
          </NuxtLink>
        </NFormItem>
      </NCard>

      <NCard class="admin-card mb-4" title="发码限流" size="small">
        <NAlert type="info" :bordered="false" class="mb-3">
          邮件与短信分开计数：短信成本远高于邮件，两条通道的滥用方式也不同，配额互不占用。
        </NAlert>
        <NSpace :size="12" class="w-full">
          <NFormItem label="验证码有效期（分钟）">
            <NInputNumber v-model:value="config.login.email_code_timeout" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="发码限流窗口（秒）">
            <NInputNumber v-model:value="config.login.email_send_window_second" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单邮箱窗口内最多发码">
            <NInputNumber v-model:value="config.login.email_send_per_email_max" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单 IP 窗口内最多发码">
            <NInputNumber v-model:value="config.login.email_send_per_ip_max" class="admin-field--number" :min="1" />
          </NFormItem>
        </NSpace>
        <NSpace :size="12" class="mt-3 w-full">
          <NFormItem label="短信验证码有效期（分钟）">
            <NInputNumber v-model:value="config.login.sms_code_timeout" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="短信限流窗口（秒）">
            <NInputNumber v-model:value="config.login.sms_send_window_second" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单手机号窗口内最多发码">
            <NInputNumber v-model:value="config.login.sms_send_per_phone_max" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单 IP 短信窗口内最多发码">
            <NInputNumber v-model:value="config.login.sms_send_per_ip_max" class="admin-field--number" :min="1" />
          </NFormItem>
        </NSpace>
      </NCard>

      <NCard class="admin-card mb-4" title="登录失败限流" size="small">
        <NSpace :size="12" class="w-full">
          <NFormItem label="登录失败统计窗口（分钟）">
            <NInputNumber v-model:value="config.login.login_fail_window_minute" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单账号窗口内最多失败">
            <NInputNumber v-model:value="config.login.login_fail_user_max" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单 IP 窗口内最多失败">
            <NInputNumber v-model:value="config.login.login_fail_ip_max" class="admin-field--number" :min="1" />
          </NFormItem>
        </NSpace>
      </NCard>

      <NButton type="primary" :loading="saving" @click="save">保存登录与发码配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
