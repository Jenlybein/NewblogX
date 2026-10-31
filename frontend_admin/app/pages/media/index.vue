<script setup lang="ts">
import { h } from 'vue'
import { NButton, NCard, NDataTable, NForm, NFormItem, NGrid, NGridItem, NInput, NSpace, NSwitch, NTag, useDialog, useMessage } from 'naive-ui'
import BannerFramingDialog from '~/components/media/BannerFramingDialog.vue'
import { createBanner, deleteBanners, deleteImages, getBanners, getImages, updateBanner } from '~/services/admin'
import { uploadImageByTask } from '~/services/image'
import type { BannerModel, ImageListItem } from '~/types/api'
import {
  buildBannerCreatePayload,
  describeBannerCoverSize,
  readImageSize,
  requiresFraming,
  saveBannerReframe,
  uploadBannerCover,
} from '~/utils/bannerCover'
import { DEFAULT_FRAMING, clampFraming } from '~/utils/framing'
import type { Framing, Size } from '~/utils/framing'
import { formatDateTime } from '~/utils/format'
import { hasNextPage } from '~/utils/pagination'

const message = useMessage()
const dialog = useDialog()
const loadingBanners = ref(false)
const loadingImages = ref(false)
const IMAGE_PAGE_SIZE = 10
const bannerPage = ref(1)
const imagePage = ref(1)
const hasMoreBanners = ref(false)
const hasMoreImages = ref(false)
const imageTotal = ref(0)
const banners = ref<BannerModel[]>([])
const images = ref<ImageListItem[]>([])
const bannerFileInputRef = ref<HTMLInputElement | null>(null)

/**
 * 当前选好的封面（可能已经上传，也可能还在等取景确认）。
 *
 * spec 要求"本地读像素尺寸、不合规不先传"：合规图判定后**立即上传**并持有
 * `image_id`；不合规图在取景确认后自动上传。上传都发生在创建之前，点「创建轮播」
 * 只创建 banner，不重复上传。取消强制取景仍中止本次上传。
 */
interface PendingBannerCover {
  file: File
  /** `URL.createObjectURL(file)`：既是本地预览，也是取景窗的图源。 */
  previewUrl: string
  size: Size
  requiresFraming: boolean
  /** 取景参数；不合规时在确认前一直是 null（创建按钮会拦住）。 */
  framing: Framing | null
  /** 是否已经经过一次取景（决定按钮写"手动取景"还是"调整取景"）。 */
  adjusted: boolean
}

const pendingCover = ref<PendingBannerCover | null>(null)
/** 已经传到对象存储、但轮播还没创建成功时的上传结果（保存失败重试不重传）。 */
const uploadedCover = ref<{ imageId: string; url: string } | null>(null)
const bannerForm = reactive({
  href: '',
  show: true,
  crop: null as Framing | null,
})
const bannerCoverPreviewUrl = computed(() => uploadedCover.value?.url ?? pendingCover.value?.previewUrl ?? '')
const bannerUploadStage = ref('')
const bannerUploading = ref(false)
const bannerSubmitting = ref(false)

/** 列表里每张封面的原图尺寸（按封面地址缓存）；null 表示加载失败，不再重试。 */
const coverSizes = ref<Record<string, Size | null>>({})
/** 同一张封面的并发测量只发一次，避免翻页时重复解码。 */
const coverSizeRequests = new Map<string, Promise<Size | null>>()
/** 正在保存取景的轮播 ID；同一时间只允许一条更新在途。 */
const reframingBannerId = ref('')

/** 上传前取景与列表重新取景共用的弹层状态；确认/取消都只改这里，不提前碰后端。 */
const framingDialog = reactive({
  show: false,
  src: '',
  imageWidth: 0,
  imageHeight: 0,
  framing: DEFAULT_FRAMING as Framing,
})

type FramingTarget = { kind: 'pending' } | { kind: 'banner'; row: BannerModel }

let framingTarget: FramingTarget | null = null
/**
 * 取消是否要中止这次上传。
 *
 * 只有"比例不合规、弹窗强制取景"这一条路径是 true：运营点取消必须等于放弃本次
 * 上传。手动再打开取景窗时取消只是关窗，不推翻已经做出的选择。
 */
let abortOnFramingCancel = false

