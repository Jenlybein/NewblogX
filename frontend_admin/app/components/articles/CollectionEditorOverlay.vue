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
 * **拿不到就失败**（返回 null，由 save() 中止），绝不"当成空树"：
 * compose 是整树覆盖，把"取不到"当成"没有节点"等于一次保存把整棵树删光，
 * 而且还会弹成功提示。宁可这次保存不成功，也不能静默毁数据。
 *
 * 唯一合法的"空树"是**编辑器根本还没挂载**（新建实体、还没有 ID 时，
 * 右栏渲染的是"先保存基本信息"的提示，本来就没有节点）。
 */
function currentNodes(): unknown[] | null {
  const editor = treeEditor.value as unknown as { nodes?: () => unknown[] } | null
  // 编辑器没挂载：新建流程里这是正常状态，空树是真实答案。
  if (!editor) return []
  // 挂载了但取不到 nodes()：装配/替身问题，不能猜。
  if (typeof editor.nodes !== 'function') return null
  return editor.nodes()
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
      // 先取节点再动任何东西：取不到就整单中止（见 currentNodes 的注释）。
      // compose 是整树覆盖，"取不到当空树"等于一次保存删光整棵树。
      const nodes = currentNodes();
      if (nodes === null) {
        message.error("目录树还没准备好，这次没有保存；请刷新页面后重试");
        return;
      }
      // 字段 + 目录树一次提交（票 348 / ADR-0041 的原子 compose）。
      //
      // 为什么必须一次：弹窗里"改标题 + 拖完树"是**一次**保存动作。分成两个请求
      // 任何一个失败都会留下"标题改了但树没改"的半截状态，而用户只点了一次保存。
      await composeAdminEntity(props.kind, entityId.value, {
        ...buildCollectionUpdatePayload(values),
        nodes,
      });
      // 树提交成功即"没有未保存改动"：脏标记原本只在树编辑器自己的保存里复位，
      // 而两栏弹窗不走那条路径 —— 不复位的话「有未保存的改动」会一直挂着。
      treeEditor.value?.markSaved?.();
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
      if (pendingNodes === null) {
        // 已经建出来了，但树没提交：如实说清"建好了、树没存"，别报成功。
        message.warning(`${label.value}已创建，但目录树没能一起保存，请再点一次「保存修改」`);
        emit("saved");
        return;
      }
      if (entityId.value && pendingNodes.length) {
        await composeAdminEntity(props.kind, entityId.value, {
          ...buildCollectionUpdatePayload(values),
          nodes: pendingNodes,
        });
        treeEditor.value?.markSaved?.();
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
          <div class="collection-overlay__heading">
            <p class="collection-overlay__eyebrow">{{ kind === "collection" ? "Collection" : "Project" }} · {{ entityId ? "编辑" : "新建" }}</p>
            <h2 class="collection-overlay__title">{{ entityId ? `编辑${label}` : `新建${label}` }}</h2>
            <p class="collection-overlay__subtitle">
              标题、描述与封面决定读者在列表里看到的样子；右侧目录树决定点进去读什么。
            </p>
          </div>
          <NTag v-if="entityId" size="small" data-role="collection-editor-id">{{ entityId }}</NTag>
        </header>

        <NForm label-placement="top" :disabled="saving" class="collection-overlay__form">
          <div class="collection-overlay__split">
            <div class="collection-overlay__cover">
              <div class="collection-overlay__cover-preview">
                <img v-if="coverPreview" :src="coverPreview" alt="" />
                <span v-else class="admin-hint">未上传时读者侧回退到首篇封面</span>
              </div>
              <NSpace :size="8">
                <NButton size="small" :disabled="coverUploading" @click="openCoverPicker">
                  {{ coverPreview ? "换一张" : "选择图片" }}
                </NButton>
                <NButton v-if="form.cover" size="small" tertiary type="error" @click="removeCover">移除</NButton>
              </NSpace>
              <p class="admin-hint collection-overlay__cover-hint">{{ coverStage || "固定 16:9，上传时会按比例定形。" }}</p>
            </div>

            <div class="collection-overlay__fields">
              <NFormItem label="标题" :feedback="errors.title ?? undefined" :validation-status="errors.title ? 'error' : undefined">
                <NInput v-model:value="form.title" :maxlength="TITLE_MAX" show-count />
              </NFormItem>
              <NFormItem label="描述" :feedback="errors.description ?? undefined" :validation-status="errors.description ? 'error' : undefined">
                <NInput
                  v-model:value="form.description"
                  type="textarea"
                  :autosize="{ minRows: 4, maxRows: 7 }"
                  :maxlength="DESCRIPTION_MAX"
                  show-count />
              </NFormItem>
            </div>
          </div>

          <div class="collection-overlay__switch-row">
            <div class="collection-overlay__switch-copy">
              <p class="collection-overlay__switch-title">展示给读者</p>
              <p class="admin-hint">关闭后读者侧不再可见，内容与目录树都会保留。</p>
            </div>
            <NSwitch v-model:value="form.displayed" />
          </div>
        </NForm>

        <footer class="collection-overlay__foot">
          <p class="admin-hint collection-overlay__foot-hint">保存会把左侧字段与右侧目录树一次提交。</p>
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
          <header class="collection-overlay__drawer-head">
            <div>
              <p class="collection-overlay__eyebrow">Outline</p>
              <h2 class="collection-overlay__title">目录树</h2>
            </div>
            <p class="admin-hint collection-overlay__drawer-hint">拖动 / 改名 / 删除 / 清理失效 / 批量收录</p>
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
