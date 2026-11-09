<script setup lang="ts">
/**
 * 新增 / 编辑轮播的同一张表单卡（票 330）。
 *
 * 为什么合成一个组件：轮播的"新增"和"编辑"字段完全一样（封面、标题、跳转地址、
 * 是否展示），差别只有"提交到哪个接口"和"打开时要不要回填"。以前新增是一张常驻
 * 的卡片、编辑只有一个改标题的弹窗，结果是**建完之后封面 / 跳转地址 / 展示状态
 * 全都改不了**——只能删了重建。
 *
 * 三处刻意的行为：
 *   1. **弹窗而不是常驻**：新增入口收成一个按钮，点开才出现表单；宽度受控（560px），
 *      不铺满全屏。
 *   2. **编辑可以只改一项**：更新接口是整体替换，所以提交时把该行其余字段一起带上；
 *      没换封面就沿用原 `cover_image_id`，不必为了改标题重传一张图。
 *   3. **缺 `cover_image_id` 的历史行必须换封面**：更新接口要求这个字段，凭空编一个
 *      只会失败；表单会明确要求先选一张新封面，而不是发一个坏请求。
 *
 * 封面仍是"上传前定形"（16:9 / 1600×900，与读者站同一份 `imageShaping`）：
 * 比例不合规才弹裁剪窗，取消即中止本次上传、保留原有封面。
 */
import { computed, reactive, ref, watch } from "vue";
import { NButton, NForm, NFormItem, NInput, NModal, NSwitch, useMessage } from "naive-ui";

import ImageCropDialog from "~/components/media/ImageCropDialog.vue";
import { useImageCropUpload, type ImageFramingRequest } from "~/composables/useImageCropUpload";
import { createBanner, updateBanner } from "~/services/admin";
import { uploadImageByTask } from "~/services/image";
import type { BannerModel } from "~/types/api";
import { buildBannerCreatePayload, uploadBannerCover, type ImageSize } from "~/utils/bannerCover";
import { bannerTitleText } from "~/utils/bannerTitle";
import {
  COVER_SHAPE_OPTIONS,
  COVER_SHAPE_RATIO_TEXT,
  COVER_SHAPE_SIZE_TEXT,
  COVER_SHAPE_TARGET,
  type ShapeFramingState,
} from "~/utils/imageShaping";

const props = withDefaults(
  defineProps<{
    show: boolean;
    /** create：新建一条；edit：改已有那条（需要 banner）。 */
    mode?: "create" | "edit";
    /** 编辑的目标行；create 时忽略。 */
    banner?: BannerModel | null;
  }>(),
  { mode: "create", banner: null },
);

const emit = defineEmits<{
  "update:show": [value: boolean];
  /** 保存成功（父级据此刷新列表）。 */
  saved: [];
  /** 用户主动关闭（取消 / 点遮罩）。 */
  cancel: [];
}>();

const message = useMessage();

const isEdit = computed(() => props.mode === "edit" && props.banner !== null);
const modalTitle = computed(() => (isEdit.value ? "编辑轮播" : "新增轮播"));
const submitText = computed(() => (isEdit.value ? "保存修改" : "创建轮播"));

const form = reactive({ title: "", href: "", show: true });

/** 定形后、尚未上传完的封面（本地预览用 object URL）。 */
interface PendingCover {
  file: File;
  previewUrl: string;
  size: ImageSize;
}

const pendingCover = ref<PendingCover | null>(null);
/** 已传到对象存储的结果（保存失败重试不重传）。 */
const uploadedCover = ref<{ imageId: string; url: string } | null>(null);
/**
 * 编辑时该行原有的封面。
 *
 * 没换封面就用它提交：更新接口要 `cover_image_id`，而"只改标题"不该逼人重传一张图。
 * 历史行的 `cover_image_id` 可能缺失（数据损坏），那时这里是 null，表单会要求换封面。
 */
const existingCover = ref<{ imageId: string; url: string } | null>(null);

