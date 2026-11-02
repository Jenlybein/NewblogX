<script setup lang="ts">
/**
 * 合集管理（管理员）。
 *
 * 三件事：**增删改合集**、**上传 16:9 封面**、**开关是否展示**。
 *
 * 封面上传复用轮播那套交互：先用本地读到的像素尺寸判比例，合规立刻上传；
 * 与 16:9 相差超过容差就打开同一个 `BannerFramingView` 取景窗，确认后上传，
 * 取消等于放弃本次上传（不产生图片记录）。取景窗本身不重写。
 *
 * 没上传封面时列表按读者的样子回退到首篇封面（`fallback_cover` 由后端算好）；
 * 两张都没有时渲染显式占位，绝不发一个空 `src` 的 img。
 *
 * 收录与排序不在本页（它们各自有独立的编排页）。
 */
import { h } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NSpace,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import BannerFramingDialog from '~/components/media/BannerFramingDialog.vue'
import {
  createCollection,
  deleteCollection,
  getAdminCollections,
  setCollectionDisplay,
  updateCollection,
} from '~/services/admin'
import { uploadImageByTask } from '~/services/image'
import type { CollectionAdminCard } from '~/types/api'
import { readImageSize, uploadBannerCover } from '~/utils/bannerCover'
import { DEFAULT_FRAMING, clampFraming } from '~/utils/framing'
import type { Framing, Size } from '~/utils/framing'
import {
  applyDisplayResult,
  buildCollectionCreatePayload,
  buildCollectionUpdatePayload,
  resolveCollectionCover,
  selectCollectionCover,
  validateCollectionForm,
} from '~/utils/collectionAdmin'

const message = useMessage()
const dialog = useDialog()

const rows = ref<CollectionAdminCard[]>([])
const loading = ref(false)
const saving = ref(false)
/** 正在切换展示开关的合集 ID；同一时间只允许一条 PATCH 在途。 */
const togglingId = ref('')
/** 非空表示表单处于"编辑这一行"的状态。 */
const editingId = ref('')

const form = reactive({ title: '', description: '', cover: '', displayed: true })
const errors = reactive<{ title: string | null; description: string | null }>({
  title: null,
  description: null,
})

/** 当前表单封面的预览地址（已上传的 URL 或本地 blob）。 */
const coverPreview = ref('')
/** 最近一次选中的原图像素尺寸；没选过就是 null。 */
const coverSize = ref<Size | null>(null)
/** 16:9 闸门的判定结果，仅用于给运营看状态。 */
const coverGate = ref<'' | 'direct' | 'framing'>('')
const coverUploading = ref(false)
const coverStage = ref('')
const coverFileInput = ref<HTMLInputElement | null>(null)

/** 等取景确认的本地文件；取消时连预览地址一起丢掉。 */
const pendingCover = ref<{ file: File; previewUrl: string } | null>(null)

/** 取景窗状态：打开/关闭、图源、原图尺寸与已有取景参数（合集没有持久化取景，回缺省）。 */
const framingDialog = reactive({
  show: false,
  src: '',
  imageWidth: 0,
  imageHeight: 0,
  framing: DEFAULT_FRAMING as Framing,
})

function renderCover(row: CollectionAdminCard) {
  const view = resolveCollectionCover(row.cover, row.fallback_cover)
  if (!view) {
    return h('span', { class: 'admin-cover-size__muted', 'data-role': 'collection-cover-empty' }, '无封面（也不会回退）')
  }

  return h('div', { class: 'admin-cover-size' }, [
    h('div', { class: 'admin-cover' }, [h('img', { src: view.url, alt: row.title || '合集封面' })]),
    view.fallback
      ? h(NTag, { size: 'small', type: 'default' }, { default: () => '回退首篇' })
      : h(NTag, { size: 'small', type: 'success' }, { default: () => '已上传' }),
  ])
}

