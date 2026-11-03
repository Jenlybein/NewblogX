<script setup lang="ts">
/**
 * 项目管理（管理员）。
 *
 * 三件事：**增删改项目**、**上传 16:9 封面**、**开关是否展示**。
 *
 * 项目与合集并列、页面形态复用，但数据与表完全独立：本页只读写
 * `/projects` 下面的管理端接口，不碰合集的任何数据。
 *
 * 封面上传在票 200 起改为**上传前定形**：选中的图先在客户端居中裁成 16:9、
 * 降采样到 1600×900，预览的是定形产物，上传也只发这份产物——原图不进上传链路。
 * 这与合集、文章封面共用同一个实现（`~/utils/imageShaping` 的 `shapeForUpload`）。
 *
 * 没上传封面时列表按读者的样子回退到首篇公开文章的封面（`fallback_cover`
 * 由后端算好）；两张都没有时渲染显式占位，绝不发一个空 `src` 的 img。
 *
 * 收录与排序不在本页：项目的内容编排走「目录编排」的整树编辑器。
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
import {
  createProject,
  deleteProject,
  getAdminProjects,
  setProjectDisplay,
  updateProject,
} from '~/services/admin'
import { uploadImageByTask } from '~/services/image'
import ImageCropDialog from '~/components/media/ImageCropDialog.vue'
import { useImageCropUpload, type ImageFramingRequest } from '~/composables/useImageCropUpload'
import type { ProjectAdminCard } from '~/types/api'
import { uploadBannerCover } from '~/utils/bannerCover'
import type { ShapeFramingState, ShapeImageTarget } from '~/utils/imageShaping'
import {
  COVER_SHAPE_OPTIONS,
  COVER_SHAPE_RATIO_TEXT,
  COVER_SHAPE_SIZE_TEXT,
  COVER_SHAPE_TARGET,
} from '~/utils/imageShaping'
import {
  applyProjectDisplayResult,
  buildProjectCreatePayload,
  buildProjectUpdatePayload,
  resolveProjectCover,
  validateProjectForm,
} from '~/utils/projectAdmin'

const message = useMessage()
const dialog = useDialog()

const rows = ref<ProjectAdminCard[]>([])
const loading = ref(false)
const saving = ref(false)
/** 正在切换展示开关的项目 ID；同一时间只允许一条 PATCH 在途。 */
const togglingId = ref('')
/** 非空表示表单处于"编辑这一行"的状态。 */
const editingId = ref('')

const form = reactive({ title: '', description: '', cover: '', displayed: true })
const errors = reactive<{ title: string | null; description: string | null }>({
  title: null,
  description: null,
})

/** 当前表单封面的预览地址（已上传的 URL 或本地定形产物的 object URL）。 */
const coverPreview = ref('')
/** 本地定形产物的预览地址：上传完成 / 换图 / 移除时释放，避免一直占着这张图的内存。 */
const coverLocalPreview = ref('')
/** 最近一次定形产物的像素尺寸（1600×900）；没选过就是 null。 */
const coverSize = ref<ShapeImageTarget | null>(null)
const coverUploading = ref(false)
const coverStage = ref('')
const coverFileInput = ref<HTMLInputElement | null>(null)

/**
 * 裁剪窗请求（票 215）：非 null 表示项目封面比例不合规，等管理员选区域。
 * 这期间没有定形产物、没有上传；取消就保留现有封面（form.cover 原样不动）。
 */
const coverCropRequest = ref<ImageFramingRequest | null>(null)
/** 待裁剪原图的本地预览地址。 */
const coverCropPreviewUrl = ref('')

function createPreviewUrl(blob: Blob): string {
  try {
    return typeof URL.createObjectURL === 'function' ? URL.createObjectURL(blob) : ''
  } catch {
    return ''
  }
}

function resetCoverCrop() {
  const url = coverCropPreviewUrl.value
  coverCropPreviewUrl.value = ''
  coverCropRequest.value = null
  revokePreviewUrl(url)
}

/**
 * 项目封面的上传顺序与其余四个入口同源（`useImageCropUpload`）：
 * 与 16:9 相差 ≤1% → 自动居中定形 1600×900 后直接上传；
 * 相差 >1% → 只打开裁剪窗，不产出文件、不上传；取消即保留现有封面。
 */
