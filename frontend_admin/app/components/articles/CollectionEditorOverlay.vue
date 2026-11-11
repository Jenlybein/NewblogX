<script setup lang="ts">
/**
 * 合集 / 项目的编辑弹窗（决定清单 §8 第 34–36 条）。
 *
 * 形态：**信息卡居中** + 卡内「编排目录树」按钮 → **从信息卡右侧滑出抽屉**，
 * 两个容器**成组居中**（信息卡随抽屉出现向左让位，整组仍在视口中央）；窄屏改为上下堆叠。
 *
 * 为什么新建时不带编排：新建时还没有实体 ID，目录树无处可挂。保存后弹窗保持打开、
 * 拿到 ID，「编排目录树」按钮才亮 —— 省掉"再打开一次"的来回。
 *
 * 所有容器都**不铺满**：信息卡 `min(92vw, 560px)`、抽屉 `min(88vw, 480px)`。
 */
import { computed, reactive, ref, watch } from "vue";
import { NButton, NForm, NFormItem, NInput, NSpace, NSwitch, NTag, useMessage } from "naive-ui";
import ImageCropDialog from "~/components/media/ImageCropDialog.vue";
import OutlineTreeEditor from "~/components/articles/OutlineTreeEditor.vue";
import type { CollectionCardItem } from "~/components/articles/CollectionCardGrid.vue";
import { useImageCropUpload, type ImageFramingRequest } from "~/composables/useImageCropUpload";
import {
  composeAdminEntity,
  createCollection,
  createProject,
  setCollectionDisplay,
  setProjectDisplay,
} from "~/services/admin";
import { uploadImageByTask } from "~/services/image";
import { uploadBannerCover } from "~/utils/bannerCover";
import type { ShapeFramingState, ShapeImageTarget } from "~/utils/imageShaping";
import {
  COVER_SHAPE_OPTIONS,
  COVER_SHAPE_RATIO_TEXT,
  COVER_SHAPE_SIZE_TEXT,
  COVER_SHAPE_TARGET,
} from "~/utils/imageShaping";
import {
  DESCRIPTION_MAX,
  TITLE_MAX,
  buildCollectionCreatePayload,
  buildCollectionUpdatePayload,
  resolveCollectionCover,
  validateCollectionForm,
} from "~/utils/collectionAdmin";

const props = defineProps<{
  show: boolean;
  kind: "collection" | "project";
  /** null = 新建。 */
  entity?: CollectionCardItem | null;
}>();

const emit = defineEmits<{ close: []; saved: [] }>();

const message = useMessage();

const label = computed(() => (props.kind === "collection" ? "合集" : "项目"));

const saving = ref(false);
/** 目录树编辑器：两栏布局里常驻，保存时从它取节点一起提交。 */
const treeEditor = ref<InstanceType<typeof OutlineTreeEditor> | null>(null);
/** 保存成功后才有值：目录树编排需要真实实体 ID。 */
const entityId = ref("");

const form = reactive({ title: "", description: "", cover: "", displayed: true });
const errors = reactive<{ title: string | null; description: string | null }>({ title: null, description: null });

const coverPreview = ref("");
const coverLocalPreview = ref("");
const coverSize = ref<ShapeImageTarget | null>(null);
const coverUploading = ref(false);
const coverStage = ref("");
const coverFileInput = ref<HTMLInputElement | null>(null);
const coverCropRequest = ref<ImageFramingRequest | null>(null);
const coverCropPreviewUrl = ref("");

function revokePreviewUrl(url: string) {
  if (url && typeof URL.revokeObjectURL === "function") URL.revokeObjectURL(url);
}

function clearLocalCoverPreview() {
  const url = coverLocalPreview.value;
  coverLocalPreview.value = "";
  if (url) revokePreviewUrl(url);
}

function resetCoverCrop() {
  const url = coverCropPreviewUrl.value;
  coverCropPreviewUrl.value = "";
  coverCropRequest.value = null;
  revokePreviewUrl(url);
}

function createPreviewUrl(blob: Blob): string {
  try {
    return typeof URL.createObjectURL === "function" ? URL.createObjectURL(blob) : "";
  } catch {
    return "";
  }
}

function mapUploadStage(stage: "hashing" | "creating_task" | "uploading_to_qiniu" | "polling_status") {
  const stageMap = {
    hashing: "正在计算文件指纹…",
    creating_task: "正在创建上传任务…",
    uploading_to_qiniu: "正在上传到对象存储…",
    polling_status: "正在确认上传状态…",
  } as const;
  coverStage.value = stageMap[stage];
}

