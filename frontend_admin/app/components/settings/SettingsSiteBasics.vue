<script setup lang="ts">
// 站点基础：备案号、SEO 与 AI 总闸（ADR-0029 拆页后的第一页）。
//
// 2026-11-28：AI 总闸（`ai.enable`）从「AI 助手」页迁到这里（ADR-0037 / D16）。
// 它管的是「服务端能不能调模型」，而 AI 助手页只管右下角那个浮窗 —— 两个决定，
// 住在两个页，数据不联动。
//
// 与其它三个站点子页共用 `site` 段的一对整段端点，所以保存走
// 「取最新整段 → 只覆盖本页的 site_info/seo → 写回」（utils/siteSectionSave）。
import { NAlert, NButton, NCard, NForm, NFormItem, NInput, NSwitch, useMessage } from 'naive-ui'

import {
  getAiRuntimeConfig,
  getSiteRuntimeConfig,
  updateAiRuntimeConfig,
  updateSiteRuntimeConfig,
} from '~/services/admin'
import type { AIRuntimeConfig, SiteRuntimeConfig } from '~/types/api'
import { applySiteSectionPatch } from '~/utils/siteSectionSave'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
const config = ref({} as SiteRuntimeConfig)
const loaded = ref(false)
// AI 总闸住在 ai 段（`PUT /site/ai`），不在 site 段里：单独读一份、单独存一次。
const aiConfig = ref({} as AIRuntimeConfig)

// 本页拥有的顶层块：只有它们会被覆盖，其余块（含 about / project / reconcile）原样往返。
const OWNED_BLOCKS = ['site_info', 'seo'] as const

async function load() {
  loading.value = true
  try {
    const [site, ai] = await Promise.all([getSiteRuntimeConfig(), getAiRuntimeConfig()])
    config.value = site
    aiConfig.value = ai
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
    // 两段各写各的：site 段（备案号 / SEO）与 ai 段（总闸）。
    // ai 段同样走 read-modify-write：那上面还有模型配置、额度、场景绑定，
    // 拿本页的旧快照整段写回去会把别的页刚改的模型列表清空。
    const latestAi = await getAiRuntimeConfig()
    await Promise.all([
      updateSiteRuntimeConfig(merged),
      updateAiRuntimeConfig({ ...latestAi, enable: aiConfig.value.enable }),
    ])
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

      <!-- AI 总闸（2026-11-28 从「AI 助手」页迁来，ADR-0037 / D16）：
           它管的是「服务端能不能调模型」，与右下角浮窗是两个独立决定。 -->
      <NCard class="admin-card mb-4" title="AI 总闸" size="small">
        <NFormItem label="启用 AI 能力（服务端可调用模型）">
          <NSwitch v-model:value="aiConfig.enable" />
        </NFormItem>
        <NAlert type="info" :bordered="false" class="mb-4">
          关掉之后服务端<strong>不再调用任何模型</strong>：文章质量评分、AI 合规判定、
          摘要、搜索向量全部不可用。读者站右下角的助手浮窗由「AI 助手」页的独立开关
          控制，两者不联动。
        </NAlert>
        <NAlert v-if="!aiConfig.enable" type="warning" :bordered="false">
          AI 总闸已关闭。此时「文章审核设置」里的质量闸门与 AI 合规判定都失去判据：
          闸门保存会被拒绝，合规判定会走「AI 不可用落点」。
        </NAlert>
      </NCard>

      <NButton type="primary" :loading="saving" @click="save">保存站点基础配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
