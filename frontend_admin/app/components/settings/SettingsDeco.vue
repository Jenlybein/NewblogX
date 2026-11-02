<script setup lang="ts">
import {
  NAlert,
  NButton,
  NForm,
  NFormItem,
  NInputNumber,
  NSlider,
  NSwitch,
  useMessage,
} from 'naive-ui'
import { getDecoRuntimeConfig, updateDecoRuntimeConfig } from '~/services/admin'
import type { DecoRuntimeConfig } from '~/types/api'
import {
  DECO_NUMBER_PARAMS,
  clampDecoParam,
  defaultDecoConfig,
  interpretDecoParamInput,
  normalizeDecoConfig,
} from '~/utils/decoParams'

/** 元数据数组的元素类型：`key` 是字面量联合，能直接索引 `DecoCharmConfig`。 */
type DecoNumberParam = (typeof DECO_NUMBER_PARAMS)[number]

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化，整份模板都会报可能为 null。
const config = ref({} as DecoRuntimeConfig)
const loaded = ref(false)

/**
 * 每个参数最近一次「数字被改过」的可见反馈（键是参数 key）。
 *
 * 滑块本身只会吐出区间内的值，所以它不产生提示；数字框可以被粘贴、被
 * devtools 改、被旧版本页面填上越界值，夹回/回落时必须让管理员当场看见。
 */
const clampNotices = ref<Record<string, string>>({})

/** 范围、单位、默认值、整数精度一律来自 utils/decoParams，表单不自己写数字。 */
const numberParams = DECO_NUMBER_PARAMS

function setClampNotice(key: string, notice: string | null) {
  // 用「重建一份不含该键的对象」代替 delete：动态 delete 被 lint 规则禁止，
  // 而整体替换 ref 的 value 也能稳定触发依赖更新。
  const next = Object.fromEntries(
    Object.entries(clampNotices.value).filter(([existingKey]) => existingKey !== key),
  )
  if (notice) next[key] = notice
  clampNotices.value = next
}

/**
 * 滑块 → 数字：滑块只吐合法区间内的值，但仍走一次钳位，保证步长取整
 * （例如 0.001 档的阻尼）不会把长浮点写进配置。
 */
function onSliderUpdate(param: DecoNumberParam, value: number) {
  config.value.charm[param.key] = clampDecoParam(value, param)
  setClampNotice(param.key, null)
}

/**
 * 数字 → 滑块：两者读同一个 `config.charm[param.key]`，写回后滑块立即跟上。
 * 越界/非法输入由 `interpretDecoParamInput` 夹回并返回一句可见提示。
 */
function onNumberUpdate(param: DecoNumberParam, value: number | null) {
  const result = interpretDecoParamInput(value, param)
  config.value.charm[param.key] = result.value
  setClampNotice(param.key, result.notice)
}

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
 * 「恢复默认」：把整张表单填回规范的十三个默认值（开关 + 十二个数值）。
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
      <!-- 十二行数值输入由同一份元数据驱动：标签 / 单位 / 范围 / 步长 / 说明
           都在 utils/decoParams 里，加参数时只改那一个文件。每行都是
           「滑块 + 数字框」双输入，两处读写同一个配置字段。 -->
      <NFormItem
        v-for="param in numberParams"
        :key="param.key"
        :label="param.label"
        :feedback="param.hint"
        :show-feedback="true"
        :class="{ 'deco-param--clamped': clampNotices[param.key] }"
      >
        <div class="deco-param-row">
          <NSlider
            class="deco-param-slider"
            :value="config.charm[param.key]"
            :min="param.min"
            :max="param.max"
            :step="param.step"
            :disabled="!config.charm.enable"
            :data-param="param.key"
            :aria-label="param.label"
            @update:value="(value: number) => onSliderUpdate(param, value)"
          />
          <NInputNumber
            :value="config.charm[param.key]"
            :min="param.min"
            :max="param.max"
            :step="param.step"
            :precision="param.precision"
            :update-value-on-input="false"
            :disabled="!config.charm.enable"
            :data-param="param.key"
            :aria-label="param.label"
            class="deco-number-input"
            @update:value="(value: number | null) => onNumberUpdate(param, value)"
          >
            <template #suffix>
              <span class="deco-number-input__unit">{{ param.unit }}</span>
            </template>
          </NInputNumber>
        </div>
        <!-- 越界/非法输入被夹回时，行内给一句可见反馈；合法输入后自动消失。 -->
        <span v-if="clampNotices[param.key]" class="deco-clamp-notice" data-clamp-notice>
          {{ clampNotices[param.key] }}
        </span>
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
/* 滑块与数字框并排：滑块负责粗调，数字框负责精确值。 */
.deco-param-row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  max-width: 420px;
}

/* 滑块占剩余宽度，但给它一个下限，避免被数字框挤成一条点。 */
.deco-param-slider {
  flex: 1;
  min-width: 140px;
}

/* 数字输入只给一个可读宽度：铺满整行会显得像文本框。 */
.deco-number-input {
  width: 128px;
  flex: none;
}

/* 单位后缀走次级墨色，不抢输入框里的数字。 */
.deco-number-input__unit {
  color: var(--color-ink-muted);
  font-size: 12px;
}

/* 越界夹回的反馈用语义 token（不写裸色值），并与说明文案区分开。 */
.deco-clamp-notice {
  display: block;
  margin-top: 4px;
  color: var(--color-danger);
  font-size: 12px;
  line-height: 1.5;
}

/* 保存与「恢复默认」并排，用间距而不是靠两个按钮挤在一起。 */
.deco-actions {
  display: flex;
  gap: 12px;
}
</style>
