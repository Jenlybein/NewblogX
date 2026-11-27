<script setup lang="ts">
import { NAlert, NButton, NCard, NForm, NFormItem, NInput, NInputNumber, NSelect, NSpace, NSwitch, useMessage } from 'naive-ui'
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
  <div class="admin-settings-panel">
    <NAlert type="warning" :bordered="false" class="mb-4">
      腾讯云 SES 只支持<strong>模板发信</strong>：正文由控制台审核过的模板渲染，
      这里只填模板 ID。模板变量为
      <code>action</code> / <code>code</code> / <code>timeout</code>，
      四个场景（注册、重置密码、绑定邮箱、邮箱登录）共用同一个模板。
    </NAlert>
    <NAlert type="info" :bordered="false" class="mb-4">
      本页的「邮箱验证码登录」开关决定读者站显示不显示这个入口：密钥没填完就打开，
      用户点「获取验证码」会直接报错。发码频率也在本页（票 481 把这三类策略按通道拆开）。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="邮箱登录" size="small">
        <!--
          开关跟着通道走（票 481）：以前它们住在 site.login，改一个邮箱入口要先跑到
          「站点配置 → 登录与发码」，而发码限流又在同一页的另一张卡里 —— 改邮件通道
          要横跨两个配置段、两个页面。
        -->
        <NFormItem label="邮箱验证码登录">
          <NSwitch v-model:value="config.email_login" data-role="mail-email-login" />
        </NFormItem>
        <!--
          邮箱注册与邮箱登录分开：绑定邮箱要用发码接口，混成一个开关会连带把
          「给已注册用户发绑定验证码」一起关掉。本站点默认注册只走手机号。
        -->
        <NFormItem label="邮箱注册（默认关闭：注册只走手机号）">
          <NSwitch v-model:value="config.email_register" data-role="mail-email-register" />
        </NFormItem>
      </NCard>

      <NFormItem label="SecretId">
        <NInput v-model:value="config.secret_id" class="admin-field--secret" placeholder="腾讯云 API 密钥 ID" />
      </NFormItem>
      <NFormItem label="SecretKey">
        <NInput
          v-model:value="config.secret_key" class="admin-field--secret"
          type="password"
          show-password-on="click"
          placeholder="留 ****** 表示不修改"
        />
      </NFormItem>
      <NFormItem label="地域">
        <NSelect v-model:value="config.region" class="admin-field--select" :options="regionOptions" />
      </NFormItem>
      <NFormItem label="Endpoint（国际站才需要改）">
        <NInput v-model:value="config.endpoint" class="admin-field--url" placeholder="留空用默认 ses.tencentcloudapi.com" />
      </NFormItem>
      <NFormItem label="发信地址">
        <NInput v-model:value="config.from_address" class="admin-field--url" placeholder="noreply@yourdomain.com（必须已在控制台验证）" />
      </NFormItem>
      <NFormItem label="发件人显示名">
        <NInput v-model:value="config.from_name" class="admin-field--key" placeholder="技术博客" />
      </NFormItem>
      <NFormItem label="验证码模板 ID">
        <NInputNumber v-model:value="config.template_id" class="admin-field--number" :min="0" placeholder="控制台里的数字 ID" />
      </NFormItem>
      <NFormItem label="单次调用超时（秒）">
        <NInputNumber v-model:value="config.timeout_sec" class="admin-field--number" :min="1" :max="60" />
      </NFormItem>
      <NCard class="admin-card mb-4" title="发码限流" size="small">
        <NAlert type="info" :bordered="false" class="mb-3">
          邮件与短信分开计数：短信成本远高于邮件，两条通道的滥用方式也不同，配额互不占用。
        </NAlert>
        <NSpace :size="12" class="w-full">
          <NFormItem label="验证码有效期（分钟）">
            <NInputNumber v-model:value="config.email_code_timeout" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="发码限流窗口（秒）">
            <NInputNumber v-model:value="config.email_send_window_second" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单邮箱窗口内最多发码">
            <NInputNumber v-model:value="config.email_send_per_email_max" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单 IP 窗口内最多发码">
            <NInputNumber v-model:value="config.email_send_per_ip_max" class="admin-field--number" :min="1" />
          </NFormItem>
        </NSpace>
      </NCard>

      <NButton type="primary" :loading="saving" @click="save">保存邮件配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>

