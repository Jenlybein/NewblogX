<script setup lang="ts">
// 发布与审核：发布/展示开关、审核开关、投稿配额、质量闸门（ADR-0029 拆页后的第三页）。
//
// 词表（阻断级 / 观察级）已随「文章词表机审退场」摘除（票 401 / ADR-0032 决策 1）：
// 命中词在落库前被全站内容过滤替换成 `*`，词表只归「站点 → 内容过滤」一页维护。
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInputNumber,
  NSwitch,
  useMessage,
} from 'naive-ui'

import { getSiteRuntimeConfig, updateSiteRuntimeConfig } from '~/services/admin'
import type { QualityGateConfig, SiteRuntimeConfig } from '~/types/api'
import { applySiteSectionPatch } from '~/utils/siteSectionSave'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
const config = ref({} as SiteRuntimeConfig)
const loaded = ref(false)

const OWNED_BLOCKS = ['article'] as const

// 闸门默认值与后端 site_default_settings.yaml 一致：管理后台只负责改，
// 不发明第二套默认值。实际放行线 = pass_score + gray_band_width。
const DEFAULT_QUALITY_GATE: QualityGateConfig = {
  enabled: false,
  pass_score: 50,
  gray_band_width: 2,
  band_poor_max: 30,
  band_unqualified_max: 59,
  band_normal_max: 80,
  band_good_max: 89,
}

const qualityGate = ref<QualityGateConfig>({ ...DEFAULT_QUALITY_GATE })
const submitDailyMax = ref(15)

// 实际放行线在界面上直接算给运营看：只写"及格线"会让人以为 50 分就能过。
const publishLine = computed(() => qualityGate.value.pass_score + qualityGate.value.gray_band_width)

async function load() {
  loading.value = true
  try {
    config.value = await getSiteRuntimeConfig()
    // 闸门与配额是后加的配置段：老库合并默认值之前这一段可能整块缺失，
    // 因此显式兜一层，表单不会因为 undefined 整页炸掉。
    qualityGate.value = { ...DEFAULT_QUALITY_GATE, ...(config.value.article?.quality_gate ?? {}) }
    submitDailyMax.value = config.value.article?.submit_daily_max ?? 15
    loaded.value = true
  } catch {
    message.error('发布与审核配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    // 两个本地 ref 在提交前写回配置：读取时兜的默认值不能只活在前端。
    if (config.value.article) {
      config.value.article.quality_gate = { ...qualityGate.value }
      config.value.article.submit_daily_max = submitDailyMax.value
    }
    const latest = await getSiteRuntimeConfig()
    const merged = applySiteSectionPatch(latest, config.value, OWNED_BLOCKS)
    await updateSiteRuntimeConfig(merged)
    message.success('发布与审核配置已保存')
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
      这一页管「发布治理」：谁能发布、什么内容能过、投稿与评分的价格。四个卡片同属一段配置，
      改完点一次保存即可。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="发布与展示" size="small">
        <NAlert type="info" :bordered="false" class="mb-4">
          这三个开关只管普通用户（非管理员）。两个发布/草稿开关都关掉时，
          普通用户只能查看和删除自己的存量草稿，不能新建、不能保存、不能发布。
        </NAlert>
        <NFormItem label="允许非管理员发布文章">
          <NSwitch v-model:value="config.article.allow_non_admin_publish" />
        </NFormItem>
        <NFormItem label="允许非管理员创建草稿">
          <NSwitch v-model:value="config.article.allow_non_admin_draft" />
        </NFormItem>
        <NFormItem label="显示非管理员发布文章">
          <NSwitch v-model:value="config.article.show_non_admin_articles" />
        </NFormItem>
      </NCard>

      <NCard class="admin-card mb-4" title="内容审核" size="small">
        <NFormItem label="文章免审核">
          <NSwitch v-model:value="config.article.skip_examining" />
        </NFormItem>
        <NAlert type="info" :bordered="false" class="mb-4">
          评论不再有独立审核（票 400）：评论一律发布，命中词由全站内容过滤替换成星号
          并进「审核 → 关键词风险」。文章那条链路的判定顺序保持不变。
        </NAlert>
      </NCard>

      <NCard class="admin-card mb-4" title="投稿配额" size="small">
        <NAlert type="info" :bordered="false" class="mb-4">
          每个普通用户每自然日可提交多少次。计量的是**提交动作**：被打回也计一次
          （每次都真的产生一条审核任务）；因系统不可用而失败的提交不占额度；管理员不受此上限约束。
        </NAlert>
        <NFormItem label="每日投稿上限">
          <NInputNumber v-model:value="submitDailyMax" class="admin-field--number" :min="1" :max="9999" />
        </NFormItem>
      </NCard>

      <!-- 发布质量闸门（票 306）：开启后提交前先评分，不达标不放行。
           ADR-0023 起是**木桶**判定：总分与六个维度都要过线。 -->
      <NCard class="admin-card mb-4" title="发布质量闸门" size="small">
        <NAlert type="warning" :bordered="false" class="mb-4">
          开启前必须先让 AI 可用（总闸打开且至少配置了一条启用模型），否则保存会被拒绝——
          闸门失败关闭，叠上 AI 不可用就等于"所有投稿都被拒"。
        </NAlert>
        <NAlert type="info" :bordered="false" class="mb-4">
          判定是**木桶**：总分要达到 {{ publishLine }} 分（及格线 + 灰带宽度），
          并且六个维度（清晰度 / 结构性 / 完整度 / 可读性 / 说服力 / 语言规范）
          **每一项**都要达到及格线 {{ qualityGate.pass_score }} 分。
          任何一项不够都不能发布，总分再高也不行——否则长板会把短板抵消掉，
          两行话也能凑出及格的加权分。
        </NAlert>
        <NFormItem label="开启质量闸门">
          <NSwitch v-model:value="qualityGate.enabled" />
        </NFormItem>
        <NFormItem label="及格线（同时是每个维度的下限）">
          <NInputNumber v-model:value="qualityGate.pass_score" class="admin-field--number" :min="0" :max="100" />
        </NFormItem>
        <NFormItem label="灰带宽度">
          <NInputNumber v-model:value="qualityGate.gray_band_width" class="admin-field--number" :min="0" :max="20" />
        </NFormItem>
        <NFormItem label="档界：较差上限">
          <NInputNumber v-model:value="qualityGate.band_poor_max" class="admin-field--number" :min="0" :max="100" />
        </NFormItem>
        <NFormItem label="档界：不合格上限">
          <NInputNumber v-model:value="qualityGate.band_unqualified_max" class="admin-field--number" :min="0" :max="100" />
        </NFormItem>
        <NFormItem label="档界：常规上限">
          <NInputNumber v-model:value="qualityGate.band_normal_max" class="admin-field--number" :min="0" :max="100" />
        </NFormItem>
        <NFormItem label="档界：优质上限">
          <NInputNumber v-model:value="qualityGate.band_good_max" class="admin-field--number" :min="0" :max="100" />
        </NFormItem>
      </NCard>

      <NButton type="primary" :loading="saving" @click="save">保存发布与审核配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
