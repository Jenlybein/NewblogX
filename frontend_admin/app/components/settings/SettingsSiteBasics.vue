<script setup lang="ts">
// 站点基础：备案号与 SEO（ADR-0029 拆页后的第一页）。
//
// 与其它三个站点子页共用 `site` 段的一对整段端点，所以保存走
// 「取最新整段 → 只覆盖本页的 site_info/seo → 写回」（utils/siteSectionSave）。
import { NAlert, NButton, NCard, NForm, NFormItem, NInput, useMessage } from 'naive-ui'

import { getSiteRuntimeConfig, updateSiteRuntimeConfig } from '~/services/admin'
import type { SiteRuntimeConfig } from '~/types/api'
import { applySiteSectionPatch } from '~/utils/siteSectionSave'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
const config = ref({} as SiteRuntimeConfig)
const loaded = ref(false)

// 本页拥有的顶层块：只有它们会被覆盖，其余块（含 about / project / reconcile）原样往返。
const OWNED_BLOCKS = ['site_info', 'seo'] as const

async function load() {
  loading.value = true
  try {
    config.value = await getSiteRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('站点基础配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    // 读-改-写：先取最新整段，否则会把其它页刚保存的字段覆盖回去。
    const latest = await getSiteRuntimeConfig()
    const merged = applySiteSectionPatch(latest, config.value, OWNED_BLOCKS)
    await updateSiteRuntimeConfig(merged)
    message.success('站点基础配置已保存')
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
      这一页只改「备案号」与「SEO」两组字段；保存时不会动站点配置的其它块
      （登录策略、发布治理、私信、以及后台没有编辑入口的「关于 / 站点项目 / 并账周期」）。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="基础信息" size="small">
        <NFormItem label="备案号">
          <NInput v-model:value="config.site_info.beian" class="admin-field--key" />
        </NFormItem>
      </NCard>

      <NCard class="admin-card mb-4" title="SEO" size="small">
        <NFormItem label="SEO 关键词">
          <NInput v-model:value="config.seo.keywords" class="admin-field--long" placeholder="多个关键词用英文逗号分隔" />
        </NFormItem>
        <NFormItem label="SEO 描述">
          <NInput
            v-model:value="config.seo.description"
            class="admin-field--long"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 5 }" />
        </NFormItem>
      </NCard>

      <NButton type="primary" :loading="saving" @click="save">保存站点基础配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