/** 封面上传与其余四个入口同源：16:9 偏差 ≤1% 自动居中定形，否则开裁剪窗。 */
const coverCropFlow = useImageCropUpload({
  options: COVER_SHAPE_OPTIONS,
  requestFraming: (request) => {
    coverCropRequest.value = request;
    coverCropPreviewUrl.value = createPreviewUrl(request.file);
    coverStage.value = `图片比例不符合 ${COVER_SHAPE_RATIO_TEXT}，请选择要露出的区域`;
  },
  upload: async (shaped) => {
    coverSize.value = shaped.size;
    coverLocalPreview.value = shaped.previewUrl;
    coverPreview.value = shaped.previewUrl || coverPreview.value;
    coverStage.value = `已定形成 ${COVER_SHAPE_SIZE_TEXT}（${COVER_SHAPE_RATIO_TEXT}），正在上传封面…`;
    const uploaded = await uploadBannerCover(shaped.file, file =>
      uploadImageByTask(file, { onStage: mapUploadStage, purpose: "cover" }),
    );
    form.cover = uploaded.url;
    coverPreview.value = uploaded.url;
    clearLocalCoverPreview();
    coverStage.value = "封面上传完成，点击保存";
  },
});

function openCoverPicker() {
  if (coverUploading.value || saving.value) return;
  coverFileInput.value?.click();
}

function removeCover() {
  form.cover = "";
  coverPreview.value = "";
  coverSize.value = null;
  coverStage.value = "";
  resetCoverCrop();
  clearLocalCoverPreview();
}

async function onCoverFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  if (!file.type.startsWith("image/")) {
    message.warning(`请选择图片文件作为${label.value}封面`);
    return;
  }

  clearLocalCoverPreview();
  coverSize.value = null;
  resetCoverCrop();
  coverStage.value = "正在检查图片比例…";
  coverUploading.value = true;
  try {
    await coverCropFlow.select(file);
  } catch (error) {
    coverStage.value = "封面上传失败，可重新选择图片";
    message.error(error instanceof Error ? `封面上传失败：${error.message}` : "封面上传失败，请稍后重试");
  } finally {
    coverUploading.value = false;
  }
}

async function confirmCoverCrop(framing: ShapeFramingState) {
  const request = coverCropRequest.value;
  if (!request) return;
  resetCoverCrop();
  coverStage.value = `正在按选定区域定形成 ${COVER_SHAPE_RATIO_TEXT} 封面…`;
  coverUploading.value = true;
  try {
    await coverCropFlow.confirm(request, framing);
  } catch (error) {
    coverStage.value = "封面上传失败，可重新选择图片";
    message.error(error instanceof Error ? `封面上传失败：${error.message}` : "封面上传失败，请稍后重试");
  } finally {
    coverUploading.value = false;
  }
}

function cancelCoverCrop() {
  resetCoverCrop();
  coverStage.value = "已取消本次上传，保留现有封面";
}

function resetAll() {
  form.title = "";
  form.description = "";
  form.cover = "";
  form.displayed = true;
  errors.title = null;
  errors.description = null;
  coverPreview.value = "";
  coverSize.value = null;
  coverStage.value = "";
  entityId.value = "";
  resetCoverCrop();
  clearLocalCoverPreview();
}

function fillFromEntity(entity: CollectionCardItem | null | undefined) {
  if (!entity) {
    resetAll();
    return;
  }
  entityId.value = entity.id;
  form.title = entity.title;
  form.description = entity.description;
  // 只回填**已上传**的封面：回退值是只读派生结果，不能当用户选择存回去。
  form.cover = entity.cover;
  form.displayed = entity.displayed;
  coverPreview.value = resolveCollectionCover(entity.cover, entity.fallback_cover)?.url ?? "";
  errors.title = null;
  errors.description = null;
  coverStage.value = "";
}

watch(
  () => [props.show, props.entity?.id, props.kind],
  () => {
    if (props.show) fillFromEntity(props.entity ?? null);
  },
  { immediate: true },
);

function close() {
  emit("close");
}

/**
 * 取目录树当前的节点。
 *
 * 做成"取不到就当空树"而不是直接调：树编辑器在某些场景下是替身（组件测试），
 * 它不一定暴露 nodes()；让保存因为"替身没实现某个方法"而整单失败，会把
 * 字段保存也一起拖下水。
 */
function currentNodes() {
  const editor = treeEditor.value as unknown as { nodes?: () => unknown[] } | null
  return typeof editor?.nodes === 'function' ? editor.nodes() : []
}