const coverCropFlow = useImageCropUpload({
  options: COVER_SHAPE_OPTIONS,
  requestFraming: (request) => {
    coverCropRequest.value = request
    coverCropPreviewUrl.value = createPreviewUrl(request.file)
    coverStage.value = `图片比例不符合 ${COVER_SHAPE_RATIO_TEXT}，请选择要露出的区域`
  },
  upload: async (shaped) => {
    coverSize.value = shaped.size
    coverLocalPreview.value = shaped.previewUrl
    coverPreview.value = shaped.previewUrl || coverPreview.value
    coverStage.value = `已定形成 ${COVER_SHAPE_SIZE_TEXT}（${COVER_SHAPE_RATIO_TEXT}），正在上传封面…`
    const uploaded = await uploadBannerCover(shaped.file, f =>
      uploadImageByTask(f, { onStage: mapUploadStage, purpose: 'cover' }),
    )
    form.cover = uploaded.url
    coverPreview.value = uploaded.url
    clearLocalCoverPreview()
    coverStage.value = '封面上传完成，点击保存'
  },
})
function renderCover(row: ProjectAdminCard) {
  const view = resolveProjectCover(row.cover, row.fallback_cover)
  if (!view) {
    return h('span', { class: 'admin-cover-size__muted', 'data-role': 'project-cover-empty' }, '无封面（也不会回退）')
  }

  return h('div', { class: 'admin-cover-size' }, [
    h('div', { class: 'admin-cover' }, [h('img', { src: view.url, alt: row.title || '项目封面' })]),
    view.fallback
      ? h(NTag, { size: 'small', type: 'default' }, { default: () => '回退首篇' })
      : h(NTag, { size: 'small', type: 'success' }, { default: () => '已上传' }),
  ])
}

const columns: DataTableColumns<ProjectAdminCard> = [
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
    width: 180,
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
    const data = await getAdminProjects()
    rows.value = data.list || []
  } catch {
    message.error('项目列表加载失败')
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
  coverStage.value = ''
  clearLocalCoverPreview()
}

function startEdit(row: ProjectAdminCard) {
  editingId.value = row.id
  form.title = row.title
  form.description = row.description
  // 只回填**已上传**的封面：回退值是只读的派生结果，不能被当成用户的选择保存回去。
  form.cover = row.cover
  form.displayed = row.displayed
  errors.title = null
  errors.description = null
  coverPreview.value = resolveProjectCover(row.cover, row.fallback_cover)?.url ?? ''
  coverSize.value = null
  coverStage.value = ''
  clearLocalCoverPreview()
}

async function submitForm() {
  const values = { title: form.title, description: form.description, cover: form.cover }
  const nextErrors = validateProjectForm(values)
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
  if (saving.value) return

  saving.value = true
  try {
    if (editingId.value) {
      await updateProject(editingId.value, buildProjectUpdatePayload(values))
      message.success('项目已保存')
    } else {
      await createProject(buildProjectCreatePayload({ ...values, displayed: form.displayed }))
      message.success('项目已创建')
    }
    resetForm()
    await refresh()
  } catch (error) {
    message.error(error instanceof Error ? `项目保存失败：${error.message}` : '项目保存失败，请稍后重试')
  } finally {
    saving.value = false
  }
}

async function toggleDisplay(row: ProjectAdminCard, displayed: boolean) {
  if (togglingId.value) return
  togglingId.value = row.id
  try {
    await setProjectDisplay(row.id, displayed)
    // 结果立即落回列表：开关不该等下一次整表刷新才变。
    rows.value = applyProjectDisplayResult(rows.value, row.id, displayed)
    message.success(displayed ? '项目已展示给读者' : '项目已隐藏，读者侧不再可见')
  } catch (error) {
    message.error(error instanceof Error ? `展示开关保存失败：${error.message}` : '展示开关保存失败，请稍后重试')
  } finally {
    togglingId.value = ''
  }
}

function removeRow(row: ProjectAdminCard) {
  dialog.warning({
    title: '删除项目',
    content: `确认删除「${row.title || '这个项目'}」？项目里的文章不会被删除，但读者侧会立即看不到这个项目。`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await deleteProject(row.id)
        if (editingId.value === row.id) resetForm()
        message.success('项目已删除')
        await refresh()
      } catch (error) {
        message.error(error instanceof Error ? `删除失败：${error.message}` : '删除失败，请稍后重试')
      }
    },
  })
}

// ── 封面选择与上传前定形 ─────────────────────────────────────

function openCoverPicker() {
  if (coverUploading.value || saving.value) return
  coverFileInput.value?.click()
}

/** 「移除封面」：清掉已上传的地址，回到"不传封面、回退首篇"的状态。 */
function removeCover() {
  form.cover = ''
  coverPreview.value = ''
  coverSize.value = null
  coverStage.value = ''
  resetCoverCrop()
  clearLocalCoverPreview()
}

/** 只释放本地那份定形产物预览（已上传的封面是远端 URL，不归我们管）。 */
function clearLocalCoverPreview() {
  const url = coverLocalPreview.value
  coverLocalPreview.value = ''
  if (url) revokePreviewUrl(url)
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

function revokePreviewUrl(url: string) {
  if (url && typeof URL.revokeObjectURL === 'function') URL.revokeObjectURL(url)
}

async function onCoverFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return

  if (!file.type.startsWith('image/')) {
    message.warning('请选择图片文件作为项目封面')
    return
  }

  clearLocalCoverPreview()
  coverSize.value = null
  resetCoverCrop()
  coverStage.value = '正在检查图片比例…'
  coverUploading.value = true
  try {
    await coverCropFlow.select(file)
  } catch (error) {
    coverStage.value = '封面上传失败，可重新选择图片'
    message.error(error instanceof Error ? `封面上传失败：${error.message}` : '封面上传失败，请稍后重试')
  } finally {
    coverUploading.value = false
  }
}

