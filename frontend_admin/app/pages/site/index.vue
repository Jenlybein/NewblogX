<script setup lang="ts">
/**
 * 站点设置。
 *
 * 八块配置各占一个 Tab，每块独立读写自己的配置段（`/site/admin/{name}` +
 * `PUT /site/{name}`）。这样做的原因是后端把八段分开存（一条记录一段 JSON），
 * 前端也按段保存，改邮件配置不会连带重写站点配置。
 *
 * 唯一的例外是「登录策略」——它属于 site 段（`site.login`），
 * 所以和站点基础信息放在同一个 Tab 里、共用一次保存，
 * 否则两个组件各自提交整段 site 会互相覆盖。
 */
import type { Component } from 'vue'
import { NCard, NTabPane, NTabs } from 'naive-ui'
import SettingsAi from '~/components/settings/SettingsAi.vue'
import SettingsDeco from '~/components/settings/SettingsDeco.vue'
import SettingsLog from '~/components/settings/SettingsLog.vue'
import SettingsMail from '~/components/settings/SettingsMail.vue'
import SettingsOAuth from '~/components/settings/SettingsOAuth.vue'
import SettingsSite from '~/components/settings/SettingsSite.vue'
import SettingsStorage from '~/components/settings/SettingsStorage.vue'
import SettingsUpload from '~/components/settings/SettingsUpload.vue'

const tabs: Array<{ key: string; label: string; component: Component }> = [
  { key: 'site', label: '站点', component: SettingsSite },
  { key: 'ai', label: 'AI 助手', component: SettingsAi },
  { key: 'mail', label: '邮件', component: SettingsMail },
  { key: 'storage', label: '对象存储', component: SettingsStorage },
  { key: 'oauth', label: '第三方登录', component: SettingsOAuth },
  { key: 'log', label: '日志', component: SettingsLog },
  { key: 'upload', label: '上传策略', component: SettingsUpload },
  { key: 'deco', label: '装饰', component: SettingsDeco },
]

const active = ref('site')
</script>

<template>
  <NCard class="admin-card">
    <NTabs v-model:value="active" type="line" animated>
      <NTabPane v-for="tab in tabs" :key="tab.key" :name="tab.key" :tab="tab.label">
        <component :is="tab.component" />
      </NTabPane>
    </NTabs>
  </NCard>
</template>