const columns: DataTableColumns<CollectionAdminCard> = [
  { title: '封面', key: 'cover', width: 180, render: renderCover },
  {
    title: '标题',
    key: 'title',
    minWidth: 220,
    render: row => h('strong', { class: 'block truncate' }, row.title || '（无标题）'),
  },
  { title: '描述', key: 'description', minWidth: 240, render: row => row.description || '—' },
  { title: '收录', key: 'article_count', width: 90, render: row => `${row.article_count} 篇` },
  {
    title: '展示',
    key: 'displayed',
    width: 120,
    render: row =>
      h(NSwitch, {
        value: row.displayed,
        loading: togglingId.value === row.id,
        'aria-label': row.displayed ? '当前展示中，点击隐藏' : '当前已隐藏，点击展示',
        'onUpdate:value': (value: boolean) => void toggleDisplay(row, value),
      }),
  },
  {
    title: '操作',
    key: 'actions',
    width: 170,
    render: row =>
      h(NSpace, { size: 4, justify: 'end' }, {
        default: () => [
          h(NButton, { size: 'small', tertiary: true, type: 'primary', onClick: () => startEdit(row) }, {
            default: () => '编辑',
          }),
          h(NButton, { size: 'small', tertiary: true, type: 'error', onClick: () => removeRow(row) }, {
            default: () => '删除',
          }),
        ],
      }),
  },
]

async function refresh() {
  loading.value = true
  try {
    const data = await getAdminCollections()
    rows.value = data.list || []
  } catch {
    message.error('合集列表加载失败')
  } finally {
    loading.value = false
  }
}

function resetForm() {
  editingId.value = ''
  form.title = ''
  form.description = ''
  form.cover = ''
  form.displayed = true
  errors.title = null
  errors.description = null
  coverPreview.value = ''
  coverSize.value = null
  coverGate.value = ''
  coverStage.value = ''
  clearPendingCover()
}

function startEdit(row: CollectionAdminCard) {
  editingId.value = row.id
  form.title = row.title
  form.description = row.description
  // 只回填**已上传**的封面：回退值是只读的派生结果，不能被当成用户的选择保存回去。
  form.cover = row.cover
  form.displayed = row.displayed
  errors.title = null
  errors.description = null
  coverPreview.value = resolveCollectionCover(row.cover, row.fallback_cover)?.url ?? ''
  coverSize.value = null
  coverGate.value = ''
  coverStage.value = ''
  clearPendingCover()
}

async function submitForm() {
  const values = { title: form.title, description: form.description, cover: form.cover }
  const nextErrors = validateCollectionForm(values)
  errors.title = nextErrors.title
  errors.description = nextErrors.description

  if (errors.title || errors.description) {
    message.warning(errors.title ?? errors.description ?? '请检查表单')
    return
  }
  if (coverUploading.value) {
    message.warning('封面还在上传，请稍候再保存')
    return
  }
  if (pendingCover.value) {
    message.warning('这张图与 16:9 相差超过 1%，请先完成取景再保存')
    return
  }
  if (saving.value) return

  saving.value = true
  try {
    if (editingId.value) {
      await updateCollection(editingId.value, buildCollectionUpdatePayload(values))
      message.success('合集已保存')
    } else {
      await createCollection(buildCollectionCreatePayload({ ...values, displayed: form.displayed }))
      message.success('合集已创建')
    }
    resetForm()
    await refresh()
  } catch (error) {
    message.error(error instanceof Error ? `合集保存失败：${error.message}` : '合集保存失败，请稍后重试')
  } finally {
    saving.value = false
  }
}

async function toggleDisplay(row: CollectionAdminCard, displayed: boolean) {
  if (togglingId.value) return
  togglingId.value = row.id
  try {
    await setCollectionDisplay(row.id, displayed)
    // 结果立即落回列表：开关不该等下一次整表刷新才变。
    rows.value = applyDisplayResult(rows.value, row.id, displayed)
    message.success(displayed ? '合集已展示给读者' : '合集已隐藏，读者侧不再可见')
  } catch (error) {
    message.error(error instanceof Error ? `展示开关保存失败：${error.message}` : '展示开关保存失败，请稍后重试')
  } finally {
    togglingId.value = ''
  }
}

function removeRow(row: CollectionAdminCard) {
  dialog.warning({
    title: '删除合集',
    content: `确认删除「${row.title || '这个合集'}」？合集里的文章不会被删除，但读者侧会立即看不到这个合集。`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await deleteCollection(row.id)
        if (editingId.value === row.id) resetForm()
        message.success('合集已删除')
        await refresh()
      } catch (error) {
        message.error(error instanceof Error ? `删除失败：${error.message}` : '删除失败，请稍后重试')
      }
    },
  })
}

// ── 封面选择与 16:9 闸门 ─────────────────────────────────────

function openCoverPicker() {
  if (coverUploading.value || saving.value) return
  coverFileInput.value?.click()
}