/** 裁剪窗确认：按选定区域定形成 1600×900 新文件，再走既有上传链路。 */
async function confirmCoverCrop(framing: ShapeFramingState) {
  const request = coverCropRequest.value
  if (!request) return
  resetCoverCrop()
  coverStage.value = `正在按选定区域定形成 ${COVER_SHAPE_RATIO_TEXT} 封面…`
  coverUploading.value = true
  try {
    await coverCropFlow.confirm(request, framing)
  } catch (error) {
    coverStage.value = '封面上传失败，可重新选择图片'
    message.error(error instanceof Error ? `封面上传失败：${error.message}` : '封面上传失败，请稍后重试')
  } finally {
    coverUploading.value = false
  }
}

/** 裁剪窗取消：中止本次上传，现有封面（form.cover）保持不变。 */
function cancelCoverCrop() {
  resetCoverCrop()
  coverStage.value = '已取消本次上传，保留现有封面'
}

onMounted(refresh)
</script>

<template>
  <NSpace vertical :size="20">
    <NCard class="admin-card" title="新建 / 编辑项目">
      <p class="admin-hint">
        项目是管理员编排的文章系列，与合集并列但数据独立。标题必填；封面上传前会自动裁成
        {{ COVER_SHAPE_RATIO_TEXT }}（{{ COVER_SHAPE_SIZE_TEXT }}），预览与上传的都是定形后的图，原图不上传。
        不传封面时，读者侧会回退到首篇收录文章的封面。
      </p>

      <NForm label-placement="top" class="admin-project-form">
        <NFormItem
          label="标题"
          :feedback="errors.title ?? ''"
          :validation-status="errors.title ? 'error' : undefined">
          <NInput
            v-model:value="form.title"
            data-role="project-title"
            placeholder="例如：重构实战"
            :maxlength="128" />
        </NFormItem>

        <NFormItem
          label="描述"
          :feedback="errors.description ?? ''"
          :validation-status="errors.description ? 'error' : undefined">
          <NInput
            v-model:value="form.description"
            type="textarea"
            placeholder="一句话说明这个项目讲什么（可空）"
            :maxlength="512" />
        </NFormItem>

        <NFormItem label="封面">
          <div class="admin-upload-panel">
            <div class="admin-upload-panel__head">
              <div>
                <div class="admin-upload-panel__title">项目封面</div>
                <p class="admin-upload-panel__hint">
                  上传前自动裁成 {{ COVER_SHAPE_RATIO_TEXT }}（{{ COVER_SHAPE_SIZE_TEXT }}），预览与上传的都是定形后的图。
                  留空则回退到首篇文章的封面。
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
              data-role="project-cover-input"
              class="admin-upload-panel__input"
              @change="onCoverFileChange">

            <button type="button" class="admin-upload-panel__picker" @click="openCoverPicker">
              <img v-if="coverPreview" :src="coverPreview" alt="项目封面预览" class="admin-upload-panel__image">
              <template v-else>
                <span class="admin-upload-panel__plus">+</span>
                <span>上传项目封面</span>
                <small>{{ COVER_SHAPE_RATIO_TEXT }} · {{ COVER_SHAPE_SIZE_TEXT }} · 支持 JPG / PNG / WebP</small>
              </template>
            </button>

            <div v-if="coverSize" class="admin-upload-panel__meta">
              <span>{{ coverSize.width }} × {{ coverSize.height }}</span>
              <NTag type="success" size="small">已定形 {{ COVER_SHAPE_RATIO_TEXT }}</NTag>
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
            data-role="project-submit"
            :loading="saving"
            :disabled="coverUploading"
            @click="submitForm">
            {{ editingId ? '保存修改' : '创建项目' }}
          </NButton>
          <NButton v-if="editingId" :disabled="saving" @click="resetForm">取消编辑</NButton>
        </NSpace>
      </NForm>
    </NCard>

    <NCard class="admin-card" title="项目列表">
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :bordered="false"
        :row-key="(row: ProjectAdminCard) => row.id" />
    </NCard>

    <ImageCropDialog
      :show="coverCropRequest !== null"
      :src="coverCropPreviewUrl"
      :image-width="coverCropRequest?.width ?? 0"
      :image-height="coverCropRequest?.height ?? 0"
      :target="COVER_SHAPE_TARGET"
      :ratio-text="COVER_SHAPE_RATIO_TEXT"
      title="裁剪项目封面（固定 16:9）"
      @confirm="confirmCoverCrop"
      @cancel="cancelCoverCrop" />
  </NSpace>
</template>
