<script setup lang="ts">
import { h } from 'vue'
import { NButton, NCard, NDataTable, NForm, NFormItem, NGrid, NGridItem, NInput, NSelect, NSpace, NSwitch, NTag, useDialog, useMessage } from 'naive-ui'
import { createBanner, deleteBanners, deleteImages, getBanners, getImages, updateBanner } from '~/services/admin'
import { uploadImageByTask } from '~/services/image'
import type { BannerModel, ImageListItem } from '~/types/api'
import {
  buildBannerCreatePayload,
  describeBannerCoverSize,
  readImageSize,
  saveBannerTitle,
  uploadBannerCover,
} from '~/utils/bannerCover'
import type { ImageSize } from '~/utils/bannerCover'
import { bannerTitleCell, bannerTitleText } from '~/utils/bannerTitle'
import { BANNER_SHOW_FILTER_OPTIONS, buildBannerListQuery } from '~/utils/bannerList'
import type { BannerShowFilter } from '~/utils/bannerList'
import {
  COVER_SHAPE_OPTIONS,
  COVER_SHAPE_RATIO_TEXT,
  COVER_SHAPE_SIZE_TEXT,
  shapeForUpload,
} from '~/utils/imageShaping'
import { formatDateTime } from '~/utils/format'
import { hasNextPage } from '~/utils/pagination'

const message = useMessage()
const dialog = useDialog()
const loadingBanners = ref(false)
const loadingImages = ref(false)
const IMAGE_PAGE_SIZE = 10
const BANNER_PAGE_SIZE = 10
const bannerPage = ref(1)
const bannerShowFilter = ref<BannerShowFilter>('all')
const imagePage = ref(1)
const hasMoreBanners = ref(false)
const hasMoreImages = ref(false)
const imageTotal = ref(0)
const banners = ref<BannerModel[]>([])
const images = ref<ImageListItem[]>([])
const bannerFileInputRef = ref<HTMLInputElement | null>(null)

/**
 * 当前选好的封面（上传前已经定形成 16:9）。
 *
 * 票 200 起：选中的原图**先在客户端定形**（居中裁成 16:9、降采样到 1600×900），
 * 上传的是定形产物，原图不进入上传链路。定形产物落地后立即可预览（本地 object URL），
 * 随后自动上传并持有 `image_id`；点「创建轮播」只创建 banner，不重复上传。
 * 票 204 起没有取景步骤：定形完就上传，读者侧固定居中。
 */
interface PendingBannerCover {
  /** 定形后的新文件（原图已被替换掉，不会上传）。 */
  file: File
  /** 定形产物的 `URL.createObjectURL`：本地预览。 */
  previewUrl: string
  /** 定形产物的像素尺寸（固定 1600×900）。 */
  size: ImageSize
}

const pendingCover = ref<PendingBannerCover | null>(null)
/** 已经传到对象存储、但轮播还没创建成功时的上传结果（保存失败重试不重传）。 */
const uploadedCover = ref<{ imageId: string; url: string } | null>(null)
const bannerForm = reactive({
  title: '',
  href: '',
  show: true,
})
const bannerCoverPreviewUrl = computed(() => uploadedCover.value?.url ?? pendingCover.value?.previewUrl ?? '')
const bannerUploadStage = ref('')
const bannerUploading = ref(false)
const bannerSubmitting = ref(false)

/** 列表里每张封面的尺寸（按封面地址缓存）；null 表示加载失败，不再重试。 */
const coverSizes = ref<Record<string, ImageSize | null>>({})
/** 同一张封面的并发测量只发一次，避免翻页时重复解码。 */
const coverSizeRequests = new Map<string, Promise<ImageSize | null>>()

/**
 * 列表里的封面尺寸读数。
 *
 * 尺寸是从封面地址解码出来的（BannerItem 契约没有宽高字段），所以刚加载时是
 * "读取中…"；读不到显示"尺寸未知"，不拿 0×0 冒充。
 */