const cropRequest = ref<ImageFramingRequest | null>(null);
const cropPreviewUrl = ref("");
const stage = ref("");
const uploading = ref(false);
const submitting = ref(false);
const fileInputRef = ref<HTMLInputElement | null>(null);

/**
 * 打开时这一行原本有没有可用的封面标识。
 *
 * 用来区分两种"没有封面可提交"：历史行本来就没有（得说清是数据问题，不是漏填），
 * 还是运营刚点了「移除」把自己的封面撤了（提示重新选一张就够）。
 */
const rowHadCover = ref(false);

const coverPreviewUrl = computed(
  () => uploadedCover.value?.url ?? pendingCover.value?.previewUrl ?? existingCover.value?.url ?? "",
);

/** 现有封面之外还能用的图片标识：编辑时没有新图也能提交。 */
const usableCoverImageId = computed(() => uploadedCover.value?.imageId ?? existingCover.value?.imageId ?? "");

function createPreviewUrl(blob: Blob): string {
  try {
    return typeof URL.createObjectURL === "function" ? URL.createObjectURL(blob) : "";
  } catch {
    return "";
  }
}

function revokePreviewUrl(url: string) {
  if (!url) return;
  try {
    if (typeof URL.revokeObjectURL === "function") URL.revokeObjectURL(url);
  } catch {
    // 已释放或环境不支持：忽略。
  }
}

function resetCrop() {
  const url = cropPreviewUrl.value;
  cropPreviewUrl.value = "";
  cropRequest.value = null;
  revokePreviewUrl(url);
}

/** 清掉本次选的封面，回到"只有原有封面"的状态。 */
function clearSelectedCover() {
  if (pendingCover.value) revokePreviewUrl(pendingCover.value.previewUrl);
  pendingCover.value = null;
  uploadedCover.value = null;
  stage.value = "";
  resetCrop();
  if (fileInputRef.value) fileInputRef.value.value = "";
}

/** 按打开时的模式回填：新建清空，编辑带入该行的四个字段。 */
function resetForm() {
  const row = isEdit.value ? props.banner : null;
  form.title = row ? bannerTitleText(row.title) : "";
  form.href = row?.href ?? "";
  form.show = row ? Boolean(row.show) : true;

  const imageId = row?.cover_image_id ? String(row.cover_image_id) : "";
  existingCover.value = imageId ? { imageId, url: row?.cover ?? "" } : null;
  rowHadCover.value = Boolean(imageId);

  clearSelectedCover();
}

watch(
  () => props.show,
  (open) => {
    if (open) {
      resetForm();
      return;
    }
    // 关闭时释放本地预览，避免每次开关都留一个 object URL。
    clearSelectedCover();
    existingCover.value = null;
  },
  { immediate: true },
);

/** 上传封面；失败时保留本地文件，修正后可重试。已上传则直接复用结果。 */
async function uploadPending(pending: PendingCover): Promise<{ imageId: string; url: string } | null> {
  if (uploadedCover.value) return uploadedCover.value;
  uploading.value = true;
  stage.value = "开始上传封面…";
  try {
    const cover = await uploadBannerCover(pending.file, (file) =>
      uploadImageByTask(file, { onStage: mapUploadStage, purpose: "cover" }),
    );
    uploadedCover.value = cover;
    stage.value = "封面上传完成";
    return cover;
  } catch (error) {
    stage.value = "";
    message.error(error instanceof Error ? `封面上传失败：${error.message}` : "封面上传失败，请稍后重试");
    return null;
  } finally {
    uploading.value = false;
  }
}

function mapUploadStage(stageKey: "hashing" | "creating_task" | "uploading_to_qiniu" | "polling_status") {
  const stageMap = {
    hashing: "正在计算文件指纹…",
    creating_task: "正在创建上传任务…",
    uploading_to_qiniu: "正在上传到对象存储…",
    polling_status: "正在确认上传状态…",
  } as const;
  stage.value = stageMap[stageKey];
}

/**
 * 封面上传顺序与其余封面入口同源（`useImageCropUpload`）：
 * 与 16:9 相差 ≤1% → 自动居中定形后直接上传；
 * 相差 >1% → 只打开裁剪窗，不产出文件、不上传；取消即中止。
 */
