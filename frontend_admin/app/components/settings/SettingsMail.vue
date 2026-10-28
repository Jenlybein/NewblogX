<script setup lang="ts">
import { NAlert, NButton, NForm, NFormItem, NInput, NInputNumber, NSelect, useMessage } from 'naive-ui'
import { getMailRuntimeConfig, updateMailRuntimeConfig } from '~/services/admin'
import type { MailRuntimeConfig } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化（第一次双向写之后就不再窄化），整份模板都会报可能为 null。
const config = ref({} as MailRuntimeConfig)
const loaded = ref(false)

// 官方只支持这两个地域。做下拉而不是自由输入：
// 填错地域只会拿到一句莫名其妙的 API 错误，而这是能提前避免的。
const regionOptions = [
  { label: '广州（ap-guangzhou）', value: 'ap-guangzhou' },
  { label: '中国香港（ap-hongkong）', value: 'ap-hongkong' },
]

async function load() {
  loading.value = true
  try {
    config.value = await getMailRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('邮件配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await updateMailRuntimeConfig(config.value)
    message.success('邮件配置已保存，下一次发信即生效')
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
      腾讯云 SES 只支持<strong>模板发信</strong>：正文由控制台审核过的模板渲染，
      这里只填模板 ID。模板变量为
      <code>action</code> / <code>code</code> / <code>timeout</code>，
      四个场景（注册、重置密码、绑定邮箱、邮箱登录）共用同一个模板。
    </NAlert>
    <NAlert type="info" :bordered="false" class="mb-4">
      填完密钥后，还需要去「站点 → 登录」把「邮箱验证码登录」打开，读者站才会显示入口。
      未填完就打开开关，用户点「获取验证码」会直接报错。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NFormItem label="SecretId">
        <NInput v-model:value="config.secret_id" placeholder="腾讯云 API 密钥 ID" />
      </NFormItem>
      <NFormItem label="SecretKey">
        <NInput
          v-model:value="config.secret_key"
          type="password"
          show-password-on="click"
          placeholder="留 ****** 表示不修改"
        />
      </NFormItem>
      <NFormItem label="地域">
        <NSelect v-model:value="config.region" :options="regionOptions" />
      </NFormItem>
      <NFormItem label="Endpoint（国际站才需要改）">
        <NInput v-model:value="config.endpoint" placeholder="留空用默认 ses.tencentcloudapi.com" />
      </NFormItem>
      <NFormItem label="发信地址">
        <NInput v-model:value="config.from_address" placeholder="noreply@yourdomain.com（必须已在控制台验证）" />
      </NFormItem>
      <NFormItem label="发件人显示名">
        <NInput v-model:value="config.from_name" placeholder="技术博客" />
      </NFormItem>
      <NFormItem label="验证码模板 ID">
        <NInputNumber v-model:value="config.template_id" :min="0" placeholder="控制台里的数字 ID" />
      </NFormItem>
      <NFormItem label="单次调用超时（秒）">
        <NInputNumber v-model:value="config.timeout_sec" :min="1" :max="60" />
      </NFormItem>
      <NButton type="primary" :loading="saving" @click="save">保存邮件配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
