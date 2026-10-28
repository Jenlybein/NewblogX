<script setup lang="ts">
import { NAlert, NButton, NForm, NFormItem, NInput, useMessage } from 'naive-ui'
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
  <div>
    <NAlert type="warning" :bordered="false" class="mb-4">
      读者站目前<strong>没有 QQ 登录入口</strong>，这里配好了也不会生效（回调链路还没补）。
      先填着无妨，等入口上线即可用。
    </NAlert>
    <NAlert type="info" :bordered="false" class="mb-4">
      填完这里之后，还要去「站点 → 登录策略」把「QQ 登录」开关打开。
      两个开关是独立的：配置为空时打开开关，用户点登录会直接报错。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NFormItem label="AppID">
        <NInput v-model:value="config.app_id" placeholder="QQ 互联应用管理里的 APP ID" />
      </NFormItem>
      <NFormItem label="AppKey">
        <NInput
          v-model:value="config.app_key"
          type="password"
          show-password-on="click"
          placeholder="留 ****** 表示不修改"
        />
      </NFormItem>
      <NFormItem label="回调地址">
        <NInput v-model:value="config.redirect" placeholder="https://yourdomain.com/oauth/qq/callback" />
      </NFormItem>
      <NButton type="primary" :loading="saving" @click="save">保存第三方登录配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