const cropFlow = useImageCropUpload({
  options: COVER_SHAPE_OPTIONS,
  requestFraming: (request) => {
    cropRequest.value = request;
    cropPreviewUrl.value = createPreviewUrl(request.file);
    stage.value = `图片比例不符合 ${COVER_SHAPE_RATIO_TEXT}，请选择要露出的区域`;
  },
  upload: async (shaped) => {
    const pending: PendingCover = { file: shaped.file, previewUrl: shaped.previewUrl, size: shaped.size };
    pendingCover.value = pending;
    stage.value = `已定形成 ${COVER_SHAPE_SIZE_TEXT}（${COVER_SHAPE_RATIO_TEXT}），正在上传封面…`;
    if (!(await uploadPending(pending))) {
      stage.value = "封面上传失败，可重新选择或保存时重试";
    }
  },
});

function openFilePicker() {
  if (uploading.value || submitting.value) return;
  fileInputRef.value?.click();
}

async function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  if (!file.type.startsWith("image/")) {
    message.warning("请选择图片文件作为轮播封面");
    return;
  }

  clearSelectedCover();
  stage.value = "正在检查图片比例…";
  uploading.value = true;
  try {
    await cropFlow.select(file);
  } catch (error) {
    stage.value = "";
    message.error(error instanceof Error ? error.message : "封面定形失败，请换一张图片");
  } finally {
    uploading.value = false;
  }
}

async function confirmCrop(framing: ShapeFramingState) {
  const request = cropRequest.value;
  if (!request) return;
  resetCrop();
  stage.value = `正在按选定区域定形成 ${COVER_SHAPE_RATIO_TEXT} 封面…`;
  uploading.value = true;
  try {
    await cropFlow.confirm(request, framing);
  } catch (error) {
    stage.value = "";
    message.error(error instanceof Error ? error.message : "封面定形失败，请换一张图片");
  } finally {
    uploading.value = false;
  }
}

function cancelCrop() {
  resetCrop();
  stage.value = "已取消本次上传，保留现有封面";
}

/** 提交：创建或整体替换更新。没有可用图片标识就当场拦住，不发坏请求。 */
async function submit() {
  if (submitting.value || uploading.value) return;

  const pending = pendingCover.value;
  if (pending && !uploadedCover.value) {
    const uploaded = await uploadPending(pending);
    if (!uploaded && !existingCover.value) {
      message.error("封面上传失败，请重新选择一张");
      return;
    }
    if (!uploaded) message.warning("新封面没传上去，这次会沿用原封面");
  }

  const coverImageId = usableCoverImageId.value;
  if (!coverImageId) {
    if (isEdit.value && !rowHadCover.value) {
      // 历史行本来就没有封面标识：说清是数据问题，而不是让运营以为自己漏填了。
      message.error("这一行缺少封面图片标识（历史数据），请先选择一张新封面再保存");
    } else {
      message.error("请先选择轮播封面");
    }
    return;
  }

  submitting.value = true;
  try {
    const payload = buildBannerCreatePayload({
      coverImageId,
      title: form.title,
      href: form.href,
      show: form.show,
    });

    if (isEdit.value && props.banner?.id) {
      await updateBanner(props.banner.id, payload);
      message.success("轮播已更新");
    } else {
      await createBanner(payload);
      message.success("轮播已创建");
    }

    emit("update:show", false);
    emit("saved");
  } catch (error) {
    message.error(error instanceof Error ? `保存失败：${error.message}` : "保存失败，请稍后重试");
  } finally {
    submitting.value = false;
  }
}

