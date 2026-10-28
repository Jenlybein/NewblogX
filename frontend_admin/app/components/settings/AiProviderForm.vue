<script setup lang="ts">
import { NButton, NForm, NFormItem, NInput, NModal, NSelect, NSpace, useMessage } from 'naive-ui'
import { createAiProvider, updateAiProvider } from '~/services/admin'
import { mergeVendorFields, vendorFieldDefaults } from '~/utils/aiProvider'
import type { AIProviderItem, AIProviderPayload, VendorItem } from '~/types/api'

const props = defineProps<{
  show: boolean
  vendors: VendorItem[]
  item: AIProviderItem | null
}>()

const emit = defineEmits<{
  'update:show': [boolean]
  saved: []
}>()

const message = useMessage()
const saving = ref(false)

const vendorId = ref('')
const name = ref('')
const model = ref('')
const fields = ref<Record<string, string>>({})

const currentVendor = computed(() => props.vendors.find(vendor => vendor.id === vendorId.value) ?? null)

// model 单独渲染：后端把它存在独立列里，其余字段进 fields。
const formFields = computed(() => (currentVendor.value?.fields ?? []).filter(field => field.key !== 'model'))

const vendorOptions = computed(() => props.vendors.map(vendor => ({ label: vendor.name, value: vendor.id })))

function defaultsOf(vendorIdValue: string): Record<string, string> {
  return vendorFieldDefaults(props.vendors.find(entry => entry.id === vendorIdValue))
}

function reset() {
  const editing = props.item
  if (editing) {
    vendorId.value = editing.vendor
    name.value = editing.name
    model.value = editing.model
    // 遮蔽过的密钥会原样回传，后端识别占位符后保留旧值。
    fields.value = mergeVendorFields(props.vendors.find(entry => entry.id === editing.vendor), editing.fields)
    return
  }
  vendorId.value = props.vendors[0]?.id ?? ''
  name.value = ''
  model.value = ''
  fields.value = defaultsOf(vendorId.value)
}

// 换供应商时重置字段：不同家的字段集合不一样，留着旧值只会提交出脏数据。
function onVendorChange(next: string) {
  vendorId.value = next
  fields.value = defaultsOf(next)
}

watch(() => props.show, (visible) => {
  if (visible) reset()
})

async function submit() {
  if (!vendorId.value) {
    message.warning('请先选择供应商')
    return
  }
  if (!model.value.trim()) {
    message.warning('请填写模型')
    return
  }

  const payload: AIProviderPayload = {
    name: name.value.trim(),
    vendor: vendorId.value,
    model: model.value.trim(),
    fields: { ...fields.value },
    // 启用是列表里的独立操作：新增/编辑只负责内容，不顺手改启用状态。
    enabled: props.item?.enabled ?? false,
  }

  saving.value = true
  try {
    if (props.item) {
      await updateAiProvider(props.item.id, payload)
      message.success('配置已更新')
    } else {
      await createAiProvider(payload)
      message.success('配置已添加，记得点「启用」让它生效')
    }
    emit('saved')
    emit('update:show', false)
  } catch {
    message.error('保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="item ? '编辑模型配置' : '添加模型配置'"
    class="w-[560px]"
    :bordered="false"
    @update:show="value => emit('update:show', value)"
  >
    <NForm label-placement="top" :disabled="saving">
      <NFormItem label="供应商" required>
        <NSelect :value="vendorId" :options="vendorOptions" @update:value="onVendorChange" />
      </NFormItem>

      <p v-if="currentVendor" class="muted m-0 mb-3 text-xs">
        {{ currentVendor.note }}
      </p>

      <NFormItem label="备注名（留空自动生成）">
        <NInput v-model:value="name" placeholder="例如：主力模型 / 便宜的那个" />
      </NFormItem>

      <NFormItem label="模型" required>
        <NInput v-model:value="model" :placeholder="currentVendor?.fields.find(f => f.key === 'model')?.placeholder ?? ''" />
      </NFormItem>

      <NFormItem
        v-for="field in formFields"
        :key="field.key"
        :label="field.label"
        :required="field.required"
      >
        <NInput
          v-model:value="fields[field.key]"
          :type="field.secret ? 'password' : 'text'"
          :show-password-on="field.secret ? 'click' : undefined"
          :placeholder="field.placeholder"
        />
      </NFormItem>

      <NSpace>
        <NButton type="primary" :loading="saving" @click="submit">保存</NButton>
        <NButton :disabled="saving" @click="emit('update:show', false)">取消</NButton>
      </NSpace>
    </NForm>
  </NModal>
</template>
