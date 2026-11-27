<script setup lang="ts">
// 登录设置：用户名密码登录开关 + 登录失败限流（ADR-0029 改写版 / 票 481）。
//
// 这一页在字段按通道拆段之后只剩两件事：
//   - **密码登录**：它是"登录这件事"本身的开关（邮箱/手机号/QQ 三条通道各自的
//     开关跟着通道走了，见「用户操作」下的第三方登录与「验证码与通知」下的邮件/短信）；
//   - **登录失败限流**：它统计的是"登录尝试"，与走哪条通道无关，所以三种登录方式
//     共用同一套计数，留在这里是对的。
//
// 图形验证码的总开关与四个入口闸门搬到了「验证码与通知 → 图形验证码」：它们决定
// 每个表单渲不渲染验证码框，与"哪种登录方式开放"是两个问题 —— 混在一页时，
// 想只给发码加验证码就得翻半页开关。
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
    message.success('登录设置已保存')
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
    <NAlert type="info" :bordered="false" class="mb-4">
      邮箱、手机号与 QQ 登录的开关跟着各自的通道走了：邮件与短信在「验证码与通知」，
      QQ 在「用户操作 → 第三方登录」。图形验证码的开关与入口闸门在「验证码与通知 → 图形验证码」。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="登录方式" size="small">
        <NFormItem label="用户名密码登录">
          <NSwitch v-model:value="config.login.username_pwd_login" />
        </NFormItem>
      </NCard>

      <NCard class="admin-card mb-4" title="登录失败限流" size="small">
        <NAlert type="info" :bordered="false" class="mb-3">
          三种登录方式共用同一套失败计数：撞库的人不会只用一种入口。
        </NAlert>
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

      <NButton type="primary" :loading="saving" @click="save">保存登录设置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
