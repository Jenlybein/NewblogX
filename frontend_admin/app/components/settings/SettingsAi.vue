<script setup lang="ts">
import { NAlert, NButton, NForm, NFormItem, NInput, NInputNumber, NSpace, NSwitch, useMessage } from 'naive-ui'
import { getAiRuntimeConfig, updateAiRuntimeConfig } from '~/services/admin'
import type { AIRuntimeConfig } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化（第一次双向写之后就不再窄化），整份模板都会报可能为 null。
const config = ref({} as AIRuntimeConfig)
const loaded = ref(false)

async function load() {
  loading.value = true
  try {
    config.value = await getAiRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('AI 配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await updateAiRuntimeConfig(config.value)
    message.success('AI 配置已保存')
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
    <NAlert type="info" :bordered="false" class="mb-4">
      读者站的 AI 对话入口只在「启用」打开、且 Secret 与 Base URL 都填对时才可用。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NFormItem label="启用 AI 助手">
        <NSwitch v-model:value="config.enable" />
      </NFormItem>
      <NFormItem label="Base URL">
        <NInput v-model:value="config.base_url" placeholder="https://api.example.com/v1/chat/completions" />
      </NFormItem>
      <NFormItem label="Secret">
        <NInput v-model:value="config.secret" type="password" show-password-on="click" placeholder="留 ****** 表示不修改" />
      </NFormItem>
      <NFormItem label="对话模型">
        <NInput v-model:value="config.chat_model" />
      </NFormItem>
      <NFormItem label="推理模型（预留）">
        <NInput v-model:value="config.reason_model" />
      </NFormItem>
      <NSpace :size="12" class="w-full">
        <NFormItem label="超时秒数">
          <NInputNumber v-model:value="config.timeout_sec" :min="1" />
        </NFormItem>
        <NFormItem label="最大输入字符">
          <NInputNumber v-model:value="config.max_input_chars" :min="100" />
        </NFormItem>
        <NFormItem label="每日配额（0 不限）">
          <NInputNumber v-model:value="config.daily_quota" :min="0" />
        </NFormItem>
      </NSpace>
      <NFormItem label="助手昵称">
        <NInput v-model:value="config.nickname" />
      </NFormItem>
      <NFormItem label="助手头像 URL">
        <NInput v-model:value="config.avatar" />
      </NFormItem>
      <NFormItem label="助手简介">
        <NInput v-model:value="config.abstract" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
      </NFormItem>
      <NButton type="primary" :loading="saving" @click="save">保存 AI 配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