/**
 * 列表里的原图尺寸与比例标记。
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
    h(
      NTag,
      { size: 'small', type: info.compliant ? 'success' : 'warning' },
      { default: () => (info.compliant ? '16:9 合规' : '比例不合规') },
    ),
  ])
}

const bannerColumns = [
  {
    title: '封面',
    key: 'cover',
    width: 180,
    render: (row: BannerModel) => h('div', { class: 'admin-cover' }, row.cover ? [h('img', { src: row.cover, alt: row.href })] : []),
  },
  { title: '跳转地址', key: 'href', minWidth: 260 },
  {
    title: '展示',
    key: 'show',
    width: 100,
    render: (row: BannerModel) => h(NTag, { type: row.show ? 'success' : 'default', size: 'small' }, { default: () => (row.show ? '展示' : '隐藏') }),
  },
  {
    title: '原图尺寸',
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
              {
                size: 'small',
                tertiary: true,
                type: 'primary',
                loading: reframingBannerId.value === row.id,
                disabled: !row.cover_image_id,
                onClick: () => void openBannerReframe(row),
              },
              { default: () => '重新取景' },
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
    const data = await getBanners({ page: bannerPage.value, limit: 10 })
    banners.value = data.list
    hasMoreBanners.value = data.has_more
    loadCoverSizes(data.list)
  } catch {
    message.error('轮播列表加载失败')
  } finally {
    loadingBanners.value = false
  }
}

/** 按封面地址量原图尺寸；同一地址只发一次，失败结果（null）也缓存。 */
function ensureCoverSize(url?: string | null): Promise<Size | null> {
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

/** 列表刷新后顺手把每行封面的原始尺寸排进解码队列（不阻塞列表本身）。 */
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
  if (pending && pending.requiresFraming && !pending.framing) {
    message.warning('这张图与 16:9 相差超过 1%，请先完成取景再创建轮播')
    return
  }

  bannerSubmitting.value = true
  try {
    // 正常路径在选图（合规）或确认取景（不合规）时已经上传；这里只兜底重试
    // （例如上传失败后运营直接点创建），已上传时不会再传一次。
    const cover = uploadedCover.value ?? (pending ? await uploadPendingBannerCover(pending) : null)
    if (!cover) return

    bannerUploadStage.value = '正在创建轮播…'
    try {
      await createBanner(
        buildBannerCreatePayload({
          coverImageId: cover.imageId,
          href: bannerForm.href,
          show: bannerForm.show,
          framing: pendingCover.value?.framing ?? bannerForm.crop,
        }),
      )
    } catch (error) {
      bannerUploadStage.value = '封面上传完成，轮播保存失败，可修正后重试'
      message.error(error instanceof Error ? `轮播保存失败：${error.message}` : '轮播保存失败，请稍后重试')
      return
    }

    message.success('轮播已创建')
    clearBannerCoverSelection()
    Object.assign(bannerForm, { href: '', show: true })
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
  bannerForm.crop = null
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
  const previewUrl = URL.createObjectURL(file)
  bannerUploadStage.value = '正在读取图片尺寸…'
  bannerUploading.value = true
  try {
    const size = await readImageSize(previewUrl, () => new Image())
    if (!size) {
      throw new Error('读不到这张图的像素尺寸，请换一张图片')
    }

    const needsFraming = requiresFraming(size)
    const pending: PendingBannerCover = {
      file,
      previewUrl,
      size,
      requiresFraming: needsFraming,
      framing: needsFraming ? null : DEFAULT_FRAMING,
      adjusted: false,
    }
    pendingCover.value = pending

    if (needsFraming) {
      bannerUploadStage.value = '比例与 16:9 相差超过 1%，请先取景'
      openPendingFraming(true)
      return
    }

    // 合规＝直接上传（spec 字面要求），不推迟到点「创建轮播」。这样运营少点一次，
    // 且上传失败时本地文件还在，点创建会走 submitBanner 的兜底重试。
    bannerUploadStage.value = `比例合规（${size.width} × ${size.height}），正在上传封面…`
    const uploaded = await uploadPendingBannerCover(pending, '封面上传完成，点击「创建轮播」保存')
    if (!uploaded) {
      bannerUploadStage.value = '封面上传失败，可点击「创建轮播」重试'
    }
  } catch (error) {
    URL.revokeObjectURL(previewUrl)
    bannerUploadStage.value = ''
    message.error(error instanceof Error ? error.message : '读取图片尺寸失败，请换一张图片')
  } finally {
    bannerUploading.value = false
  }
}

function openPendingFraming(required: boolean) {
  const pending = pendingCover.value
  if (!pending) return
  framingTarget = { kind: 'pending' }
  abortOnFramingCancel = required
  framingDialog.src = pending.previewUrl
  framingDialog.imageWidth = pending.size.width
  framingDialog.imageHeight = pending.size.height
  framingDialog.framing = clampFraming(pending.framing)
  framingDialog.show = true
}

/** 「手动取景」：已上传或待上传的合规图都能再调构图，保存时随 crop 创建。 */
function openBannerFraming() {
  if (pendingCover.value) {
    openPendingFraming(false)
    return
  }
  message.warning('请先选择封面图片')
}

/**
 * 列表行「重新取景」：带上当前取景参数打开同一个取景窗。
 *
 * 历史图没有取景记录时 `crop` 已经由后端回传缺省（居中、缩放 1），这里再做一次
 * `clampFraming` 只是兜底，并不改写已有值。尺寸读不到就不开窗——没有原图宽高，
 * 拖动换算出的焦点没有意义。
 */
async function openBannerReframe(row: BannerModel) {
  if (reframingBannerId.value) return
  if (!row.cover_image_id) {
    message.warning('这张轮播缺少封面图片 ID，无法重新取景')
    return
  }

  const size = await ensureCoverSize(row.cover)
  if (!size) {
    message.error('读不到这张封面的原图尺寸，暂时无法重新取景')
    return
  }

  framingTarget = { kind: 'banner', row }
  abortOnFramingCancel = false
  framingDialog.src = row.cover
  framingDialog.imageWidth = size.width
  framingDialog.imageHeight = size.height
  framingDialog.framing = clampFraming(row.crop ?? DEFAULT_FRAMING)
  framingDialog.show = true
}

/** 保存重新取景：走更新接口；失败只提示，不把界面假装成已保存。 */
async function persistBannerReframe(row: BannerModel, framing: Framing) {
  if (reframingBannerId.value) return
  reframingBannerId.value = row.id
  try {
    const saved = await saveBannerReframe(row, framing, (id, payload) => updateBanner(id, payload))
    if (!saved) {
      message.error('这张轮播缺少封面图片 ID，无法保存取景')
      return
    }
    message.success('取景已更新，阅读站立即生效')
    await refreshBanners()
  } catch (error) {
    message.error(error instanceof Error ? `取景保存失败：${error.message}` : '取景保存失败，请稍后重试')
  } finally {
    reframingBannerId.value = ''
  }
}

function handleFramingConfirm(framing: Framing) {
  const target = framingTarget
  framingTarget = null
  abortOnFramingCancel = false
  if (!target) return

  const safe = clampFraming(framing)
  if (target.kind === 'banner') {
    void persistBannerReframe(target.row, safe)
    return
  }

  const pending = pendingCover.value
  if (!pending) return
  pending.framing = safe
  pending.adjusted = true
  bannerForm.crop = safe

  // 没上传成功的图（强制取景确认 / 合规图上传失败后手动重试）在此自动补传；
  // 已经上传过的图只更新本地取景参数，点「创建轮播」时随 crop 一次提交。
  if (!uploadedCover.value) {
    bannerUploadStage.value = '取景已确认，正在上传封面…'
    void uploadPendingBannerCover(pending, '封面上传完成，点击「创建轮播」保存')
    return
  }
  bannerUploadStage.value = '取景已更新，点击「创建轮播」保存'
}

/**
 * 取消取景。
 *
 * 强制取景路径（比例不合规）取消 = 中止本次上传：本地预览与待传文件一起丢掉，
 * 没有上传、没有图片记录、没有轮播。手动再打开取景窗时取消只是关窗。
 * 「重新取景」取消连写请求都不经过（保存只在 confirm 分支里发生）。
 */
function handleFramingCancel() {
  const target = framingTarget
  const shouldAbort = abortOnFramingCancel
  framingTarget = null
  abortOnFramingCancel = false
  if (target?.kind !== 'pending' || !shouldAbort) return

  clearBannerCoverSelection()
  message.info('已取消取景，本次上传未开始：没有产生图片，也没有产生轮播')
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
                    <p class="admin-upload-panel__hint">推荐比例 16:9：与 16:9 相差 ≤1% 会立即上传；超出 1% 先取景，确认后自动上传</p>
                  </div>
                  <div class="admin-upload-panel__actions">
                    <NButton
                      v-if="pendingCover"
                      quaternary
                      size="small"
                      :disabled="bannerSubmitting || bannerUploading"
                      @click="openBannerFraming">
                      {{ pendingCover.adjusted ? '调整取景' : '手动取景' }}
                    </NButton>
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
                    <small>推荐 16:9 · 支持 JPG / PNG / WebP</small>
                  </template>
                </button>

                <div v-if="pendingCover" class="admin-upload-panel__meta">
                  <span>{{ pendingCover.size.width }} × {{ pendingCover.size.height }}</span>
                  <NTag :type="pendingCover.requiresFraming ? 'warning' : 'success'" size="small">
                    {{ pendingCover.requiresFraming ? (pendingCover.framing ? '已取景' : '比例不合规，需取景') : '16:9 合规' }}
                  </NTag>
                </div>

                <p v-if="bannerUploadStage" class="admin-upload-panel__stage">{{ bannerUploadStage }}</p>
              </div>
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

    <BannerFramingDialog
      v-model:show="framingDialog.show"
      :src="framingDialog.src"
      :image-width="framingDialog.imageWidth"
      :image-height="framingDialog.imageHeight"
      :framing="framingDialog.framing"
      @confirm="handleFramingConfirm"
      @cancel="handleFramingCancel" />
  </NSpace>
</template>
