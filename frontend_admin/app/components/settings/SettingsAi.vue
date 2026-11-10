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
    message.success('AI 助手配置已保存')
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
      这里有两层开关：「启用 AI 助手」是整站总闸；下方「模型配置」里<strong>启用</strong>的那一条
      决定实际用哪家模型。总闸关掉时，即使有启用的模型配置也不可用。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NFormItem label="启用 AI 助手">
        <NSwitch v-model:value="config.enable" />
      </NFormItem>
      <NFormItem label="助手昵称">
        <NInput v-model:value="config.nickname" class="admin-field--key" />
      </NFormItem>
      <NFormItem label="助手头像 URL">
        <NInput v-model:value="config.avatar" class="admin-field--url" />
      </NFormItem>
      <NFormItem label="助手简介">
        <NInput v-model:value="config.abstract" class="admin-field--long" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" />
      </NFormItem>
      <NSpace :size="12" class="w-full">
        <NFormItem label="请求超时（秒）">
          <NInputNumber v-model:value="config.timeout_sec" class="admin-field--number" :min="1" :max="600" />
        </NFormItem>
        <NFormItem label="最大输入字符">
          <NInputNumber v-model:value="config.max_input_chars" class="admin-field--number" :min="100" />
        </NFormItem>
        <NFormItem label="采样温度">
          <NInputNumber v-model:value="config.temperature" class="admin-field--number" :min="0" :max="2" :step="0.1" />
        </NFormItem>
      </NSpace>

      <!-- 每日额度按场景分设（ADR-0038）：一次 30 秒的评分和一次两行的改写
           成本差两个数量级，共用一个预算只会让运营在无感的地方先撞墙。
           0 表示不限；负数与超过 9999 的值保存时会被拒绝。 -->
      <NDivider />
      <NAlert type="info" :bordered="false" class="mb-4">
        每人每自然日的调用上限，<strong>0 表示不限</strong>。计量口径是「一次模型调用算一次」，
        唯一例外是评分：一次评分动作只算一次（长文按段落分块，内部多次调用不重复扣）。
        AI 搜索一次问答会各消耗「改写」与「作答」各一次；管理员不受这些上限约束。
      </NAlert>
      <NSpace :size="12" class="w-full">
        <NFormItem label="评分（投稿闸门 / 主动重评）">
          <NInputNumber v-model:value="config.daily_limits.scoring" class="admin-field--number" :min="0" :max="9999" />
        </NFormItem>
        <NFormItem label="元信息（标题/摘要/标签）">
          <NInputNumber v-model:value="config.daily_limits.metainfo" class="admin-field--number" :min="0" :max="9999" />
        </NFormItem>
        <NFormItem label="改写（选区改写）">
          <NInputNumber v-model:value="config.daily_limits.overwrite" class="admin-field--number" :min="0" :max="9999" />
        </NFormItem>
        <NFormItem label="诊断（选区诊断）">
          <NInputNumber v-model:value="config.daily_limits.diagnose" class="admin-field--number" :min="0" :max="9999" />
        </NFormItem>
        <NFormItem label="搜索改写（检索意图）">
          <NInputNumber v-model:value="config.daily_limits.search_rewrite" class="admin-field--number" :min="0" :max="9999" />
        </NFormItem>
        <NFormItem label="搜索作答（检索结果回答）">
          <NInputNumber v-model:value="config.daily_limits.search_answer" class="admin-field--number" :min="0" :max="9999" />
        </NFormItem>
        <NFormItem label="合规判定（异步机审）">
          <NInputNumber v-model:value="config.daily_limits.moderation" class="admin-field--number" :min="0" :max="9999" />
        </NFormItem>
      </NSpace>

      <NButton type="primary" :loading="saving" @click="save">保存 AI 助手配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>

    <NDivider />

    <AiProviderList />
  </div>
</template>