function renderCoverSize(row: BannerModel) {
  const cached = row.cover && Object.prototype.hasOwnProperty.call(coverSizes.value, row.cover) ? coverSizes.value[row.cover] : undefined
  if (cached === undefined) {
    return h('span', { class: 'admin-cover-size__muted' }, '读取中…')
  }

  const info = describeBannerCoverSize(cached)
  if (!info) {
    return h('span', { class: 'admin-cover-size__muted' }, '尺寸未知')
  }
  return h('div', { class: 'admin-cover-size' }, [
    h('span', { class: 'admin-cover-size__pixels' }, info.label),
  ])
}

const bannerColumns = [
  {
    title: '封面',
    key: 'cover',
    width: 180,
    render: (row: BannerModel) => h('div', { class: 'admin-cover' }, row.cover ? [h('img', { src: row.cover, alt: row.href })] : []),
  },
  {
    title: '标题',
    key: 'title',
    minWidth: 200,
    render: (row: BannerModel) => {
      const text = bannerTitleCell(row.title)
      const empty = text === '未填写'
      return h('span', { class: empty ? 'admin-cover-size__muted' : undefined }, text)
    },
  },
  { title: '跳转地址', key: 'href', minWidth: 260 },
  {
    title: '展示',
    key: 'show',
    width: 100,
    render: (row: BannerModel) => h(NTag, { type: row.show ? 'success' : 'default', size: 'small' }, { default: () => (row.show ? '展示' : '隐藏') }),
  },
  {
    title: '封面尺寸',
    key: 'cover_size',
    width: 190,
    render: (row: BannerModel) => renderCoverSize(row),
  },
  {
    title: '更新时间',
    key: 'updated_at',
    width: 180,
    render: (row: BannerModel) => formatDateTime(row.updated_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 190,
    render: (row: BannerModel) =>
      h(
        NSpace,
        { size: 4, justify: 'end' },
        {
          default: () => [
            h(
              NButton,
              { size: 'small', tertiary: true, onClick: () => openTitleEditor(row) },
              { default: () => '编辑标题' },
            ),
            h(NButton, { size: 'small', tertiary: true, type: 'error', onClick: () => removeBanner(row) }, { default: () => '删除' }),
          ],
        },
      ),
  },
]

const imageColumns = [
  { title: 'ID', key: 'id', minWidth: 220, render: (row: ImageListItem) => String(row.id || '-') },
  {
    title: '预览',
    key: 'url',
    width: 180,
    render: (row: ImageListItem) => {
      const src = String(row.url || '')
      return h('div', { class: 'admin-cover' }, src ? [h('img', { src, alt: String(row.file_name || row.id || '') })] : [])
    },
  },
  { title: '状态', key: 'status', width: 120, render: (row: ImageListItem) => h(NTag, { size: 'small' }, { default: () => String(row.status ?? '-') }) },
  {
    title: '创建时间',
    key: 'created_at',
    width: 180,
    render: (row: ImageListItem) => formatDateTime(String(row.created_at || '')),
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row: ImageListItem) =>
      h(
        NButton,
        { size: 'small', tertiary: true, type: 'error', disabled: !row.id, onClick: () => removeImage(row) },
        { default: () => '删除' },
      ),
  },
]

async function refreshBanners() {
  loadingBanners.value = true
  try {
    const data = await getBanners(buildBannerListQuery(bannerPage.value, BANNER_PAGE_SIZE, bannerShowFilter.value))
    banners.value = data.list
    hasMoreBanners.value = data.has_more
    loadCoverSizes(data.list)
  } catch {
    message.error('轮播列表加载失败')
  } finally {
    loadingBanners.value = false
  }
}

/** 按封面地址量尺寸；同一地址只发一次，失败结果（null）也缓存。 */
function ensureCoverSize(url?: string | null): Promise<ImageSize | null> {
  if (!url) return Promise.resolve(null)
  if (Object.prototype.hasOwnProperty.call(coverSizes.value, url)) {
    return Promise.resolve(coverSizes.value[url] ?? null)
  }
  const pendingRequest = coverSizeRequests.get(url)
  if (pendingRequest) return pendingRequest

  const request = readImageSize(url, () => new Image()).then((size) => {
    coverSizes.value[url] = size
    coverSizeRequests.delete(url)
    // 尺寸是图片解码后才到位的，复制一份 data 让表格重新渲染。
    banners.value = [...banners.value]
    return size
  })
  coverSizeRequests.set(url, request)
  return request
}

