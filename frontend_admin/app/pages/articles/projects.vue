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
 * 「收录管理」浮层只做两件事：看当前收录行（含失效占位并标出原因）、清理失效行。
 * 批量收录与两级手动排序属于独立的「项目编排」页，不在这里。
 */
import { h } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NSpace,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  cleanupProjectStale,
  createProject,
  deleteProject,
  getAdminProjectItems,
  getAdminProjects,
  removeProjectArticle,
  setProjectDisplay,
  updateProject,
} from '~/services/admin'
import { uploadImageByTask } from '~/services/image'
import type { ProjectAdminCard, ProjectAdminItem } from '~/types/api'
import { uploadBannerCover } from '~/utils/bannerCover'
import type { ShapeImageTarget } from '~/utils/imageShaping'
import {
  COVER_SHAPE_OPTIONS,
  COVER_SHAPE_RATIO_TEXT,
  COVER_SHAPE_SIZE_TEXT,
  shapeForUpload,
} from '~/utils/imageShaping'
import {
  applyProjectDisplayResult,
  buildProjectCreatePayload,
  buildProjectUpdatePayload,
  resolveProjectCover,
  validateProjectForm,
} from '~/utils/projectAdmin'
import { staleItemCount, staleItemIds, staleReasonLabel } from '~/utils/staleItems'

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

// ── 收录管理浮层（保留失效占位 + 标记 + 一键清理）──

const itemsProject = ref<ProjectAdminCard | null>(null)
const itemsRows = ref<ProjectAdminItem[]>([])
const itemsLoading = ref(false)
const itemsRemovingId = ref('')
const itemsCleaning = ref(false)

