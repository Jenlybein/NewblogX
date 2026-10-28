<script setup lang="ts">
import { NAlert, NButton, NForm, NFormItem, NInput, NInputNumber, NSelect, NSpace, NSwitch, useMessage } from 'naive-ui'
import { getStorageRuntimeConfig, updateStorageRuntimeConfig } from '~/services/admin'
import type { StorageRuntimeConfig } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化（第一次双向写之后就不再窄化），整份模板都会报可能为 null。
const config = ref({} as StorageRuntimeConfig)
const loaded = ref(false)

// 七牛的区域代号由上传 SDK（qiniu-js）直接消费，必须是这几个固定值。
// 后端只是把它原样回传给前端，不参与校验——所以选错不会报错，只会传不动。
const regionOptions = [
  { label: '华东（z0）', value: 'z0' },
  { label: '华北（z1）', value: 'z1' },
  { label: '华南（z2）', value: 'z2' },
  { label: '北美（na0）', value: 'na0' },
  { label: '东南亚（as0）', value: 'as0' },
  { label: '华东-浙江2（cn-east-2）', value: 'cn-east-2' },
]

async function load() {
  loading.value = true
  try {
    config.value = await getStorageRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('对象存储配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await updateStorageRuntimeConfig(config.value)
    message.success('对象存储配置已保存，下一次上传即生效')
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
    <NAlert type="warning" :bordered="false" class="mb-4">
      这是图片上传的<strong>唯一</strong>落地方式：关掉之后上传接口直接返回「站点未启用图片上传」，
      不会退化成存到服务器本地磁盘（容器重建就丢）。
    </NAlert>
    <NAlert type="info" :bordered="false" class="mb-4">
      「对象 key 前缀」用来从图片 URL 反解出对象名，文章的图片引用关系靠它维护。
      改成别的前缀，已有图片的引用关系就对不上了。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NFormItem label="启用七牛云存储">
        <NSwitch v-model:value="config.enable" />
      </NFormItem>
      <NFormItem label="AccessKey">
        <NInput v-model:value="config.access_key" placeholder="七牛云密钥管理里的 AK" />
      </NFormItem>
      <NFormItem label="SecretKey">
        <NInput
          v-model:value="config.secret_key"
          type="password"
          show-password-on="click"
          placeholder="留 ****** 表示不修改"
        />
      </NFormItem>
      <NFormItem label="Bucket">
        <NInput v-model:value="config.bucket" placeholder="存储空间名称" />
      </NFormItem>
      <NFormItem label="访问域名">
        <NInput v-model:value="config.uri" placeholder="https://cdn.yourdomain.com（含协议头）" />
      </NFormItem>
      <NFormItem label="存储区域">
        <NSelect v-model:value="config.region" :options="regionOptions" />
      </NFormItem>
      <NFormItem label="对象 key 前缀">
        <NInput v-model:value="config.prefix" placeholder="myblogx" />
      </NFormItem>
      <NSpace :size="12" class="w-full">
        <NFormItem label="单图大小上限（MB）">
          <NInputNumber v-model:value="config.size" :min="1" :max="100" />
        </NFormItem>
        <NFormItem label="上传凭证有效期（秒）">
          <NInputNumber v-model:value="config.expiry" :min="60" />
        </NFormItem>
      </NSpace>
      <NFormItem label="上传回调地址（留空不启用）">
        <NInput v-model:value="config.callback_url" placeholder="https://api.yourdomain.com/..." />
      </NFormItem>
      <NButton type="primary" :loading="saving" @click="save">保存存储配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