function close() {
  if (submitting.value || uploading.value) return;
  emit("update:show", false);
  emit("cancel");
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="modalTitle"
    class="banner-form-modal"
    :bordered="false"
    :mask-closable="!submitting && !uploading"
    @update:show="value => { if (!value) close() }"
  >
    <NForm class="admin-banner-form" label-placement="top" :show-feedback="false" :disabled="submitting">
      <NFormItem label="封面图片">
        <div class="banner-form__cover" data-role="banner-form-cover">
          <div class="banner-form__preview">
            <img v-if="coverPreviewUrl" :src="coverPreviewUrl" alt="轮播封面预览">
            <span v-else class="banner-form__placeholder">还没有选择封面</span>
          </div>
          <p class="banner-form__hint">
            选图后自动裁成 {{ COVER_SHAPE_RATIO_TEXT }}（{{ COVER_SHAPE_SIZE_TEXT }}），只上传定形后的图；读者侧固定居中显示
          </p>
          <div class="banner-form__cover-actions">
            <NButton quaternary size="small" :loading="uploading" data-role="banner-form-pick" @click="openFilePicker">
              {{ coverPreviewUrl ? "重新选择" : "选择图片" }}
            </NButton>
            <NButton
              v-if="coverPreviewUrl"
              quaternary
              size="small"
              :disabled="submitting || uploading"
              data-role="banner-form-clear-cover"
              @click="clearSelectedCover">移除</NButton>
          </div>
          <input
            ref="fileInputRef"
            type="file"
            accept="image/*"
            class="banner-form__file"
            data-role="banner-form-file"
            @change="onFileChange">
        </div>
      </NFormItem>

      <NFormItem label="标题">
        <NInput
          v-model:value="form.title"
          maxlength="256"
          show-count
          placeholder="给这一屏起个标题（可留空）"
          data-role="banner-form-title" />
      </NFormItem>

      <NFormItem label="跳转地址">
        <NInput
          v-model:value="form.href"
          placeholder="点击这一屏跳到哪里，例如 /article/123 或 https://…"
          data-role="banner-form-href" />
      </NFormItem>

      <NFormItem label="展示">
        <div class="banner-form__show">
          <NSwitch v-model:value="form.show" data-role="banner-form-show" />
          <span class="muted text-xs">{{ form.show ? "读者站展示这一屏" : "读者站不展示这一屏" }}</span>
        </div>
      </NFormItem>
    </NForm>

    <p v-if="stage" class="banner-form__stage" data-role="banner-form-stage">{{ stage }}</p>

    <div class="banner-form__actions">
      <NButton type="primary" :loading="submitting" data-role="banner-form-submit" @click="submit">
        {{ submitText }}
      </NButton>
      <NButton :disabled="submitting || uploading" data-role="banner-form-cancel" @click="close">取消</NButton>
    </div>

    <ImageCropDialog
      :show="cropRequest !== null"
      :src="cropPreviewUrl"
      :image-width="cropRequest?.width ?? 0"
      :image-height="cropRequest?.height ?? 0"
      :target="COVER_SHAPE_TARGET"
      :ratio-text="COVER_SHAPE_RATIO_TEXT"
      title="裁剪轮播封面（固定 16:9）"
      @confirm="confirmCrop"
      @cancel="cancelCrop" />
  </NModal>
</template>

<style scoped>
/*
 * 宽度受控：弹窗按卡片宽度收窄（560px），窄屏留边距。
 * 新增 / 编辑是同一张卡，所以这里不存在"新建那张铺满、编辑那张很窄"的错位。
 */
.banner-form-modal {
  width: 560px;
  max-width: calc(100vw - 32px);
}

.banner-form__cover {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  width: 100%;
}

/* 预览固定 16:9：看到的就是读者站那一屏的形状。 */
.banner-form__preview {
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  width: 100%;
  aspect-ratio: 16 / 9;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
}

.banner-form__preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center;
}

.banner-form__placeholder {
  color: var(--color-ink-muted);
  font-size: 0.8125rem;
}

.banner-form__hint {
  flex: 1 1 12rem;
  margin: 0;
  color: var(--color-ink-muted);
  font-size: 0.75rem;
  line-height: 1.6;
}

.banner-form__cover-actions {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

/* 文件选择器只做触发，不占版面。 */
.banner-form__file {
  display: none;
}

.banner-form__show {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.banner-form__stage {
  margin: 0.5rem 0 0;
  color: var(--color-ink-muted);
  font-size: 0.75rem;
}

.banner-form__actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 1rem;
}
</style>
