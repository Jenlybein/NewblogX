<script setup lang="ts">
import { NAlert, NButton, NForm, NFormItem, NInputNumber, NSwitch, useMessage } from 'naive-ui'
import { getDecoRuntimeConfig, updateDecoRuntimeConfig } from '~/services/admin'
import type { DecoRuntimeConfig } from '~/types/api'
import { DECO_LOGO_SIZE_PARAM, normalizeDecoConfig } from '~/utils/decoParams'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化，整份模板都会报可能为 null。
const config = ref({} as DecoRuntimeConfig)
const loaded = ref(false)

/** 范围、单位、默认值一律来自 utils/decoParams，表单不自己写数字。 */
const logoSize = DECO_LOGO_SIZE_PARAM

async function load() {
  loading.value = true
  try {
    // 读回来也走一次归一化：旧库里可能存着越界值（钳位是后加的），
    // 直接回填会让管理员看到一个不该存在的数字。
    config.value = normalizeDecoConfig(await getDecoRuntimeConfig())
    loaded.value = true
  } catch {
    message.error('装饰配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    // 提交前再钳一次：NInputNumber 的 min/max 只是 UI 约束，粘贴与 devtools 都能绕过。
    await updateDecoRuntimeConfig(normalizeDecoConfig(config.value))
    message.success('装饰配置已保存')
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
      装饰只影响页面看起来怎样，不影响任何功能。光标挂件是一条挂绳把站标系在指针上：
      它不响应点击、不进 Tab 序列、读屏读不到，只会在有精确指针的桌面设备上出现，
      系统开启「减少动态」时也不会出现。
    </NAlert>
    <NAlert type="warning" :bordered="false" class="mb-4">
      关掉总开关后，读者站不再挂载这一层，也不再加载站标素材；改完参数保存后，
      读者**下一次页面请求**就能看到，不用重新构建镜像。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NFormItem label="启用光标挂件">
        <NSwitch v-model:value="config.charm.enable" />
      </NFormItem>
      <NFormItem
        :label="logoSize.label"
        :feedback="logoSize.hint"
        :show-feedback="true"
      >
        <NInputNumber
          v-model:value="config.charm.logo_size"
          :min="logoSize.min"
          :max="logoSize.max"
          :step="logoSize.step"
          :disabled="!config.charm.enable"
          class="deco-size-input"
        >
          <template #suffix>
            <span class="deco-size-input__unit">{{ logoSize.unit }}</span>
          </template>
        </NInputNumber>
      </NFormItem>
      <NButton type="primary" :loading="saving" @click="save">保存装饰配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>

<style scoped>
/* 只给尺寸输入一个可读宽度：数值框铺满整行会显得像文本框。 */
.deco-size-input {
  width: 160px;
}

/* 单位后缀走次级墨色，不抢输入框里的数字。 */
.deco-size-input__unit {
  color: var(--color-ink-muted);
  font-size: 12px;
}
</style>
