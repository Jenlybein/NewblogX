<script setup lang="ts">
import { NAlert, NButton, NCard, NForm, NFormItem, NInput, NSwitch, useMessage } from 'naive-ui'
import { getOAuthRuntimeConfig, updateOAuthRuntimeConfig } from '~/services/admin'
import type { OAuthRuntimeConfig } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化（第一次双向写之后就不再窄化），整份模板都会报可能为 null。
const config = ref({} as OAuthRuntimeConfig)
const loaded = ref(false)

async function load() {
  loading.value = true
  try {
    config.value = await getOAuthRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('第三方登录配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await updateOAuthRuntimeConfig(config.value)
    message.success('第三方登录配置已保存')
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
      读者站目前<strong>没有 QQ 登录入口</strong>，这里配好了也不会生效（回调链路还没补）。
      先填着无妨，等入口上线即可用。
    </NAlert>
    <NAlert type="info" :bordered="false" class="mb-4">
      「QQ 登录」开关已搬回本页（票 票 481）：通道开关跟着通道走，配完密钥顺手就能开，
      不必再跑到「站点配置 → 登录设置」。配置为空时打开开关，用户点登录会直接报错。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="QQ 登录" size="small">
        <NFormItem label="启用 QQ 登录">
          <NSwitch v-model:value="config.qq_login" data-role="oauth-qq-login" />
        </NFormItem>
      </NCard>

      <NFormItem label="AppID">
        <NInput v-model:value="config.app_id" class="admin-field--key" placeholder="QQ 互联应用管理里的 APP ID" />
      </NFormItem>
      <NFormItem label="AppKey">
        <NInput
          v-model:value="config.app_key" class="admin-field--secret"
          type="password"
          show-password-on="click"
          placeholder="留 ****** 表示不修改"
        />
      </NFormItem>
      <NFormItem label="回调地址">
        <NInput v-model:value="config.redirect" class="admin-field--url" placeholder="https://yourdomain.com/oauth/qq/callback" />
      </NFormItem>
      <NButton type="primary" :loading="saving" @click="save">保存第三方登录配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>