/** 列表刷新后顺手把每行封面的尺寸排进解码队列（不阻塞列表本身）。 */
function loadCoverSizes(rows: BannerModel[]) {
  const urls = new Set(rows.map((row) => row.cover).filter((url) => Boolean(url)))
  for (const url of urls) {
    void ensureCoverSize(url)
  }
}

async function refreshImages() {
  loadingImages.value = true
  try {
    const data = await getImages({ page: imagePage.value, limit: IMAGE_PAGE_SIZE })
    images.value = data.list || []
    // `/images` 是 page/count 分页（后端 `res.OkWithList` → `{list, count}`），没有 has_more。
    // 这里曾经写 `Boolean(data.has_more)`，字段恒为 undefined → 按钮永久禁用、只能看第一页。
    imageTotal.value = data.count ?? 0
    hasMoreImages.value = hasNextPage(imagePage.value, IMAGE_PAGE_SIZE, imageTotal.value)
  } catch {
    message.error('图片资源加载失败')
  } finally {
    loadingImages.value = false
  }
}

async function submitBanner() {
  if (bannerSubmitting.value || bannerUploading.value) return

  const pending = pendingCover.value
  if (!pending && !uploadedCover.value) {
    message.warning('请先选择轮播封面')
    return
  }

  bannerSubmitting.value = true
  try {
    // 正常路径在选图时已经上传；这里只兜底重试（例如上传失败后运营直接点创建），
    // 已上传时不会再传一次。
    const cover = uploadedCover.value ?? (pending ? await uploadPendingBannerCover(pending) : null)
    if (!cover) return

    bannerUploadStage.value = '正在创建轮播…'
    try {
      await createBanner(
        buildBannerCreatePayload({
          coverImageId: cover.imageId,
          title: bannerForm.title,
          href: bannerForm.href,
          show: bannerForm.show,
        }),
      )
    } catch (error) {
      bannerUploadStage.value = '封面上传完成，轮播保存失败，可修正后重试'
      message.error(error instanceof Error ? `轮播保存失败：${error.message}` : '轮播保存失败，请稍后重试')
      return
    }

    message.success('轮播已创建')
    clearBannerCoverSelection()
    Object.assign(bannerForm, { title: '', href: '', show: true })
    await refreshBanners()
  } finally {
    bannerSubmitting.value = false
  }
}

/** 上传封面；失败时保留本地文件，修正后可重试。已上传则直接复用结果。 */
async function uploadPendingBannerCover(pending: PendingBannerCover, nextStage = '封面上传完成') {
  if (uploadedCover.value) return uploadedCover.value
  bannerUploading.value = true
  bannerUploadStage.value = '开始上传封面…'
  try {
    const cover = await uploadBannerCover(pending.file, (file) => uploadImageByTask(file, { onStage: mapBannerUploadStage }))
    uploadedCover.value = cover
    bannerUploadStage.value = nextStage
    imagePage.value = 1
    await refreshImages()
    return cover
  } catch (error) {
    bannerUploadStage.value = ''
    message.error(error instanceof Error ? `封面上传失败：${error.message}` : '封面上传失败，请稍后重试')
    return null
  } finally {
    bannerUploading.value = false
  }
}

function openBannerCoverPicker() {
  if (bannerUploading.value || bannerSubmitting.value) return
  bannerFileInputRef.value?.click()
}

function clearBannerCoverSelection() {
  if (pendingCover.value) {
    URL.revokeObjectURL(pendingCover.value.previewUrl)
  }
  pendingCover.value = null
  uploadedCover.value = null
  bannerUploadStage.value = ''
  if (bannerFileInputRef.value) {
    bannerFileInputRef.value.value = ''
  }
}

function mapBannerUploadStage(stage: 'hashing' | 'creating_task' | 'uploading_to_qiniu' | 'polling_status') {
  const stageMap = {
    hashing: '正在计算文件指纹…',
    creating_task: '正在创建上传任务…',
    uploading_to_qiniu: '正在上传到对象存储…',
    polling_status: '正在确认上传状态…',
  } as const
  bannerUploadStage.value = stageMap[stage]
}

