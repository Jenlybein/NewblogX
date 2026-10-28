<script setup lang="ts">
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSpace,
  NSwitch,
  useMessage,
} from 'naive-ui'
import { getLogRuntimeConfig, updateLogRuntimeConfig } from '~/services/admin'
import type { LogRuntimeConfig } from '~/types/api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染，这些字段值不会被看到。
// 不用 `ref<T | null>(null)` 是因为 vue-tsc 在「可空 ref + v-model」下会
// 丢掉 v-if 的窄化（第一次双向写之后就不再窄化），整份模板都会报可能为 null。
const config = ref({} as LogRuntimeConfig)
const loaded = ref(false)

// 与 logrus 的级别名一致。写错不会报错，后端会静默退回 info——
// 所以这里给下拉而不是自由输入。
const levelOptions = ['trace', 'debug', 'info', 'warn', 'error'].map(v => ({ label: v, value: v }))

async function load() {
  loading.value = true
  try {
    config.value = await getLogRuntimeConfig()
    loaded.value = true
  } catch {
    message.error('日志配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await updateLogRuntimeConfig(config.value)
    message.success('日志配置已保存；级别即时生效，其余项下一次请求生效')
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
      下方「进程级」的四项由部署时的配置文件决定，在这里改了也不会生效
      （保存时后端会把它们覆盖回当前值）——日志写到哪个目录、用什么格式，
      不该由网页决定，改错了连记录错误的日志都会一起消失。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="进程级（只读）" size="small">
        <NFormItem label="服务名">
          <NInput :value="config.app" disabled placeholder="由启动参数决定" />
        </NFormItem>
        <NFormItem label="日志目录">
          <NInput :value="config.dir" disabled placeholder="由启动参数决定" />
        </NFormItem>
        <NFormItem label="本地兜底目录">
          <NInput :value="config.wal_dir" disabled placeholder="留空时为 <日志目录>/wal" />
        </NFormItem>
        <NFormItem label="标准输出格式">
          <NInput :value="config.stdout_format" disabled />
        </NFormItem>
      </NCard>

      <NFormItem label="日志级别">
        <NSelect v-model:value="config.level" :options="levelOptions" />
      </NFormItem>
      <NFormItem label="记录请求访问日志">
        <NSwitch v-model:value="config.request_log_enabled" />
      </NFormItem>
      <NSpace :size="12" class="w-full">
        <NFormItem label="日志查询默认条数">
          <NInputNumber v-model:value="config.query_default_limit" :min="1" />
        </NFormItem>
        <NFormItem label="日志查询最大条数">
          <NInputNumber v-model:value="config.query_max_limit" :min="1" />
        </NFormItem>
      </NSpace>

      <NCard class="admin-card mb-4" title="错误日志采集" size="small">
        <NFormItem label="采集调用栈">
          <NSwitch v-model:value="config.error.capture_stack" />
        </NFormItem>
        <NFormItem label="采集的最低级别">
          <NSelect v-model:value="config.error.capture_min_level" :options="levelOptions" />
        </NFormItem>
        <NSpace :size="12" class="w-full">
          <NFormItem label="调用栈最大字节数">
            <NInputNumber v-model:value="config.error.stack_max_bytes" :min="0" />
          </NFormItem>
          <NFormItem label="错误链追溯深度">
            <NInputNumber v-model:value="config.error.cause_chain_depth" :min="0" />
          </NFormItem>
        </NSpace>
      </NCard>

      <NCard class="admin-card mb-4" title="链路追踪" size="small">
        <NFormItem label="启用 trace_id">
          <NSwitch v-model:value="config.trace.enabled" />
        </NFormItem>
        <NFormItem label="request_id 与 trace_id 同值">
          <NSwitch v-model:value="config.trace.request_id_equals_trace_id" />
        </NFormItem>
        <NFormItem label="从网关继承 trace_id">
          <NSwitch v-model:value="config.trace.inherit_from_gateway" />
        </NFormItem>
        <NFormItem label="网关头的取用顺序">
          <NInput v-model:value="config.trace.gateway_header_priority" placeholder="多个头名用逗号分隔" />
        </NFormItem>
      </NCard>

      <NCard class="admin-card mb-4" title="本地日志清理" size="small">
        <NFormItem label="启用清理任务">
          <NSwitch v-model:value="config.cleanup.enabled" />
        </NFormItem>
        <NSpace :size="12" class="w-full">
          <NFormItem label="保留天数">
            <NInputNumber v-model:value="config.cleanup.retention_days" :min="1" />
          </NFormItem>
          <NFormItem label="执行时刻">
            <NInput v-model:value="config.cleanup.run_at" placeholder="03:30" />
          </NFormItem>
        </NSpace>
      </NCard>

      <NButton type="primary" :loading="saving" @click="save">保存日志配置</NButton>
    </NForm>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>
  </div>
</template>
