<script setup lang="ts">
import { NAlert, NButton, NDynamicTags, NForm, NFormItem, useMessage } from 'naive-ui'
import { getUploadRuntimeConfig, updateUploadRuntimeConfig } from '~/services/admin'
import type { UploadRuntimeConfig } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化（第一次双向写之后就不再窄化），整份模板都会报可能为 null。
const config = ref({} as UploadRuntimeConfig)
const loaded = ref(false)

async function load() {
  loading.value = true
  try {
    config.value = await getUploadRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('上传策略加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await updateUploadRuntimeConfig(config.value)
    message.success('上传策略已保存')
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
      白名单是<strong>整体替换</strong>，不是追加：删掉某一项再保存，该格式就立刻不能再上传。
      填写时不要带点（写 <code>jpg</code>，不要写 <code>.jpg</code>），大小写不敏感。
    </NAlert>
    <NAlert type="warning" :bordered="false" class="mb-4">
      移除一个已有图片在用的格式不会让旧图失效，但用户就再也传不上同类图片了。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NFormItem label="允许上传的图片格式">
        <NDynamicTags v-model:value="config.whitelist" />
      </NFormItem>
      <NButton type="primary" :loading="saving" @click="save">保存上传策略</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