/** 「移除封面」：清掉已上传的地址，回到"不传封面、回退首篇"的状态。 */
function removeCover() {
  form.cover = ''
  coverPreview.value = ''
  coverSize.value = null
  coverGate.value = ''
  coverStage.value = ''
  clearPendingCover()
}

function clearPendingCover() {
  const pending = pendingCover.value
  pendingCover.value = null
  if (pending) revokePreviewUrl(pending.previewUrl)
}

function mapUploadStage(stage: 'hashing' | 'creating_task' | 'uploading_to_qiniu' | 'polling_status') {
  const stageMap = {
    hashing: '正在计算文件指纹…',
    creating_task: '正在创建上传任务…',
    uploading_to_qiniu: '正在上传到对象存储…',
    polling_status: '正在确认上传状态…',
  } as const
  coverStage.value = stageMap[stage]
}

function createPreviewUrl(file: File): string {
  return typeof URL.createObjectURL === 'function' ? URL.createObjectURL(file) : ''
}

function revokePreviewUrl(url: string) {
  if (url && typeof URL.revokeObjectURL === 'function') URL.revokeObjectURL(url)
}

async function onCoverFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return

  if (!file.type.startsWith('image/')) {
    message.warning('请选择图片文件作为合集封面')
    return
  }

  clearPendingCover()
  coverStage.value = '正在读取图片尺寸…'
  coverUploading.value = true
  try {
    const result = await selectCollectionCover(file, {
      readSize: url => readImageSize(url, () => new Image()),
      createPreviewUrl,
      revokePreviewUrl,
      upload: next => uploadBannerCover(next, f => uploadImageByTask(f, { onStage: mapUploadStage })),
    })

    if (result.kind === 'unreadable') {
      coverStage.value = ''
      message.error('读不到这张图的像素尺寸，请换一张图片')
      return
    }

    coverSize.value = result.size
    if (result.kind === 'framing') {
      coverGate.value = 'framing'
      pendingCover.value = { file, previewUrl: result.previewUrl }
      coverPreview.value = result.previewUrl
      coverStage.value = '比例与 16:9 相差超过 1%，请先取景'
      openFramingDialog(result.previewUrl, result.size)
      return
    }

    coverGate.value = 'direct'
    form.cover = result.cover
    coverPreview.value = result.cover
    coverStage.value = `比例合规（${result.size.width} × ${result.size.height}），封面上传完成`
  } catch (error) {
    coverStage.value = ''
    message.error(error instanceof Error ? `封面上传失败：${error.message}` : '封面上传失败，请稍后重试')
  } finally {
    coverUploading.value = false
  }
}

function openFramingDialog(src: string, size: Size) {
  framingDialog.src = src
  framingDialog.imageWidth = size.width
  framingDialog.imageHeight = size.height
  framingDialog.framing = clampFraming(DEFAULT_FRAMING)
  framingDialog.show = true
}

/** 取景确认：取景窗只当"必须按 16:9 看过一遍"的闸门，确认后上传原图。 */
async function onFramingConfirm(framing: Framing) {
  const pending = pendingCover.value
  if (!pending) return
  framingDialog.framing = clampFraming(framing)
  framingDialog.show = false
  coverStage.value = '取景已确认，正在上传封面…'
  coverUploading.value = true
  try {
    const uploaded = await uploadBannerCover(pending.file, f => uploadImageByTask(f, { onStage: mapUploadStage }))
    form.cover = uploaded.url
    coverPreview.value = uploaded.url
    coverGate.value = ''
    coverStage.value = '封面上传完成，点击保存'
    revokePreviewUrl(pending.previewUrl)
    pendingCover.value = null
  } catch (error) {
    coverStage.value = '封面上传失败，可重新选择图片'
    message.error(error instanceof Error ? `封面上传失败：${error.message}` : '封面上传失败，请稍后重试')
  } finally {
    coverUploading.value = false
  }
}

/** 取消取景 = 放弃本次上传：没有上传、没有新的封面、表单保持原样。 */
function onFramingCancel() {
  framingDialog.show = false
  if (!pendingCover.value) return
  clearPendingCover()
  coverPreview.value = form.cover
  coverSize.value = null
  coverGate.value = ''
  coverStage.value = '已取消取景，本次上传未开始'
  message.info('已取消取景，本次上传未开始')
}

