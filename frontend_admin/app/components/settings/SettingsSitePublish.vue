<script setup lang="ts">
// 发布与审核：发布/展示开关、审核开关、投稿配额、质量闸门、机审词表（ADR-0029 拆页后的第三页）。
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
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

const OWNED_BLOCKS = ['article', 'comment'] as const

// 词表在界面上是"一行一个词"的文本，配置里是字符串数组：转换只在这里做一次。
// 空行与首尾空白一律丢掉，运营从聊天窗口复制过来的词不会带进一个空格条目。
function wordListToText(list: string[] | undefined): string {
  return (list ?? []).join('\n')
}

function textToWordList(text: string): string[] {
  return text
    .split(/\r?\n/u)
    .map((word) => word.trim())
    .filter((word) => word !== '')
}

// setter 走可选赋值：旧库合并默认值之前 article.moderation 可能整块缺失，
// 直接写 config.value.article.moderation.block_words 会在编辑时抛异常（摘页前就有的坑）。
const blockWordsText = computed({
  get: () => wordListToText(config.value.article?.moderation?.block_words),
  set: (value: string) => {
    const article = config.value.article
    if (!article) return
    ;(article.moderation ??= { block_words: [], watch_words: [] }).block_words = textToWordList(value)
  },
})

const watchWordsText = computed({
  get: () => wordListToText(config.value.article?.moderation?.watch_words),
  set: (value: string) => {
    const article = config.value.article
    if (!article) return
    ;(article.moderation ??= { block_words: [], watch_words: [] }).watch_words = textToWordList(value)
  },
})

// 评论的词表（票 359）：形状与文章同构，但**各自独立**——评论是短文本，
// 适合拦的词与文章不同（比如"加微信"在文章里可能只是举例，在评论里就是导流）。
const commentBlockWordsText = computed({
  get: () => wordListToText(config.value.comment?.moderation?.block_words),
  set: (value: string) => {
    const comment = config.value.comment
    if (!comment) return
    ;(comment.moderation ??= { block_words: [], watch_words: [] }).block_words = textToWordList(value)
  },
})

const commentWatchWordsText = computed({
  get: () => wordListToText(config.value.comment?.moderation?.watch_words),
  set: (value: string) => {
    const comment = config.value.comment
    if (!comment) return
    ;(comment.moderation ??= { block_words: [], watch_words: [] }).watch_words = textToWordList(value)
  },
})

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
      这一页管「发布治理」：谁能发布、什么内容能过、投稿与评分的价格。五个卡片同属一段配置，
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
        <NFormItem label="评论免审核">
          <NSwitch v-model:value="config.comment.skip_examining" />
        </NFormItem>
        <NAlert type="info" :bordered="false" class="mb-4">
          评论的判定顺序（票 359）：**先词表、再 AI**。命中阻断词直接拒绝（不入库）；
          命中观察词强制留待人工（即使开了免审核）；两者都没命中且开了 AI 审核时，
          AI 判为干净才直发，可疑/违规/判定失败/超时一律留待人工。
        </NAlert>
      </NCard>

      <NCard class="admin-card mb-4" title="评论审核" size="small">
        <NAlert type="info" :bordered="false" class="mb-4">
          评论是短文本，词表口径与文章**分开维护**：一行一个词，空行忽略。
          阻断级命中即拒绝，观察级命中留待人工。
        </NAlert>
        <NFormItem label="评论阻断级词表">
          <NInput
            v-model:value="commentBlockWordsText"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 8 }"
            class="admin-field--long"
            placeholder="例如：加微信付款"
            data-role="site-comment-moderation-block-words" />
        </NFormItem>
        <NFormItem label="评论观察级词表">
          <NInput
            v-model:value="commentWatchWordsText"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 8 }"
            class="admin-field--long"
            placeholder="例如：加微信"
            data-role="site-comment-moderation-watch-words" />
        </NFormItem>
        <NFormItem label="启用 AI 审核">
          <NSwitch v-model:value="config.comment.ai.enabled" data-role="site-comment-ai-enabled" />
        </NFormItem>
        <NFormItem label="AI 输出上限（token）">
          <NInputNumber
            v-model:value="config.comment.ai.max_tokens"
            class="admin-field--number"
            :min="64"
            :max="4000"
            :step="50"
            data-role="site-comment-ai-max-tokens" />
        </NFormItem>
        <NFormItem label="AI 判定超时（秒）">
          <NInputNumber
            v-model:value="config.comment.ai.timeout_sec"
            class="admin-field--number"
            :min="1"
            :max="60"
            data-role="site-comment-ai-timeout" />
        </NFormItem>
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

      <!-- 机审前两层的高危词表（票 311）。词表是运行期数据，保存后下一次读取立即按新词表判。 -->
      <NCard class="admin-card mb-4" title="机审词表" size="small">
        <NAlert type="info" :bordered="false" class="mb-4">
          一行一个词，空行忽略。「阻断级」命中会直接拒绝投稿；「观察级」命中也会拒，
          分档只为让审核列表能分清命中了哪类词。词表只做字符串匹配：英文不区分大小写，
          中文按字面命中，短词可能出现在更长的词里（已知误报，靠分档与人工兜底）。
        </NAlert>
        <NFormItem label="阻断级词表">
          <NInput
            v-model:value="blockWordsText"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 8 }"
            class="admin-field--long"
            placeholder="例如：代开发票"
            data-role="site-moderation-block-words" />
        </NFormItem>
        <NFormItem label="观察级词表">
          <NInput
            v-model:value="watchWordsText"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 8 }"
            class="admin-field--long"
            placeholder="例如：加微信"
            data-role="site-moderation-watch-words" />
        </NFormItem>
      </NCard>

      <NButton type="primary" :loading="saving" @click="save">保存发布与审核配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
