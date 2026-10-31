<script setup lang="ts">
import { NAlert, NButton, NForm, NFormItem, NInputNumber, NSwitch, useMessage } from 'naive-ui'
import { getDecoRuntimeConfig, updateDecoRuntimeConfig } from '~/services/admin'
import type { DecoRuntimeConfig } from '~/types/api'
import { DECO_NUMBER_PARAMS, defaultDecoConfig, normalizeDecoConfig } from '~/utils/decoParams'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化，整份模板都会报可能为 null。
const config = ref({} as DecoRuntimeConfig)
const loaded = ref(false)

/** 范围、单位、默认值、整数精度一律来自 utils/decoParams，表单不自己写数字。 */
const numberParams = DECO_NUMBER_PARAMS

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

/**
 * 「恢复默认」：把整张表单填回规范的九个默认值（开关 + 八个数值）。
 *
 * 刻意**不自动保存** —— 恢复默认只是把表单改成一个已知良好的起点，
 * 是否落库仍由管理员按「保存装饰配置」决定。这里只写本地 ref，不碰接口；
 * 组件测试按源码断言 resetDefaults 函数体里没有 updateDecoRuntimeConfig。
 */
function resetDefaults() {
  config.value = defaultDecoConfig()
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
      <!-- 八行数值输入由同一份元数据驱动：标签 / 单位 / 范围 / 步长 / 说明
           都在 utils/decoParams 里，加参数时只改那一个文件。 -->
      <NFormItem
        v-for="param in numberParams"
        :key="param.key"
        :label="param.label"
        :feedback="param.hint"
        :show-feedback="true"
      >
        <NInputNumber
          v-model:value="config.charm[param.key]"
          :min="param.min"
          :max="param.max"
          :step="param.step"
          :precision="param.precision"
          :disabled="!config.charm.enable"
          class="deco-number-input"
        >
          <template #suffix>
            <span class="deco-number-input__unit">{{ param.unit }}</span>
          </template>
        </NInputNumber>
      </NFormItem>
      <div class="deco-actions">
        <NButton type="primary" :loading="saving" @click="save">保存装饰配置</NButton>
        <!-- 只填表单、不保存：恢复默认之后管理员仍要自己按保存。 -->
        <NButton quaternary :disabled="saving" @click="resetDefaults">恢复默认</NButton>
      </div>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>

<style scoped>
/* 只给数值输入一个可读宽度：铺满整行会显得像文本框。 */
.deco-number-input {
  width: 200px;
}

/* 单位后缀走次级墨色，不抢输入框里的数字。 */
.deco-number-input__unit {
  color: var(--color-ink-muted);
  font-size: 12px;
}

/* 保存与「恢复默认」并排，用间距而不是靠两个按钮挤在一起。 */
.deco-actions {
  display: flex;
  gap: 12px;
}
</style>
