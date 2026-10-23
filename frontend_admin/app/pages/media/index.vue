<script setup lang="ts">
import { h } from 'vue'
import { NButton, NCard, NDataTable, NForm, NFormItem, NGrid, NGridItem, NInput, NSpace, NSwitch, NTag, useDialog, useMessage } from 'naive-ui'
import { createBanner, deleteBanners, deleteImages, getBanners, getImages } from '~/services/admin'
import { uploadImageByTask } from '~/services/image'
import type { BannerModel, ImageListItem } from '~/types/api'
import { formatDateTime } from '~/utils/format'

const message = useMessage()
const dialog = useDialog()
const loadingBanners = ref(false)
const loadingImages = ref(false)
const bannerPage = ref(1)
const imagePage = ref(1)
const hasMoreBanners = ref(false)
const hasMoreImages = ref(false)
const banners = ref<BannerModel[]>([])
const images = ref<ImageListItem[]>([])
const bannerFileInputRef = ref<HTMLInputElement | null>(null)

const bannerForm = reactive({
  cover_image_id: '',
  href: '',
  show: true,
})
const bannerCoverPreviewUrl = ref('')
const bannerUploadStage = ref('')
const bannerUploading = ref(false)
const bannerSubmitting = ref(false)

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
    title: '更新时间',
    key: 'updated_at',
    width: 180,
    render: (row: BannerModel) => formatDateTime(row.updated_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row: BannerModel) => h(NButton, { size: 'small', tertiary: true, type: 'error', onClick: () => removeBanner(row) }, { default: () => '删除' }),
  },
]

const imageColumns = [
  { title: 'ID', key: 'id', minWidth: 220, render: (row: ImageListItem) => String(row.id || '-') },
  {
    title: '预览',
    key: 'url',
    width: 180,
    render: (row: ImageListItem) => {
      const src = String(row.url || row.path || '')
      return h('div', { class: 'admin-cover' }, src ? [h('img', { src, alt: String(row.name || row.id || '') })] : [])
    },
  },
  { title: '状态', key: 'status', width: 120, render: (row: ImageListItem) => h(NTag, { size: 'small' }, { default: () => String(row.status || '-') }) },
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
  } catch {
    message.error('轮播列表加载失败')
  } finally {
    loadingBanners.value = false
  }
}

async function refreshImages() {
  loadingImages.value = true
  try {
    const data = await getImages({ page: imagePage.value, limit: 10 })
    images.value = data.list || []
    hasMoreImages.value = Boolean(data.has_more)
  } catch {
    message.error('图片资源加载失败')
  } finally {
    loadingImages.value = false
  }
}

async function submitBanner() {
  if (bannerSubmitting.value) return
  if (bannerUploading.value) {
    message.warning('封面仍在上传中，请稍后再创建轮播')
    return
  }
  if (!bannerForm.cover_image_id) {
    message.warning('请先上传轮播封面')
    return
  }

  bannerSubmitting.value = true
  try {
    await createBanner({
      cover_image_id: bannerForm.cover_image_id || null,
      href: bannerForm.href,
      show: bannerForm.show,
    })
    message.success('轮播已创建')
    Object.assign(bannerForm, { cover_image_id: '', href: '', show: true })
    bannerCoverPreviewUrl.value = ''
    bannerUploadStage.value = ''
    await refreshBanners()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '轮播创建失败')
  } finally {
    bannerSubmitting.value = false
  }
}

function openBannerCoverPicker() {
  if (bannerUploading.value) return
  bannerFileInputRef.value?.click()
}

function clearBannerCoverSelection() {
  bannerForm.cover_image_id = ''
  bannerCoverPreviewUrl.value = ''
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
  if (!file) return

  if (!file.type.startsWith('image/')) {
    message.warning('请选择图片文件作为轮播封面')
    input.value = ''
    return
  }

  bannerUploading.value = true
  bannerUploadStage.value = '开始上传封面…'
  try {
    const uploadResult = await uploadImageByTask(file, {
      onStage: mapBannerUploadStage,
    })
    if (!uploadResult.image_id || !uploadResult.url) {
      throw new Error('上传成功但缺少图片标识')
    }
    bannerForm.cover_image_id = uploadResult.image_id
    bannerCoverPreviewUrl.value = uploadResult.url
    bannerUploadStage.value = '封面上传完成'
    message.success('轮播封面上传成功')
    imagePage.value = 1
    await refreshImages()
  } catch (error) {
    bannerForm.cover_image_id = ''
    bannerCoverPreviewUrl.value = ''
    bannerUploadStage.value = ''
    message.error(error instanceof Error ? error.message : '轮播封面上传失败')
  } finally {
    bannerUploading.value = false
    input.value = ''
  }
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
                    <p class="admin-upload-panel__hint">建议使用横向封面，系统会自动上传并回填图片 ID</p>
                  </div>
                  <div class="admin-upload-panel__actions">
                    <NButton quaternary size="small" :loading="bannerUploading" @click="openBannerCoverPicker">
                      {{ bannerCoverPreviewUrl ? '重新上传' : '选择图片' }}
                    </NButton>
                    <NButton v-if="bannerCoverPreviewUrl || bannerForm.cover_image_id" quaternary size="small" @click="clearBannerCoverSelection">
                      移除
                    </NButton>
                  </div>
                </div>

                <input
                  ref="bannerFileInputRef"
                  type="file"
                  accept="image/*"
                  class="admin-upload-panel__input"
                  @change="handleBannerCoverFileChange" />

                <button
                  type="button"
                  class="admin-upload-panel__picker"
                  :class="{ 'admin-upload-panel__picker--busy': bannerUploading }"
                  @click="openBannerCoverPicker">
                  <img
                    v-if="bannerCoverPreviewUrl"
                    :src="bannerCoverPreviewUrl"
                    alt="轮播封面预览"
                    class="admin-upload-panel__image" />
                  <template v-else>
                    <span class="admin-upload-panel__plus">+</span>
                    <span>上传轮播封面</span>
                    <small>支持 JPG / PNG / WebP</small>
                  </template>
                </button>

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
  </NSpace>
</template>
