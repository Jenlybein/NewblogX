<script setup lang="ts">
import { NAlert, NButton, NForm, NFormItem, NInput, NInputNumber, useMessage } from 'naive-ui'
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
      填完之后还要去「站点 → 登录策略」把「手机号登录」开关打开。
      开关与配置是独立的：配置为空时打开开关，用户点「发送验证码」会直接报错。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
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
      <NButton type="primary" :loading="saving" @click="save">保存短信配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