onMounted(refresh)
</script>

<template>
  <NSpace vertical :size="20">
    <NCard class="admin-card" title="新建 / 编辑合集">
      <p class="admin-hint">
        合集是管理员编排的文章系列。标题必填；封面推荐 16:9 —— 与 16:9 相差不超过 1% 会立即上传，
        超出 1% 先取景、确认后自动上传。不传封面时，读者侧会回退到首篇收录文章的封面。
      </p>

      <NForm label-placement="top" class="admin-collection-form">
        <NFormItem
          label="标题"
          :feedback="errors.title ?? ''"
          :validation-status="errors.title ? 'error' : undefined">
          <NInput
            v-model:value="form.title"
            data-role="collection-title"
            placeholder="例如：Nuxt 部署实战"
            :maxlength="128" />
        </NFormItem>

        <NFormItem
          label="描述"
          :feedback="errors.description ?? ''"
          :validation-status="errors.description ? 'error' : undefined">
          <NInput
            v-model:value="form.description"
            type="textarea"
            placeholder="一句话说明这个系列讲什么（可空）"
            :maxlength="512" />
        </NFormItem>

        <NFormItem label="封面">
          <div class="admin-upload-panel">
            <div class="admin-upload-panel__head">
              <div>
                <div class="admin-upload-panel__title">合集封面</div>
                <p class="admin-upload-panel__hint">
                  推荐 16:9：合规图立即上传；不合规先取景，确认后自动上传。留空则回退到首篇文章的封面。
                </p>
              </div>
              <div class="admin-upload-panel__actions">
                <NButton quaternary size="small" :loading="coverUploading" @click="openCoverPicker">
                  {{ coverPreview ? '重新选择' : '选择图片' }}
                </NButton>
                <NButton
                  v-if="form.cover"
                  quaternary
                  size="small"
                  :disabled="coverUploading"
                  @click="removeCover">
                  移除封面
                </NButton>
              </div>
            </div>

            <input
              ref="coverFileInput"
              type="file"
              accept="image/*"
              data-role="collection-cover-input"
              class="admin-upload-panel__input"
              @change="onCoverFileChange">

            <button type="button" class="admin-upload-panel__picker" @click="openCoverPicker">
              <img v-if="coverPreview" :src="coverPreview" alt="合集封面预览" class="admin-upload-panel__image">
              <template v-else>
                <span class="admin-upload-panel__plus">+</span>
                <span>上传合集封面</span>
                <small>推荐 16:9 · 支持 JPG / PNG / WebP</small>
              </template>
            </button>

            <div v-if="coverSize" class="admin-upload-panel__meta">
              <span>{{ coverSize.width }} × {{ coverSize.height }}</span>
              <NTag :type="coverGate === 'framing' ? 'warning' : 'success'" size="small">
                {{ coverGate === 'framing' ? '比例不合规，需取景' : coverGate === 'direct' ? '16:9 合规' : '已取景确认' }}
              </NTag>
            </div>

            <p v-if="coverStage" class="admin-upload-panel__stage">{{ coverStage }}</p>
          </div>
        </NFormItem>

        <NFormItem v-if="!editingId" label="展示">
          <div class="admin-switch-row">
            <div>
              <div class="admin-switch-row__title">立即展示给读者</div>
              <p class="admin-switch-row__hint">关闭时先建好，稍后可在列表里打开。</p>
            </div>
            <NSwitch v-model:value="form.displayed" />
          </div>
        </NFormItem>

        <NSpace>
          <NButton
            type="primary"
            data-role="collection-submit"
            :loading="saving"
            :disabled="coverUploading"
            @click="submitForm">
            {{ editingId ? '保存修改' : '创建合集' }}
          </NButton>
          <NButton v-if="editingId" :disabled="saving" @click="resetForm">取消编辑</NButton>
        </NSpace>
      </NForm>
    </NCard>

    <NCard class="admin-card" title="合集列表">
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :bordered="false"
        :row-key="(row: CollectionAdminCard) => row.id" />
    </NCard>

    <BannerFramingDialog
      v-model:show="framingDialog.show"
      :src="framingDialog.src"
      :image-width="framingDialog.imageWidth"
      :image-height="framingDialog.imageHeight"
      :framing="framingDialog.framing"
      @confirm="onFramingConfirm"
      @cancel="onFramingCancel" />
  </NSpace>
</template>