async function handleBannerCoverFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return

  if (!file.type.startsWith('image/')) {
    message.warning('请选择图片文件作为轮播封面')
    return
  }

  clearBannerCoverSelection()
  bannerUploadStage.value = '正在定形成 16:9 封面…'
  bannerUploading.value = true
  try {
    // 上传前定形（票 200）：居中裁成 16:9、降采样到 1600×900，产出新文件。
    // 上传只发定形产物，原图不进入链路（GLOSSARY「定形」）。
    const shaped = await shapeForUpload(file, COVER_SHAPE_OPTIONS)
    const pending: PendingBannerCover = {
      file: shaped.file,
      previewUrl: shaped.previewUrl,
      size: shaped.size,
    }
    pendingCover.value = pending

    // 定形结果先落到预览上（运营看到的就是即将上传的那张图），再自动上传。
    bannerUploadStage.value = `已定形成 ${COVER_SHAPE_SIZE_TEXT}（${COVER_SHAPE_RATIO_TEXT}），正在上传封面…`
    const uploaded = await uploadPendingBannerCover(pending, '封面上传完成，点击「创建轮播」保存')
    if (!uploaded) {
      bannerUploadStage.value = '封面上传失败，可点击「创建轮播」重试'
    }
  } catch (error) {
    bannerUploadStage.value = ''
    message.error(error instanceof Error ? error.message : '封面定形失败，请换一张图片')
  } finally {
    bannerUploading.value = false
  }
}

/**
 * 列表行「编辑标题」：一个只改标题的输入弹窗。
 *
 * 保存走更新接口（整体替换），请求体由 `saveBannerTitle`
 * 带上该行原有的封面 / 跳转地址 / 展示状态，避免只改标题却把别的字段清掉。
 * 标题是纯文本：输入框就是唯一来源，不做任何富文本解析。
 */
function openTitleEditor(row: BannerModel) {
  const draft = ref(bannerTitleText(row.title))
  dialog.info({
    title: '编辑轮播标题',
    content: () =>
      h(NInput, {
        value: draft.value,
        maxlength: 256,
        showCount: true,
        placeholder: '给这一屏起个标题（可留空）',
        'onUpdate:value': (value: string) => {
          draft.value = value
        },
      }),
    positiveText: '保存',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        const saved = await saveBannerTitle(row, draft.value, (id, payload) => updateBanner(id, payload))
        if (!saved) {
          message.error('这张轮播缺少封面图片 ID，无法保存标题')
          return false
        }
        message.success('标题已更新')
        await refreshBanners()
        return true
      } catch (error) {
        message.error(error instanceof Error ? `标题保存失败：${error.message}` : '标题保存失败，请稍后重试')
        return false
      }
    },
  })
}

function removeBanner(row: BannerModel) {
  dialog.warning({
    title: '删除轮播',
    content: row.href,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      await deleteBanners([row.id])
      message.success('已删除')
      await refreshBanners()
    },
  })
}

function removeImage(row: ImageListItem) {
  if (!row.id) return
  dialog.warning({
    title: '删除图片',
    content: String(row.id),
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      await deleteImages([String(row.id)])
      message.success('图片已删除')
      await refreshImages()
    },
  })
}

watch(bannerPage, refreshBanners)
watch(bannerShowFilter, () => {
  if (bannerPage.value === 1) {
    void refreshBanners()
    return
  }
  // 页码 watcher 负责刷新，避免筛选切换时重复请求。
  bannerPage.value = 1
})
watch(imagePage, refreshImages)
onMounted(() => {
  void refreshBanners()
  void refreshImages()
})
</script>