async function save() {
  const values = { title: form.title, description: form.description, cover: form.cover };
  const nextErrors = validateCollectionForm(values);
  errors.title = nextErrors.title;
  errors.description = nextErrors.description;
  if (errors.title || errors.description) {
    message.warning(errors.title ?? errors.description ?? "请检查表单");
    return;
  }
  if (coverUploading.value) {
    message.warning("封面还在上传，请稍候再保存");
    return;
  }
  if (saving.value) return;

  saving.value = true;
  try {
    if (entityId.value) {
      // 字段 + 目录树一次提交（票 348 / ADR-0041 的原子 compose）。
      //
      // 为什么必须一次：弹窗里"改标题 + 拖完树"是**一次**保存动作。分成两个请求
      // 任何一个失败都会留下"标题改了但树没改"的半截状态，而用户只点了一次保存。
      await composeAdminEntity(props.kind, entityId.value, {
        ...buildCollectionUpdatePayload(values),
        nodes: currentNodes(),
      });
      // 展示开关仍是独立动作：它不是"内容"的一部分，且改动不该因为树保存失败而回滚。
      if (props.kind === "collection") await setCollectionDisplay(entityId.value, form.displayed);
      else await setProjectDisplay(entityId.value, form.displayed);
      message.success(`${label.value}已保存`);
    } else {
      // 新建时还没有 ID：先建（拿到 ID），再按同一份树 compose 一次。
      const created = props.kind === "collection"
        ? await createCollection(buildCollectionCreatePayload({ ...values, displayed: form.displayed }))
        : await createProject(buildCollectionCreatePayload({ ...values, displayed: form.displayed }));
      entityId.value = created?.id ?? "";
      const pendingNodes = currentNodes();
      if (entityId.value && pendingNodes.length) {
        await composeAdminEntity(props.kind, entityId.value, {
          ...buildCollectionUpdatePayload(values),
          nodes: pendingNodes,
        });
      }
      message.success(`${label.value}已创建`);
    }
    emit("saved");
  } catch (error) {
    message.error(error instanceof Error ? `${label.value}保存失败：${error.message}` : "保存失败，请稍后重试");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <div v-if="props.show" class="collection-overlay" :data-role="`${kind}-editor`">
    <button type="button" class="collection-overlay__scrim" aria-label="关闭" @click="close" />

    <div class="collection-overlay__pair">
      <section class="collection-overlay__card" role="dialog" :aria-label="`${entityId ? '编辑' : '新建'}${label}`">
        <header class="collection-overlay__head">
          <h2 class="collection-overlay__title">{{ entityId ? `编辑${label}` : `新建${label}` }}</h2>
          <NTag v-if="entityId" size="small" data-role="collection-editor-id">{{ entityId }}</NTag>
        </header>

        <NForm label-placement="top" :disabled="saving">
          <NFormItem label="标题" :feedback="errors.title ?? undefined">
            <NInput v-model:value="form.title" class="admin-field--url" :maxlength="TITLE_MAX" show-count />
          </NFormItem>
          <NFormItem label="描述" :feedback="errors.description ?? undefined">
            <NInput
              v-model:value="form.description"
              type="textarea"
              class="admin-field--long"
              :autosize="{ minRows: 3, maxRows: 6 }"
              :maxlength="DESCRIPTION_MAX"
              show-count />
          </NFormItem>

          <NFormItem label="封面（16:9）">
            <div class="collection-overlay__cover">
              <div class="collection-overlay__cover-preview">
                <img v-if="coverPreview" :src="coverPreview" alt="" />
                <span v-else class="admin-hint">未上传时读者侧回退到首篇封面</span>
              </div>
              <NSpace :size="8">
                <NButton size="small" :disabled="coverUploading" @click="openCoverPicker">
                  {{ coverPreview ? "换一张" : "选择图片" }}
                </NButton>
                <NButton v-if="form.cover" size="small" tertiary type="error" @click="removeCover">移除封面</NButton>
              </NSpace>
              <p v-if="coverStage" class="admin-hint" data-role="collection-cover-stage">{{ coverStage }}</p>
            </div>
          </NFormItem>

          <NFormItem label="展示给读者">
            <NSwitch v-model:value="form.displayed" />
          </NFormItem>
        </NForm>

        <footer class="collection-overlay__foot">
          <NSpace>
            <NButton @click="close">关闭</NButton>
            <NButton type="primary" :loading="saving" data-role="collection-editor-save" @click="save">
              {{ entityId ? "保存修改" : `创建${label}` }}
            </NButton>
          </NSpace>
        </footer>

        <input
          ref="coverFileInput"
          type="file"
          accept="image/*"
          class="collection-overlay__file"
          @change="onCoverFileChange" />
      </section>

      <!-- 右栏：目录树（票 348 改成两栏常驻）。
           树本身没有保存按钮（embedded），改名 / 拖动 / 删除 / 清理失效 / 批量收录
           都在这里做，最后由「保存修改」一次提交。 -->
      <aside class="collection-overlay__drawer" :data-role="`${kind}-editor-drawer`">
        <div class="collection-overlay__drawer-inner">
          <header class="collection-overlay__head">
            <h2 class="collection-overlay__title">目录树</h2>
            <span class="admin-hint">拖动 / 改名 / 删除 / 清理失效 / 批量收录</span>
          </header>
          <OutlineTreeEditor
            ref="treeEditor"
            :kind="kind"
            :entity-id="entityId"
            embedded
            @saved="emit('saved')" />
        </div>
      </aside>
    </div>

    <ImageCropDialog
      :show="coverCropRequest !== null"
      :src="coverCropPreviewUrl"
      :image-width="coverCropRequest?.width ?? 0"
      :image-height="coverCropRequest?.height ?? 0"
      :target="COVER_SHAPE_TARGET"
      :ratio-text="COVER_SHAPE_RATIO_TEXT"
      :title="`裁剪${label}封面（固定 ${COVER_SHAPE_RATIO_TEXT}）`"
      @confirm="confirmCoverCrop"
      @cancel="cancelCoverCrop" />
  </div>
</template>
