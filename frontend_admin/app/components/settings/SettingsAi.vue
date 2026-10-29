<script setup lang="ts">
import { NAlert, NButton, NDivider, NForm, NFormItem, NInput, NInputNumber, NSpace, NSwitch, useMessage } from 'naive-ui'
import { getAiRuntimeConfig, updateAiRuntimeConfig } from '~/services/admin'
import type { AIRuntimeConfig } from '~/types/api'
// 必须显式 import：components/ 下的子目录会带路径前缀注册成 `SettingsAiProviderList`，
// 模板里写 `<AiProviderList />` 解析不到（不报错、不渲染，整块 UI 静默消失）。
import AiProviderList from '~/components/settings/AiProviderList.vue'

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
    message.success('站点级设置已保存')
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
      这里有两层开关：「启用 AI 助手」是整站总闸；下方「模型配置」里<strong>启用</strong>的那一条
      决定实际用哪家模型。总闸关掉时，即使有启用的模型配置也不可用。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NFormItem label="启用 AI 助手">
        <NSwitch v-model:value="config.enable" />
      </NFormItem>
      <NFormItem label="助手昵称">
        <NInput v-model:value="config.nickname" />
      </NFormItem>
      <NFormItem label="助手头像 URL">
        <NInput v-model:value="config.avatar" />
      </NFormItem>
      <NFormItem label="助手简介">
        <NInput v-model:value="config.abstract" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
      </NFormItem>
      <NSpace :size="12" class="w-full">
        <NFormItem label="请求超时（秒）">
          <NInputNumber v-model:value="config.timeout_sec" :min="1" :max="600" />
        </NFormItem>
        <NFormItem label="最大输入字符">
          <NInputNumber v-model:value="config.max_input_chars" :min="100" />
        </NFormItem>
        <NFormItem label="每日配额（0 不限）">
          <NInputNumber v-model:value="config.daily_quota" :min="0" />
        </NFormItem>
        <NFormItem label="采样温度">
          <NInputNumber v-model:value="config.temperature" :min="0" :max="2" :step="0.1" />
        </NFormItem>
      </NSpace>
      <NButton type="primary" :loading="saving" @click="save">保存站点级设置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>

    <NDivider />

    <AiProviderList />
  </div>
</template>
