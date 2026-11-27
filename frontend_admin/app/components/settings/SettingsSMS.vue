<script setup lang="ts">
import { NAlert, NButton, NCard, NForm, NFormItem, NInput, NInputNumber, NSpace, NSwitch, useMessage } from 'naive-ui'
import { getSMSRuntimeConfig, updateSMSRuntimeConfig } from '~/services/admin'
import type { SMSRuntimeConfig } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化，整份模板都会报可能为 null。
const config = ref({} as SMSRuntimeConfig)
const loaded = ref(false)

async function load() {
  loading.value = true
  try {
    config.value = await getSMSRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('短信配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await updateSMSRuntimeConfig(config.value)
    message.success('短信配置已保存')
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
      走阿里云「号码认证服务 → 短信认证」。它免申请签名与模板，验证码由阿里云生成并核验，
      所以这里<strong>不需要选验证码长度或文案</strong>，只需填控制台给的那一套签名与模板。
    </NAlert>
    <NAlert type="warning" :bordered="false" class="mb-4">
      本页的「手机号登录」开关决定读者站显示不显示这个入口（票 票 481 把它按通道搬到这里）。
      开关与配置是独立的：配置为空时打开开关，用户点「发送验证码」会直接报错。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="手机号登录" size="small">
        <!--
          手机号登录兼注册（"首次验证即建档"）：这里没有单独的注册开关，因为请求体层面
          分不出这次是登录还是注册。注册的验证码闸门在「验证码与通知 → 图形验证码」。
        -->
        <NFormItem label="手机号登录（首次登录即注册）">
          <NSwitch v-model:value="config.phone_login" data-role="sms-phone-login" />
        </NFormItem>
      </NCard>

      <NFormItem label="AccessKey ID">
        <NInput v-model:value="config.access_key_id" class="admin-field--key" placeholder="阿里云 RAM 用户的 AccessKey ID" />
      </NFormItem>
      <NFormItem label="AccessKey Secret">
        <NInput
          v-model:value="config.access_key_secret" class="admin-field--secret"
          type="password"
          show-password-on="click"
          placeholder="留 ****** 表示不修改"
        />
      </NFormItem>
      <NFormItem label="接入点">
        <NInput v-model:value="config.endpoint" class="admin-field--url" placeholder="留空使用 dypnsapi.aliyuncs.com" />
      </NFormItem>
      <NFormItem label="短信签名">
        <NInput v-model:value="config.sign_name" class="admin-field--key" placeholder="控制台赠送的签名，需与赠送模板配套" />
      </NFormItem>
      <NFormItem label="短信模板 Code">
        <NInput v-model:value="config.template_code" class="admin-field--key" placeholder="控制台赠送的短信认证模板 Code" />
      </NFormItem>
      <NFormItem label="调用超时（秒）">
        <NInputNumber v-model:value="config.timeout_sec" class="admin-field--number" :min="1" :max="60" placeholder="默认 10" />
      </NFormItem>
      <NCard class="admin-card mb-4" title="发码限流" size="small">
        <NAlert type="info" :bordered="false" class="mb-3">
          短信成本远高于邮件，所以两条通道分开计数、互不占用额度。
        </NAlert>
        <NSpace :size="12" class="w-full">
          <NFormItem label="短信验证码有效期（分钟）">
            <NInputNumber v-model:value="config.sms_code_timeout" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="短信限流窗口（秒）">
            <NInputNumber v-model:value="config.sms_send_window_second" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单手机号窗口内最多发码">
            <NInputNumber v-model:value="config.sms_send_per_phone_max" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单 IP 短信窗口内最多发码">
            <NInputNumber v-model:value="config.sms_send_per_ip_max" class="admin-field--number" :min="1" />
          </NFormItem>
        </NSpace>
      </NCard>

      <NButton type="primary" :loading="saving" @click="save">保存短信配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>