/** 一键清理只挑 stale 为真的行；判定由后端共用领域函数给出，前端不另算。 */
const itemsStaleIds = computed(() => staleItemIds(itemsRows.value))
const itemsStaleCount = computed(() => staleItemCount(itemsRows.value))

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
    width: 260,
    render: row =>
      h(NSpace, { size: 4, justify: 'end' }, {
        default: () => [
          h(NButton, { size: 'small', tertiary: true, onClick: () => void openItems(row) }, {
            default: () => '收录管理',
          }),
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

/**
 * 收录管理表：序号 + 封面 + 文章 + 状态（含失效原因）+ 单独移除。
 *
 * 失效行**保留**并标红：文章后来被删/退回草稿/隐藏时，关系行还在，运营要能
 * 看到是哪一篇、为什么失效，再决定单独移除还是整批清理。
 */
const itemColumns: DataTableColumns<ProjectAdminItem> = [
  {
    title: '序号',
    key: 'sort_order',
    width: 72,
    render: (_row, index) => index + 1,
  },
  {
    title: '封面',
    key: 'cover',
    width: 110,
    render: row =>
      h('div', { class: 'admin-cover' }, row.cover ? [h('img', { src: row.cover, alt: row.title || '收录封面' })] : []),
  },
  {
    title: '文章',
    key: 'title',
    minWidth: 220,
    render: row =>
      h('strong', { class: 'block truncate' }, row.title || (row.stale ? '（文章已删除，仅剩收录占位）' : '（无标题）')),
  },
  {
    title: '状态',
    key: 'stale',
    width: 190,
    render: row =>
      h(NTag, { size: 'small', type: row.stale ? 'warning' : 'success' }, {
        default: () => staleReasonLabel(row.stale_reason),
      }),
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: row =>
      h(
        NButton,
        {
          size: 'small',
          tertiary: true,
          type: row.stale ? 'error' : 'default',
          loading: itemsRemovingId.value === row.article_id,
          disabled: itemsRemovingId.value !== '' || itemsCleaning.value,
          onClick: () => removeItem(row),
        },
        { default: () => '移除' },
      ),
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

// ── 收录管理与一键清理 ───────────────────────────────────────

/** 打开某个项目的收录管理浮层，并按需加载全部收录行（含失效占位）。 */
async function openItems(row: ProjectAdminCard) {
  itemsProject.value = row
  itemsRows.value = []
  await refreshItems()
}

async function refreshItems() {
  const target = itemsProject.value
  if (!target) return
  itemsLoading.value = true
  try {
    const data = await getAdminProjectItems(target.id)
    itemsRows.value = data.list || []
  } catch {
    message.error('收录行加载失败')
  } finally {
    itemsLoading.value = false
  }
}

function closeItems() {
  itemsProject.value = null
  itemsRows.value = []
  itemsRemovingId.value = ''
}

function removeItem(row: ProjectAdminItem) {
  const target = itemsProject.value
  if (!target || itemsRemovingId.value) return

  dialog.warning({
    title: row.stale ? '移除失效收录' : '移除收录',
    content: row.stale
      ? `「${row.title || '这篇文章'}」已失效（${staleReasonLabel(row.stale_reason)}）。移除只解除收录关系，不删文章。`
      : `确认把「${row.title || '这篇文章'}」从项目移除？读者侧会立即少一篇。`,
    positiveText: '移除',
    negativeText: '取消',
    async onPositiveClick() {
      itemsRemovingId.value = row.article_id
      try {
        await removeProjectArticle(target.id, row.article_id)
        message.success('已移除')
        await refreshItems()
        await refresh()
      } catch (error) {
        message.error(error instanceof Error ? `移除失败：${error.message}` : '移除失败，请稍后重试')
      } finally {
        itemsRemovingId.value = ''
      }
    },
  })
}

/**
 * 一键清理：只提交「失效行」。
 *
 * 失效判定不在前端重算——行上的 stale 来自后端与合集/热门置顶共用的领域函数；
 * 管理端接口是幂等的，第二次点击返回 removed=0，不会删到展示中的文章。
 */
function cleanupStaleItems() {
  const target = itemsProject.value
  if (!target || itemsCleaning.value) return
  const staleIds = itemsStaleIds.value.slice()
  if (staleIds.length === 0) return

  dialog.warning({
    title: '清理失效收录',
    content: `将移除 ${staleIds.length} 条已失效的收录（文章已删除、退回草稿或不再可见）。展示中的收录不受影响。`,
    positiveText: '清理',
    negativeText: '取消',
    async onPositiveClick() {
      itemsCleaning.value = true
      try {
        const result = await cleanupProjectStale(target.id)
        message.success(
          result.removed > 0 ? `已清理 ${result.removed} 条失效收录` : '没有需要清理的失效收录',
        )
      } catch (error) {
        message.error(error instanceof Error ? `清理失败：${error.message}` : '清理失败，请稍后重试')
      } finally {
        itemsCleaning.value = false
        await refreshItems()
        await refresh()
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
  coverStage.value = '正在定形成 16:9 封面…'
  coverUploading.value = true
  try {
    // 上传前定形（票 200）：居中裁成 16:9、降采样到 1600×900，产出新文件。
    // 预览用定形产物（运营看到的就是即将上传的那张图），原图不进入上传链路。
    const shaped = await shapeForUpload(file, COVER_SHAPE_OPTIONS)
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
  } catch (error) {
    coverStage.value = '封面上传失败，可重新选择图片'
    message.error(error instanceof Error ? `封面上传失败：${error.message}` : '封面上传失败，请稍后重试')
  } finally {
    coverUploading.value = false
  }
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

    <NModal
      :show="itemsProject !== null"
      preset="card"
      class="max-w-[900px]"
      :title="`收录管理：${itemsProject?.title ?? ''}`"
      @update:show="(value: boolean) => { if (!value) closeItems() }">
      <div class="admin-toolbar">
        <p class="admin-hint">
          收录行按展示顺序排列。文章后来被删除、退回草稿或不再可见时，这一行会保留并标为失效：
          读者侧已不再展示，这里可以单独移除或一键清理。清理只解除收录关系，不删除文章。
        </p>
        <NSpace>
          <NButton size="small" quaternary :loading="itemsLoading" @click="refreshItems">刷新</NButton>
          <NButton
            size="small"
            type="error"
            :loading="itemsCleaning"
            :disabled="itemsStaleCount === 0"
            @click="cleanupStaleItems">
            清理失效（{{ itemsStaleCount }}）
          </NButton>
        </NSpace>
      </div>

      <NDataTable
        :columns="itemColumns"
        :data="itemsRows"
        :loading="itemsLoading"
        :bordered="false"
        :row-key="(row: ProjectAdminItem) => row.article_id" />
    </NModal>
  </NSpace>
</template>
