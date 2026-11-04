<script setup lang="ts">
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSpace,
  NSwitch,
  useMessage,
} from 'naive-ui'
import { getSiteRuntimeConfig, updateSiteRuntimeConfig } from '~/services/admin'
import type { SiteRuntimeConfig } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化（第一次双向写之后就不再窄化），整份模板都会报可能为 null。
const config = ref({} as SiteRuntimeConfig)
const loaded = ref(false)

async function load() {
  loading.value = true
  try {
    config.value = await getSiteRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('站点配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await updateSiteRuntimeConfig(config.value)
    message.success('站点配置已保存')
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
    <!-- 登录策略单独一块，但和基础信息同一个表单：它属于 site 段，
         拆成两个组件各自读写整段，会互相把对方刚存的值覆盖回去。 -->
    <NAlert type="info" :bordered="false" class="mb-4">
      下面五组设置（基础信息 / 登录策略 / 发布与展示 / 内容审核 / 私信）属于同一段配置，
      改完点一次「保存站点配置」即可，不需要分别保存。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="基础信息" size="small">
        <!-- 站点标题与 Logo 不再可自定义（票 192）：读者站的页头字标、浏览器标题与
             分享卡片标题都来自品牌常量，favicon 走项目图标。这里只留备案号。 -->
        <NFormItem label="备案号">
          <NInput v-model:value="config.site_info.beian" class="admin-field--key" />
        </NFormItem>
        <NFormItem label="SEO 关键词">
          <NInput v-model:value="config.seo.keywords" class="admin-field--long" placeholder="多个关键词用英文逗号分隔" />
        </NFormItem>
        <NFormItem label="SEO 描述">
          <NInput v-model:value="config.seo.description" class="admin-field--long" type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" />
        </NFormItem>
      </NCard>

      <NCard class="admin-card mb-4" title="登录策略" size="small">
        <NAlert type="warning" :bordered="false" class="mb-4">
          「邮箱验证码登录」打开前，必须先在「邮件」页把腾讯云 SES 配完。
          没配就打开，用户点「获取验证码」会直接报错。
        </NAlert>
        <NFormItem label="用户名密码登录">
          <NSwitch v-model:value="config.login.username_pwd_login" />
        </NFormItem>
        <NFormItem label="邮箱验证码登录">
          <NSwitch v-model:value="config.login.email_login" />
        </NFormItem>
        <NFormItem label="QQ 登录">
          <NSwitch v-model:value="config.login.qq_login" />
        </NFormItem>
        <NFormItem label="登录与发码接口启用图形验证码">
          <NSwitch v-model:value="config.login.captcha" />
        </NFormItem>
        <NSpace :size="12" class="w-full">
          <NFormItem label="验证码有效期（分钟）">
            <NInputNumber v-model:value="config.login.email_code_timeout" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="发码限流窗口（秒）">
            <NInputNumber v-model:value="config.login.email_send_window_second" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单邮箱窗口内最多发码">
            <NInputNumber v-model:value="config.login.email_send_per_email_max" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单 IP 窗口内最多发码">
            <NInputNumber v-model:value="config.login.email_send_per_ip_max" class="admin-field--number" :min="1" />
          </NFormItem>
        </NSpace>
        <NSpace :size="12" class="w-full">
          <NFormItem label="登录失败统计窗口（分钟）">
            <NInputNumber v-model:value="config.login.login_fail_window_minute" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单账号窗口内最多失败">
            <NInputNumber v-model:value="config.login.login_fail_user_max" class="admin-field--number" :min="1" />
          </NFormItem>
          <NFormItem label="单 IP 窗口内最多失败">
            <NInputNumber v-model:value="config.login.login_fail_ip_max" class="admin-field--number" :min="1" />
          </NFormItem>
        </NSpace>
      </NCard>

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
      </NCard>

      <!-- 私信的站点级策略（票 202 / ADR-0026）。保存后立即生效：后端每次发送都现读
           运行时配置缓存，所以不用重启、也不用等下一次请求。 -->
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

      <NButton type="primary" :loading="saving" @click="save">保存站点配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
