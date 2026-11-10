<script setup lang="ts">
// 私信：站点级陌生人策略与按对限流（ADR-0026 / ADR-0039 拆页后的第四页）。
import { NAlert, NButton, NCard, NForm, NFormItem, NInputNumber, NSwitch, useMessage } from 'naive-ui'

import { getSiteRuntimeConfig, updateSiteRuntimeConfig } from '~/services/admin'
import type { SiteRuntimeConfig } from '~/types/api'
import { applySiteSectionPatch } from '~/utils/siteSectionSave'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
const config = ref({} as SiteRuntimeConfig)
const loaded = ref(false)

const OWNED_BLOCKS = ['chat'] as const

async function load() {
  loading.value = true
  try {
    config.value = await getSiteRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('私信配置加载失败')
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
    message.success('私信配置已保存')
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
    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="私信" size="small">
        <NAlert type="info" :bordered="false" class="mb-4">
          这里是站点级策略，对所有人统一生效。关闭后，未关注你、也不是你粉丝的用户
          无法给你发私信；已关注的熟人不受影响。限流按「发件人 → 收件人」分别计数，
          两个参数保存后立即生效，不需要重启服务。
        </NAlert>
        <NFormItem label="允许陌生人发私信">
          <NSwitch v-model:value="config.chat.stranger_msg_enabled" />
        </NFormItem>
        <NFormItem label="发送间隔（秒）">
          <NInputNumber
            v-model:value="config.chat.rate_window_sec"
            :min="1"
            :max="86400"
            :step="1"
            class="admin-field--number"
            data-role="site-chat-rate-window">
            <template #suffix>秒内只允许 1 条</template>
          </NInputNumber>
        </NFormItem>
        <NFormItem label="每日上限（条）">
          <NInputNumber
            v-model:value="config.chat.rate_daily_max"
            :min="1"
            :max="9999"
            :step="1"
            class="admin-field--number"
            data-role="site-chat-rate-daily">
            <template #suffix>条 / 自然日</template>
          </NInputNumber>
        </NFormItem>
      </NCard>

      <NButton type="primary" :loading="saving" @click="save">保存私信配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
