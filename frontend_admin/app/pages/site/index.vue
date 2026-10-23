<script setup lang="ts">
import { NButton, NCard, NForm, NFormItem, NGrid, NGridItem, NInput, NInputNumber, NSelect, NSpace, NSwitch, useMessage } from 'naive-ui'
import { getAiRuntimeConfig, getSiteRuntimeConfig, updateAiRuntimeConfig, updateSiteRuntimeConfig } from '~/services/admin'
import type { AIRuntimeConfig, SiteRuntimeConfig } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const savingSite = ref(false)
const savingAi = ref(false)

const site = ref<SiteRuntimeConfig | null>(null)
const ai = ref<AIRuntimeConfig | null>(null)

const modeOptions = [
  { label: '社区模式', value: 1 },
  { label: '博客模式', value: 2 },
]

async function refresh() {
  loading.value = true
  try {
    const [siteConfig, aiConfig] = await Promise.all([getSiteRuntimeConfig(), getAiRuntimeConfig()])
    site.value = siteConfig
    ai.value = aiConfig
  } catch {
    message.error('站点配置加载失败')
  } finally {
    loading.value = false
  }
}

async function saveSite() {
  if (!site.value) return
  savingSite.value = true
  try {
    await updateSiteRuntimeConfig(site.value)
    message.success('站点配置已保存')
  } finally {
    savingSite.value = false
  }
}

async function saveAi() {
  if (!ai.value) return
  savingAi.value = true
  try {
    await updateAiRuntimeConfig(ai.value)
    message.success('AI 配置已保存')
  } finally {
    savingAi.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <NGrid :cols="24" :x-gap="20" :y-gap="20" responsive="screen">
    <NGridItem :span="12">
      <NCard class="admin-card" title="基础站点配置">
        <NForm v-if="site" label-placement="top" :disabled="loading">
          <NFormItem label="站点标题"><NInput v-model:value="site.site_info.title" /></NFormItem>
          <NFormItem label="Logo URL"><NInput v-model:value="site.site_info.logo" /></NFormItem>
          <NFormItem label="备案号"><NInput v-model:value="site.site_info.beian" /></NFormItem>
          <NFormItem label="站点模式"><NSelect v-model:value="site.site_info.mode" :options="modeOptions" /></NFormItem>
          <NFormItem label="SEO 关键词"><NInput v-model:value="site.seo.keywords" /></NFormItem>
          <NFormItem label="SEO 描述">
            <NInput v-model:value="site.seo.description" type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" />
          </NFormItem>
          <NFormItem label="文章免审核"><NSwitch v-model:value="site.article.skip_examining" /></NFormItem>
          <NFormItem label="评论免审核"><NSwitch v-model:value="site.comment.skip_examining" /></NFormItem>
          <NButton type="primary" :loading="savingSite" @click="saveSite">保存基础配置</NButton>
        </NForm>
        <div v-else class="admin-empty">配置加载中</div>
      </NCard>
    </NGridItem>

    <NGridItem :span="12">
      <NCard class="admin-card" title="AI 运行配置">
        <NForm v-if="ai" label-placement="top" :disabled="loading">
          <NFormItem label="启用 AI"><NSwitch v-model:value="ai.enable" /></NFormItem>
          <NFormItem label="Base URL"><NInput v-model:value="ai.base_url" /></NFormItem>
          <NFormItem label="Secret"><NInput v-model:value="ai.secret" type="password" show-password-on="click" /></NFormItem>
          <NFormItem label="对话模型"><NInput v-model:value="ai.chat_model" /></NFormItem>
          <NFormItem label="推理模型"><NInput v-model:value="ai.reason_model" /></NFormItem>
          <NSpace :size="12" class="w-full">
            <NFormItem label="超时秒数"><NInputNumber v-model:value="ai.timeout_sec" :min="1" /></NFormItem>
            <NFormItem label="最大输入"><NInputNumber v-model:value="ai.max_input_chars" :min="100" /></NFormItem>
            <NFormItem label="每日配额"><NInputNumber v-model:value="ai.daily_quota" :min="0" /></NFormItem>
          </NSpace>
          <NButton type="primary" :loading="savingAi" @click="saveAi">保存 AI 配置</NButton>
        </NForm>
        <div v-else class="admin-empty">配置加载中</div>
      </NCard>
    </NGridItem>
  </NGrid>
</template>
