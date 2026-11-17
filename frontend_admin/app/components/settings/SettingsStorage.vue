<script setup lang="ts">
import { NAlert, NButton, NDataTable, NDynamicTags, NForm, NFormItem, NInput, NInputNumber, NSelect, NSpace, NSwitch, NTag, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { getStorageRuntimeConfig, probeImageStyles, updateStorageRuntimeConfig } from '~/services/admin'
import type { StorageRuntimeConfig, StyleProbeItem } from '~/types/api'

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

// 样式分隔符必须与七牛控制台的「访问设置」一致。
// 这里只给已经确认不会破坏 URL 结构的几种（后端也会拒掉其余写法）。
const delimiterOptions = [
  { label: '-（七牛默认）', value: '-' },
  { label: '_', value: '_' },
  { label: '!', value: '!' },
  { label: '~', value: '~' },
  { label: '.', value: '.' },
]

// —— 探活 ——
const probing = ref(false)
const probeKey = ref('')
const probeRows = ref<StyleProbeItem[]>([])

/** 把字节数变成人看的体积。 */
function formatSize(bytes: number) {
  if (bytes >= 1024 * 1024) {
    return `${(bytes / 1024 / 1024).toFixed(2)} MB`
  }
  if (bytes >= 1024) {
    return `${(bytes / 1024).toFixed(0)} KB`
  }
  return `${bytes} B`
}

/** 探活结果表：槽位 / 样式名 / 状态码 / 体积。 */
const probeColumns: DataTableColumns<StyleProbeItem> = [
  { title: '槽位', key: 'tier', width: 100 },
  { title: '样式名', key: 'style_name', width: 130, render: row => row.style_name || '（原图）' },
  {
    title: '状态',
    key: 'status',
    width: 140,
    render: (row) => {
      if (row.error) {
        return h(NTag, { type: 'error', size: 'small' }, { default: () => row.error })
      }
      const ok = row.status >= 200 && row.status < 300
      return h(NTag, { type: ok ? 'success' : 'error', size: 'small' }, { default: () => String(row.status) })
    },
  },
  {
    title: '体积',
    key: 'size',
    width: 120,
    render: row => (row.size > 0 ? formatSize(row.size) : '-'),
  },
]

/**
 * 探活：对同一张图分别请求原图与四档样式，把状态码与体积并排显示。
 *
 * 为什么必须有这一步：七牛对"样式名不存在"的响应是 404，**不会回退原图**，
 * 也不会报任何错。样式名填错一个字，站内那一档的图片就全裂了，而唯一能
 * 提前发现的地方就是这个对照表。
 */
async function probe() {
  probing.value = true
  try {
    const result = await probeImageStyles(probeKey.value || undefined)
    probeRows.value = result.items
    probeKey.value = result.object_key
    const failed = result.items.filter(item => item.error || item.status < 200 || item.status >= 300)
    if (failed.length === 0) {
      message.success('探活通过：原图与四档样式都可访问')
    } else {
      message.warning(`有 ${failed.length} 项不可访问，检查样式名是否与七牛控制台一致`)
    }
  } catch {
    message.error('探活失败：请确认对象存储已启用且填好了访问域名')
  } finally {
    probing.value = false
  }
}

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
    message.success('对象存储配置已保存，下一次请求即生效')
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
      这是图片上传的<strong>唯一</strong>落地方式：关掉之后上传接口直接返回「站点未启用图片上传」，
      不会退化成存到服务器本地磁盘（容器重建就丢）。
    </NAlert>
    <NAlert type="info" :bordered="false" class="mb-4">
      「对象 key 前缀」用来从图片 URL 反解出对象名，文章的图片引用关系靠它维护。
      改成别的前缀，已有图片的引用关系就对不上了。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <h3 class="admin-settings-title">
        存储与上传
      </h3>
      <NFormItem label="启用七牛云存储">
        <NSwitch v-model:value="config.enable" />
      </NFormItem>
      <NFormItem label="AccessKey">
        <NInput v-model:value="config.access_key" class="admin-field--secret" placeholder="七牛云密钥管理里的 AK" />
      </NFormItem>
      <NFormItem label="SecretKey">
        <NInput
          v-model:value="config.secret_key"
          class="admin-field--secret"
          type="password"
          show-password-on="click"
          placeholder="留 ****** 表示不修改"
        />
      </NFormItem>
      <NFormItem label="Bucket">
        <NInput v-model:value="config.bucket" class="admin-field--key" placeholder="存储空间名称" />
      </NFormItem>
      <NFormItem label="访问域名">
        <NInput v-model:value="config.uri" class="admin-field--url" placeholder="https://cdn.yourdomain.com（含协议头）" />
      </NFormItem>
      <NFormItem label="存储区域">
        <NSelect v-model:value="config.region" class="admin-field--select" :options="regionOptions" />
      </NFormItem>
      <NFormItem label="对象 key 前缀">
        <NInput v-model:value="config.prefix" class="admin-field--key" placeholder="myblogx" />
      </NFormItem>
      <NSpace :size="12" class="w-full">
        <NFormItem label="单图大小上限（MB）">
          <NInputNumber v-model:value="config.size" class="admin-field--number" :min="1" :max="100" />
        </NFormItem>
        <NFormItem label="上传凭证有效期（秒）">
          <NInputNumber v-model:value="config.expiry" class="admin-field--number" :min="60" />
        </NFormItem>
      </NSpace>
      <NFormItem label="上传回调地址（留空不启用）">
        <NInput v-model:value="config.callback_url" class="admin-field--url" placeholder="https://api.yourdomain.com/..." />
      </NFormItem>
      <NFormItem label="允许上传的文件后缀">
        <NDynamicTags v-model:value="config.whitelist" class="admin-field--long" />
      </NFormItem>

      <h3 class="admin-settings-title">
        图片展示样式
      </h3>
      <NAlert type="info" :bordered="false" class="mb-4">
        样式名必须与七牛控制台「多媒体样式」里已创建的<strong>名字完全一致</strong>。
        留空表示该档用原图。填错不会回退原图，而是 404 —— 保存后请点下面的「探活检测」。
        库里只存原图地址，所以清空这里会立刻恢复原图，不需要回填数据。
      </NAlert>
      <NSpace :size="12" class="w-full">
        <NFormItem label="封面样式">
          <NInput v-model:value="config.image_style_cover" class="admin-field--key" placeholder="cover" />
        </NFormItem>
        <NFormItem label="头像样式">
          <NInput v-model:value="config.image_style_avatar" class="admin-field--key" placeholder="avatar" />
        </NFormItem>
      </NSpace>
      <NSpace :size="12" class="w-full">
        <NFormItem label="正文样式">
          <NInput v-model:value="config.image_style_content" class="admin-field--key" placeholder="content" />
        </NFormItem>
        <NFormItem label="私信样式">
          <NInput v-model:value="config.image_style_chat" class="admin-field--key" placeholder="chat" />
        </NFormItem>
      </NSpace>
      <NSpace :size="12" class="w-full">
        <NFormItem label="样式分隔符">
          <NSelect v-model:value="config.style_delimiter" class="admin-field--select" :options="delimiterOptions" />
        </NFormItem>
        <NFormItem label="下载签名有效期（秒）">
          <NInputNumber v-model:value="config.download_expiry" class="admin-field--number" :min="60" :max="604800" />
        </NFormItem>
      </NSpace>
      <NFormItem label="私有空间（所有图片地址都要签名）">
        <NSwitch v-model:value="config.private_read" />
      </NFormItem>

      <h3 class="admin-settings-title">
        探活检测
      </h3>
      <NAlert type="info" :bordered="false" class="mb-4">
        对同一张图分别请求原图与四档样式，把状态码与体积并排显示。
        对象 key 留空时用图库里最新一张图。
      </NAlert>
      <NSpace :size="12" align="center" class="w-full mb-4">
        <NInput v-model:value="probeKey" class="admin-field--url" placeholder="对象 key（留空用最新一张图）" />
        <NButton :loading="probing" @click="probe">
          探活检测
        </NButton>
      </NSpace>
      <NDataTable
        v-if="probeRows.length > 0"
        :columns="probeColumns"
        :data="probeRows"
        :row-key="(row: StyleProbeItem) => row.tier"
        size="small"
        class="mb-4"
      />

      <h3 class="admin-settings-title">
        图片内容审核
      </h3>
      <NAlert type="warning" :bordered="false" class="mb-4">
        未通过审核的图片<strong>不会展示给任何人</strong>（管理员除外，否则没法审图）。
        超时只会把图转进人工队列，<strong>不会</strong>因为等太久就放行。
      </NAlert>
      <NFormItem label="启用图片审核">
        <NSwitch v-model:value="config.audit_enabled" />
      </NFormItem>
      <NFormItem label="审核未出结论时转人工的时限（秒）">
        <NInputNumber v-model:value="config.audit_timeout_seconds" class="admin-field--number" :min="5" :max="3600" />
      </NFormItem>
      <NFormItem label="判定违规时立即删除存储对象（同图永久拒收）">
        <NSwitch v-model:value="config.audit_auto_delete_block" />
      </NFormItem>
      <NFormItem label="展示闸门（关掉等于未过审也可展示，仅用于审核链路故障时紧急放行）">
        <NSwitch v-model:value="config.audit_gate_enabled" />
      </NFormItem>

      <NButton type="primary" :loading="saving" @click="save">
        保存存储配置
      </NButton>
    </NForm>
    <div v-else class="admin-empty">
      {{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}
    </div>
  </div>
</template>