<template>
  <NSpace vertical :size="20">
    <NGrid :cols="24" :x-gap="20" :y-gap="20" responsive="screen">
      <NGridItem :span="9">
        <NCard class="admin-card" title="新增轮播">
          <NForm label-placement="top" :show-feedback="false" class="admin-banner-form">
            <NFormItem>
              <div class="admin-upload-panel">
                <div class="admin-upload-panel__head">
                  <div>
                    <div class="admin-upload-panel__title">封面图片</div>
                    <p class="admin-upload-panel__hint">
                      上传前自动裁成 {{ COVER_SHAPE_RATIO_TEXT }}（{{ COVER_SHAPE_SIZE_TEXT }}），只上传定形后的图；读者侧固定居中显示
                    </p>
                  </div>
                  <div class="admin-upload-panel__actions">
                    <NButton quaternary size="small" :loading="bannerUploading" @click="openBannerCoverPicker">
                      {{ bannerCoverPreviewUrl ? '重新选择' : '选择图片' }}
                    </NButton>
                    <NButton
                      v-if="bannerCoverPreviewUrl"
                      quaternary
                      size="small"
                      :disabled="bannerSubmitting || bannerUploading"
                      @click="clearBannerCoverSelection">
                      移除
                    </NButton>
                  </div>
                </div>

                <input
                  ref="bannerFileInputRef"
                  type="file"
                  accept="image/*"
                  class="admin-upload-panel__input"
                  @change="handleBannerCoverFileChange">

                <button
                  type="button"
                  class="admin-upload-panel__picker"
                  :class="{ 'admin-upload-panel__picker--busy': bannerUploading }"
                  @click="openBannerCoverPicker">
                  <img
                    v-if="bannerCoverPreviewUrl"
                    :src="bannerCoverPreviewUrl"
                    alt="轮播封面预览"
                    class="admin-upload-panel__image">
                  <template v-else>
                    <span class="admin-upload-panel__plus">+</span>
                    <span>上传轮播封面</span>
                    <small>{{ COVER_SHAPE_RATIO_TEXT }} · {{ COVER_SHAPE_SIZE_TEXT }} · 支持 JPG / PNG / WebP</small>
                  </template>
                </button>

                <div v-if="pendingCover" class="admin-upload-panel__meta">
                  <span>{{ pendingCover.size.width }} × {{ pendingCover.size.height }}</span>
                  <NTag type="success" size="small">已定形 {{ COVER_SHAPE_RATIO_TEXT }}</NTag>
                </div>

                <p v-if="bannerUploadStage" class="admin-upload-panel__stage">{{ bannerUploadStage }}</p>
              </div>
            </NFormItem>
            <NFormItem label="标题">
              <NInput
                v-model:value="bannerForm.title"
                :maxlength="256"
                show-count
                placeholder="这一屏讲什么（可留空，空标题在读者站不显示文字区）" />
            </NFormItem>
            <NFormItem label="跳转地址"><NInput v-model:value="bannerForm.href" placeholder="https://..." /></NFormItem>
            <NFormItem>
              <div class="admin-switch-row">
                <div>
                  <div class="admin-switch-row__title">立即展示</div>
                  <p class="admin-switch-row__hint">关闭后会先创建为隐藏轮播，稍后可再启用。</p>
                </div>
                <NSwitch v-model:value="bannerForm.show" />
              </div>
            </NFormItem>
            <NButton type="primary" block :loading="bannerSubmitting" :disabled="bannerUploading" @click="submitBanner">创建轮播</NButton>
          </NForm>
        </NCard>
      </NGridItem>

      <NGridItem :span="15">
        <NCard class="admin-card" title="轮播运营">
          <template #header-extra>
            <NSelect
              v-model:value="bannerShowFilter"
              :options="BANNER_SHOW_FILTER_OPTIONS"
              aria-label="展示状态筛选"
              class="w-40" />
          </template>
          <NDataTable :columns="bannerColumns" :data="banners" :loading="loadingBanners" :bordered="false" />
          <div class="mt-4 flex justify-end gap-2">
            <NButton :disabled="bannerPage <= 1" @click="bannerPage--">上一页</NButton>
            <NButton :disabled="!hasMoreBanners" @click="bannerPage++">下一页</NButton>
          </div>
        </NCard>
      </NGridItem>
    </NGrid>

    <NCard class="admin-card" title="图片资源库">
      <NDataTable :columns="imageColumns" :data="images" :loading="loadingImages" :bordered="false" />
      <div class="mt-4 flex justify-end gap-2">
        <NButton :disabled="imagePage <= 1" @click="imagePage--">上一页</NButton>
        <NButton :disabled="!hasMoreImages" @click="imagePage++">下一页</NButton>
      </div>
    </NCard>
  </NSpace>
</template>
